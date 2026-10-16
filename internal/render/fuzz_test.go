package render

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/templates"
)

// FuzzParseTemplate targets the §3.3 template parser (fuzz-tested). Invariants: never panics;
// every successfully parsed node list is finite and well-formed.
func FuzzParseTemplate(f *testing.F) {
	for _, s := range []string{
		"plain text",
		"{{ .name }}",
		"{{ .name | kebab }} and {{ .other | upper }}",
		"{{ if .toggle }}x{{ end }}",
		"{{ if .outer }}a{{ if .inner }}b{{ end }}{{ end }}",
		"{{ .name }}",
		"{{ if .x }} unterminated",
		"{{ }}",
		"{{ .9bad }}",
		"{{ .x | unknown-filter }}",
		"{{",
		"}}",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		nodes, err := parseTemplate(src)
		if err != nil {
			return
		}

		vars := Vars{}
		toggles := Toggles{}
		walkNodes(nodes, func(n node) {
			switch n.kind {
			case varNode:
				vars[n.name] = "v"
			case ifNode:

				toggles[manifest.Axis(n.name)] = true
			}
		})
		if _, err := execNodes(nodes, vars, toggles, func(node) error { return nil }, "fuzz"); err != nil {
			t.Errorf("parsed template failed to execute: %v", err)
		}
	})
}

// FuzzExpr targets the exported expression evaluator used by gp-verb
// command rendering and doctor. Invariants: never panics; on success the
// result contains no unexpanded substitution.
func FuzzExpr(f *testing.F) {
	f.Add("bazel test //...", "My Project", "example.com/x")
	f.Add("{{ .module_path }}", "My Project", "example.com/x")
	f.Add("{{ .name | kebab }}-suffix", "My Project", "example.com/x")
	f.Add("{{ if .bazel }}on{{ end }}", "My Project", "example.com/x")
	f.Add("{{", "My Project", "example.com/x")
	// Regression: fuzzed *values* may themselves contain "{{" — verbatim
	// substitution puts them in the output, which is not an unexpanded
	// template substitution. The invariant only holds for template-clean
	// values, so seed the boundary case here.
	f.Add("{{ .name }}", "{{", "example.com/x")
	f.Fuzz(func(t *testing.T, src, name, module string) {
		vars := map[string]string{"name": name, "module_path": module}
		out, err := Expr(src, vars)
		if err != nil {
			return
		}
		if strings.Contains(name, "{{") || strings.Contains(module, "{{") {
			// A value carrying "{{" is copied into the output verbatim;
			// that is substitution working as designed, not a leftover
			// from the template. The no-unexpanded-substitution invariant
			// is untestable for such values (Expr itself already ran —
			// the never-panics invariant still held).
			return
		}
		if strings.Contains(out, "{{") {
			t.Errorf("unexpanded substitution in %q", out)
		}
	})
}

// FuzzRender drives the full pure render pipeline with fuzzed payload
// file content (fuzz-tested). Invariants on success: deterministic (same inputs → same
// FileSet), sorted paths, no path escapes, and no unexpanded `{{`.
// The agreement invariant: lint and render share one
// analyzer, so "lint passes but render fails" (and vice versa) is
// impossible on fully-provided vars.
func FuzzRender(f *testing.F) {
	f.Add("hello {{ .name }}", true)
	f.Add("{{ if .agents_md }}# AGENTS{{ end }}", false)
	f.Add("{{ .name | upper }}", true)
	f.Add("{{ if .unknown }}x{{ end }}", true)
	f.Add("unterminated {{ .name", false)
	f.Fuzz(func(t *testing.T, content string, toggle bool) {
		tmpl := templates.Template{
			Manifest: manifest.Manifest{
				ID: "fuzz", Version: "1.0.0", Kind: "cli", Language: "none",
				Vars:     map[string]manifest.VarSpec{"name": {Default: "proj"}},
				Defaults: map[manifest.Axis]bool{manifest.AxisAgentsMD: true},
			},
			Payload: fstest.MapFS{
				"main.hbs":  &fstest.MapFile{Data: []byte(content)},
				"plain.txt": &fstest.MapFile{Data: []byte("static")},
			},
		}
		vars := Vars{"name": "proj"}
		toggles := Toggles{"agents_md": toggle}

		lintIssues := Lint(tmpl.Payload, tmpl.Manifest)
		_, renderErr := (Engine{}).Render(tmpl, vars, toggles)
		if got, want := renderErr != nil, manifest.HasErrors(lintIssues); got != want {
			t.Errorf("lint/render disagree: renderErr=%v, lint errors=%v (issues %v)", renderErr, want, lintIssues)
		}

		got, err := (Engine{}).Render(tmpl, vars, toggles)
		if err != nil {
			return
		}
		again, err := (Engine{}).Render(tmpl, vars, toggles)
		if err != nil {
			t.Fatalf("render not total: second run failed: %v", err)
		}
		assertSameFileSet(t, got, again)
		if !sortedPaths(got) {
			t.Errorf("FileSet not sorted: %v", pathsOf(got))
		}
		for _, file := range got.Files {
			p := file.Path
			if p == "" || strings.HasPrefix(p, "/") || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "\\") {
				t.Errorf("unsafe output path %q", p)
			}
			if strings.Contains(string(file.Content), "{{") {
				t.Errorf("unexpanded substitution in %s: %q", p, file.Content)
			}
		}
	})
}

func assertSameFileSet(t *testing.T, a, b FileSet) {
	t.Helper()
	if len(a.Files) != len(b.Files) {
		t.Fatalf("render not deterministic: %d vs %d files", len(a.Files), len(b.Files))
	}
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path || string(a.Files[i].Content) != string(b.Files[i].Content) {
			t.Fatalf("render not deterministic at %d: %q vs %q", i, a.Files[i].Path, b.Files[i].Path)
		}
	}
}

func sortedPaths(fs FileSet) bool {
	for i := 1; i < len(fs.Files); i++ {
		if fs.Files[i-1].Path >= fs.Files[i].Path {
			return false
		}
	}
	return true
}

func pathsOf(fs FileSet) []string {
	out := make([]string, len(fs.Files))
	for i, f := range fs.Files {
		out[i] = f.Path
	}
	return out
}
