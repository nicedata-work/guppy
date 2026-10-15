package render

import "testing"

func TestNewFileSetEstablishesSortedInvariant(t *testing.T) {
	fs := NewFileSet([]File{
		{Path: "src/main.go"},
		{Path: ".gitignore"},
		{Path: "AGENTS.md"},
		{Path: "README.md"},
	})
	want := []string{".gitignore", "AGENTS.md", "README.md", "src/main.go"}
	for i, f := range fs.Files {
		if f.Path != want[i] {
			t.Fatalf("Files[%d] = %q, want %q", i, f.Path, want[i])
		}
	}
}

// TestAddKeepsInvariant pins the invariant: appending through Add keeps
// the set sorted — consumers never normalize by hand (T5).
func TestAddKeepsInvariant(t *testing.T) {
	var fs FileSet
	fs.Add(File{Path: "b"})
	fs.Add(File{Path: "a"}, File{Path: "c"})
	want := []string{"a", "b", "c"}
	for i, f := range fs.Files {
		if f.Path != want[i] {
			t.Fatalf("Files[%d] = %q, want %q", i, f.Path, want[i])
		}
	}
}
