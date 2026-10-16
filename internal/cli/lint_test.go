package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintCleanBuiltin(t *testing.T) {
	code, stdout, _ := run([]string{"dev", "lint", "go-cli"}, nil)
	if code != ExitOK {
		t.Errorf("exit = %d, want 0 (stdout: %s)", code, stdout)
	}
}

func TestLintReportsSpecTableErrors(t *testing.T) {
	dir := copyBuiltin(t, "go-cli")

	corrupt := filepath.Join(dir, "template", "go.mod.hbs")
	if err := os.WriteFile(corrupt, []byte("module {{ .module_path }}{{ .oops }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run([]string{"dev", "lint", dir}, nil)
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "[FAIL] go.mod.hbs: undeclared variable {{ .oops }}") {
		t.Errorf("stderr = %q, want undeclared-variable error", stderr)
	}

	if stdout != "" {
		t.Errorf("stdout = %q, want empty (lint findings are diagnostics, stderr)", stdout)
	}
}

func TestLintSchemaErrorsAndWarnings(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFiles(t, dir, map[string]string{
		"guppy.toml":     "id = \"Bad_ID\"\nversion = \"nope\"\nkind = \"website\"\nlanguage = \"cobol\"\n\n[defaults]\nwidgets = true\n",
		"template/x.txt": "x",
	})
	code, stdout, stderr := run([]string{"dev", "lint", dir}, nil)
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFailure, stderr)
	}
	for _, want := range []string{
		"[FAIL] invalid id",
		"[FAIL] invalid version",
		"[FAIL] invalid kind",
		"[FAIL] invalid language",
		`[FAIL] unknown default axis "widgets"`,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q: %s", want, stderr)
		}
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty (lint findings are diagnostics, stderr)", stdout)
	}
}

func TestLintExtendsCycleIsError(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFiles(t, dir, map[string]string{
		"guppy.toml": "id = \"a\"\nversion = \"1.0.0\"\nkind = \"lib\"\nlanguage = \"none\"\nextends = [\"b\"]\n",
		"template/x": "x",
	})

	code, stdout, stderr := run([]string{"dev", "lint", dir}, nil)
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, `extends "b"`) {
		t.Errorf("stderr = %q, want unresolvable-extends error", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty (lint findings are diagnostics, stderr)", stdout)
	}
}

func TestTestCatchesCorruptedBuiltInCopy(t *testing.T) {
	dir := copyBuiltin(t, "go-cli")
	code, _, stderr := run([]string{"dev", "test", dir}, nil)
	if code != ExitOK {
		t.Fatalf("clean copy must pass, exit = %d (stderr: %s)", code, stderr)
	}

	if err := os.WriteFile(filepath.Join(dir, "template", "go.mod.hbs"),
		[]byte("module {{ .module_path }}\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := run([]string{"dev", "test", dir}, nil)
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d (stdout: %s)", code, ExitFailure, stdout)
	}
	if !strings.Contains(stdout, "go.mod differs") {
		t.Errorf("stdout = %q, want go.mod diff", stdout)
	}
}

func TestTestUpdateThenPass(t *testing.T) {
	dir := copyBuiltin(t, "go-cli")

	if err := os.WriteFile(filepath.Join(dir, "template", "go.mod.hbs"),
		[]byte("module {{ .module_path }}\n\ngo 1.99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run([]string{"dev", "test", dir}, nil); code != ExitFailure {
		t.Fatalf("pre-update mismatch expected, got exit=%d stderr=%s", code, stderr)
	}
	code, _, stderr := run([]string{"dev", "test", "-u", dir}, nil)
	if code != ExitOK {
		t.Fatalf("update exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, stderr := run([]string{"dev", "test", dir}, nil); code != ExitOK {
		t.Fatalf("post-update exit = %d (stderr: %s)", code, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "testdata", "snapshot", "default.toml"))
	if !strings.Contains(string(data), "go 1.99") {
		t.Errorf("golden file should carry the new content: %s", data)
	}
}

func TestTemplateShowMergedChain(t *testing.T) {
	code, stdout, _ := run([]string{"template", "show", "go-cli"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d (stdout: %s)", code, stdout)
	}
	for _, want := range []string{
		"builtin/go-cli@0.2.0 (builtin)",
		"builtin/base-toolchain@0.2.0",
		"module_path",
		`default="github.com/acme/{{ .name }}"`,
		"agents_md=true",
		"main.go.hbs",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("show output missing %q:\n%s", want, stdout)
		}
	}

	code, stdout, _ = run([]string{"template", "show", "go-cli", "--contents"}, nil)
	if code != ExitOK {
		t.Fatalf("contents exit = %d", code)
	}
	if !strings.Contains(stdout, "--- go.mod.hbs ---") || !strings.Contains(stdout, "module {{ .module_path }}") {
		t.Errorf("--contents should dump merged payload:\n%s", stdout)
	}
}
