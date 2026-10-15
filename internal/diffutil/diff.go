// Package diffutil renders unified diffs of template trees (design §7's
// trust gate: "prints the manifest, the file list, and the diff for a
// previously-installed version"). Pure code — line-oriented LCS diff,
// no dependencies. Output is capped so a hostile 10 MB payload cannot
// flood a review screen.
package diffutil

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// MaxLines caps the per-file hunk output; beyond it the diff is
// truncated with a marker.
const MaxLines = 200

// ChangeStatus classifies how a path differs between two trees (styleguide
// T1: a named type over the closed added/removed/modified vocabulary,
// not a bare string).
type ChangeStatus string

// The change vocabulary of a tree diff.
const (
	StatusAdded    ChangeStatus = "added"
	StatusRemoved  ChangeStatus = "removed"
	StatusModified ChangeStatus = "modified"
)

// Change is one path that differs between two trees.
type Change struct {
	Path   string
	Status ChangeStatus
}

// Trees walks both fs.FS values and returns the sorted list of paths
// that were added, removed, or modified (byte comparison). Identical
// trees yield no changes.
func Trees(old, new fs.FS) ([]Change, error) {
	oldFiles, err := allFiles(old)
	if err != nil {
		return nil, err
	}
	newFiles, err := allFiles(new)
	if err != nil {
		return nil, err
	}
	var out []Change
	for path := range oldFiles {
		if _, ok := newFiles[path]; !ok {
			out = append(out, Change{Path: path, Status: StatusRemoved})
		}
	}
	for path, body := range newFiles {
		if oldBody, ok := oldFiles[path]; !ok {
			out = append(out, Change{Path: path, Status: StatusAdded})
		} else if string(oldBody) != string(body) {
			out = append(out, Change{Path: path, Status: StatusModified})
		}
	}
	slices.SortFunc(out, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// Unified renders the unified diff of one file's old and new contents
// (context: 3 lines). Identical contents yield no output.
func Unified(path string, oldContent, newContent []byte) []string {
	if string(oldContent) == string(newContent) {
		return nil
	}
	oldLines := splitLines(string(oldContent))
	newLines := splitLines(string(newContent))
	ops := diffLines(oldLines, newLines)

	var out []string

	const context = 3
	var hunks []string
	for i := 0; i < len(ops); {
		if ops[i].kind == opEqual {
			i++
			continue
		}

		start := i
		back := 0
		for start > 0 && ops[start-1].kind == opEqual && back < context {
			start--
			back++
		}

		end := i
		forward := 0
		for end < len(ops) {
			if ops[end].kind != opEqual {
				forward = 0
				end++
				continue
			}
			if forward >= context {
				break
			}
			forward++
			end++
		}

		o0, n0 := indices(ops, start)
		o1, n1 := indices(ops, end)
		hunks = append(hunks, fmt.Sprintf("@@ -%d,%d +%d,%d @@", o0+1, o1-o0, n0+1, n1-n0))
		for _, op := range ops[start:end] {
			switch op.kind {
			case opEqual:
				hunks = append(hunks, " "+op.line)
			case opDelete:
				hunks = append(hunks, "-"+op.line)
			case opInsert:
				hunks = append(hunks, "+"+op.line)
			}
		}
		i = end
	}
	if len(hunks) == 0 {
		return nil
	}
	out = append(out, fmt.Sprintf("--- a/%s", path), fmt.Sprintf("+++ b/%s", path))
	out = append(out, hunks...)
	if len(out) > MaxLines+2 {
		truncated := len(out) - MaxLines - 2
		out = append(out[:MaxLines+2], fmt.Sprintf("… (%d more diff lines truncated)", truncated))
	}
	return out
}

// ChangeSummary renders "path (status)" lines for a change list.
func ChangeSummary(changes []Change) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = fmt.Sprintf("%s (%s)", c.Path, c.Status)
	}
	return out
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type op struct {
	kind opKind
	line string
}

// diffLines is a classic LCS-based line diff (Myers' O(ND) in spirit,
// simple LCS table here — file sizes are review-sized, not repo-sized).
func diffLines(a, b []string) []op {
	n, m := len(a), len(b)

	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{opEqual, a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, op{opDelete, a[i]})
			i++
		default:
			out = append(out, op{opInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, op{opDelete, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, op{opInsert, b[j]})
	}
	return out
}

// indices returns (oldIdx, newIdx) at operation boundary k.
func indices(ops []op, k int) (int, int) {
	o, n := 0, 0
	for _, op := range ops[:k] {
		switch op.kind {
		case opEqual:
			o++
			n++
		case opDelete:
			o++
		case opInsert:
			n++
		}
	}
	return o, n
}

// allFiles reads an fs.FS into path → content (paths in slash form).
func allFiles(fsys fs.FS) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		out[rel] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// splitLines splits on \n, keeping a trailing empty element out (diffs
// compare complete lines; a trailing newline does not add a line).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
