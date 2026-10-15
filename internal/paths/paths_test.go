package paths

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
)

// withEnv sets env vars for the duration of a test and reloads the xdg
// layer (which caches at init — ADR 0007 notes Reload() exists for exactly
// this).
func withEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })
}

func TestResolveHonorsXdgEnv(t *testing.T) {
	root := t.TempDir()
	withEnv(t, map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "cfg"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"HOME":            filepath.Join(root, "home"),
	})

	l := Resolve()
	assertEq(t, "ConfigHome", l.ConfigHome, filepath.Join(root, "cfg"))
	assertEq(t, "DataHome", l.DataHome, filepath.Join(root, "data"))
	assertEq(t, "CacheHome", l.CacheHome, filepath.Join(root, "cache"))

	assertEq(t, "BinDir", l.BinDir, filepath.Join(root, "home", ".local", "bin"))
}

func TestLayoutPaths(t *testing.T) {
	root := t.TempDir()
	withEnv(t, map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "cfg"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
		"HOME":            filepath.Join(root, "home"),
	})
	l := Resolve()

	assertEq(t, "SettingsFile", l.SettingsFile(), filepath.Join(root, "cfg", "guppy", "guppy.toml"))
	assertEq(t, "DataDir", l.DataDir(), filepath.Join(root, "data", "guppy"))
	assertEq(t, "TemplatesDir", l.TemplatesDir(), filepath.Join(root, "data", "guppy", "templates"))
	assertEq(t, "CommunityDir", l.CommunityDir(), filepath.Join(root, "data", "guppy", "community"))
	assertEq(t, "IndexFile", l.IndexFile(), filepath.Join(root, "data", "guppy", "index.toml"))
	assertEq(t, "CatalogDir", l.CatalogDir(), filepath.Join(root, "data", "guppy", "catalog"))
	assertEq(t, "CacheDir", l.CacheDir(), filepath.Join(root, "cache", "guppy"))
	assertEq(t, "BinaryPath", l.BinaryPath(), filepath.Join(root, "home", ".local", "bin", BinaryName()))
}

func TestResolveFallsBackToHome(t *testing.T) {

	home := t.TempDir()
	l := Layout{
		ConfigHome: firstNonEmpty("", homeJoin(home, ".config")),
		DataHome:   firstNonEmpty("", homeJoin(home, ".local", "share")),
	}
	assertEq(t, "ConfigHome fallback", l.ConfigHome, filepath.Join(home, ".config"))
	assertEq(t, "DataHome fallback", l.DataHome, filepath.Join(home, ".local", "share"))
}

func TestBinaryName(t *testing.T) {
	want := "guppy"
	if runtime.GOOS == "windows" {
		want = "guppy.exe"
	}
	assertEq(t, "BinaryName", BinaryName(), want)
}

func assertEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}
