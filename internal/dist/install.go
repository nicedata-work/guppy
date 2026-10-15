package dist

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallOptions parameterizes InstallTree.
type InstallOptions struct {
	AllowExisting bool
}

// InstallTree copies the template authoring tree in src (fs.FS — embed,
// disk, git checkout, or verified tarball all satisfy it) into dest
// atomically: everything lands in a staging sibling first, then a single
// rename publishes it. A failed install never leaves a partial dest.
func InstallTree(src fs.FS, dest string, opts InstallOptions) error {
	if dest == "" {
		return fmt.Errorf("install: empty destination")
	}
	if !opts.AllowExisting {
		if st, err := os.Stat(dest); err == nil && st.IsDir() {
			entries, err := os.ReadDir(dest)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return fmt.Errorf("destination %s already exists (use update, or remove it first)", dest)
			}
		}
	}

	staging := dest + ".staging"
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := copyFS(src, staging); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("install into %s: %w", dest, err)
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Rename(staging, dest); err != nil {
		return err
	}
	return nil
}

// RemoveTree deletes an installed template dir.
func RemoveTree(dest string) error { return os.RemoveAll(dest) }

// copyFS copies fsys into dir, preserving regular-file permissions
// (normalized to 0644/0755 — templates are text, and umask-independent
// output keeps scaffolds deterministic across machines).
func copyFS(fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if info, err := d.Info(); err == nil && info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, mode)
	})
}

// CommunityDirName renders the community-scope directory name for
// id@version (design §4.1).
func CommunityDirName(id, version string) string {
	if version == "" {
		return id
	}
	return id + "@" + version
}

// ParseCommunityDirName splits "<id>[@<version>]".
func ParseCommunityDirName(name string) (id, version string) {
	if i := strings.LastIndex(name, "@"); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}
