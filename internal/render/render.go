// Package render is the pure substitution engine (design §3.3) and its
// deterministic output, FileSet — the unit guppy test snapshots, doctor
// diffs, and gp verbs all consume.
//
// Invariant: Engine has no filesystem access beyond reading the
// template's own payload fs.FS — ever. No env, no network, no clock:
// render(vars) is a pure function, which is what makes snapshot testing
// and property testing trivial.
package render

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/templates"
)

// Vars are the resolved values of a template's declared [vars].
type Vars map[string]string

// Toggles are the resolved values of a template's [defaults] axes
// (git, bazel, direnv, linters, tests, agents_md — all with --no-* inverses).
// Keyed by the closed Axis vocabulary (styleguide T1), never bare strings.
type Toggles map[manifest.Axis]bool

// File is one rendered output file: path relative to the project root,
// permission mode, and content.
type File struct {
	Path    string
	Mode    fs.FileMode
	Content []byte
}

// FileSet is the deterministic, ordered output of a render. Paths are
// unique and sorted; that determinism is what makes golden snapshots and
// doctor diffs meaningful. The invariant is established at construction
// (styleguide T5) — build through NewFileSet/Add, never normalize by
// hand.
type FileSet struct {
	Files []File
}

// NewFileSet builds a FileSet from files, establishing the invariant
// (ordered by path) at construction.
func NewFileSet(files []File) FileSet {
	out := FileSet{Files: files}
	out.sort()
	return out
}

// Add appends files to the set, keeping the sorted invariant.
func (fs *FileSet) Add(files ...File) {
	fs.Files = append(fs.Files, files...)
	fs.sort()
}

// sort enforces the FileSet ordering invariant (internal: consumers must
// not need to call it — T5).
func (fs *FileSet) sort() {
	slices.SortFunc(fs.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
}

// Engine is the built-in renderer: the §3.3 grammar, nothing more.
type Engine struct{}

// errMissingVar marks a substitution whose variable is declared but not
// (yet) resolved — the defer signal of ResolveVars' fixpoint.
var errMissingVar = errors.New("missing variable")

// Render renders t's payload (including the merged extends payload — the
// resolver overlays chain layers into Template.Payload) into a FileSet.
//
// Static verification happens here too, not just in lint: undeclared vars,
// undeclared toggles, and output-path collisions are render errors, never
// silently-empty output (strict allow-list, §3.2). The rules live once, in
// analyze (styleguide D3) — Render is analyze + strict emit, Lint is
// analyze + report, so "lint passes but render fails" cannot happen.
func (Engine) Render(t templates.Template, vars Vars, toggles Toggles) (FileSet, error) {
	analyses, issues := analyze(t.Payload, t.Manifest)
	for _, issue := range issues {
		if issue.Level == manifest.LevelError {
			return FileSet{}, issueError(issue)
		}
	}

	var out FileSet
	for _, fa := range analyses {
		content := fa.content
		if !fa.verbatim {
			if fa.gate != "" && !toggles[fa.gate] {

				continue
			}

			rendered, err := execNodes(fa.nodes, vars, toggles, nil, fa.rel)
			if err != nil {
				return FileSet{}, err
			}
			content = []byte(rendered)
		}
		out.Files = append(out.Files, File{Path: fa.outPath, Mode: fa.mode, Content: content})
	}
	out.sort()
	return out, nil
}

// execNodes renders parsed nodes. check optionally validates each
// reference (declaration rules — Expr's "no {{ if }}" rule); nil means
// declarations were already verified by analyze. The lookup uses
// vars/toggles. A missing var value yields errMissingVar (wrapped) so
// ResolveVars can defer it.
func execNodes(nodes []node, vars Vars, toggles Toggles, check func(node) error, path string) (string, error) {
	var b strings.Builder
	for _, n := range nodes {
		switch n.kind {
		case textNode:
			b.WriteString(n.text)
		case varNode:
			if check != nil {
				if err := check(n); err != nil {
					return "", prefixed(path, err)
				}
			}
			v, ok := vars[n.name]
			if !ok {
				return "", prefixed(path, fmt.Errorf("%w {{ .%s }}", errMissingVar, n.name))
			}
			for _, f := range n.filters {
				v = applyFilter(f, v)
			}
			b.WriteString(v)
		case ifNode:
			if check != nil {
				if err := check(n); err != nil {
					return "", prefixed(path, err)
				}
			}

			if toggles[manifest.Axis(n.name)] {
				inner, err := execNodes(n.body, vars, toggles, check, path)
				if err != nil {
					return "", err
				}
				b.WriteString(inner)
			}
		}
	}
	return b.String(), nil
}

func prefixed(path string, err error) error {
	if path == "" {
		return err
	}
	return fmt.Errorf("%s: %w", path, err)
}

// singleToggleGate reports whether a parsed template consists solely of
// one {{ if .toggle }} block surrounded by whitespace — the toggle-gated
// file form (bodies inside the block are unrestricted).
func singleToggleGate(nodes []node) (string, bool) {
	gate := ""
	found := 0
	for _, n := range nodes {
		switch n.kind {
		case textNode:
			if strings.TrimSpace(n.text) != "" {
				return "", false
			}
		case ifNode:
			found++
			gate = n.name
		case varNode:
			return "", false
		}
	}
	if found != 1 {
		return "", false
	}
	return gate, true
}

// Expr renders src (a var default or a test-matrix var override) against
// vars. Only variable substitutions are allowed — no {{ if }} — and an
// unresolved reference yields errMissingVar so callers can defer it.
func Expr(src string, vars Vars) (string, error) {
	nodes, err := parseTemplate(src)
	if err != nil {
		return "", err
	}
	check := func(n node) error {
		if n.kind == ifNode {
			return fmt.Errorf("{{ if }} is not allowed in variable expressions")
		}
		return nil
	}
	return execNodes(nodes, vars, Toggles{}, check, "")
}

// ResolveVars completes the declared [vars] of m with the user-provided
// values: provided values win; the rest resolve from their defaults, which
// are themselves variable expressions (fixed-point: `github.com/acme/
// {{ .name }}` works regardless of declaration order). Returns the
// resolved set plus the names of declared vars that could not be resolved
// (no default and not provided — the caller prompts or exits 2).
func ResolveVars(m manifest.Manifest, provided Vars) (Vars, []string, error) {
	if err := checkVarDefaults(m); err != nil {
		return nil, nil, err
	}
	resolved := Vars{}
	for k, v := range provided {
		if _, ok := m.Vars[k]; !ok {
			return nil, nil, fmt.Errorf("undeclared variable %q (declare it in [vars] or drop --var %s)", k, k)
		}
		resolved[k] = v
	}
	for {
		progress := false
		for _, name := range sortedVarNames(m.Vars) {
			if _, ok := resolved[name]; ok {
				continue
			}
			def := m.Vars[name].Default
			if def == "" {
				continue
			}
			val, err := Expr(def, resolved)
			if errors.Is(err, errMissingVar) {
				continue
			}
			if err != nil {
				return nil, nil, fmt.Errorf("var %q default: %w", name, err)
			}
			resolved[name] = val
			progress = true
		}
		if !progress {
			break
		}
	}

	var missing []string
	unresolved := map[string]bool{}
	for _, name := range sortedVarNames(m.Vars) {
		if _, ok := resolved[name]; !ok {
			unresolved[name] = true
			if m.Vars[name].Default == "" {
				missing = append(missing, name)
			}
		}
	}

	blocked := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name := range unresolved {
			if blocked[name] || m.Vars[name].Default == "" {
				continue
			}
			for _, ref := range varRefs(m, name) {
				if unresolved[ref] && (m.Vars[ref].Default == "" || blocked[ref]) {
					blocked[name] = true
					changed = true
					break
				}
			}
		}
	}
	var circular []string
	for name := range unresolved {
		if m.Vars[name].Default != "" && !blocked[name] {
			circular = append(circular, name)
		}
	}
	slices.Sort(circular)
	if len(circular) > 0 {
		return nil, nil, fmt.Errorf("circular var default reference between: %s", strings.Join(circular, ", "))
	}
	return resolved, missing, nil
}

// varRefs returns the variable names referenced by a var's default.
func varRefs(m manifest.Manifest, name string) []string {
	nodes, err := parseTemplate(m.Vars[name].Default)
	if err != nil {
		return nil
	}
	var refs []string
	walkNodes(nodes, func(n node) {
		if n.kind == varNode {
			refs = append(refs, n.name)
		}
	})
	return refs
}

// checkVarDefaults statically verifies var defaults: no {{ if }}, and every
// referenced name must be a declared var.
func checkVarDefaults(m manifest.Manifest) error {
	for _, name := range sortedVarNames(m.Vars) {
		def := m.Vars[name].Default
		if def == "" {
			continue
		}
		nodes, err := parseTemplate(def)
		if err != nil {
			return fmt.Errorf("var %q default: %w", name, err)
		}
		var walkErr error
		walkNodes(nodes, func(n node) {
			switch n.kind {
			case ifNode:
				walkErr = fmt.Errorf("var %q default: {{ if }} is not allowed in variable expressions", name)
			case varNode:
				if _, ok := m.Vars[n.name]; !ok && walkErr == nil {
					walkErr = fmt.Errorf("var %q default references undeclared variable {{ .%s }}", name, n.name)
				}
			}
		})
		if walkErr != nil {
			return walkErr
		}
	}
	return nil
}

func walkNodes(nodes []node, fn func(node)) {
	for _, n := range nodes {
		fn(n)
		if len(n.body) > 0 {
			walkNodes(n.body, fn)
		}
	}
}

func sortedVarNames(vars map[string]manifest.VarSpec) []string {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}
