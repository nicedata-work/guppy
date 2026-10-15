// Package catalog is the global project index (design §9): entries keyed by
// project path, maintained by the shellhook via marker presence and pruned
// on read. No daemon, no fs watcher, no manual GC.
//
// The marker file on disk is the truth; the index is only a cache of where
// markers have been seen. The shellhook never invokes guppy (stub hot
// path): it appends the current directory to a seen-log when it differs
// from the last one, and every index reader folds that log in
// (FoldSeen), reading each marker for template id and mtime off the hot
// path, then prunes entries whose project or marker has disappeared.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/nicedata-work/guppy/internal/fileutil"
	"github.com/nicedata-work/guppy/internal/project"
)

// SchemaVersion is the index.toml schema version.
const SchemaVersion = 1

// Entry is one managed project as recorded in
// ~/.local/share/guppy/index.toml. The marker file on disk is the truth;
// the index is only a cache of where markers have been seen.
type Entry struct {
	Path string `toml:"path"`

	TemplateID string `toml:"template_id"`

	MarkerModTime time.Time `toml:"marker_mtime"`

	LastSeen time.Time `toml:"last_seen"`
}

// FileIndex persists the index at Path (index.toml) with atomic rewrites
// and prune-on-read semantics (prune-on-read: the index self-heals
// whenever a project moves or is deleted). Now is the clock seam for
// tests; nil means time.Now.
type FileIndex struct {
	Path string
	Now  func() time.Time
}

func (f *FileIndex) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now().UTC()
}

type indexFile struct {
	Schema   int     `toml:"schema"`
	Projects []Entry `toml:"project"`
}

// Load reads the index; a missing file is an empty (not failed) index.
func (f *FileIndex) Load() ([]Entry, error) {
	data, err := os.ReadFile(f.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Path, err)
	}
	var doc indexFile
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	if doc.Schema != SchemaVersion {
		return nil, fmt.Errorf("%s: unsupported schema %d (want %d)", f.Path, doc.Schema, SchemaVersion)
	}
	return doc.Projects, nil
}

// Save rewrites the index atomically, entries sorted by path (the marker's
// determinism discipline applied to the index — stable diffs, no flapping).
func (f *FileIndex) Save(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	sorted := append([]Entry(nil), entries...)
	slices.SortFunc(sorted, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })

	var b strings.Builder
	b.WriteString("# guppy project index — maintained by the shellhook (guppy init-shell)\n")
	b.WriteString("# and pruned on read by `guppy list`/`guppy doctor`. The marker file is\n")
	b.WriteString("# the truth; this index is only a cache of where markers were seen.\n")
	if err := toml.NewEncoder(&b).Encode(indexFile{Schema: SchemaVersion, Projects: sorted}); err != nil {
		return err
	}
	return fileutil.WriteAtomic(f.Path, 0o644, []byte(b.String()))
}

// Entries returns the current entries with prune-on-read applied: stale
// entries are removed from disk before the list is returned. Load errors
// propagate — a corrupt index is surfaced, never silently rebuilt (the
// marker files can always re-seed it via FoldSeen).
func (f *FileIndex) Entries() ([]Entry, error) {
	if _, err := f.Prune(); err != nil {
		return nil, err
	}
	return f.Load()
}

// Upsert inserts or refreshes the entry for e.Path,
// stamping LastSeen with the index clock.
func (f *FileIndex) Upsert(e Entry) error {
	entries, err := f.Load()
	if err != nil {
		return err
	}
	e.LastSeen = f.now()
	for i, x := range entries {
		if x.Path == e.Path {
			entries[i] = e
			return f.Save(entries)
		}
	}
	return f.Save(append(entries, e))
}

// markerPath is the marker location inside a project entry's path.
func markerPath(dir string) string {
	return filepath.Join(dir, filepath.FromSlash(project.MarkerRelPath))
}

// Stale returns entries whose project directory or marker no longer
// exists (deleted projects, moved trees — the self-healing rule §9).
func (f *FileIndex) Stale() []Entry {
	entries, err := f.Load()
	if err != nil {
		return nil
	}
	var out []Entry
	for _, e := range entries {
		if st, err := os.Stat(markerPath(e.Path)); err != nil || st.IsDir() {
			out = append(out, e)
		}
	}
	return out
}

// Prune removes stale entries; removed is the count.
func (f *FileIndex) Prune() (int, error) {
	entries, err := f.Load()
	if err != nil {
		return 0, err
	}
	kept := entries[:0:0]
	removed := 0
	for _, e := range entries {
		if st, err := os.Stat(markerPath(e.Path)); err == nil && !st.IsDir() {
			kept = append(kept, e)
		} else {
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, f.Save(kept)
}

// Record reads the marker at dir and returns the upsert entry for it.
// Used by guppy commands that observe a project (scaffold, doctor, list)
// — the off-hot-path half of the shellhook contract.
func Record(dir string, now time.Time) (Entry, error) {
	data, err := os.ReadFile(markerPath(dir))
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", dir, err)
	}
	m, err := project.Decode(data)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", markerPath(dir), err)
	}
	st, err := os.Stat(markerPath(dir))
	if err != nil {
		return Entry{}, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Path:          abs,
		TemplateID:    m.Project.Template,
		MarkerModTime: st.ModTime().UTC(),
		LastSeen:      now.UTC(),
	}, nil
}

// FoldSeen folds the shellhook's seen-log into the index and clears it:
// each logged path with a live marker is upserted (template id and mtime
// read here, never on the hook's hot path), then the log is truncated.
// Paths whose marker is gone are skipped — Prune handles anything the
// log knew about. Returns the number of entries recorded.
func (f *FileIndex) FoldSeen(seenLog string) (int, error) {
	data, err := os.ReadFile(seenLog)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", seenLog, err)
	}

	var paths []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}

	entries, err := f.Load()
	if err != nil {
		return 0, err
	}
	byPath := map[string]int{}
	for i, e := range entries {
		byPath[e.Path] = i
	}
	recorded := 0
	now := f.now()
	for _, p := range paths {
		e, err := Record(p, now)
		if err != nil {
			continue
		}
		if i, ok := byPath[e.Path]; ok {
			entries[i] = e
		} else {
			byPath[e.Path] = len(entries)
			entries = append(entries, e)
		}
		recorded++
	}
	if recorded > 0 {
		if err := f.Save(entries); err != nil {
			return recorded, err
		}
	}

	if err := clearFile(seenLog); err != nil {
		return recorded, err
	}
	return recorded, nil
}

// clearFile truncates path in place; a missing file is fine.
func clearFile(path string) error {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return fh.Close()
}
