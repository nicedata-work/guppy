package fileutil

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteAtomicWritesAndSetsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.toml")
	if err := WriteAtomic(path, 0o644, []byte("hello")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello" {
		t.Fatalf("content = %q, err = %v", got, err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, err = %v", st.Mode().Perm(), err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("dir contents after write: %v (err %v)", entries, err)
	}
}

func TestWriteAtomicReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, 0o644, []byte("new")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content = %q, want new", got)
	}
}

// TestWriteAtomicConcurrent pins the contract: two
// goroutines writing the same path never corrupt the result and never
// collide on a temp name (fixed `.tmp` names fail this).
func TestWriteAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.toml")
	const payloadSize = 4096
	a, b := strings.Repeat("a", payloadSize), strings.Repeat("b", payloadSize)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, content := range []string{a, b} {
		wg.Go(func() {
			errs <- WriteAtomic(path, 0o644, []byte(content))
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent WriteAtomic: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(got); s != a && s != b {
		t.Fatalf("corrupted result: %d bytes, neither whole payload", len(s))
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("dir contents after concurrent writes: %v (err %v)", entries, err)
	}
}
