package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicedata-work/guppy/internal/templates"
)

// copyBuiltin copies an embedded built-in template to a temp dir (the
// local-authoring view of it).
func copyBuiltin(t *testing.T, id string) string {
	t.Helper()
	src := templates.BuiltinTemplateFS(id)
	dst := t.TempDir()
	err := fs.WalkDir(src, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, rel)
		if err != nil {
			return err
		}
		dest := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy builtin %s: %v", id, err)
	}
	return dst
}

// noGit disables the git default so `guppy new` never shells out in tests.
func noGit(env map[string]string) map[string]string {
	if env == nil {
		env = map[string]string{}
	}
	env["GUPPY_DEFAULTS_GIT"] = "false"
	return env
}

func TestNewScaffoldsProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-cli")
	code, _, stderr := run([]string{"new", "--template", "go-cli", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	for path, wantSub := range map[string]string{
		"go.mod":     "module github.com/acme/my-cli",
		"main.go":    "Command my-cli is a command-line tool",
		"README.md":  "# my-cli",
		".gitignore": ".direnv/",
		"AGENTS.md":  "Guidance for agents",
	} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if !strings.Contains(string(data), wantSub) {
			t.Errorf("%s = %q, want substring %q", path, data, wantSub)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		t.Error("git default was off; no .git expected")
	}
}

func TestNewToggleFlagsAndBare(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x")
	code, _, stderr := run([]string{"new", "--template", "go-cli", "--no-agents-md", "--no-direnv", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("--no-agents-md must omit AGENTS.md entirely")
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gi), ".direnv/") {
		t.Error("--no-direnv must drop direnv ignores")
	}

	bare := filepath.Join(t.TempDir(), "bare")
	code, _, stderr = run([]string{"new", "--template", "go-cli", "--bare", bare}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("bare exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(bare, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("--bare must omit toggle-gated files")
	}
}

func TestNewDirectoryGateAndForce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "existing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := run([]string{"new", "--template", "go-cli", dir}, noGit(nil))
	if code != ExitInput {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("stderr should suggest --force: %s", stderr)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "keep.txt")); string(data) != "precious" {
		t.Error("refusal must not touch the tree")
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("USER EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run([]string{"new", "--template", "go-cli", "--force", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("force exit = %d (stderr: %s)", code, stderr)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "main.go")); string(data) != "USER EDIT" {
		t.Error("--force must never clobber user content")
	}
	if !strings.Contains(stderr, "kept main.go") {
		t.Errorf("kept files must be reported: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Error("--force must still create missing files")
	}

	code, _, stderr = run([]string{"new", "--template", "go-cli", "--force", dir}, noGit(nil))
	if code != ExitOK || !strings.Contains(stderr, "0 created") {
		t.Errorf("idempotent re-run: exit=%d stderr=%s", code, stderr)
	}
}

func TestNewAmbiguousTemplateIsInputError(t *testing.T) {
	code, _, stderr := run([]string{"new", filepath.Join(t.TempDir(), "p")}, noGit(nil))
	if code != ExitInput {
		t.Fatalf("exit = %d, want %d", code, ExitInput)
	}
	if !strings.Contains(stderr, "ambiguous") || !strings.Contains(stderr, "--template") {
		t.Errorf("stderr = %q, want ambiguity listing", stderr)
	}
}

func TestNewKindLanguageMatrix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "svc")
	code, _, stderr := run([]string{"new", "--kind", "cli", "--lang", "go", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Error("kind=cli lang=go must resolve to go-cli")
	}
}

func TestNewMissingVarIsInputError(t *testing.T) {

	tmpl := t.TempDir()
	writeTemplateFiles(t, tmpl, map[string]string{
		"guppy.toml":           "id = \"req\"\nversion = \"0.1.0\"\nkind = \"lib\"\nlanguage = \"none\"\n\n[vars]\nname = { prompt = \"Name\" }\nauthor = { prompt = \"Author\" }\n",
		"template/CREDITS.hbs": "by {{ .author }}\n",
	})
	dir := filepath.Join(t.TempDir(), "out")
	code, _, stderr := run([]string{"new", "--template", tmpl, dir}, noGit(nil))
	if code != ExitInput {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, "--var author") {
		t.Errorf("stderr = %q, want --var hint", stderr)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("nothing may be written when input is unresolved")
	}

	code, _, stderr = run([]string{"new", "--template", tmpl, "--var", "author=Ada", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "CREDITS"))
	if string(data) != "by Ada\n" {
		t.Errorf("CREDITS = %q", data)
	}
}

func TestNewDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dry")
	code, stdout, _ := run([]string{"new", "--template", "go-cli", "--dry-run", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "main.go") {
		t.Errorf("dry-run should list files, got %q", stdout)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("dry-run must not create the directory")
	}
}

func TestNewNameVarOverrides(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dirname")
	code, _, stderr := run([]string{"new", "--template", "go-cli", "--var", "name=custom", dir}, noGit(nil))
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if !strings.Contains(string(data), "# custom") {
		t.Errorf("--var name must win over the directory name: %q", data)
	}
}

func writeTemplateFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		dest := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
