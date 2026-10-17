// The per-project marker file, `.guppy/project.toml` (design §9): written
// at scaffold time, committed to the repo, and *being there* is the whole
// definition of a guppy-managed project. It records provenance (template
// identity, resolved chain, var values) and the resolved [gup] command map
// (§3.5) — what `guppy project <verb>` / `gup` dispatch through.
//
// Encoding is deterministic (sorted verbs, no timestamps) so idempotent
// re-scaffolds keep the marker byte-identical instead of reporting it as
// "kept".
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
)

// MarkerRelPath is the marker's location relative to the project root —
// the single source of truth for scaffold, discovery, and doctor.
const MarkerRelPath = ".guppy/project.toml"

// SchemaVersion is the marker schema version. v2 adds [toggles] (doctor
// needs the recorded axes to re-render the expected file set); v1
// markers still decode and fall back to the template's [defaults].
const SchemaVersion = 2

// Marker is the decoded content of .guppy/project.toml.
type Marker struct {
	Schema  int                    `toml:"schema"`
	Guppy   GuppyInfo              `toml:"guppy"`
	Project ProjectInfo            `toml:"project"`
	Vars    map[string]string      `toml:"vars"`
	Toggles map[manifest.Axis]bool `toml:"toggles"`
	GUP     VerbMap                `toml:"gup"`
}

// GuppyInfo records which guppy wrote the marker (provenance, §9).
type GuppyInfo struct {
	Version string `toml:"version"`
}

// ProjectInfo identifies the template the project was scaffolded from.
type ProjectInfo struct {
	Template string   `toml:"template"`
	Version  string   `toml:"version"`
	Chain    []string `toml:"chain"`
}

// MarkerInput is everything Encode needs to write a marker.
type MarkerInput struct {
	TemplateID   string
	TemplateVer  string
	Chain        []string
	Vars         map[string]string
	Toggles      map[manifest.Axis]bool
	Verbs        VerbMap
	GuppyVersion string
}

// Encode renders the marker file content. Deterministic: vars and verbs
// are written sorted; the guppy version and chain are recorded verbatim.
func Encode(in MarkerInput) ([]byte, error) {
	m := Marker{
		Schema: SchemaVersion,
		Guppy:  GuppyInfo{Version: in.GuppyVersion},
		Project: ProjectInfo{
			Template: in.TemplateID,
			Version:  in.TemplateVer,
			Chain:    in.Chain,
		},
		Vars:    in.Vars,
		Toggles: in.Toggles,
		GUP:     in.Verbs,
	}
	data, err := toml.Marshal(m)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("# guppy project marker — the presence of this file defines a\n" +
		"# guppy-managed project. Written by `guppy new`; commit it.\n")
	return append([]byte(header), data...), nil
}

// Decode parses a marker file. A missing/unknown schema version is an
// error: a newer guppy wrote it, and guessing would be worse than saying
// so.
func Decode(data []byte) (Marker, error) {
	var m Marker
	if err := toml.Unmarshal(data, &m); err != nil {
		return Marker{}, err
	}
	switch {
	case m.Schema == 0:
		return Marker{}, fmt.Errorf("missing schema version")
	case m.Schema > SchemaVersion:
		return Marker{}, fmt.Errorf("marker schema %d is newer than this guppy supports (%d) — upgrade guppy", m.Schema, SchemaVersion)
	}
	return m, nil
}

// ResolveVerbs resolves a template's [gup] map into the project's verb map:
// commands are rendered as variable expressions against the resolved vars
// (strict — undeclared vars are errors, never empty strings), and verbs
// whose required axes are off are dropped (`--no-bazel` means no
// `bazel build` verb). The result is sorted for deterministic markers.
func ResolveVerbs(gup map[string]manifest.GUPCommand, toggles render.Toggles, vars render.Vars) (VerbMap, error) {
	out := VerbMap{}
	for _, verb := range sortedKeys(gup) {
		entry := gup[verb]
		skip := false
		for _, axis := range entry.Requires {
			if !toggles[axis] {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		cmd, err := render.Expr(entry.Command, vars)
		if err != nil {

			return nil, fmt.Errorf("[gup] verb %q: %w — command expressions may only use vars declared in [vars]", verb, err)
		}
		out[verb] = Command(cmd)
	}
	return out, nil
}

// File returns the marker as a rendered File for the scaffold FileSet,
// so marker writes inherit Apply's transactional never-clobber behavior.
func File(in MarkerInput) (render.File, error) {
	data, err := Encode(in)
	if err != nil {
		return render.File{}, err
	}
	return render.File{Path: MarkerRelPath, Mode: 0o644, Content: data}, nil
}

// ErrOutsideProject marks "no marker found above me" — the refusal of
// `gup`/`guppy project` outside a managed project (§3.5).
var ErrOutsideProject = fmt.Errorf("not inside a guppy-managed project")

// FindUp walks up from start to the filesystem root looking for the
// marker; it returns the project root directory and the decoded marker.
// Nothing outside the marker directory is read — discovery is one stat
// per directory level.
func FindUp(start string) (string, Marker, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", Marker{}, err
	}
	for {
		path := filepath.Join(dir, MarkerRelPath)
		data, rerr := os.ReadFile(path)
		if rerr == nil {
			m, derr := Decode(data)
			if derr != nil {
				return "", Marker{}, fmt.Errorf("%s: %w", path, derr)
			}
			return dir, m, nil
		}
		if !os.IsNotExist(rerr) {
			return "", Marker{}, fmt.Errorf("%s: %w", path, rerr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", Marker{}, fmt.Errorf("%w: no %s found in this directory or any parent — scaffold one with \"guppy new\"", ErrOutsideProject, MarkerRelPath)
		}
		dir = parent
	}
}

// sortedKeys returns map keys sorted (deterministic output).
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// SortedVerbNames lists a verb map's verbs: baseline verbs first in
// canonical order (build, test, lint), then the rest alphabetically.
func SortedVerbNames(v VerbMap) []string {
	names := sortedKeys(v)
	var base, rest []string
	for _, b := range []string{VerbBuild, VerbTest, VerbLint} {
		if _, ok := v[b]; ok {
			base = append(base, b)
		}
	}
	baseline := map[string]bool{VerbBuild: true, VerbTest: true, VerbLint: true}
	for _, n := range names {
		if !baseline[n] {
			rest = append(rest, n)
		}
	}
	return append(base, rest...)
}

// FormatMap renders the verb map for human-readable output (unknown-verb
// errors, `gup` help): aligned verb names and commands.
func FormatMap(v VerbMap) string {
	var b strings.Builder
	names := SortedVerbNames(v)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		_, _ = fmt.Fprintf(&b, "  %-*s  %s\n", width, n, v[n])
	}
	return b.String()
}
