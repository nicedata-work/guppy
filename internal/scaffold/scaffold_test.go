package scaffold

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/nicedata-work/guppy/internal/render"
)

func fsOf(files ...render.File) render.FileSet {
	return render.NewFileSet(files)
}

// failWrites returns an options value whose write seam fails on paths
// matching match — the injected-failure pattern for rollback tests
// (styleguide T11/G3: per-call injection, no package-global mutation).
func failWrites(match string, inner error) ApplyOptions {
	return ApplyOptions{WriteFile: func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, match) {
			return inner
		}
		return os.WriteFile(path, data, perm)
	}}
}

func TestApplyCreatesIdempotently(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := fsOf(
		render.File{Path: "README.md", Mode: 0o644, Content: []byte("readme\n")},
		render.File{Path: "src/main.go", Mode: 0o644, Content: []byte("package main\n")},
	)
	res, err := Apply(context.Background(), files, dir, ApplyOptions{})
	assert.NilError(t, err)
	assert.DeepEqual(t, res.Created, []string{"README.md", "src/main.go"})
	assert.Equal(t, res.Summary(), "2 created, 0 unchanged, 0 kept")

	res, err = Apply(context.Background(), files, dir, ApplyOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(res.Created), 0)
	assert.DeepEqual(t, res.Unchanged, []string{"README.md", "src/main.go"})
}

func TestApplyNeverClobbers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := fsOf(render.File{Path: "main.go", Mode: 0o644, Content: []byte("original\n")})
	_, err := Apply(context.Background(), first, dir, ApplyOptions{})
	assert.NilError(t, err)

	assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("user edit\n"), 0o644))
	res, err := Apply(context.Background(), fsOf(render.File{Path: "main.go", Mode: 0o644, Content: []byte("rendered\n")}), dir, ApplyOptions{})
	assert.NilError(t, err)
	assert.DeepEqual(t, res.Kept, []string{"main.go"})
	got, err := os.ReadFile(filepath.Join(dir, "main.go"))
	assert.NilError(t, err)
	assert.Equal(t, string(got), "user edit\n")
}

func TestApplyRollsBackOnFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files := fsOf(
		render.File{Path: "a.txt", Mode: 0o644, Content: []byte("a")},
		render.File{Path: "deep/nested/boom.txt", Mode: 0o644, Content: []byte("boom")},
	)
	_, err := Apply(context.Background(), files, dir, failWrites("boom.txt", errors.New("injected write failure")))
	assert.ErrorContains(t, err, "injected write failure")

	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt must be rolled back, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deep")); !os.IsNotExist(err) {
		t.Errorf("freshly-created dirs must be rolled back, stat err = %v", err)
	}
}

// TestApplyRollsBackOnCancellation pins (styleguide
// T14/U14): an interrupt mid-scaffold aborts through the rollback path —
// the ctx is cancelled from inside the write seam (as a signal would
// land mid-write) and the already-created files are reverted.
func TestApplyRollsBackOnCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := ApplyOptions{WriteFile: func(path string, data []byte, perm os.FileMode) error {
		if err := os.WriteFile(path, data, perm); err != nil {
			return err
		}
		if strings.HasSuffix(path, "b.txt") {
			cancel()
		}
		return nil
	}}
	_, err := Apply(ctx, fsOf(
		render.File{Path: "a.txt", Mode: 0o644, Content: []byte("a")},
		render.File{Path: "b.txt", Mode: 0o644, Content: []byte("b")},
		render.File{Path: "deep/nested/c.txt", Mode: 0o644, Content: []byte("c")},
	), dir, opts)
	assert.ErrorIs(t, err, context.Canceled)

	for _, p := range []string{"a.txt", "b.txt", "deep"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("%s must be rolled back after cancellation, stat err = %v", p, err)
		}
	}
}

func TestApplyRollsBackFreshProjectDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "newproj")

	_, err := Apply(context.Background(), fsOf(
		render.File{Path: "a.txt", Mode: 0o644, Content: []byte("a")},
		render.File{Path: "b.txt", Mode: 0o644, Content: []byte("b")},
	), dir, failWrites("b.txt", errors.New("injected write failure")))
	assert.ErrorContains(t, err, "injected write failure")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("fresh project dir must be removed on failure, stat err = %v", err)
	}
}

func TestApplyRejectsEscapingPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, p := range []string{"../evil", "/abs/evil", "..", ""} {
		_, err := Apply(context.Background(), fsOf(render.File{Path: p, Mode: 0o644, Content: []byte("x")}), dir, ApplyOptions{})
		assert.ErrorContains(t, err, strings.TrimSpace(p+""))
		if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
			t.Fatalf("path %q escaped the sandbox", p)
		}
	}
}

func TestApplyPreservesExecutableMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := Apply(context.Background(), fsOf(render.File{Path: "bin/tool", Mode: 0o755, Content: []byte("#!/bin/sh\n")}), dir, ApplyOptions{})
	assert.NilError(t, err)
	st, err := os.Stat(filepath.Join(dir, "bin", "tool"))
	assert.NilError(t, err)
	assert.Equal(t, st.Mode().Perm(), os.FileMode(0o755))
}

// TestApplyRefusesSymlinkedParent pins the security pass: a
// pre-existing symlinked directory inside the project must never be
// written through — the write would land outside the project tree.
func TestApplyRefusesSymlinkedParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Apply(context.Background(), fsOf(render.File{Path: "escape/evil.txt", Mode: 0o644, Content: []byte("x")}), dir, ApplyOptions{})
	assert.ErrorContains(t, err, "symlink")

	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Fatal("write escaped the project directory through a symlinked parent")
	}
}

// TestApplyRefusesSymlinkedDest: a symlinked destination *file* must be
// refused too — os.WriteFile would follow it and clobber its target.
func TestApplyRefusesSymlinkedDest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(target, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "config.toml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Apply(context.Background(), fsOf(render.File{Path: "config.toml", Mode: 0o644, Content: []byte("scaffolded")}), dir, ApplyOptions{})
	assert.ErrorContains(t, err, "symlink")

	got, err := os.ReadFile(target)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "user data")
}

// TestRepairRefusesSymlinkedPath: doctor's safe-subset restore obeys the
// same rule — a missing file reappearing as a symlink is not restored.
func TestRepairRefusesSymlinkedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "tools")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	l := Local{Files: fsOf(render.File{Path: "tools/env.sh", Mode: 0o644, Content: []byte("#!/bin/sh\n")})}

	err := l.Repair(dir, Diffs{{Kind: DiffMissing, Path: filepath.Join("tools", "env.sh")}})
	assert.ErrorContains(t, err, "symlink")

	if _, err := os.Stat(filepath.Join(outside, "env.sh")); err == nil {
		t.Fatal("repair escaped the project directory through a symlink")
	}
}
