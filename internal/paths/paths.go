// Package paths resolves guppy's on-disk layout (design §10.2, ADR 0007).
//
// This package is the single chokepoint for path resolution: nothing else
// in the codebase may call xdg.* or os.User*Dir directly. Defaults follow
// uv's XDG layout, honored per-OS, overridable via XDG_*_HOME on
// Linux/macOS and the standard Windows equivalents.
package paths

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
)

// Layout is the resolved set of base directories guppy reads and writes.
type Layout struct {
	ConfigHome string

	DataHome string

	CacheHome string

	BinDir string
}

// Resolve returns the current layout from the environment, falling back to
// sensible $HOME-derived defaults (ADR 0007) when the XDG layer cannot
// provide a base. Resolve never fails: guppy --help must work anywhere.
func Resolve() Layout {
	home, _ := os.UserHomeDir()
	return Layout{
		ConfigHome: firstNonEmpty(xdg.ConfigHome, homeJoin(home, ".config")),
		DataHome:   firstNonEmpty(xdg.DataHome, homeJoin(home, ".local", "share")),
		CacheHome:  firstNonEmpty(xdg.CacheHome, homeJoin(home, ".cache")),
		BinDir:     homeJoin(home, ".local", "bin"),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func homeJoin(home string, elem ...string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(append([]string{home}, elem...)...)
}

func (l Layout) appDir(base string) string { return filepath.Join(base, "guppy") }

// SettingsFile is the user settings file (§10.3): <config>/guppy/guppy.toml.
func (l Layout) SettingsFile() string { return filepath.Join(l.appDir(l.ConfigHome), "guppy.toml") }

// DataDir is the guppy data root: <data>/guppy/ (§10.2 table).
func (l Layout) DataDir() string { return l.appDir(l.DataHome) }

// TemplatesDir is the user-scope template directory (§4.1).
func (l Layout) TemplatesDir() string { return filepath.Join(l.DataDir(), "templates") }

// CommunityDir is the community-scope template root (§4.1): <id>@<version> subdirs.
func (l Layout) CommunityDir() string { return filepath.Join(l.DataDir(), "community") }

// IndexFile is the global project index (§9): <data>/guppy/index.toml.
func (l Layout) IndexFile() string { return filepath.Join(l.DataDir(), "index.toml") }

// CatalogDir holds shellhook state (§10.2).
func (l Layout) CatalogDir() string { return filepath.Join(l.DataDir(), "catalog") }

// SeenLog is the shellhook's append-only observation log (§9): the hook
// appends project paths here; every guppy index reader folds it into
// index.toml. Named so `catalog/` never collides with the log itself.
func (l Layout) SeenLog() string { return filepath.Join(l.CatalogDir(), "seen.log") }

// CacheDir is the guppy cache root: <cache>/guppy/ (§10.2 table).
func (l Layout) CacheDir() string { return l.appDir(l.CacheHome) }

// BinaryName is the installed binary's name for the current OS.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "guppy.exe"
	}
	return "guppy"
}

// BinaryPath is the default install location of the binary.
func (l Layout) BinaryPath() string { return filepath.Join(l.BinDir, BinaryName()) }
