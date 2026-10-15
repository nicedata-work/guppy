// Loading a template from an fs.FS (embed, disk, and later git/registry
// all satisfy it — the unifying abstraction of §2.2). The payload root is
// template/ when present, else the template root itself ("thin" templates,
// §3.1); in thin mode the authoring files (guppy.toml, tests.toml,
// testdata/) never leak into scaffolds.
package templates

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// PayloadDir is the conventional payload directory (§3.1). A template
// without it is thin: the root is the payload.
const PayloadDir = "template"

// authoringFiles are excluded from a thin template's payload so authoring
// metadata never lands in a scaffold.
var authoringFiles = map[string]bool{"guppy.toml": true, "tests.toml": true, "testdata": true}

// LoadOptions parameterizes Load: where the template came from.
type LoadOptions struct {
	Scope Scope
	Kind  SourceKind

	Pin string
}

// Load parses guppy.toml at fsys's root and returns the template with its
// payload rooted appropriately. Schema-level error issues (§3.2) are
// returned, not raised: lint wants to report them, resolvers want to refuse
// the template. Only a missing or unparseable manifest is a hard error.
func Load(fsys fs.FS, opts LoadOptions) (Template, []manifest.Issue, error) {
	data, err := fs.ReadFile(fsys, "guppy.toml")
	if err != nil {
		return Template{}, nil, fmt.Errorf("not a template: no guppy.toml (%w)", err)
	}
	m, parseWarnings, err := manifest.Parse(data)
	if err != nil {
		return Template{}, nil, fmt.Errorf("guppy.toml: %w", err)
	}
	issues := manifest.Validate(m)
	for _, w := range parseWarnings {
		issues = append(issues, manifest.Issue{Level: manifest.LevelWarning, Message: w})
	}

	version := semver.Version{}
	if v, err := semver.StrictNewVersion(m.Version); err == nil {
		version = *v
	} else {

	}

	payload := fs.FS(fsys)
	var info fs.FileInfo
	if st, err := fs.Stat(fsys, PayloadDir); err == nil && st.IsDir() {
		info = st
	}
	if info != nil {
		sub, err := fs.Sub(fsys, PayloadDir)
		if err != nil {
			return Template{}, nil, err
		}
		payload = sub
	} else {
		payload = thinPayload{fsys}
	}

	return Template{
		Identity: Identity{
			Scope:   opts.Scope,
			Kind:    opts.Kind,
			ID:      m.ID,
			Version: version,
			Pin:     opts.Pin,
		},
		Manifest: m,
		Chain:    []Identity{{Scope: opts.Scope, Kind: opts.Kind, ID: m.ID, Version: version, Pin: opts.Pin}},
		Payload:  payload,
	}, issues, nil
}

// thinPayload hides authoring files when the template root doubles as the
// payload (§3.1 thin templates), from listings, opens, and stats alike.
type thinPayload struct{ root fs.FS }

// hidden reports whether a top-level name is an authoring file that must
// never leak into a scaffold.
func hidden(name string) bool {
	return name != "." && !strings.Contains(name, "/") && authoringFiles[name]
}

func (t thinPayload) Open(name string) (fs.File, error) {
	if hidden(name) {
		return nil, fs.ErrNotExist
	}
	return t.root.Open(name)
}

// Stat implements fs.StatFS so the filtering contract holds for
// stat-based consumers too (doctor, dist tooling): authoring
// files stat as if absent, not as present files a later filter must
// remember to skip.
func (t thinPayload) Stat(name string) (fs.FileInfo, error) {
	if hidden(name) {
		return nil, fs.ErrNotExist
	}
	return fs.Stat(t.root, name)
}

// ReadDir implements fs.ReadDirFS so walkers see the filtered view too.
func (t thinPayload) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(t.root, name)
	if err != nil {
		return nil, err
	}
	if name != "." {
		return entries, nil
	}
	out := entries[:0:0]
	for _, e := range entries {
		if authoringFiles[e.Name()] {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// compile-time check: stat-based consumers get the filtered view (D8).
var _ fs.StatFS = thinPayload{}
