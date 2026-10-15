package templates

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nicedata-work/guppy/internal/manifest"
)

func TestLoadSubPayloadRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"guppy.toml":       &fstest.MapFile{Data: []byte(miniManifest("sub"))},
		"template/a.txt":   &fstest.MapFile{Data: []byte("a")},
		"template/.hidden": &fstest.MapFile{Data: []byte("h")},
		"docs/leak.txt":    &fstest.MapFile{Data: []byte("outside payload")},
	}
	m, issues, err := Load(fsys, LoadOptions{Scope: ScopeBuiltin, Kind: SourceEmbedded})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if manifest.HasErrors(issues) {
		t.Fatalf("issues: %v", issues)
	}
	if m.Manifest.ID != "sub" || m.Identity.Version.String() != "1.0.0" {
		t.Errorf("identity: %+v", m.Identity)
	}
	if _, err := fs.ReadFile(m.Payload, "a.txt"); err != nil {
		t.Errorf("payload must be rooted at template/: %v", err)
	}
	if _, err := fs.ReadFile(m.Payload, ".hidden"); err != nil {
		t.Errorf("dotfiles are ordinary payload files (§3.1): %v", err)
	}
	if _, err := fs.ReadFile(m.Payload, "docs/leak.txt"); err == nil {
		t.Error("files outside template/ must not leak into the payload")
	}
}

func TestLoadThinPayloadHidesAuthoringFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"guppy.toml": &fstest.MapFile{Data: []byte(miniManifest("thin"))},
		"tests.toml": &fstest.MapFile{Data: []byte("")},
		"testdata/x": &fstest.MapFile{Data: []byte("")},
		"main.go":    &fstest.MapFile{Data: []byte("package main")},
	}
	m, _, err := Load(fsys, LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var listed []string
	_ = fs.WalkDir(m.Payload, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			listed = append(listed, p)
		}
		return nil
	})
	if strings.Join(listed, ",") != "main.go" {
		t.Errorf("thin payload listing = %v, want [main.go]", listed)
	}
	if _, err := fs.ReadFile(m.Payload, "main.go"); err != nil {
		t.Errorf("main.go must be readable: %v", err)
	}
	if _, err := fs.ReadFile(m.Payload, "guppy.toml"); err == nil {
		t.Error("guppy.toml must never be part of the payload")
	}

	for _, hiddenName := range []string{"guppy.toml", "tests.toml", "testdata"} {
		if _, err := fs.Stat(m.Payload, hiddenName); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%s) = %v, want ErrNotExist", hiddenName, err)
		}
	}
	if st, err := fs.Stat(m.Payload, "main.go"); err != nil || st.IsDir() {
		t.Errorf("Stat(main.go) = %v, %v; want a regular file", st, err)
	}
}

func TestOverlayLastWins(t *testing.T) {
	base := fstest.MapFS{
		"only-base.txt": &fstest.MapFile{Data: []byte("b")},
		"shared.txt":    &fstest.MapFile{Data: []byte("base")},
		"dir/base.txt":  &fstest.MapFile{Data: []byte("x")},
	}
	child := fstest.MapFS{
		"shared.txt":   &fstest.MapFile{Data: []byte("child")},
		"only-kid.txt": &fstest.MapFile{Data: []byte("k")},
	}
	o := Overlay([]fs.FS{base, child})
	for path, want := range map[string]string{
		"only-base.txt": "b",
		"shared.txt":    "child",
		"only-kid.txt":  "k",
		"dir/base.txt":  "x",
	} {
		data, err := fs.ReadFile(o, path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if string(data) != want {
			t.Errorf("%q = %q, want %q", path, data, want)
		}
	}
	if _, err := fs.ReadFile(o, "nope.txt"); err == nil {
		t.Error("missing file must error")
	}
	entries, err := fs.ReadDir(o, ".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "dir,only-base.txt,only-kid.txt,shared.txt" {
		t.Errorf("merged listing = %v", names)
	}
}

// TestOverlaySubtreeShadowing pins the documented contract (design §5,
// styleguide D1): a layer where a name is a file shadows that name — and
// its whole subtree — in all earlier layers, whether reached by Open or
// by ReadDir.
func TestOverlaySubtreeShadowing(t *testing.T) {
	earlier := fstest.MapFS{
		"x/sub/deep.txt": &fstest.MapFile{Data: []byte("buried")},
	}
	later := fstest.MapFS{
		"x": &fstest.MapFile{Data: []byte("now a file")},
	}
	o := Overlay([]fs.FS{earlier, later})

	data, err := fs.ReadFile(o, "x")
	if err != nil || string(data) != "now a file" {
		t.Fatalf("ReadFile(x) = %q, %v", data, err)
	}

	if _, err := fs.ReadFile(o, "x/sub/deep.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(x/sub/deep.txt) = %v, want ErrNotExist (subtree shadowed)", err)
	}
	if _, err := fs.ReadDir(o, "x/sub"); err == nil {
		t.Error("ReadDir(x/sub) must fail: the subtree is shadowed by the file x")
	}
	if _, err := fs.Stat(o, "x/sub"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(x/sub) = %v, want ErrNotExist (subtree shadowed)", err)
	}

	if _, err := fs.ReadDir(o, "x"); err == nil {
		t.Error("ReadDir(x) on a file must error")
	}
}

// mapResolver is a stub Resolver: raw lookup from a map (chain resolution
// is the caller's job, matching Standard.Resolve).
type mapResolver struct{ byID map[string]Template }

func (r mapResolver) Resolve(_ context.Context, id string) (Template, error) {
	t, ok := r.byID[id]
	if !ok {
		return Template{}, errNotFound{id: id}
	}
	return t, nil
}

func (r mapResolver) List(context.Context, bool) ([]Template, error) { return nil, nil }

func mkTemplate(id string, extends []string, payload fstest.MapFS) Template {
	m := manifest.Manifest{
		ID: id, Version: "1.0.0", Kind: "lib", Language: "none",
		Extends:  extends,
		Vars:     map[string]manifest.VarSpec{id + "_var": {}},
		Defaults: map[manifest.Axis]bool{manifest.Axis(id + "_tog"): true},
	}
	return Template{
		Identity: Identity{ID: id},
		Manifest: m,
		Payload:  payload,
	}
}

func TestResolveChainMergesParentsFirstChildWins(t *testing.T) {
	base := mkTemplate("base", nil, fstest.MapFS{
		"base.txt":   &fstest.MapFile{Data: []byte("from base")},
		"shared.txt": &fstest.MapFile{Data: []byte("base")},
	})
	mid := mkTemplate("mid", []string{"base"}, fstest.MapFS{
		"shared.txt": &fstest.MapFile{Data: []byte("mid")},
	})
	child := mkTemplate("child", []string{"mid"}, fstest.MapFS{
		"shared.txt": &fstest.MapFile{Data: []byte("child")},
		"child.txt":  &fstest.MapFile{Data: []byte("from child")},
	})
	r := mapResolver{byID: map[string]Template{"base": base, "mid": mid, "child": child}}

	got, err := ResolveChain(context.Background(), r, child)
	if err != nil {
		t.Fatalf("ResolveChain: %v", err)
	}

	var ids []string
	for _, id := range got.Chain {
		ids = append(ids, id.ID)
	}
	if strings.Join(ids, ",") != "base,mid,child" {
		t.Errorf("chain = %v", ids)
	}

	if _, ok := got.Manifest.Vars["base_var"]; !ok {
		t.Errorf("parent var not inherited: %v", got.Manifest.Vars)
	}
	if _, ok := got.Manifest.Defaults["mid_tog"]; !ok {
		t.Errorf("parent default not inherited: %v", got.Manifest.Defaults)
	}

	for path, want := range map[string]string{
		"shared.txt": "child",
		"base.txt":   "from base",
		"child.txt":  "from child",
	} {
		data, err := fs.ReadFile(got.Payload, path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if string(data) != want {
			t.Errorf("%q = %q, want %q", path, data, want)
		}
	}
}

func TestResolveChainCycleIsError(t *testing.T) {
	a := mkTemplate("a", []string{"b"}, fstest.MapFS{})
	b := mkTemplate("b", []string{"a"}, fstest.MapFS{})
	r := mapResolver{byID: map[string]Template{"a": a, "b": b}}
	if _, err := ResolveChain(context.Background(), r, a); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: err = %v, want cycle error", err)
	}
}

func TestResolveChainDiamondIsFine(t *testing.T) {
	base := mkTemplate("base", nil, fstest.MapFS{
		"base.txt": &fstest.MapFile{Data: []byte("b")},
	})
	left := mkTemplate("left", []string{"base"}, fstest.MapFS{})
	right := mkTemplate("right", []string{"base"}, fstest.MapFS{})
	child := mkTemplate("child", []string{"left", "right"}, fstest.MapFS{})
	r := mapResolver{byID: map[string]Template{"base": base, "left": left, "right": right, "child": child}}
	got, err := ResolveChain(context.Background(), r, child)
	if err != nil {
		t.Fatalf("diamond must not be a cycle: %v", err)
	}
	if data, err := fs.ReadFile(got.Payload, "base.txt"); err != nil || string(data) != "b" {
		t.Errorf("grandparent payload lost: %q, %v", data, err)
	}
}

func TestResolveChainUnresolvableExtendsIsError(t *testing.T) {
	child := mkTemplate("child", []string{"ghost"}, fstest.MapFS{})
	r := mapResolver{byID: map[string]Template{"child": child}}
	if _, err := ResolveChain(context.Background(), r, child); err == nil || !strings.Contains(err.Error(), `extends "ghost"`) {
		t.Fatalf("unresolvable extends: err = %v", err)
	}
}

func TestStandardResolverBuiltins(t *testing.T) {
	s := NewStandard(Builtin(), nil)
	got, err := s.Resolve(context.Background(), "go-cli")
	if err != nil {
		t.Fatalf("Resolve(go-cli): %v", err)
	}
	if got.Identity.Scope != ScopeBuiltin || got.Identity.Kind != SourceEmbedded {
		t.Errorf("identity: %+v", got.Identity)
	}

	chain, err := ResolveChain(context.Background(), s, got)
	if err != nil {
		t.Fatalf("ResolveChain: %v", err)
	}
	if len(chain.Chain) != 2 || chain.Chain[0].ID != "base-toolchain" {
		t.Errorf("chain = %+v", chain.Chain)
	}
	if _, ok := chain.Manifest.Vars["name"]; !ok {
		t.Errorf("parent [vars] not merged: %v", chain.Manifest.Vars)
	}

	if _, err := s.Resolve(context.Background(), "nope"); err == nil || !strings.Contains(err.Error(), "unknown template") {
		t.Fatalf("unknown id: err = %v", err)
	}
}

func TestStandardResolverUserScopeNeverBeatsBuiltin(t *testing.T) {
	dir := t.TempDir()

	user := filepath.Join(dir, "go-cli")
	if err := os.MkdirAll(filepath.Join(user, "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "guppy.toml"), []byte(miniManifest("go-cli")+"\n[defaults]\ngit = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "template", "squatted.txt"), []byte("imposter"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStandard(Builtin(), []string{dir})
	got, err := s.Resolve(context.Background(), "go-cli")
	if err != nil {
		t.Fatalf("Resolve(go-cli): %v", err)
	}
	if got.Identity.Scope != ScopeBuiltin || got.Identity.Kind != SourceEmbedded {
		t.Errorf("builtin must win the collision, got %+v", got.Identity)
	}
	if _, err := fs.ReadFile(got.Payload, "squatted.txt"); err == nil {
		t.Error("user payload must not leak into the builtin resolution")
	}
}

func miniManifest(id string) string {
	return "id = \"" + id + "\"\nversion = \"1.0.0\"\nkind = \"lib\"\nlanguage = \"none\"\n"
}

// TestLoadInvalidVersionIsRefused pins T9: a version-less-semver
// manifest never becomes a silently-usable 0.0.0 identity — Load surfaces
// the error issue (Validate's, the single source of the rule per D4) and
// the resolver refuses the template outright.
func TestLoadInvalidVersionIsRefused(t *testing.T) {
	fsys := fstest.MapFS{
		"guppy.toml": &fstest.MapFile{Data: []byte(
			"id = \"badver\"\nversion = \"banana\"\nkind = \"lib\"\nlanguage = \"none\"\n")},
		"template/a.txt": &fstest.MapFile{Data: []byte("a")},
	}
	_, issues, err := Load(fsys, LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !manifest.HasErrors(issues) {
		t.Fatalf("invalid version must be an error issue, got %v", issues)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "badver"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "badver", "guppy.toml"),
		[]byte("id = \"badver\"\nversion = \"banana\"\nkind = \"lib\"\nlanguage = \"none\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewStandard(fstest.MapFS{}, []string{dir})
	if _, err := r.Resolve(context.Background(), "badver"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("Resolve(badver) = %v, want refusal naming the invalid version", err)
	}

	list, err := r.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range list {
		if got.Manifest.ID == "badver" {
			t.Fatal("invalid-version template must not be listed")
		}
	}
}
