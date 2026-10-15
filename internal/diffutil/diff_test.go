package diffutil

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestTreesIdentical(t *testing.T) {
	a := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("hello\n")}}
	b := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("hello\n")}}
	changes, err := Trees(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("identical trees: got %v, want none", changes)
	}
}

func TestTreesChangeSet(t *testing.T) {
	old := fstest.MapFS{
		"same.txt":    &fstest.MapFile{Data: []byte("x\n")},
		"changed.txt": &fstest.MapFile{Data: []byte("before\n")},
		"gone.txt":    &fstest.MapFile{Data: []byte("bye\n")},
	}
	new := fstest.MapFS{
		"same.txt":    &fstest.MapFile{Data: []byte("x\n")},
		"changed.txt": &fstest.MapFile{Data: []byte("after\n")},
		"added.txt":   &fstest.MapFile{Data: []byte("hi\n")},
	}
	changes, err := Trees(old, new)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ChangeStatus{"added.txt": StatusAdded, "changed.txt": StatusModified, "gone.txt": StatusRemoved}
	if len(changes) != len(want) {
		t.Fatalf("got %v, want %v", changes, want)
	}
	for _, c := range changes {
		if want[c.Path] != c.Status {
			t.Errorf("%s: got %s, want %s", c.Path, c.Status, want[c.Path])
		}
	}
}

func TestUnifiedDiffShape(t *testing.T) {
	lines := Unified("f.txt", []byte("a\nb\nc\nd\ne\n"), []byte("a\nB\nc\nd\ne\n"))
	if len(lines) == 0 {
		t.Fatal("expected diff output")
	}
	if lines[0] != "--- a/f.txt" || lines[1] != "+++ b/f.txt" {
		t.Errorf("headers = %q %q", lines[0], lines[1])
	}
	joined := join(lines)
	if !contains(lines, "-b") || !contains(lines, "+B") {
		t.Errorf("diff missing change lines:\n%s", joined)
	}
}

func TestUnifiedIdenticalIsEmpty(t *testing.T) {
	if got := Unified("f.txt", []byte("same\n"), []byte("same\n")); got != nil {
		t.Errorf("identical: got %v, want nil", got)
	}
}

func TestUnifiedTruncatesLongDiffs(t *testing.T) {
	var old, new []byte
	for range MaxLines + 50 {
		old = append(old, []byte("line\n")...)
		new = append(new, []byte("LINE\n")...)
	}
	lines := Unified("big.txt", old, new)
	if len(lines) > MaxLines+3 {
		t.Errorf("diff not truncated: %d lines", len(lines))
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "truncated") {
		t.Errorf("truncation marker missing: %q", last)
	}
}

func TestChangeSummary(t *testing.T) {
	got := ChangeSummary([]Change{{Path: "a", Status: "added"}})
	if len(got) != 1 || got[0] != "a (added)" {
		t.Errorf("ChangeSummary = %v", got)
	}
}

func join(lines []string) string {
	s := ""
	for _, l := range lines {
		s += l + "\n"
	}
	return s
}

func contains(lines []string, want string) bool {
	for _, l := range lines {
		if l[1:] == want || l == want {
			return true
		}
	}
	return false
}
