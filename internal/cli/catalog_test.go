// The index self-heals on
// read (create 3 projects, delete 1 → guppy list prunes it), and doctor
// flags a hand-corrupted project and fixes the safe subset without
// touching user edits.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

// catalogEnv sandboxes XDG so index/seen-log land in a temp root.
func catalogEnv(t *testing.T) (root string, env map[string]string) {
	t.Helper()
	root = t.TempDir()
	for k, v := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "cfg"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"HOME":            filepath.Join(root, "home"),
	} {
		t.Setenv(k, v)
	}
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })
	if err := os.MkdirAll(filepath.Join(root, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, map[string]string{
		"GUPPY_DEFAULTS_GIT": "false",
	}
}

// scaffoldIn scaffolds go-cli at dir with vars supplied.
func scaffoldIn(t *testing.T, env map[string]string, dir string) {
	t.Helper()
	code, _, stderr := run([]string{"new", dir, "--template", "go-cli", "--var", "name=" + filepath.Base(dir)}, env)
	if code != ExitOK {
		t.Fatalf("scaffold %s: exit = %d (stderr: %s)", dir, code, stderr)
	}
}

func TestListRecordsAndPrunes(t *testing.T) {
	root, env := catalogEnv(t)
	var dirs []string
	for _, name := range []string{"alpha", "beta", "gamma"} {
		d := filepath.Join(root, name)
		scaffoldIn(t, env, d)
		dirs = append(dirs, d)
	}

	code, stdout, stderr := run([]string{"project", "list"}, env)
	if code != ExitOK {
		t.Fatalf("list: exit = %d (stderr: %s)", code, stderr)
	}
	for _, want := range dirs {
		if !strings.Contains(stdout, want) {
			t.Errorf("list missing %s:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stdout, "alpha") || !strings.Contains(stdout, "LAST SEEN") {
		t.Errorf("list output lacks template/last-seen columns:\n%s", stdout)
	}

	if err := os.RemoveAll(dirs[1]); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = run([]string{"project", "list"}, env)
	if code != ExitOK {
		t.Fatalf("list after delete: exit = %d", code)
	}
	if strings.Contains(stdout, dirs[1]) {
		t.Errorf("deleted project survived prune-on-read:\n%s", stdout)
	}
	for _, want := range []string{dirs[0], dirs[2]} {
		if !strings.Contains(stdout, want) {
			t.Errorf("live project pruned by mistake (%s):\n%s", want, stdout)
		}
	}

	if err := os.Remove(filepath.Join(dirs[0], ".guppy", "project.toml")); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = run([]string{"project", "list"}, env)
	if code != ExitOK {
		t.Fatalf("list after marker delete: exit = %d", code)
	}
	if strings.Contains(stdout, dirs[0]) {
		t.Errorf("markerless project survived prune:\n%s", stdout)
	}
}

func TestListFoldsSeenLog(t *testing.T) {

	root, env := catalogEnv(t)
	d := filepath.Join(root, "hooked")
	scaffoldIn(t, env, d)

	manual := filepath.Join(root, "manual")
	if err := os.MkdirAll(filepath.Join(manual, ".guppy"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := "schema = 2\n\n[guppy]\nversion = \"dev\"\n\n[project]\ntemplate = \"go-cli\"\nversion = \"0.2.0\"\n"
	if err := os.WriteFile(filepath.Join(manual, ".guppy", "project.toml"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	seen := filepath.Join(root, "data", "guppy", "catalog", "seen.log")
	if err := os.MkdirAll(filepath.Dir(seen), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seen, []byte(manual+"\n"+manual+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run([]string{"project", "list"}, env)
	if code != ExitOK {
		t.Fatalf("list: exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, manual) {
		t.Errorf("folded project missing from list:\n%s", stdout)
	}

	if data, _ := os.ReadFile(seen); len(data) != 0 {
		t.Errorf("seen log not cleared after fold: %q", data)
	}
}

func TestListEmptyIsFriendly(t *testing.T) {
	_, env := catalogEnv(t)
	code, stdout, _ := run([]string{"project", "list"}, env)
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "no guppy-managed projects") {
		t.Errorf("stdout = %q, want friendly empty message", stdout)
	}
}

func TestDoctorFixesSafeSubsetOnly(t *testing.T) {
	root, env := catalogEnv(t)
	dir := filepath.Join(root, "proj")
	scaffoldIn(t, env, dir)

	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatal(err)
	}
	userEdit := "package main // my precious edit\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(userEdit), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runIn(t, dir, []string{"doctor"}, env)
	if code != ExitFailure {
		t.Errorf("drifted doctor: exit = %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(stdout, "missing") || !strings.Contains(stdout, "README.md") ||
		!strings.Contains(stdout, "modified") || !strings.Contains(stdout, "main.go") {
		t.Errorf("report should flag missing README + modified main.go:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--fix") {
		t.Errorf("report should mention --fix:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); !os.IsNotExist(err) {
		t.Error("doctor without --fix must not write")
	}

	code, stdout, _ = runIn(t, dir, []string{"doctor", "--fix"}, env)
	if code != ExitFailure {
		t.Errorf("post-fix doctor: exit = %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(stdout, "fixed") || !strings.Contains(stdout, "README.md") {
		t.Errorf("--fix output should report the restore:\n%s", stdout)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "README.md")); len(data) == 0 {
		t.Error("--fix did not restore README.md")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "main.go")); string(data) != userEdit {
		t.Errorf("--fix touched the user edit: %q", data)
	}

	code, stdout, _ = runIn(t, dir, []string{"doctor"}, env)
	if code != ExitFailure || !strings.Contains(stdout, "modified") {
		t.Errorf("modified-only drift: exit = %d, stdout = %s", code, stdout)
	}
}

func TestDoctorCleanProjectExitsZero(t *testing.T) {
	root, env := catalogEnv(t)
	dir := filepath.Join(root, "clean")
	scaffoldIn(t, env, dir)

	code, stdout, stderr := runIn(t, dir, []string{"doctor"}, env)
	if code != ExitOK {
		t.Errorf("clean doctor: exit = %d, want 0 (stdout: %s, stderr: %s)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "clean") {
		t.Errorf("stdout = %q, want clean verdict", stdout)
	}
}

func TestDoctorOutsideProjectIsInputError(t *testing.T) {
	_, env := catalogEnv(t)
	dir := t.TempDir()
	code, _, stderr := runIn(t, dir, []string{"doctor"}, env)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, ".guppy/project.toml") {
		t.Errorf("stderr = %q, want marker explanation", stderr)
	}
}

func TestDoctorAllBatchMode(t *testing.T) {
	root, env := catalogEnv(t)
	good := filepath.Join(root, "good")
	bad := filepath.Join(root, "bad")
	scaffoldIn(t, env, good)
	scaffoldIn(t, env, bad)
	if err := os.Remove(filepath.Join(bad, "README.md")); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := run([]string{"doctor", "--all"}, env)
	if code != ExitFailure {
		t.Errorf("batch doctor: exit = %d, want %d (stdout: %s)", code, ExitFailure, stdout)
	}
	if !strings.Contains(stdout, "[OK ]") || !strings.Contains(stdout, good) {
		t.Errorf("batch output should mark the clean project ok:\n%s", stdout)
	}
	if !strings.Contains(stdout, "missing") || !strings.Contains(stdout, "README.md") {
		t.Errorf("batch output should flag the drifted project:\n%s", stdout)
	}

	code, _, _ = run([]string{"doctor", "--all", "--fix"}, env)
	if code != ExitOK {
		t.Errorf("batch fix: exit = %d, want 0", code)
	}
}

// TestDoctorAllMarkerFailureIsDiagnostic pins the stream policy
// (U7): a per-project failure record (unreadable marker) is a
// diagnostic — stderr — never mixed into the stdout report stream.
func TestDoctorAllMarkerFailureIsDiagnostic(t *testing.T) {
	root, env := catalogEnv(t)
	good := filepath.Join(root, "good")
	broken := filepath.Join(root, "broken")
	scaffoldIn(t, env, good)
	scaffoldIn(t, env, broken)

	marker := filepath.Join(broken, ".guppy", "project.toml")
	if err := os.WriteFile(marker, []byte("schema = 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := run([]string{"doctor", "--all"}, env)
	if code != ExitFailure {
		t.Errorf("batch doctor: exit = %d, want %d (stderr: %s)", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "FAIL") || !strings.Contains(stderr, broken) {
		t.Errorf("stderr should carry the per-project failure record:\n%s", stderr)
	}
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout must not carry failure records, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "[OK ]") || !strings.Contains(stdout, good) {
		t.Errorf("stdout report should still mark the clean project ok:\n%s", stdout)
	}
}

func TestInitShellEmitsAndInstalls(t *testing.T) {
	root, env := catalogEnv(t)
	env["SHELL"] = "/bin/zsh"

	code, stdout, stderr := run([]string{"shell-env"}, env)
	if code != ExitOK {
		t.Fatalf("init-shell: exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, ">>> guppy shellhook >>>") || !strings.Contains(stdout, "__guppy_hook") {
		t.Errorf("stdout should be the hook snippet:\n%s", stdout)
	}
	if !strings.Contains(stdout, filepath.Join(root, "data", "guppy", "catalog", "seen.log")) {
		t.Errorf("hook should bake in the sandboxed seen-log path:\n%s", stdout)
	}

	rc := filepath.Join(root, "home", ".zshrc")
	if err := os.WriteFile(rc, []byte("export EDITOR=vim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run([]string{"shell-env", "--install"}, env)
	if code != ExitOK {
		t.Fatalf("init-shell --install: exit = %d (stderr: %s)", code, stderr)
	}
	code, _, _ = run([]string{"shell-env", "--install"}, env)
	if code != ExitOK {
		t.Fatalf("re-install: exit = %d", code)
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), ">>> guppy shellhook >>>") != 1 {
		t.Errorf("re-install duplicated the block:\n%s", data)
	}
	if !strings.Contains(string(data), "export EDITOR=vim") {
		t.Errorf("install clobbered the rc file:\n%s", data)
	}

	dir := filepath.Join(root, "hooked")
	scaffoldIn(t, env, dir)
	code, stdout, _ = runIn(t, dir, []string{"doctor"}, env)
	if code != ExitOK {
		t.Fatalf("doctor: exit = %d (stdout: %s)", code, stdout)
	}
	if strings.Contains(stdout, "shellhook not installed") {
		t.Errorf("hook installed but doctor still warns:\n%s", stdout)
	}
}

func TestInitShellRejectsUnsupportedShell(t *testing.T) {
	_, env := catalogEnv(t)
	code, _, stderr := run([]string{"shell-env", "fish"}, env)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, "unsupported shell") {
		t.Errorf("stderr = %q, want unsupported-shell error", stderr)
	}
}
