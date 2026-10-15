// Package scaffold materializes a rendered FileSet onto disk — the
// file-touching half of `guppy new`. Three invariants:
//
//   - idempotent: re-runs create nothing (existing identical files are
//     "unchanged");
//   - never clobbers: existing files with different content are kept and
//     reported, even under --force — --force only relaxes the
//     "directory already exists" gate at the CLI layer;
//   - transactional: the whole FileSet is validated before the first write
//     (scaffolding steps are sequential), and if any write fails or the
//     context is cancelled, every file and directory created so far is
//     reverted — a failed or interrupted `guppy new` never leaves a
//     half-scaffolded tree behind.
//
// Every file-touching rule here is golden-tested (AGENTS.md posture).
package scaffold

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/render"
)

// Result reports what Apply did — the "merge and report" of §2.1.
type Result struct {
	Created []string

	Unchanged []string

	Kept []string
}

// Summary renders a one-line result for CLI output.
func (r Result) Summary() string {
	return fmt.Sprintf("%d created, %d unchanged, %d kept", len(r.Created), len(r.Unchanged), len(r.Kept))
}

// plan is the pre-validated write plan: one entry per file, in FileSet
// (sorted) order.
type plan struct {
	rel       string
	dest      string
	mode      fs.FileMode
	write     bool
	identical bool
	content   []byte
}

// ApplyOptions parameterizes Apply; the zero value is production (styleguide
// T10: nil/empty fields mean "inherit the default").
type ApplyOptions struct {
	WriteFile func(path string, data []byte, perm os.FileMode) error
}

// write resolves the write seam: production default when unset.
func (o ApplyOptions) write() func(string, []byte, os.FileMode) error {
	if o.WriteFile != nil {
		return o.WriteFile
	}
	return os.WriteFile
}

// Apply writes files into dir (created if needed), never overwriting
// different existing content. Deterministic: processes files in FileSet
// order (sorted by path). On any failure — including ctx cancellation
// (styleguide T14: an interrupted scaffold aborts through the rollback
// path, never a crash) — everything created so far is reverted before
// the error is returned.
func Apply(ctx context.Context, files render.FileSet, dir string, opts ApplyOptions) (Result, error) {

	steps, err := planAll(files, dir)
	if err != nil {
		return Result{}, err
	}
	write := opts.write()

	var (
		res         Result
		createdDirs []string
		created     []string
		rollback    = func() {
			for _, c := range slices.Backward(created) {
				_ = os.Remove(c)
			}

			for _, createdDir := range slices.Backward(createdDirs) {
				_ = os.Remove(createdDir)
			}
		}
	)
	for _, s := range steps {
		if err := ctx.Err(); err != nil {
			rollback()
			return Result{}, fmt.Errorf("scaffold interrupted: %w", err)
		}
		if !s.write {
			if s.identical {
				res.Unchanged = append(res.Unchanged, s.rel)
			} else {
				res.Kept = append(res.Kept, s.rel)
			}
			continue
		}
		if err := mkdirAllTrack(filepath.Dir(s.dest), &createdDirs); err != nil {
			rollback()
			return Result{}, fmt.Errorf("create dir for %s: %w", s.rel, err)
		}
		mode := s.mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := write(s.dest, s.content, mode); err != nil {
			rollback()
			return Result{}, fmt.Errorf("write %s: %w", s.rel, err)
		}
		created = append(created, s.dest)
		res.Created = append(res.Created, s.rel)
	}

	if err := ctx.Err(); err != nil {
		rollback()
		return Result{}, fmt.Errorf("scaffold interrupted: %w", err)
	}
	return res, nil
}

// planAll validates every path and classifies every file up front.
func planAll(files render.FileSet, dir string) ([]plan, error) {
	steps := make([]plan, 0, len(files.Files))
	seen := map[string]string{}
	for _, f := range files.Files {
		rel, err := safeRel(f.Path)
		if err != nil {
			return nil, err
		}

		if err := checkNoSymlink(dir, rel); err != nil {
			return nil, err
		}
		if prev, dup := seen[rel]; dup {
			return nil, fmt.Errorf("duplicate output path %q (%s and %s)", rel, prev, f.Path)
		}
		seen[rel] = f.Path
		dest := filepath.Join(dir, rel)
		s := plan{rel: rel, dest: dest, mode: f.Mode}
		existing, err := os.ReadFile(dest)
		switch {
		case err == nil:
			s.identical = string(existing) == string(f.Content)

		case errors.Is(err, fs.ErrNotExist):
			s.write = true
			s.content = f.Content
		default:
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		steps = append(steps, s)
	}
	return steps, nil
}

// mkdirAllTrack creates dir and every missing ancestor (including a
// freshly-targeted project dir), recording only the directories it
// actually created so rollback can remove exactly those — never a
// pre-existing directory.
func mkdirAllTrack(dir string, created *[]string) error {
	var made []string
	for d := dir; ; {
		if st, err := os.Stat(d); err == nil {
			if !st.IsDir() {
				return fmt.Errorf("%s exists and is not a directory", d)
			}
			break
		}
		made = append(made, d)
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	for _, m := range slices.Backward(made) {
		if err := os.Mkdir(m, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		*created = append(*created, m)
	}
	return nil
}

// safeRel rejects output paths that could escape the target directory —
// defense in depth even though renders only produce clean relative paths
// (the security pass audits this surface too).
func safeRel(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty output path")
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output path escapes the project directory: %q", p)
	}
	return clean, nil
}

// checkNoSymlink walks rel under root and refuses if any existing
// component is a symlink (security pass). Components that do not
// exist yet are fine — they will be created as real directories/files.
// Only components *below* root are checked: root itself may legitimately
// be reached through a symlink (/tmp on macOS is one).
func checkNoSymlink(root, rel string) error {
	parts := strings.Split(rel, string(filepath.Separator))
	cur := root
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect %s: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			kind := "directory"
			if i == len(parts)-1 {
				kind = "file"
			}
			return fmt.Errorf("%s %s is a symlink — refusing to write through it", kind, cur)
		}
	}
	return nil
}

// Local implements the audit/repair half of the environment contract
// over a directory on disk (design §8): diff the tree against the
// expected FileSet and restore the safe subset.
type Local struct {
	Files render.FileSet
}

// Audit diffs dir against the expected FileSet (doctor, design §8).
//
//   - DiffMissing: an expected file is absent (safe to restore);
//   - DiffModified: present but different content — may be a user edit,
//     never auto-clobbered;
//   - DiffUnexpected: a file exists inside a contract-managed location
//     (the .guppy/ metadata dir) that the template never produced.
//
// The marker itself (.guppy/project.toml) is excluded: it records which
// guppy wrote the tree and changes with guppy versions, so diffing it
// would be pure noise.
func (l Local) Audit(dir string) (Diffs, error) {
	markerRel := filepath.Clean(filepath.FromSlash(project.MarkerRelPath))
	expected := map[string]bool{}
	for _, f := range l.Files.Files {
		rel, err := safeRel(f.Path)
		if err != nil {
			return nil, err
		}
		expected[rel] = true
	}
	var diffs Diffs
	for _, f := range l.Files.Files {
		rel, err := safeRel(f.Path)
		if err != nil {
			return nil, err
		}
		if rel == markerRel {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, rel))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			diffs = append(diffs, Diff{Kind: DiffMissing, Path: rel})
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", rel, err)
		case !bytes.Equal(data, f.Content):
			diffs = append(diffs, Diff{Kind: DiffModified, Path: rel})
		}
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".guppy"))
	if err == nil {
		for _, e := range entries {
			rel := filepath.Join(".guppy", e.Name())
			if !expected[rel] && rel != markerRel {
				diffs = append(diffs, Diff{Kind: DiffUnexpected, Path: rel})
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	slices.SortFunc(diffs, func(a, b Diff) int {
		if a.Kind != b.Kind {
			return cmp.Compare(a.Kind, b.Kind)
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return diffs, nil
}

// Repair applies the safe subset of diffs: it restores DiffMissing files
// from the expected FileSet. DiffModified (user edits) and
// DiffUnexpected are never touched — restoring a missing file cannot
// destroy user content, which is the whole safety argument of §8.
func (l Local) Repair(dir string, safe Diffs) error {
	byPath := map[string]render.File{}
	for _, f := range l.Files.Files {
		rel, err := safeRel(f.Path)
		if err != nil {
			return err
		}
		byPath[rel] = f
	}
	for _, d := range safe {
		if d.Kind != DiffMissing {
			return fmt.Errorf("refusing to repair %s: only missing files are in the safe subset", d.Path)
		}
		f, ok := byPath[d.Path]
		if !ok {
			return fmt.Errorf("no expected content for %s", d.Path)
		}
		if err := checkNoSymlink(dir, d.Path); err != nil {
			return err
		}
		dest := filepath.Join(dir, d.Path)
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s reappeared; refusing to overwrite", d.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		mode := f.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dest, f.Content, mode); err != nil {
			return fmt.Errorf("restore %s: %w", d.Path, err)
		}
	}
	return nil
}
