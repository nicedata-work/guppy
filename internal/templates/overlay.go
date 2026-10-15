// Payload merging for extends (design §5): parent payloads are overlaid
// onto the child's, file-level — the child wins whole-file, later extends
// entries win over earlier ones. Implemented as an fs.FS overlay so the
// renderer keeps consuming "just an fs.FS"; no file is ever copied.
package templates

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
)

// Overlay returns an fs.FS whose files come from the last layer that has
// them (layers in merge order: earliest parent first, child last). A layer
// where a name is a file shadows that name — and its whole subtree — in
// all earlier layers (the documented contract, now enforced and pinned by
// test: Open/ReadDir/Stat below all honor ancestor shadowing).
func Overlay(layers []fs.FS) fs.FS {
	return overlay{layers: layers}
}

type overlay struct{ layers []fs.FS }

func (o overlay) Open(name string) (fs.File, error) {
	for _, layer := range slices.Backward(o.layers) {
		if shadowedSubtree(layer, name) {

			break
		}
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fs.ErrNotExist
}

// Stat implements fs.StatFS with the same shadowing contract as Open
// (D8: walkers and stat-based filtering rely on it).
func (o overlay) Stat(name string) (fs.FileInfo, error) {
	for _, layer := range slices.Backward(o.layers) {
		if shadowedSubtree(layer, name) {
			break
		}
		st, err := fs.Stat(layer, name)
		if err == nil {
			return st, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fs.ErrNotExist
}

// ReadDir implements fs.ReadDirFS: the union of all layers' entries, later
// layers deciding both membership and type. Shadowing applies: a layer
// where name is a file hides earlier layers' name-subtrees, and a file at
// an ancestor hides name itself in earlier layers.
func (o overlay) ReadDir(name string) ([]fs.DirEntry, error) {
	var (
		out   []fs.DirEntry
		seen  = map[string]bool{}
		found bool
	)
	for _, layer := range slices.Backward(o.layers) {

		st, serr := fs.Stat(layer, name)
		if serr == nil {
			found = true
			if !st.IsDir() {

				if len(out) == 0 {
					return nil, &fs.PathError{Op: "readdirent", Path: name, Err: errors.New("not a directory")}
				}
				break
			}
		} else if !errors.Is(serr, fs.ErrNotExist) {
			return nil, serr
		} else if shadowedSubtree(layer, name) {
			break
		} else {
			continue
		}
		entries, err := fs.ReadDir(layer, name)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !seen[e.Name()] {
				seen[e.Name()] = true
				out = append(out, e)
			}
		}
	}
	if !found {
		return nil, fs.ErrNotExist
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

// shadowedSubtree reports whether any proper ancestor of name is a file
// in fsys — the "a file shadows its whole subtree" rule (D1).
func shadowedSubtree(fsys fs.FS, name string) bool {
	for rest := name; rest != "." && rest != ""; {
		i := strings.LastIndex(rest, "/")
		if i < 0 {
			return false
		}
		rest = rest[:i]
		if st, err := fs.Stat(fsys, rest); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// compile-time checks that overlay satisfies the interfaces the rest of
// the codebase relies on (walkers need ReadDirFS, readers need Open,
// stat-based filtering needs StatFS).
var (
	_ fs.FS        = overlay{}
	_ fs.ReadDirFS = overlay{}
	_ fs.StatFS    = overlay{}
)
