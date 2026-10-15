// Golden-style tests for the doctor half of the Environment contract
// (design §8): Audit classifies drift exactly; Repair restores only the
// safe subset. File-touching code gets file-touching tests (AGENTS.md).
package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/nicedata-work/guppy/internal/render"
)

func TestAuditClassifiesDrift(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := fsOf(
		render.File{Path: "keep.md", Mode: 0o644, Content: []byte("keep\n")},
		render.File{Path: "edit.md", Mode: 0o644, Content: []byte("original\n")},
		render.File{Path: "gone.md", Mode: 0o644, Content: []byte("gone\n")},
		render.File{Path: ".guppy/project.toml", Mode: 0o644, Content: []byte("schema = 2\n")},
	)
	_, err := Apply(context.Background(), files, dir, ApplyOptions{})
	assert.NilError(t, err)

	assert.NilError(t, os.WriteFile(filepath.Join(dir, "edit.md"), []byte("user edit\n"), 0o644))
	assert.NilError(t, os.Remove(filepath.Join(dir, "gone.md")))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".guppy", "stray"), []byte("x"), 0o644))

	diffs, err := (Local{Files: files}).Audit(dir)
	assert.NilError(t, err)
	assert.DeepEqual(t, diffs, Diffs{
		{Kind: DiffMissing, Path: "gone.md"},
		{Kind: DiffUnexpected, Path: filepath.Join(".guppy", "stray")},
		{Kind: DiffModified, Path: "edit.md"},
	})
}

func TestAuditCleanTreeHasNoFindings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := fsOf(render.File{Path: "a.txt", Mode: 0o644, Content: []byte("a\n")})
	_, err := Apply(context.Background(), files, dir, ApplyOptions{})
	assert.NilError(t, err)

	assert.NilError(t, os.MkdirAll(filepath.Join(dir, ".guppy"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".guppy", "project.toml"), []byte("different\n"), 0o644))

	diffs, err := (Local{Files: files}).Audit(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(diffs), 0)
}

func TestRepairRestoresOnlyMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := fsOf(
		render.File{Path: "edit.md", Mode: 0o644, Content: []byte("original\n")},
		render.File{Path: "gone.md", Mode: 0o755, Content: []byte("gone\n")},
	)
	_, err := Apply(context.Background(), files, dir, ApplyOptions{})
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "edit.md"), []byte("user edit\n"), 0o644))
	assert.NilError(t, os.Remove(filepath.Join(dir, "gone.md")))

	diffs, err := (Local{Files: files}).Audit(dir)
	assert.NilError(t, err)

	var safe Diffs
	for _, d := range diffs {
		if d.Kind == DiffMissing {
			safe = append(safe, d)
		}
	}
	l := Local{Files: files}
	assert.NilError(t, l.Repair(dir, safe))

	got, err := os.ReadFile(filepath.Join(dir, "gone.md"))
	assert.NilError(t, err)
	assert.Equal(t, string(got), "gone\n")
	st, err := os.Stat(filepath.Join(dir, "gone.md"))
	assert.NilError(t, err)
	assert.Equal(t, st.Mode().Perm(), os.FileMode(0o755).Perm())

	got, err = os.ReadFile(filepath.Join(dir, "edit.md"))
	assert.NilError(t, err)
	assert.Equal(t, string(got), "user edit\n")
}

func TestRepairRefusesNonMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := (Local{}).Repair(dir, Diffs{
		{Kind: DiffModified, Path: "x.md"},
	})
	assert.ErrorContains(t, err, "only missing files")
}
