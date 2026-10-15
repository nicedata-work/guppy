package render

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/templates"
)

// tmpl builds a renderable Template over an in-memory payload.
func tmpl(t *testing.T, m manifest.Manifest, files map[string]string) templates.Template {
	t.Helper()
	fsys := fstest.MapFS{}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}
	return templates.Template{Manifest: m, Payload: fsys}
}

func renderOK(t *testing.T, m manifest.Manifest, files map[string]string, vars Vars, toggles Toggles) FileSet {
	t.Helper()
	fs, err := (Engine{}).Render(tmpl(t, m, files), vars, toggles)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return fs
}

func fileByPath(t *testing.T, fs FileSet, path string) File {
	t.Helper()
	for _, f := range fs.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no file %q in output %v", path, paths(fs))
	return File{}
}

func paths(fs FileSet) []string {
	var out []string
	for _, f := range fs.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestRenderSubstitutesVarsAndFilters(t *testing.T) {
	m := manifest.Manifest{
		ID: "x", Version: "1.0.0", Kind: "cli", Language: "go",
		Vars: map[string]manifest.VarSpec{
			"name":        {},
			"module_path": {},
		},
		Defaults: map[manifest.Axis]bool{manifest.AxisBazel: true},
	}
	files := map[string]string{
		"go.mod.hbs":     "module {{ .module_path }}\n",
		"README.md.hbs":  "# {{ .name | kebab }}\n",
		"nested/x.hbs":   "{{ .name | upper }}\n",
		"verbatim.txt":   "no {{ substitution }} here\n",
		".gitignore.hbs": "{{ if .bazel }}bazel-{{ .name | snake }}/{{ end }}\n",
	}
	fs := renderOK(t, m, files, Vars{"name": "My CLI", "module_path": "example.com/x/my-cli"}, Toggles{"bazel": true})

	if got := string(fileByPath(t, fs, "go.mod").Content); got != "module example.com/x/my-cli\n" {
		t.Errorf("go.mod = %q", got)
	}
	if got := string(fileByPath(t, fs, "README.md").Content); got != "# my-cli\n" {
		t.Errorf("README.md = %q", got)
	}
	if got := string(fileByPath(t, fs, "nested/x").Content); got != "MY CLI\n" {
		t.Errorf("nested/x = %q", got)
	}

	if got := string(fileByPath(t, fs, "verbatim.txt").Content); got != "no {{ substitution }} here\n" {
		t.Errorf("verbatim.txt = %q", got)
	}
	if got := string(fileByPath(t, fs, ".gitignore").Content); got != "bazel-my_cli/\n" {
		t.Errorf(".gitignore = %q", got)
	}

	for _, p := range paths(fs) {
		if strings.HasSuffix(p, ".hbs") {
			t.Errorf("output path %q still has .hbs", p)
		}
	}
}

func TestRenderUndeclaredAreErrorsNeverSilentlyEmpty(t *testing.T) {
	m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars:     map[string]manifest.VarSpec{"name": {}},
		Defaults: map[manifest.Axis]bool{}}
	_, err := (Engine{}).Render(tmpl(t, m, map[string]string{"a.hbs": "{{ .nope }}"}), Vars{"name": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "undeclared variable") {
		t.Fatalf("undeclared var: err = %v, want undeclared-variable error", err)
	}

	_, err = (Engine{}).Render(tmpl(t, m, map[string]string{"a.hbs": "{{ if .ghost }}x{{ end }}"}), Vars{"name": "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "undeclared toggle") {
		t.Fatalf("undeclared toggle: err = %v, want undeclared-toggle error", err)
	}
}

func TestRenderToggleGatedFileOmitted(t *testing.T) {
	m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars:     map[string]manifest.VarSpec{"name": {}},
		Defaults: map[manifest.Axis]bool{manifest.AxisAgentsMD: true}}
	files := map[string]string{"AGENTS.md.hbs": "{{ if .agents_md }}# AGENTS for {{ .name }}\n{{ end }}\n"}

	on := renderOK(t, m, files, Vars{"name": "p"}, Toggles{"agents_md": true})
	if got := string(fileByPath(t, on, "AGENTS.md").Content); !strings.Contains(got, "AGENTS for p") {
		t.Errorf("AGENTS.md (on) = %q", got)
	}
	off := renderOK(t, m, files, Vars{"name": "p"}, Toggles{"agents_md": false})
	if len(off.Files) != 0 {
		t.Errorf("AGENTS.md must be omitted when the gate is off, got %v", paths(off))
	}
}

// TestRenderGatedOffFileWithUndeclaredVarIsError pins the analyzer's
// strictness guarantee: the analyzer
// walks every node of every .hbs file, including whole-file toggle
// gates whose toggle is off, so an undeclared var inside a gated-off
// file is a render error with exactly lint's message — previously
// render succeeded for those files while lint failed them, the reverse
// of the disagreement the analyzer eliminates. Gate state changes
// emission, never the declaration verdict.
func TestRenderGatedOffFileWithUndeclaredVarIsError(t *testing.T) {
	const gated = "{{ if .agents_md }}# {{ .undeclared }}\n{{ end }}\n"
	undeclared := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars:     map[string]manifest.VarSpec{"name": {}},
		Defaults: map[manifest.Axis]bool{manifest.AxisAgentsMD: true}}
	declared := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars:     map[string]manifest.VarSpec{"name": {}, "undeclared": {}},
		Defaults: map[manifest.Axis]bool{manifest.AxisAgentsMD: true}}
	files := map[string]string{"AGENTS.md.hbs": gated}

	_, err := (Engine{}).Render(tmpl(t, undeclared, files), Vars{"name": "p"}, Toggles{"agents_md": false})
	if err == nil {
		t.Fatal("gated-off file with undeclared var rendered without error — gate state must not change the declaration verdict")
	}
	var lintErr *manifest.Issue
	for _, i := range Lint(tmpl(t, undeclared, files).Payload, undeclared) {
		if i.Level == manifest.LevelError && i.Path == "AGENTS.md.hbs" {
			lintErr = &i
			break
		}
	}
	if lintErr == nil {
		t.Fatalf("lint reports no error issue for AGENTS.md.hbs")
	}
	if got, want := err.Error(), lintErr.Path+": "+lintErr.Message; got != want {
		t.Errorf("render error = %q, want lint's message %q", got, want)
	}

	fs := renderOK(t, declared, files, Vars{"name": "p", "undeclared": "v"}, Toggles{"agents_md": false})
	if len(fs.Files) != 0 {
		t.Errorf("gated-off file must be omitted, got %v", paths(fs))
	}

	_, err = (Engine{}).Render(tmpl(t, undeclared, files), Vars{"name": "p"}, Toggles{"agents_md": true})
	want := "AGENTS.md.hbs: undeclared variable {{ .undeclared }} (declare it in [vars])"
	if err == nil || err.Error() != want {
		t.Errorf("gated-on undeclared var: err = %v, want %q", err, want)
	}
}

func TestRenderIncludesNarrowOnly(t *testing.T) {
	m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Includes: manifest.Includes{
			Include: []string{"**/*"},
			Exclude: []string{"template/scratch/**"},
		}}
	files := map[string]string{
		"keep.txt":         "keep",
		"scratch/tmp.txt":  "drop",
		"scratch/deep/x.y": "drop",
	}
	fs := renderOK(t, m, files, nil, nil)
	if got := paths(fs); len(got) != 1 || got[0] != "keep.txt" {
		t.Errorf("included files = %v, want [keep.txt]", got)
	}
}

func TestRenderOutputPathCollisionIsError(t *testing.T) {
	m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none"}
	_, err := (Engine{}).Render(tmpl(t, m, map[string]string{
		"a.txt":     "one",
		"a.txt.hbs": "two",
	}), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "would both write") {
		t.Fatalf("collision: err = %v, want collision error", err)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars: map[string]manifest.VarSpec{"name": {}}}
	files := map[string]string{"b.hbs": "{{ .name }}", "a.txt": "a", "c/d.hbs": "{{ .name }}"}
	a := renderOK(t, m, files, Vars{"name": "n"}, nil)
	b := renderOK(t, m, files, Vars{"name": "n"}, nil)
	if strings.Join(paths(a), ",") != strings.Join(paths(b), ",") {
		t.Errorf("unstable ordering: %v vs %v", paths(a), paths(b))
	}

	if paths(a)[0] != "a.txt" || paths(a)[1] != "b" || paths(a)[2] != "c/d" {
		t.Errorf("files not sorted: %v", paths(a))
	}
}

func TestResolveVarsFixpointAndErrors(t *testing.T) {
	m := manifest.Manifest{
		Vars: map[string]manifest.VarSpec{
			"name":        {},
			"module_path": {Default: "github.com/acme/{{ .name }}"},
			"upper_path":  {Default: "{{ .module_path | upper }}"},
		},
	}
	resolved, missing, err := ResolveVars(m, Vars{"name": "widget"})
	if err != nil {
		t.Fatalf("ResolveVars: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v", missing)
	}
	if resolved["module_path"] != "github.com/acme/widget" {
		t.Errorf("module_path = %q", resolved["module_path"])
	}

	if resolved["upper_path"] != "GITHUB.COM/ACME/WIDGET" {
		t.Errorf("upper_path = %q", resolved["upper_path"])
	}

	_, missing, err = ResolveVars(m, nil)
	if err != nil || len(missing) != 1 || missing[0] != "name" {
		t.Errorf("missing var: resolved err=%v missing=%v", err, missing)
	}

	_, _, err = ResolveVars(m, Vars{"nope": "x"})
	if err == nil || !strings.Contains(err.Error(), "undeclared variable") {
		t.Errorf("undeclared provided: err = %v", err)
	}

	circ := manifest.Manifest{Vars: map[string]manifest.VarSpec{
		"a": {Default: "x{{ .b }}"},
		"b": {Default: "y{{ .a }}"},
	}}
	_, _, err = ResolveVars(circ, nil)
	if err == nil || !strings.Contains(err.Error(), "circular") {
		t.Errorf("circular defaults: err = %v", err)
	}

	bad := manifest.Manifest{Vars: map[string]manifest.VarSpec{
		"a": {Default: "{{ .ghost }}"},
	}}
	_, _, err = ResolveVars(bad, nil)
	if err == nil || !strings.Contains(err.Error(), "undeclared variable") {
		t.Errorf("undeclared ref in default: err = %v", err)
	}
}

func TestExprRejectsIf(t *testing.T) {
	if _, err := Expr("{{ if .x }}a{{ end }}", Vars{"x": "1"}); err == nil {
		t.Fatal("Expr must reject {{ if }}")
	}
	if _, err := Expr("a{{ .missing }}", nil); !errors.Is(err, errMissingVar) {
		t.Fatalf("Expr missing ref: err = %v, want errMissingVar", err)
	}
}

func TestLintReportsSpecTableIssues(t *testing.T) {
	m := manifest.Manifest{
		ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Vars:     map[string]manifest.VarSpec{"name": {}, "unused": {}},
		Defaults: map[manifest.Axis]bool{manifest.AxisGit: true},
	}
	files := map[string]string{
		"ok.hbs":        "hi {{ .name }}",
		"bad.hbs":       "{{ .oops }}",
		"gate.hbs":      "{{ if .ghost }}x{{ end }}",
		"syntax.hbs":    "{{ if .git }}unclosed",
		"plain.txt":     "has {{ braces }} but no suffix",
		"collision":     "x",
		"collision.hbs": "y",
	}
	issues := Lint(tmpl(t, m, files).Payload, m)

	bySub := func(sub string) bool {
		for _, i := range issues {
			if strings.Contains(i.String(), sub) {
				return true
			}
		}
		return false
	}
	for _, want := range []string{
		"bad.hbs: undeclared variable {{ .oops }}",
		"undeclared toggle {{ if .ghost }}",
		"never closed",
		"collision.hbs",
		"declared var \"unused\" is never used",
		"not a .hbs file",
	} {
		if !bySub(want) {
			t.Errorf("lint issues missing %q in %v", want, issues)
		}
	}
	if !manifest.HasErrors(issues) {
		t.Error("issues must include errors")
	}
}

// TestRenderPropertyRandomVars is the property test for the pure engine:
// for arbitrary declared vars and random value sets, render is
// total, deterministic, and never leaves an unexpanded substitution.
func TestRenderPropertyRandomVars(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for iter := range 200 {

		n := 1 + rng.Intn(5)
		vars := map[string]manifest.VarSpec{}
		values := Vars{}
		for i := range n {
			name := "v" + string(rune('a'+i))
			vars[name] = manifest.VarSpec{}
			values[name] = randomValue(rng)
		}
		m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
			Vars: vars, Defaults: map[manifest.Axis]bool{"tog": true}}

		var b strings.Builder
		for _, name := range sortedVarNames(vars) {
			switch rng.Intn(3) {
			case 0:
				b.WriteString("{{ ." + name + " }}")
			case 1:
				b.WriteString("{{ ." + name + " | kebab }}")
			case 2:
				b.WriteString("{{ if .tog }}gated{{ end }}")
			}
			b.WriteString("\n")
		}
		files := map[string]string{"f.hbs": b.String(), "plain.txt": "plain"}

		tog := rng.Intn(2) == 0
		first, err := (Engine{}).Render(tmpl(t, m, files), values, Toggles{"tog": tog})
		if err != nil {
			t.Fatalf("iter %d: Render: %v (template %q)", iter, err, b.String())
		}
		second, err := (Engine{}).Render(tmpl(t, m, files), values, Toggles{"tog": tog})
		if err != nil {
			t.Fatalf("iter %d: second Render: %v", iter, err)
		}
		if strings.Join(paths(first), ",") != strings.Join(paths(second), ",") {
			t.Fatalf("iter %d: nondeterministic paths", iter)
		}
		for _, f := range first.Files {
			if strings.Contains(string(f.Content), "{{") {
				t.Fatalf("iter %d: unexpanded substitution in %s: %q", iter, f.Path, f.Content)
			}
		}
	}

	for iter := range 50 {
		m := manifest.Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
			Vars: map[string]manifest.VarSpec{"known": {}}}
		name := "ghost" + string(rune('a'+rng.Intn(26)))
		_, err := (Engine{}).Render(tmpl(t, m, map[string]string{"f.hbs": "{{ ." + name + " }}"}), Vars{"known": "v"}, nil)
		if err == nil {
			t.Fatalf("iter %d: undeclared var %q rendered without error", iter, name)
		}
	}
}

func randomValue(rng *rand.Rand) string {
	const alphabet = "abc XYZ-_.09"
	n := rng.Intn(12)
	var b strings.Builder
	for range n {
		b.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return b.String()
}
