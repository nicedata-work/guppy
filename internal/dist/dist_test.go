package dist

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "installs.toml")}
	records, err := store.Load()
	if err != nil || len(records) != 0 {
		t.Fatalf("empty load = %v, %v; want empty, nil", records, err)
	}

	records = Upsert(records, Record{ID: "acme", Scope: ScopeUser, Kind: KindRegistry, Source: "registry:acme@1.0.0", Version: "1.0.0", Pin: "1.0.0"})
	records = Upsert(records, Record{ID: "acme", Scope: ScopeCommunity, Kind: KindGit, Source: "community:git:https://x/repo.git@main", Version: "2.0.0", Pin: "abc123"})
	if err := store.Save(records); err != nil {
		t.Fatal(err)
	}

	back, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Fatalf("round-trip = %d records, want 2", len(back))
	}
	if back[0].Source != "registry:acme@1.0.0" || back[1].Pin != "abc123" {
		t.Errorf("round-trip lost data: %+v", back)
	}
}

func TestUpsertReplacesByScope(t *testing.T) {
	var records []Record
	records = Upsert(records, Record{ID: "x", Scope: ScopeUser, Version: "1.0.0"})
	records = Upsert(records, Record{ID: "x", Scope: ScopeUser, Version: "1.1.0"})
	if len(records) != 1 || records[0].Version != "1.1.0" {
		t.Errorf("upsert = %+v", records)
	}
}

func TestDelete(t *testing.T) {
	records := []Record{{ID: "x", Scope: ScopeUser}, {ID: "x", Scope: ScopeCommunity}}
	rest, removed := Delete(records, "x", ScopeCommunity)
	if !removed || len(rest) != 1 || rest[0].Scope != ScopeUser {
		t.Errorf("Delete = %v, %v", rest, removed)
	}
	_, removed = Delete(records, "y", ScopeUser)
	if removed {
		t.Error("deleting missing record reported removal")
	}
}

func TestTrustedIsPerVersion(t *testing.T) {
	records := []Record{{ID: "acme", Scope: ScopeCommunity, Version: "1.0.0"}}
	if !Trusted(records, "acme", "1.0.0") {
		t.Error("1.0.0 should be trusted")
	}
	if Trusted(records, "acme", "2.0.0") {
		t.Error("new version must re-enter the review gate")
	}
	if Trusted(records, "other", "1.0.0") {
		t.Error("unknown id must not be trusted")
	}
	user := []Record{{ID: "acme", Scope: ScopeUser, Version: "1.0.0"}}
	if Trusted(user, "acme", "1.0.0") {
		t.Error("user-scope records are not the community trust ledger")
	}
}

func TestInstallTreeAtomicAndRefusesExisting(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "guppy.toml"), []byte("id = \"x\"\n"), 0o644)
	os.MkdirAll(filepath.Join(src, "bin"), 0o755)
	os.WriteFile(filepath.Join(src, "bin", "tool.sh"), []byte("#!/bin/sh\n"), 0o755)

	dest := filepath.Join(t.TempDir(), "installed")
	if err := InstallTree(os.DirFS(src), dest, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := InstallTree(os.DirFS(src), dest, InstallOptions{}); err == nil {
		t.Error("install over existing non-empty dir should fail without AllowExisting")
	}
	if err := InstallTree(os.DirFS(src), dest, InstallOptions{AllowExisting: true}); err != nil {
		t.Errorf("AllowExisting install: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "bin", "tool.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/sh\n" {
		t.Errorf("content = %q", data)
	}
	if _, err := os.Stat(dest + ".staging"); !os.IsNotExist(err) {
		t.Error("staging dir left behind")
	}
}

func TestInstallTreeFailureLeavesNoPartialDest(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "sub", "installed")

	if err := InstallTree(unreadableFS{}, dest, InstallOptions{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("failed install left a partial destination")
	}
}

type unreadableFS struct{}

func (unreadableFS) Open(name string) (fs.File, error) { return nil, fs.ErrNotExist }

func TestCommunityDirNameRoundTrip(t *testing.T) {
	name := CommunityDirName("acme", "1.2.3")
	if name != "acme@1.2.3" {
		t.Fatalf("CommunityDirName = %q", name)
	}
	id, ver := ParseCommunityDirName(name)
	if id != "acme" || ver != "1.2.3" {
		t.Errorf("ParseCommunityDirName = %q,%q", id, ver)
	}
	if id, ver := ParseCommunityDirName("plain"); id != "plain" || ver != "" {
		t.Errorf("ParseCommunityDirName(plain) = %q,%q", id, ver)
	}
}
