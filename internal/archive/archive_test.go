package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz builds a gzipped tar from name → content maps ("" content
// with dir=true means a directory entry).
func makeTarGz(t testing.TB, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := byte(tar.TypeReg)
		if e.dir {
			typ = tar.TypeDir
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

type tarEntry struct {
	name string
	body string
	dir  bool
}

func TestReadTarGzRoundTrip(t *testing.T) {
	data := makeTarGz(t,
		tarEntry{name: "top/", dir: true},
		tarEntry{name: "top/guppy.toml", body: "id = \"x\"\n"},
		tarEntry{name: "top/template/main.go", body: "package main\n"},
	)
	files, err := ReadTarGz(data)
	if err != nil {
		t.Fatalf("ReadTarGz: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	root, ok := TopDir(files)
	if !ok || root != "top" {
		t.Errorf("TopDir = %q,%v; want top,true", root, ok)
	}
	dest := t.TempDir()
	if err := Write(files, dest, 1); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "guppy.toml"))
	if err != nil || string(got) != "id = \"x\"\n" {
		t.Errorf("extracted guppy.toml = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "template", "main.go")); err != nil {
		t.Errorf("nested file missing: %v", err)
	}
}

func TestReadTarGzRefusesTraversal(t *testing.T) {
	for _, name := range []string{"../evil", "/abs/evil", "a/../../evil", `..\evil`, `a\..\..\evil`, `wrat\\\]plate/a.txt`} {
		data := makeTarGz(t, tarEntry{name: name, body: "boom"})
		if _, err := ReadTarGz(data); err == nil {
			t.Errorf("entry %q: expected error, got nil", name)
		}
	}
}

func TestReadTarGzRefusesSymlinks(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	if _, err := ReadTarGz(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "link") {
		t.Errorf("symlink archive: got %v, want link refusal", err)
	}
}

func TestReadTarGzRefusesEmpty(t *testing.T) {
	if _, err := ReadTarGz(makeTarGz(t)); err == nil {
		t.Error("empty archive: expected error")
	}
}

func TestWriteNoStripKeepsPaths(t *testing.T) {
	data := makeTarGz(t,
		tarEntry{name: "guppy.toml", body: "id = \"x\"\n"},
		tarEntry{name: "template/a.txt", body: "a"},
	)
	files, err := ReadTarGz(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := TopDir(files); ok {
		t.Error("TopDir should be absent for top-level files")
	}
	dest := t.TempDir()
	if err := Write(files, dest, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "guppy.toml")); err != nil {
		t.Errorf("guppy.toml missing: %v", err)
	}
}

// TestWriteRefusesSymlinkedPath: extraction must never travel through a
// symlinked directory or into a symlinked file in the destination tree
// (security pass; same rule as scaffold.Apply).
func TestWriteRefusesSymlinkedPath(t *testing.T) {
	files := []File{{Rel: "escape/evil.txt", Mode: 0o644, Body: []byte("x")}}

	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := Write(files, dest, 0); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Fatal("extraction escaped dest through a symlinked directory")
	}

	dest2 := t.TempDir()
	target := filepath.Join(t.TempDir(), "precious")
	_ = os.WriteFile(target, []byte("user data"), 0o644)
	_ = os.Symlink(target, filepath.Join(dest2, "f.txt"))
	if err := Write([]File{{Rel: "f.txt", Mode: 0o644, Body: []byte("x")}}, dest2, 0); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "user data" {
		t.Fatalf("symlink target was clobbered: %q", got)
	}
}
