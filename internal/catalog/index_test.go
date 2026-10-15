package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// writeMarker creates a managed project dir with a minimal schema-2 marker.
func writeMarker(t *testing.T, root, id string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	assert.NilError(t, os.MkdirAll(filepath.Join(dir, ".guppy"), 0o755))
	content := "schema = 2\n\n[project]\ntemplate = \"" + id + "\"\nversion = \"0.1.0\"\n"
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".guppy", "project.toml"), []byte(content), 0o644))
	return dir
}

func fixedNow() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }

func TestUpsertAndLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := writeMarker(t, root, "alpha")
	idx := &FileIndex{Path: filepath.Join(root, "index.toml"), Now: fixedNow}

	e, err := Record(dir, fixedNow())
	assert.NilError(t, err)
	assert.Equal(t, e.TemplateID, "alpha")
	assert.Equal(t, e.MarkerModTime.IsZero(), false)
	assert.NilError(t, idx.Upsert(e))

	entries, err := idx.Load()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)
	assert.Equal(t, entries[0].Path, dir)
	assert.Equal(t, entries[0].LastSeen, fixedNow())

	e.LastSeen = fixedNow().Add(time.Hour)
	assert.NilError(t, idx.Upsert(e))
	entries, err = idx.Load()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)
}

func TestPruneOnReadRemovesDeletedProjects(t *testing.T) {
	root := t.TempDir()
	keep := writeMarker(t, root, "keep")
	drop := writeMarker(t, root, "drop")
	idx := &FileIndex{Path: filepath.Join(root, "index.toml"), Now: fixedNow}
	for _, d := range []string{keep, drop} {
		e, err := Record(d, fixedNow())
		assert.NilError(t, err)
		assert.NilError(t, idx.Upsert(e))
	}

	assert.NilError(t, os.RemoveAll(drop))
	assert.NilError(t, os.Remove(filepath.Join(keep, ".guppy", "project.toml")))

	stale := idx.Stale()
	assert.Equal(t, len(stale), 2)

	entries, err := idx.Entries()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0)

	entries, err = idx.Load()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0)
}

func TestPruneKeepsLiveEntries(t *testing.T) {
	root := t.TempDir()
	dir := writeMarker(t, root, "live")
	idx := &FileIndex{Path: filepath.Join(root, "index.toml"), Now: fixedNow}
	e, err := Record(dir, fixedNow())
	assert.NilError(t, err)
	assert.NilError(t, idx.Upsert(e))

	removed, err := idx.Prune()
	assert.NilError(t, err)
	assert.Equal(t, removed, 0)
	entries, err := idx.Entries()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)
}

func TestFoldSeenRecordsHookObservations(t *testing.T) {
	root := t.TempDir()
	dir := writeMarker(t, root, "hooked")
	seen := filepath.Join(root, "catalog", "seen.log")
	assert.NilError(t, os.MkdirAll(filepath.Dir(seen), 0o755))

	assert.NilError(t, os.WriteFile(seen, []byte(dir+"\n"+dir+"\n"+filepath.Join(root, "deleted")+"\n"), 0o644))

	idx := &FileIndex{Path: filepath.Join(root, "index.toml"), Now: fixedNow}
	n, err := idx.FoldSeen(seen)
	assert.NilError(t, err)
	assert.Equal(t, n, 1)

	entries, err := idx.Entries()
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1)
	assert.Equal(t, entries[0].TemplateID, "hooked")

	data, err := os.ReadFile(seen)
	assert.NilError(t, err)
	assert.Equal(t, string(data), "")
	n, err = idx.FoldSeen(seen)
	assert.NilError(t, err)
	assert.Equal(t, n, 0)
}

func TestLoadRejectsUnknownSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.toml")
	assert.NilError(t, os.WriteFile(path, []byte("schema = 99\n"), 0o644))
	_, err := (&FileIndex{Path: path}).Load()
	assert.ErrorContains(t, err, "schema")
}

func TestSaveIsSortedAndDeterministic(t *testing.T) {
	root := t.TempDir()
	idx := &FileIndex{Path: filepath.Join(root, "index.toml"), Now: fixedNow}
	for _, id := range []string{"zeta", "alpha", "mid"} {
		dir := writeMarker(t, root, id)
		e, err := Record(dir, fixedNow())
		assert.NilError(t, err)
		assert.NilError(t, idx.Upsert(e))
	}
	first, err := os.ReadFile(idx.Path)
	assert.NilError(t, err)
	assert.NilError(t, idx.Save(mustLoad(t, idx)))
	second, err := os.ReadFile(idx.Path)
	assert.NilError(t, err)
	assert.Equal(t, string(first), string(second))

	if strings.Index(string(first), "alpha") > strings.Index(string(first), "mid") {
		t.Error("index entries not written sorted by path")
	}
}

func mustLoad(t *testing.T, idx *FileIndex) []Entry {
	t.Helper()
	entries, err := idx.Load()
	assert.NilError(t, err)
	return entries
}
