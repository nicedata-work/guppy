package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
)

func TestMarkerRoundTrip(t *testing.T) {
	in := MarkerInput{
		TemplateID:   "go-cli",
		TemplateVer:  "0.2.0",
		Chain:        []string{"base-toolchain@0.2.0", "go-cli@0.2.0"},
		Vars:         map[string]string{"name": "widget", "module_path": "example.com/x/widget"},
		Verbs:        VerbMap{"test": "bazel test //...", "build": "bazel build //..."},
		GuppyVersion: "dev",
	}
	data, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Schema != SchemaVersion {
		t.Errorf("schema = %d, want %d", m.Schema, SchemaVersion)
	}
	if m.Project.Template != "go-cli" || m.Project.Version != "0.2.0" {
		t.Errorf("project = %+v", m.Project)
	}
	if len(m.Project.Chain) != 2 || m.Project.Chain[0] != "base-toolchain@0.2.0" {
		t.Errorf("chain = %v", m.Project.Chain)
	}
	if m.Vars["name"] != "widget" || m.GUP["test"] != "bazel test //..." {
		t.Errorf("vars/verbs = %v / %v", m.Vars, m.GUP)
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	in := MarkerInput{
		TemplateID: "x", TemplateVer: "1.0.0", GuppyVersion: "dev",
		Vars:  map[string]string{"b": "2", "a": "1"},
		Verbs: VerbMap{"test": "t", "build": "b", "lint": "l"},
	}
	first, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	second, _ := Encode(in)
	if string(first) != string(second) {
		t.Fatal("Encode is not deterministic")
	}

	if strings.Index(string(first), `a = "1"`) > strings.Index(string(first), `b = "2"`) {
		t.Error("vars not written sorted")
	}
	if strings.Index(string(first), "build") > strings.Index(string(first), "lint") {
		t.Error("verbs not written sorted")
	}
}

func TestDecodeRejectsNewerSchema(t *testing.T) {
	_, err := Decode([]byte("schema = 999\n"))
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("Decode error = %v, want newer-schema refusal", err)
	}
	if _, err := Decode([]byte("project = {}\n")); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("Decode error = %v, want missing-schema refusal", err)
	}
}

func TestFindUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := FindUp(nested); err == nil {

		t.Skip("temp dir is inside an existing managed project")
	}

	marker := []byte("schema = 1\n[project]\ntemplate = \"go-cli\"\n")
	if err := os.MkdirAll(filepath.Join(root, ".guppy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".guppy", "project.toml"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	gotRoot, m, err := FindUp(nested)
	if err != nil {
		t.Fatalf("FindUp: %v", err)
	}
	if gotRoot != root {
		t.Errorf("root = %q, want %q", gotRoot, root)
	}
	if m.Project.Template != "go-cli" {
		t.Errorf("marker = %+v", m)
	}

	closer := filepath.Join(root, "a")
	if err := os.MkdirAll(filepath.Join(closer, ".guppy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(closer, ".guppy", "project.toml"),
		[]byte("schema = 1\n[project]\ntemplate = \"inner\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, m, err := FindUp(nested); err != nil || m.Project.Template != "inner" {
		t.Errorf("nearest marker: m = %+v, err = %v, want inner", m, err)
	}
}

func TestResolveVerbs(t *testing.T) {
	gup := map[string]manifest.GUPCommand{
		"build": {Command: "bazel build //...", Requires: []manifest.Axis{"bazel"}},
		"test":  {Command: "bazel test //...", Requires: []manifest.Axis{"bazel", "tests"}},
		"lint":  {Command: "runlint {{ .name }}"},
	}
	vars := render.Vars{"name": "widget"}
	all := render.Toggles{"bazel": true, "tests": true}

	got, err := ResolveVerbs(gup, all, vars)
	if err != nil {
		t.Fatalf("ResolveVerbs: %v", err)
	}
	if len(got) != 3 || got["lint"] != "runlint widget" {
		t.Errorf("verbs = %v", got)
	}

	noTests := render.Toggles{"bazel": true, "tests": false}
	got, err = ResolveVerbs(gup, noTests, vars)
	if err != nil {
		t.Fatalf("ResolveVerbs: %v", err)
	}
	if _, ok := got["test"]; ok {
		t.Errorf("test verb should be dropped without tests: %v", got)
	}
	if _, ok := got["build"]; !ok {
		t.Errorf("build verb should survive: %v", got)
	}

	_, err = ResolveVerbs(map[string]manifest.GUPCommand{"x": {Command: "{{ .nope }}"}}, all, vars)
	if err == nil || !strings.Contains(err.Error(), "declared in [vars]") {
		t.Errorf("error = %v, want undeclared-var explanation", err)
	}
}

func TestSelectRunner(t *testing.T) {
	tests := []struct {
		name   string
		direnv string
		envrc  bool
		want   Mode
	}{
		{"direnv installed + .envrc", "/usr/bin/direnv", true, ModeDirenv},
		{"direnv missing", "", true, ModeDirect},
		{"no .envrc", "/usr/bin/direnv", false, ModeDirect},
		{"neither", "", false, ModeDirect},
	}
	for _, tt := range tests {
		if got := SelectRunner("/proj", tt.direnv, tt.envrc); got != tt.want {
			t.Errorf("%s: SelectRunner = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRunDirectExecutesAndPropagatesExitCode(t *testing.T) {
	var out strings.Builder
	code, err := Run(RunOptions{
		Ctx:  context.Background(),
		Root: t.TempDir(), Mode: ModeDirect, Verb: "test",
		Stdout: &out, Stderr: &out,
	}, "echo testing; echo failing >&2; exit 3")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 3 {
		t.Errorf("code = %d, want 3", code)
	}
	if !strings.Contains(out.String(), "testing") || !strings.Contains(out.String(), "failing") {
		t.Errorf("output = %q", out.String())
	}
}

// TestRunCancelledSurfacesContextCanceled pins (styleguide
// T14/U14): Run uses exec.CommandContext and reports cancellation as an
// error wrapping context.Canceled, so the CLI can map the interrupted
// verb to exit 130 rather than reporting a bogus child exit code.
func TestRunCancelledSurfacesContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code, err := Run(RunOptions{
		Ctx: ctx, Root: t.TempDir(), Mode: ModeDirect, Verb: "test",
	}, "sleep 30")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if code != -1 {
		t.Errorf("code = %d, want -1 (no meaningful child verdict when interrupted)", code)
	}
}

func TestArgv(t *testing.T) {
	got := argv(ModeDirenv, "/usr/bin/direnv", "/proj", "bazel test //...")
	want := []string{"/usr/bin/direnv", "exec", "/proj", "sh", "-c", "bazel test //..."}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv = %v, want %v", got, want)
		}
	}
	got = argv(ModeDirect, "/usr/bin/direnv", "/proj", "true")
	if len(got) != 3 || got[0] != "sh" || got[1] != "-c" || got[2] != "true" {
		t.Errorf("direct argv = %v", got)
	}
}

func TestWithArgsQuotes(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "cmd"},
		{[]string{"--test_filter=Foo"}, "cmd --test_filter=Foo"},
		{[]string{"a b", "it's"}, `cmd 'a b' 'it'\''s'`},
	}
	for _, tt := range tests {
		if got := withArgs("cmd", tt.args); got != tt.want {
			t.Errorf("withArgs(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestFormatMapOrdersBaselineFirst(t *testing.T) {
	got := FormatMap(VerbMap{"deploy": "d", "test": "t", "build": "b", "lint": "l"})
	want := "  build   b\n  test    t\n  lint    l\n  deploy  d\n"
	if got != want {
		t.Errorf("FormatMap =\n%q\nwant\n%q", got, want)
	}
}
