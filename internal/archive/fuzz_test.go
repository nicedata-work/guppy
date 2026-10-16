package archive

import (
	"path"
	"strings"
	"testing"
)

// FuzzReadTarGz targets the tar reader (fuzz-tested). Invariants: never panics; every accepted archive yields
// only clean, relative, non-escaping paths — the exact property the
// extraction guard relies on.
func FuzzReadTarGz(f *testing.F) {
	f.Add(makeTarGz(f, tarEntry{name: "guppy.toml", body: "id = \"x\"\n"}))
	f.Add(makeTarGz(f, tarEntry{name: "wrap/template/a.txt", body: "a"}))
	f.Add([]byte("not a tarball at all"))
	f.Add([]byte{})
	f.Add(makeTarGz(f, tarEntry{name: "../escape.txt", body: "x"}))
	f.Add(makeTarGz(f, tarEntry{name: "/abs.txt", body: "x"}))
	// Regression: backslashes are POSIX filename bytes but Windows path
	// separators — `..\\evil` slips past every "/"-semantics check yet
	// means traversal on a Windows consumer (found by fuzzing).
	f.Add(makeTarGz(f, tarEntry{name: `..\evil.txt`, body: "x"}))
	f.Add(makeTarGz(f, tarEntry{name: `wrat\\\]plate/a.txt`, body: "x"}))
	f.Fuzz(func(t *testing.T, data []byte) {
		files, err := ReadTarGz(data)
		if err != nil {
			return
		}
		if len(files) == 0 {
			t.Error("accepted archive has no files")
		}
		for _, file := range files {
			clean := path.Clean(file.Rel)
			if clean != file.Rel || clean == "." || clean == ".." ||
				path.IsAbs(file.Rel) || strings.HasPrefix(clean, "../") {
				t.Errorf("accepted unsafe path %q", file.Rel)
			}
			if strings.Contains(file.Rel, "\\") {
				t.Errorf("accepted suspicious path %q", file.Rel)
			}
		}
	})
}
