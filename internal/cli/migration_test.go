// The command-refactor migration surface (docs: guppy-projects
// cli-commands-refactor): project gains lifecycle subcommands (new/list/
// jump/clear) beside the [gp] verb dispatcher, the authoring loop moves
// under `guppy dev`, config becomes a noun-with-verbs, update grows the
// `--all` / `.` ladder, and template gains refresh.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProjectLifecycleCommands covers new → jump → clear over the real
// index: jump resolves by base name, clear forgets everything.
func TestProjectLifecycleCommands(t *testing.T) {
	root, env := catalogEnv(t)
	dir := filepath.Join(root, "alpha")
	scaffoldProject(t, env, dir, "go-cli")

	if code, stdout, stderr := run([]string{"project", "jump", "alpha"}, env); code != ExitOK || strings.TrimSpace(stdout) != dir {
		t.Fatalf("jump: exit = %d, stdout = %q (stderr: %s)", code, stdout, stderr)
	}

	// jump without a name needs exactly one project to be unambiguous;
	// with more it must refuse and list the candidates.
	scaffoldProject(t, env, filepath.Join(root, "beta"), "go-cli")
	if code, _, stderr := run([]string{"project", "jump"}, env); code != ExitInput || !strings.Contains(stderr, "multiple projects") {
		t.Errorf("ambiguous jump: exit = %d, stderr = %q", code, stderr)
	}
	if code, _, stderr := run([]string{"project", "jump", "nope"}, env); code != ExitInput || !strings.Contains(stderr, "no project named") {
		t.Errorf("unknown jump: exit = %d, stderr = %q", code, stderr)
	}

	if code, _, stderr := run([]string{"project", "clear"}, env); code != ExitOK {
		t.Fatalf("clear: exit = %d (stderr: %s)", code, stderr)
	}
	if code, stdout, _ := run([]string{"project", "list"}, env); code != ExitOK || !strings.Contains(stdout, "no guppy-managed projects found") {
		t.Errorf("post-clear list: exit = %d, stdout = %q", code, stdout)
	}
}

// TestProjectBuiltinsShadowMapVerbs pins the precedence decision: the
// project's [gp] map may define its own "list" verb, but the built-in
// lifecycle subcommand wins (`gp`'s explicit fallback is the escape
// hatch for map verbs).
func TestProjectBuiltinsShadowMapVerbs(t *testing.T) {
	root, env := catalogEnv(t)
	templates := filepath.Join(root, "templates")
	writeDevTemplate(t, templates, "mytpl", "0.1.0", "list = \"echo custom-list-verb\"\n")
	env["GUPPY_TEMPLATE_DIRS"] = templates

	dir := filepath.Join(root, "proj")
	scaffoldProject(t, env, dir, "mytpl")

	code, stdout, stderr := run([]string{"project", "list"}, env)
	if code != ExitOK || !strings.Contains(stdout, "PROJECT") {
		t.Fatalf("built-in list must win over the map verb: exit = %d, stdout = %q (stderr: %s)", code, stdout, stderr)
	}
	// A non-colliding map verb still dispatches (verb dispatch needs the
	// project face's FindUp, so run inside the project). The verb's exec
	// output lands on the CLI's stdout.
	if code, stdout, _ := runIn(t, dir, []string{"project", "greet"}, env); code != ExitOK || !strings.Contains(stdout, "base-greet") {
		t.Fatalf("map verb greet: exit = %d, stdout = %q", code, stdout)
	}
}

// TestDevLoop covers dev new → lint → test → publish on a fresh shell:
// the seeded snapshot keeps test green from the start, and publish
// produces the tarball + checksum + index entry the registry PR needs.
func TestDevLoop(t *testing.T) {
	root, env := catalogEnv(t)
	dir := filepath.Join(root, "mytpl")

	if code, _, stderr := run([]string{"dev", "new", dir, "--description", "My test template"}, env); code != ExitOK {
		t.Fatalf("dev new: exit = %d (stderr: %s)", code, stderr)
	}
	for _, name := range []string{"guppy.toml", filepath.Join("template", "README.md"), filepath.Join("testdata", "snapshot", "default.toml")} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("dev new: missing %s", name)
		}
	}

	if code, _, stderr := run([]string{"dev", "lint", dir}, env); code != ExitOK {
		t.Fatalf("dev lint: exit = %d (stderr: %s)", code, stderr)
	}
	if code, _, stderr := run([]string{"dev", "test", dir}, env); code != ExitOK {
		t.Fatalf("dev test on the seeded snapshot: exit = %d (stderr: %s)", code, stderr)
	}

	out := filepath.Join(root, "dist")
	code, stdout, stderr := run([]string{"dev", "publish", dir, "--out", out}, env)
	if code != ExitOK {
		t.Fatalf("dev publish: exit = %d (stderr: %s)", code, stderr)
	}
	tball := filepath.Join(out, "guppy-mytpl-0.1.0.tar.gz")
	if _, err := os.Stat(tball); err != nil {
		t.Errorf("publish: missing %s", tball)
	}
	if _, err := os.Stat(tball + ".sha256"); err != nil {
		t.Errorf("publish: missing checksum sidecar")
	}
	if !strings.Contains(stdout, `"id": "mytpl"`) || !strings.Contains(stdout, `"guppy-template"`) {
		t.Errorf("publish: stdout should carry the index entry, got %q", stdout)
	}

	// dev list starts empty and says how to register one (the friendly
	// line is real output, not a diagnostic — stdout).
	if code, stdout, _ := run([]string{"dev", "list"}, env); code != ExitOK || !strings.Contains(stdout, "no locally developed templates") {
		t.Errorf("dev list: exit = %d, stdout = %q", code, stdout)
	}
}

// TestConfigVerbs covers get/set/list over an explicit --config file,
// including that set writes TOML a reload reads back.
func TestConfigVerbs(t *testing.T) {
	root, env := catalogEnv(t)
	cfg := filepath.Join(root, "guppy.toml")
	if err := os.WriteFile(cfg, []byte("# my settings\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env["GUPPY_CONFIG"] = cfg

	if code, _, stderr := run([]string{"config", "set", "shell.hook", "false"}, env); code != ExitOK {
		t.Fatalf("config set: exit = %d (stderr: %s)", code, stderr)
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "[shell]") || !strings.Contains(string(data), "hook = false") {
		t.Errorf("config set must edit the file, got %q", string(data))
	}
	if !strings.Contains(string(data), "# my settings") {
		t.Errorf("config set must preserve comments, got %q", string(data))
	}
	if code, stdout, _ := run([]string{"config", "get", "shell.hook"}, env); code != ExitOK || strings.TrimSpace(stdout) != "false" {
		t.Errorf("config get: exit = %d, stdout = %q", code, stdout)
	}
	if code, stdout, _ := run([]string{"config", "list"}, env); code != ExitOK || !strings.Contains(stdout, "template.registry") {
		t.Errorf("config list: exit = %d, stdout = %q", code, stdout)
	}
	if code, _, stderr := run([]string{"config", "get", "bogus.key"}, env); code != ExitInput || !strings.Contains(stderr, "unknown config key") {
		t.Errorf("unknown key: exit = %d, stderr = %q", code, stderr)
	}
	if code, _, stderr := run([]string{"config", "set", "shell.hook", "maybe"}, env); code != ExitInput || !strings.Contains(stderr, "want true or false") {
		t.Errorf("bad value: exit = %d, stderr = %q", code, stderr)
	}
}

// TestUpdateDotLadder covers `update .` against a local template whose
// version moves: the project re-renders, the marker advances, and a
// second run reports up-to-date.
func TestUpdateDotLadder(t *testing.T) {
	root, env := catalogEnv(t)
	templates := filepath.Join(root, "templates")
	writeDevTemplate(t, templates, "mytpl", "0.1.0", "")
	env["GUPPY_TEMPLATE_DIRS"] = templates

	dir := filepath.Join(root, "proj")
	scaffoldProject(t, env, dir, "mytpl")
	markerPath := filepath.Join(dir, ".guppy", "project.toml")
	assertMarkerVersion(t, markerPath, "0.1.0")

	// Ship a new template version with a new payload file.
	writeDevTemplate(t, templates, "mytpl", "0.2.0", "")
	if err := os.WriteFile(filepath.Join(templates, "mytpl", "template", "NEW.md"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runIn(t, dir, []string{"update", "."}, env)
	if code != ExitOK {
		t.Fatalf("update .: exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "0.1.0 → 0.2.0") {
		t.Errorf("update . should report the version move, got %q", stderr)
	}
	assertMarkerVersion(t, markerPath, "0.2.0")
	if _, err := os.Stat(filepath.Join(dir, "NEW.md")); err != nil {
		t.Errorf("update .: new payload file not rendered")
	}

	if code, _, stderr := runIn(t, dir, []string{"update", "."}, env); code != ExitOK || !strings.Contains(stderr, "latest version") {
		t.Errorf("second update .: exit = %d (stderr: %s)", code, stderr)
	}

	// PATH + --all is refused: the ladder is either/or.
	if code, _, stderr := run([]string{"update", ".", "--all"}, env); code != ExitInput || !strings.Contains(stderr, "not both") {
		t.Errorf("update . --all: exit = %d, stderr = %q", code, stderr)
	}
}

// TestTemplateRefreshReportsEmpty covers the cold-cache no-op.
func TestTemplateRefreshReportsEmpty(t *testing.T) {
	_, env := catalogEnv(t)
	// infof is diagnostics: it lands on stderr (U7).
	if code, _, stderr := run([]string{"template", "refresh"}, env); code != ExitOK || !strings.Contains(stderr, "registry cache is empty") {
		t.Errorf("template refresh: exit = %d (stderr: %s)", code, stderr)
	}
}

// TestInteractiveIsOptIn pins the prompt gate: without --interactive the
// confirm path refuses with the flag to pass, even on a would-be TTY.
func TestInteractiveIsOptIn(t *testing.T) {
	_, env := catalogEnv(t)
	// stdin is a plain reader here, so the char-device check already
	// refuses; the assertion is the message naming --interactive.
	if code, _, stderr := run([]string{"uninstall"}, env); code != ExitInput || !strings.Contains(stderr, "--interactive") {
		t.Errorf("uninstall without --yes: exit = %d, stderr = %q", code, stderr)
	}
}

// scaffoldProject scaffolds name's template into a fresh dir under root.
func scaffoldProject(t *testing.T, env map[string]string, dir, template string) {
	t.Helper()
	code, _, stderr := run([]string{"project", "new", dir, "--template", template, "--var", "name=" + filepath.Base(dir)}, env)
	if code != ExitOK {
		t.Fatalf("project new %s: exit = %d (stderr: %s)", dir, code, stderr)
	}
}

// writeDevTemplate writes (or overwrites) a minimal local template
// <dir>/<id>/ with the given version and extra manifest text — the
// fixture behind the update-ladder and precedence tests.
func writeDevTemplate(t *testing.T, templatesDir, id, version, extraGP string) string {
	t.Helper()
	dir := filepath.Join(templatesDir, id)
	if err := os.MkdirAll(filepath.Join(dir, "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id          = \"" + id + "\"\n" +
		"version     = \"" + version + "\"\n" +
		"kind        = \"cli\"\n" +
		"language    = \"none\"\n" +
		"description = \"test template\"\n" +
		"\n[vars]\nname = { prompt = \"Project name\" }\n" +
		"\n[gp]\ngreet = \"echo base-greet\"\n" +
		extraGP + "\n"
	if err := os.WriteFile(filepath.Join(dir, "guppy.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "template", "README.md"), []byte("# {{ .name }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertMarkerVersion(t *testing.T, markerPath, want string) {
	t.Helper()
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !strings.Contains(string(data), "version = \""+want+"\"") {
		t.Errorf("marker version = want %s, got:\n%s", want, string(data))
	}
}
