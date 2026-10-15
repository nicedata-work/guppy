package selfmanage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/catalog"
	"github.com/nicedata-work/guppy/internal/paths"
)

// Marker lines delimit the block the installer adds to shell rc files
// (install/install.sh and install/install.ps1 write exactly these). The
// uninstaller only ever removes whole marked blocks — never guesses.
const (
	MarkerOpen  = "# >>> guppy initialize >>>"
	MarkerClose = "# <<< guppy initialize <<<"
)

// UninstallOptions parameterizes Uninstall.
type UninstallOptions struct {
	BinaryPath string

	Layout paths.Layout

	HomeDir string

	GOOS string

	Purge bool
}

// UninstallResult reports what was removed (and what could not be).
type UninstallResult struct {
	BinaryPath  string
	CleanedRcs  []string
	PurgedDirs  []string
	Warnings    []string
	KeptDataMsg string
}

// Uninstall removes the guppy binary and the marker blocks the installer
// added to shell rc files. With Purge it also removes the data and cache
// dirs. It never touches anything outside those locations.
func Uninstall(_ context.Context, opts UninstallOptions) (UninstallResult, error) {
	var res UninstallResult

	binary := opts.BinaryPath
	if binary == "" {
		var err error
		binary, err = os.Executable()
		if err != nil {
			return res, fmt.Errorf("locate current executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(binary); err == nil {
			binary = resolved
		}
	}

	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return res, fmt.Errorf("locate home dir: %w", err)
		}
	}

	for _, rc := range rcFiles(home, opts.GOOS) {
		removed, err := removeMarkerBlock(rc)
		if err != nil {
			res.Warnings = append(res.Warnings, err.Error())
			continue
		}
		if removed {
			res.CleanedRcs = append(res.CleanedRcs, rc)
		}
	}

	for _, rc := range rcFiles(home, opts.GOOS) {
		removed, err := removeMarkedRange(rc, catalog.HookMarkerOpen, catalog.HookMarkerClose)
		if err != nil {
			res.Warnings = append(res.Warnings, err.Error())
			continue
		}
		if removed && !slices.Contains(res.CleanedRcs, rc) {
			res.CleanedRcs = append(res.CleanedRcs, rc)
		}
	}

	if err := os.Remove(binary); err != nil && !os.IsNotExist(err) {
		return res, fmt.Errorf("remove %s: %w", binary, err)
	}
	res.BinaryPath = binary

	if opts.Purge {
		for _, dir := range []string{opts.Layout.DataDir(), opts.Layout.CacheDir()} {
			if dir == "" {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				return res, fmt.Errorf("purge %s: %w", dir, err)
			}
			res.PurgedDirs = append(res.PurgedDirs, dir)
		}
	} else if d := opts.Layout.DataDir(); d != "" {
		if _, err := os.Stat(d); err == nil {
			res.KeptDataMsg = d
		}
	}
	return res, nil
}

// rcFiles lists the shell rc files the installer may have touched, in
// probe order, for the current OS.
func rcFiles(home, goos string) []string {
	if goos == "" || goos == "linux" || goos == "darwin" {
		return []string{
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".profile"),
			filepath.Join(home, ".config", "fish", "config.fish"),
		}
	}
	return nil
}

// removeMarkerBlock deletes the installer's marked block (open marker
// through close marker, inclusive) from the file. Returns whether
// anything was removed. An unterminated open marker is a warning, not a
// destructive guess.
func removeMarkerBlock(path string) (bool, error) {
	return removeMarkedRange(path, MarkerOpen, MarkerClose)
}

// removeMarkedRange deletes the first open..close block (inclusive).
func removeMarkedRange(path, open, close string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	start, end := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n")
		switch trimmed {
		case open:
			if start == -1 {
				start = i
			}
		case close:
			if start != -1 && end == -1 {
				end = i
			}
		}
	}
	if start == -1 {
		return false, nil
	}
	if end == -1 {
		return false, fmt.Errorf("%s: unterminated %s block (open at line %d); left untouched", path, open, start+1)
	}

	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	for i, line := range lines {
		if i >= start && i <= end {
			continue
		}
		w.WriteString(line)
	}
	if err := w.Flush(); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("rewrite %s: %w", path, err)
	}
	return true, nil
}
