package templates

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemplate writes a minimal template dir under dir/id (or id@ver).
func writeTemplate(t *testing.T, dir, name, id, version string, extra map[string]string) string {
	t.Helper()
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "id = \"" + id + "\"\n"
	if version != "" {
		body += "version = \"" + version + "\"\n"
	}
	body += "kind = \"cli\"\nlanguage = \"go\"\n"
	if err := os.WriteFile(filepath.Join(root, "guppy.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range extra {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func trustAll(string, string) bool  { return true }
func trustNone(string, string) bool { return false }

// Table-driven scope-precedence tests : collision warnings +
// built-in-wins shadowing.
func TestScopePrecedence(t *testing.T) {

	cases := []struct {
		name        string
		userID      string
		communityID string
		trusted     func(string, string) bool
		wantID      string
		wantScope   Scope
		wantErr     string
		wantWarn    string
	}{
		{
			name:   "builtin beats user and community",
			userID: "go-cli", communityID: "go-cli", trusted: trustAll,
			wantID: "go-cli", wantScope: ScopeBuiltin,
			wantWarn: "shadowed by the builtin",
		},
		{
			name:   "user beats community",
			userID: "solo", communityID: "solo", trusted: trustAll,
			wantID: "solo", wantScope: ScopeUser,
			wantWarn: "shadowed by the user",
		},
		{
			name:        "community resolves alone",
			communityID: "solo", trusted: trustAll,
			wantID: "solo", wantScope: ScopeCommunity,
		},
		{
			name:        "untrusted community is refused with review hint",
			communityID: "solo", trusted: trustNone,
			wantErr: "not trusted",
		},
		{
			name:    "unknown template lists candidates",
			trusted: trustAll,
			wantErr: "unknown template",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			userDir := t.TempDir()
			communityDir := t.TempDir()
			if c.userID != "" {
				writeTemplate(t, userDir, c.userID, c.userID, "1.0.0", nil)
			}
			if c.communityID != "" {
				writeTemplate(t, communityDir, c.communityID+"@2.0.0", c.communityID, "2.0.0", nil)
			}
			var warnings []string
			r := NewStandard(Builtin(), []string{userDir})
			r.CommunityDir = communityDir
			r.Trusted = c.trusted
			r.Warn = func(format string, args ...any) {
				warnings = append(warnings, fmt.Sprintf(format, args...))
			}

			got, err := r.Resolve(context.Background(), firstNonEmpty(c.userID, c.communityID, "no-such-template"))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("Resolve err = %v, want substring %q", err, c.wantErr)
				}
				if c.wantErr == "not trusted" {
					var ut *UntrustedError
					if !asUntrusted(err, &ut) {
						t.Errorf("untrusted failure should be *UntrustedError, got %T", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Manifest.ID != c.wantID || got.Identity.Scope != c.wantScope {
				t.Errorf("Resolve = %s/%s, want %s/%s", got.Identity.Scope, got.Manifest.ID, c.wantScope, c.wantID)
			}
			if c.wantWarn != "" {
				if !anyContains(warnings, c.wantWarn) {
					t.Errorf("warnings = %v, want one containing %q", warnings, c.wantWarn)
				}
			}
		})
	}
}

// Community resolution picks the newest trusted version.
func TestCommunityPicksNewestTrusted(t *testing.T) {
	communityDir := t.TempDir()
	writeTemplate(t, communityDir, "acme@1.0.0", "acme", "1.0.0", nil)
	writeTemplate(t, communityDir, "acme@1.2.0", "acme", "1.2.0", nil)
	writeTemplate(t, communityDir, "acme@1.10.0", "acme", "1.10.0", nil)

	r := NewStandard(Builtin(), nil)
	r.CommunityDir = communityDir
	r.Trusted = trustAll

	got, err := r.Resolve(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Version != "1.10.0" {
		t.Errorf("newest = %s, want 1.10.0 (semver order, not lexicographic)", got.Manifest.Version)
	}
}

// With 1.10.0 untrusted, resolution falls back to the newest trusted one.
func TestCommunitySkipsUntrustedVersions(t *testing.T) {
	communityDir := t.TempDir()
	writeTemplate(t, communityDir, "acme@1.0.0", "acme", "1.0.0", nil)
	writeTemplate(t, communityDir, "acme@1.10.0", "acme", "1.10.0", nil)

	r := NewStandard(Builtin(), nil)
	r.CommunityDir = communityDir
	r.Trusted = func(_, version string) bool { return version == "1.0.0" }

	got, err := r.Resolve(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Version != "1.0.0" {
		t.Errorf("trusted fallback = %s, want 1.0.0", got.Manifest.Version)
	}
}

func TestListScopes(t *testing.T) {
	userDir := t.TempDir()
	communityDir := t.TempDir()
	writeTemplate(t, userDir, "mine", "mine", "1.0.0", nil)
	writeTemplate(t, communityDir, "theirs@1.0.0", "theirs", "1.0.0", nil)

	r := NewStandard(Builtin(), []string{userDir})
	r.CommunityDir = communityDir
	r.Trusted = trustAll

	without, err := r.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if containsID(without, "theirs") {
		t.Error("community scope leaked into allScopes=false")
	}
	if !containsID(without, "mine") || !containsID(without, "go-cli") {
		t.Errorf("List(false) missing expected ids: %v", without)
	}

	with, err := r.List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !containsID(with, "theirs") {
		t.Errorf("List(true) missing community id: %v", with)
	}
}

func TestListHidesUntrustedCommunity(t *testing.T) {
	communityDir := t.TempDir()
	writeTemplate(t, communityDir, "theirs@1.0.0", "theirs", "1.0.0", nil)
	r := NewStandard(Builtin(), nil)
	r.CommunityDir = communityDir
	r.Trusted = trustNone

	list, err := r.List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if containsID(list, "theirs") {
		t.Error("untrusted community template must not be resolvable/listable")
	}
}

// GitSource Pin/Fetch against a real local repository (ADR 0004: exec
// git). Skipped when git is absent.
func TestGitSourcePinsAndFetches(t *testing.T) {
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(repo, "guppy.toml"), []byte("id = \"gitty\"\nversion = \"0.1.0\"\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(repo, "template"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, "template", "hello.txt"), []byte("hi\n"), 0o644)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init")

	checkout := filepath.Join(t.TempDir(), "checkout")
	src := &GitSource{URL: repo, Ref: "main", Dir: checkout}
	pin, err := src.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if len(pin) != 40 {
		t.Fatalf("Pin = %q, want 40-char SHA", pin)
	}
	fsys, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	data, err := fs.ReadFile(fsys, "template/hello.txt")
	if err != nil || string(data) != "hi\n" {
		t.Errorf("fetched payload = %q, %v", data, err)
	}

	_ = os.WriteFile(filepath.Join(repo, "template", "hello.txt"), []byte("v2\n"), 0o644)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "v2")
	pin2, err := src.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pin2 == pin {
		t.Error("pin did not move after a new commit on the ref")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func containsID(list []Template, id string) bool {
	for _, t := range list {
		if t.Manifest.ID == id {
			return true
		}
	}
	return false
}

func anyContains(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func asUntrusted(err error, target **UntrustedError) bool {
	ut, ok := err.(*UntrustedError)
	if ok {
		*target = ut
	}
	return ok
}
