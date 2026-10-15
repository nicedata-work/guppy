// Package fileutil holds the shared file-writing helpers (styleguide
// T16: one atomic-write implementation — CreateTemp in the target dir +
// rename — never per-package temp+rename copies, never fixed `.tmp`
// names, which collide under concurrent guppy invocations).
//
// Note: `selfmanage`'s binary swap is deliberately NOT this helper — it
// is a backup/restore transaction around the rename (ADR 0006), not a
// plain atomic write.
package fileutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteAtomic writes content to path atomically: a uniquely-named
// temp file in the target directory (os.CreateTemp — collision-free
// under concurrent invocations) is written, fsynced, chmod'ed to perm,
// then renamed over path. A crash mid-write leaves the previous path
// content intact; a concurrent writer wins or loses wholesale, never
// corrupts.
func WriteAtomic(path string, perm fs.FileMode, content []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Chmod(perm.Perm()); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
