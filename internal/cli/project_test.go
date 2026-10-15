package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvokedAsGP(t *testing.T) {
	tests := []struct {
		argv0 string
		want  bool
	}{
		{"/usr/local/bin/gp", true},
		{"gp", true},
		{`C:\tools\gp.exe`, true},
		{"/usr/local/bin/guppy", false},
		{"guppy", false},
		{"guppyx", false},
	}
	for _, tt := range tests {
		if got := InvokedAsGP(tt.argv0); got != tt.want {
			t.Errorf("InvokedAsGP(%q) = %v, want %v", tt.argv0, got, tt.want)
		}
	}
}

func TestArgsForGPShim(t *testing.T) {
	got := ArgsFor("/x/gp", []string{"test", "--foo"})
	if len(got) != 3 || got[0] != "project" || got[1] != "test" || got[2] != "--foo" {
		t.Errorf("ArgsFor = %v", got)
	}
	if got := ArgsFor("/x/guppy", []string{"new"}); len(got) != 1 || got[0] != "new" {
		t.Errorf("ArgsFor = %v", got)
	}
}

// writeProject creates a guppy-managed project dir with the given [gp]
// map body, returning its root.
func writeProject(t *testing.T, gpBody string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".guppy"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := "schema = 1\n\n[guppy]\nversion = \"dev\"\n\n[project]\ntemplate = \"go-cli\"\nversion = \"0.2.0\"\n"
	if gpBody != "" {
		marker += "\n[gp]\n" + gpBody
	}
	if err := os.WriteFile(filepath.Join(root, ".guppy", "project.toml"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// runIn runs the CLI in dir (chdir restored afterwards — Execute's
// --directory handling chdirs the process).
func runIn(t *testing.T, dir string, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return run(append([]string{"--directory", dir}, args...), env)
}

func TestProjectOutsideIsInputError(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runIn(t, dir, []string{"project", "test"}, nil)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d", code, ExitInput)
	}
	if !strings.Contains(stderr, ".guppy/project.toml") || !strings.Contains(stderr, "guppy new") {
		t.Errorf("stderr = %q, want marker path + scaffold hint", stderr)
	}
}

func TestProjectUnknownVerbPrintsMap(t *testing.T) {
	root := writeProject(t, "build = \"echo building\"\ntest = \"echo testing\"\n")
	code, _, stderr := runIn(t, root, []string{"project", "deploy"}, nil)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d", code, ExitInput)
	}
	if !strings.Contains(stderr, "unknown verb") || !strings.Contains(stderr, "build") || !strings.Contains(stderr, "echo building") {
		t.Errorf("stderr = %q, want verb + map", stderr)
	}
}

func TestProjectVerbRuns(t *testing.T) {
	root := writeProject(t, "build = \"echo building\"\ntest = \"echo testing $((1+1))\"\n")
	code, stdout, _ := runIn(t, root, []string{"project", "test"}, nil)
	if code != ExitOK {
		t.Errorf("exit = %d, want 0 (stdout: %s)", code, stdout)
	}
	if !strings.Contains(stdout, "testing 2") {
		t.Errorf("stdout = %q, want verb output (shell-evaluated)", stdout)
	}
}

func TestProjectVerbPassThroughArgs(t *testing.T) {
	root := writeProject(t, "test = \"echo testing\"\n")
	code, stdout, _ := runIn(t, root, []string{"project", "test", "--test_filter=a b"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d (stdout: %s)", code, stdout)
	}
	if !strings.Contains(stdout, "--test_filter=a b") {
		t.Errorf("stdout = %q, want quoted pass-through arg intact", stdout)
	}
}

func TestProjectVerbFailureIsExit1(t *testing.T) {
	root := writeProject(t, "test = \"exit 7\"\n")
	code, _, stderr := runIn(t, root, []string{"project", "test"}, nil)
	if code != ExitFailure {
		t.Errorf("exit = %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(stderr, "exit 7") {
		t.Errorf("stderr = %q, want child exit code", stderr)
	}
}

// TestGlobalFlagAfterVerbWarns pins (styleguide U3): a
// known guppy global flag in the pass-through region still reaches the
// verb, but the user is warned instead of silently re-purposed. Shorthand
// and =value forms warn under their canonical long name.
func TestGlobalFlagAfterVerbWarns(t *testing.T) {
	root := writeProject(t, "build = \"echo building\"\n")
	code, stdout, stderr := runIn(t, root, []string{"project", "build", "--offline", "-q", "--color=always"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "building") {
		t.Errorf("stdout = %q, flags must still pass through to the verb", stdout)
	}
	for _, want := range []string{
		"--offline reached the verb, not guppy",
		"--quiet reached the verb, not guppy",
		"--color reached the verb, not guppy",
		"before the verb",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
}

// TestHelpAfterVerbIsNotWarned pins the deliberate exception
// (styleguide U3): --help after the verb is the invoked tool's help —
// pass-through by design, never trapped as a misplaced global flag.
func TestHelpAfterVerbIsNotWarned(t *testing.T) {
	root := writeProject(t, "build = \"echo building --help\"\n")
	code, stdout, stderr := runIn(t, root, []string{"project", "build", "--help"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "--help") {
		t.Errorf("stdout = %q, --help must reach the verb", stdout)
	}
	if strings.Contains(stderr, "before the verb") {
		t.Errorf("--help is deliberate pass-through, no warning wanted:\n%s", stderr)
	}
}

func TestProjectNoArgsPrintsMap(t *testing.T) {
	root := writeProject(t, "build = \"echo building\"\ntest = \"echo testing\"\n")
	code, stdout, _ := runIn(t, root, []string{"project"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d (stdout: %s)", code, stdout)
	}
	for _, want := range []string{"build", "test", "doctor", "echo building"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestProjectDoctorAudits(t *testing.T) {

	dir := filepath.Join(t.TempDir(), "mycli")
	code, _, stderr := run([]string{"new", dir, "--template", "go-cli", "--var", "name=mycli"}, nil)
	if code != ExitOK {
		t.Fatalf("new: exit = %d (stderr: %s)", code, stderr)
	}

	code, stdout, _ := runIn(t, dir, []string{"project", "doctor"}, nil)
	if code != ExitOK {
		t.Errorf("clean doctor: exit = %d, want 0 (stdout: %s)", code, stdout)
	}
	if !strings.Contains(stdout, "clean") {
		t.Errorf("stdout = %q, want clean verdict", stdout)
	}

	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = runIn(t, dir, []string{"project", "doctor"}, nil)
	if code != ExitFailure {
		t.Errorf("drifted doctor: exit = %d, want %d (stdout: %s)", code, ExitFailure, stdout)
	}
	if !strings.Contains(stdout, "missing") || !strings.Contains(stdout, "README.md") {
		t.Errorf("stdout = %q, want missing-README finding", stdout)
	}
}

// The scaffold → project-mode round trip: `guppy new` writes the marker,
// and `guppy project <verb>` dispatches through it.
func TestNewThenProjectMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "widget")
	code, _, stderr := run([]string{"new", dir, "--template", "go-cli", "--var", "name=widget"}, nil)
	if code != ExitOK {
		t.Fatalf("new: exit = %d (stderr: %s)", code, stderr)
	}
	marker, err := os.ReadFile(filepath.Join(dir, ".guppy", "project.toml"))
	if err != nil {
		t.Fatalf("marker: %v", err)
	}
	for _, want := range []string{
		"schema = 2",
		"[project]",
		`template = "go-cli"`,
		`base-toolchain@0.2.0`,
		`name = "widget"`,
		`[toggles]`,
		"[gp]",
		`build = "bazel build //..."`,
		`test = "bazel test //..."`,
		`lint = "bazel run //:gazelle -- -mode=diff"`,
	} {
		if !strings.Contains(string(marker), want) {
			t.Errorf("marker missing %q:\n%s", want, marker)
		}
	}

	code, _, stderr = run([]string{"new", dir, "--template", "go-cli", "--var", "name=widget", "--force"}, nil)
	if code != ExitOK {
		t.Fatalf("renew: exit = %d (stderr: %s)", code, stderr)
	}
	if strings.Contains(stderr, "never clobbered") {
		t.Errorf("re-run reported kept files, want idempotent no-op (stderr: %s)", stderr)
	}

	code, stdout, _ := runIn(t, dir, []string{"project"}, nil)
	if code != ExitOK || !strings.Contains(stdout, "bazel test //...") {
		t.Errorf("project map: exit = %d, stdout = %q", code, stdout)
	}
}

// --no-bazel scaffolds carry no bazel verbs in the marker (the requires
// axis is off); the recorded toggles still say bazel = false — that's
// the provenance doctor re-renders from.
func TestNewNoBazelDropsBazelVerbs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bare")
	code, _, stderr := run([]string{"new", dir, "--template", "go-cli", "--var", "name=bare", "--no-bazel"}, nil)
	if code != ExitOK {
		t.Fatalf("new: exit = %d (stderr: %s)", code, stderr)
	}
	marker, err := os.ReadFile(filepath.Join(dir, ".guppy", "project.toml"))
	if err != nil {
		t.Fatalf("marker: %v", err)
	}
	if strings.Contains(string(marker), "bazel build") || strings.Contains(string(marker), "bazel test") {
		t.Errorf("--no-bazel marker still carries bazel verbs:\n%s", marker)
	}

	if _, err := os.Stat(filepath.Join(dir, "MODULE.bazel")); !os.IsNotExist(err) {
		t.Errorf("MODULE.bazel should not exist with --no-bazel (err = %v)", err)
	}
}

func TestProjectDirectFallbackHintsWithoutDirenv(t *testing.T) {

	root := writeProject(t, "test = \"echo testing\"\n")
	if err := os.WriteFile(filepath.Join(root, ".envrc"), []byte("source_env tools/env/setup.sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	code, stdout, stderr := run([]string{"--directory", root, "project", "test"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "testing") {
		t.Errorf("stdout = %q, want verb output without direnv", stdout)
	}
	if !strings.Contains(stderr, "direnv not found") {
		t.Errorf("stderr = %q, want direnv fallback hint", stderr)
	}
}
