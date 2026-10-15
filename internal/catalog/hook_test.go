package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHookScriptIsStubOnHotPath(t *testing.T) {
	h := Hook{SeenLog: "/data/guppy/catalog/seen.log"}
	script := h.Script()

	assert.Assert(t, strings.Contains(script, `[ -f "$PWD/`+projectMarkerRel()+`" ]`))
	assert.Assert(t, strings.Contains(script, `>> '/data/guppy/catalog/seen.log'`))
	assert.Assert(t, !strings.Contains(script, "guppy list") && !strings.Contains(script, "$(guppy"))

	assert.Assert(t, strings.Contains(script, "add-zsh-hook precmd __guppy_hook"))
	assert.Assert(t, strings.Contains(script, `PROMPT_COMMAND="__guppy_hook`))

	assert.Assert(t, strings.HasPrefix(script, HookMarkerOpen))
	assert.Assert(t, strings.HasSuffix(strings.TrimSpace(script), HookMarkerClose))
}

func TestHookScriptQuotesPathsWithSpaces(t *testing.T) {
	h := Hook{SeenLog: "/home/me/my data/seen.log"}
	assert.Assert(t, strings.Contains(h.Script(), `'/home/me/my data/seen.log'`))

	h = Hook{SeenLog: "/home/me/it's/seen.log"}
	assert.Assert(t, strings.Contains(h.Script(), `'/home/me/it'\''s/seen.log'`))
}

func TestInstallHookIsIdempotentAndRemovable(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	h := Hook{SeenLog: filepath.Join(dir, "catalog", "seen.log")}

	assert.NilError(t, InstallHook(rc, h))
	assert.NilError(t, InstallHook(rc, h))

	data, err := os.ReadFile(rc)
	assert.NilError(t, err)
	assert.Equal(t, strings.Count(string(data), HookMarkerOpen), 1)
	assert.Equal(t, strings.Count(string(data), HookMarkerClose), 1)

	if _, err := os.Stat(filepath.Join(dir, "catalog")); err != nil {
		t.Errorf("catalog dir not created: %v", err)
	}

	assert.NilError(t, os.WriteFile(rc, []byte("alias ll='ls -l'\n"+h.Script()+"\nexport EDITOR=vim\n"), 0o644))
	removed, err := removeBlockForTest(rc)
	assert.NilError(t, err)
	assert.Assert(t, removed)
	data, err = os.ReadFile(rc)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(data), "alias ll"))
	assert.Assert(t, strings.Contains(string(data), "export EDITOR=vim"))
	assert.Assert(t, !strings.Contains(string(data), HookMarkerOpen))
}

func TestInstallHookPreservesUnterminatedBlock(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".bashrc")
	content := "before\n" + HookMarkerOpen + "\npartial\n"
	assert.NilError(t, os.WriteFile(rc, []byte(content), 0o644))

	err := InstallHook(rc, Hook{SeenLog: filepath.Join(dir, "seen.log")})
	assert.ErrorContains(t, err, "unterminated")
	data, _ := os.ReadFile(rc)
	assert.Equal(t, string(data), content)
}

func TestHookInstalledProbes(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	assert.Assert(t, !HookInstalled([]string{rc}))
	assert.NilError(t, os.WriteFile(rc, []byte(HookMarkerOpen+"\n"+HookMarkerClose+"\n"), 0o644))
	assert.Assert(t, HookInstalled([]string{rc}))
}

// removeBlockForTest mirrors selfmanage's block removal against the
// hook markers (same algorithm, exercised here so hook tests don't
// import selfmanage).
func removeBlockForTest(rc string) (bool, error) {
	data, err := os.ReadFile(rc)
	if err != nil {
		return false, err
	}
	out, err := replaceMarkedBlock(string(data), HookMarkerOpen, HookMarkerClose, "")
	if err != nil {
		return false, err
	}
	if out == string(data) {
		return false, nil
	}
	return true, os.WriteFile(rc, []byte(out), 0o644)
}

func projectMarkerRel() string {
	h := Hook{SeenLog: "x"}

	s := h.Script()
	_, after, _ := strings.Cut(s, `[ -f "$PWD/`)
	rest := after
	return rest[:strings.Index(rest, `" ]`)]
}
