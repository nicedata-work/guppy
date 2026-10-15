// Package dist records template provenance for installed templates
// (design §4.3 identity rules, §7 trust gate). The record file lives in
// the data dir (<data>/guppy/installs.toml) and answers two questions:
//
//   - where did this installed template come from? (registry/git/local
//     spec + immutable pin — what `guppy template update` reconciles);
//   - is this community template trusted? A community-scope install is
//     trusted exactly when a record exists for its id@version — records
//     are written only by the review-and-confirm flow, so a directory
//     dropped into the community dir by hand is untrusted until reviewed.
package dist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/nicedata-work/guppy/internal/fileutil"
)

// ScopeName is the persisted trust/precedence layer of an install
// record (design §4.1). It stays a string-backed type at the file
// format boundary (records are a file format, not a code coupling —
// D13), while the vocabulary itself is closed (styleguide T1).
type ScopeName string

// Scope names as persisted (templates.Scope values, without importing
// the enum — records are a file format, not a code coupling).
const (
	ScopeUser      ScopeName = "user"
	ScopeCommunity ScopeName = "community"
)

// SourceKind is how an installed template's bytes arrived (design
// §4.3), persisted in the records file (templates.SourceKind names).
type SourceKind string

// Source kinds as persisted (templates.SourceKind names).
const (
	KindRegistry SourceKind = "registry"
	KindGit      SourceKind = "git"
	KindLocal    SourceKind = "local"
)

// Record is one installed template: identity + how to refresh it.
type Record struct {
	ID      string     `toml:"id"`
	Scope   ScopeName  `toml:"scope"`
	Kind    SourceKind `toml:"kind"`
	Source  string     `toml:"source"`
	Version string     `toml:"version"`

	Pin string `toml:"pin"`
}

type fileFormat struct {
	Schema   int      `toml:"schema"`
	Installs []Record `toml:"install"`
}

// Store persists records at Path (one file, atomic rewrite).
type Store struct {
	Path string
}

// Load reads the records; a missing file is an empty (not failed) store.
func (s Store) Load() ([]Record, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}
	var f fileFormat
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", s.Path, err)
	}
	if f.Schema != 1 {
		return nil, fmt.Errorf("%s: unsupported schema %d (want 1)", s.Path, f.Schema)
	}
	return f.Installs, nil
}

// Save rewrites the records file atomically (fileutil.WriteAtomic).
func (s Store) Save(records []Record) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# guppy installed-template records — managed by `guppy template`.\n")
	b.WriteString("# A record is provenance (identity + pin) and, for the community\n")
	b.WriteString("# scope, the trust ledger: no record, no trust.\n")
	if err := toml.NewEncoder(&b).Encode(fileFormat{Schema: 1, Installs: records}); err != nil {
		return err
	}
	return fileutil.WriteAtomic(s.Path, 0o644, []byte(b.String()))
}

// Find returns the record for id in scope ("" = any scope, first wins:
// user before community, matching file order).
func Find(records []Record, id string, scope ScopeName) (Record, bool) {
	for _, r := range records {
		if r.ID == id && (scope == "" || r.Scope == scope) {
			return r, true
		}
	}
	return Record{}, false
}

// Upsert replaces the record for (id, scope) or appends it. Returns the
// updated list.
func Upsert(records []Record, rec Record) []Record {
	for i, r := range records {
		if r.ID == rec.ID && r.Scope == rec.Scope {
			records[i] = rec
			return records
		}
	}
	return append(records, rec)
}

// Delete removes the record for (id, scope); returns the list and
// whether anything was removed.
func Delete(records []Record, id string, scope ScopeName) ([]Record, bool) {
	out := records[:0:0]
	removed := false
	for _, r := range records {
		if r.ID == id && r.Scope == scope {
			removed = true
			continue
		}
		out = append(out, r)
	}
	return out, removed
}

// Trusted reports whether id@version is trusted in the community scope:
// a record exists for exactly that identity (upgrades re-enter the
// review gate — a new version is a new trust decision, design §7).
func Trusted(records []Record, id, version string) bool {
	for _, r := range records {
		if r.ID == id && r.Scope == ScopeCommunity && r.Version == version {
			return true
		}
	}
	return false
}
