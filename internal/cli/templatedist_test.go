package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"

	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/registry"
)

// distHarness sandboxes the XDG layout for a distribution test and
// serves a mutable registry (index + tarballs) from memory.
type distHarness struct {
	t        *testing.T
	root     string
	server   *httptest.Server
	entries  []registry.Entry
	tarballs map[string][]byte
	env      map[string]string
}

func newDistHarness(t *testing.T) *distHarness {
	t.Helper()
	h := &distHarness{t: t, root: t.TempDir(), tarballs: map[string][]byte{}}
	for k, v := range map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(h.root, "cfg"),
		"XDG_DATA_HOME":   filepath.Join(h.root, "data"),
		"XDG_CACHE_HOME":  filepath.Join(h.root, "cache"),
		"HOME":            filepath.Join(h.root, "home"),
	} {
		t.Setenv(k, v)
	}
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })

	_ = os.MkdirAll(filepath.Join(h.root, "home"), 0o755)
	h.server = httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(h.server.Close)
	h.env = map[string]string{
		"GUPPY_REGISTRY":         h.server.URL,
		"GUPPY_DEFAULTS_GIT":     "false",
		"GUPPY_DEFAULTS_BAZEL":   "false",
		"GUPPY_DEFAULTS_DIRENV":  "false",
		"GUPPY_DEFAULTS_LINTERS": "false",
		"GUPPY_DEFAULTS_TESTS":   "false",
	}
	return h
}

func (h *distHarness) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/index.json":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"schema": 1, "templates": h.entries})
	case strings.HasPrefix(r.URL.Path, "/tarballs/"):
		data, ok := h.tarballs[strings.TrimPrefix(r.URL.Path, "/tarballs/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

// publish adds id@version to the index. files maps template-relative
// paths to contents; "guppy.toml" is required.
func (h *distHarness) publish(id, version string, files map[string]string) {
	h.t.Helper()
	data := buildTemplateTarball(h.t, files)
	name := fmt.Sprintf("%s-%s.tgz", id, version)
	h.tarballs[name] = data
	sum := sha256.Sum256(data)

	h.entries = append(h.entries, registry.Entry{
		ID: id, Version: version, Kind: "cli", Language: "go",
		Description: "an acme template", Keywords: []string{"guppy-template"},
		Tarball: "/tarballs/" + name, SHA256: hex.EncodeToString(sum[:]),
	})
}

func buildTemplateTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func manifestFor(id, version string, extra string) string {
	return fmt.Sprintf("id = %q\nversion = %q\nkind = \"cli\"\nlanguage = \"go\"\n%s", id, version, extra)
}

func TestTemplateAddUserScopeNeedsNoGate(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{
		"guppy.toml":     manifestFor("acme", "1.0.0", ""),
		"template/a.txt": "hello\n",
	})

	code, _, stderr := run([]string{"template", "add", "registry:acme@1.0.0"}, h.env)
	if code != ExitOK {
		t.Fatalf("add exit = %d (stderr: %s)", code, stderr)
	}

	dir := filepath.Join(h.root, "data", "guppy", "templates", "acme")
	if _, err := os.Stat(filepath.Join(dir, "guppy.toml")); err != nil {
		t.Fatalf("user-scope install missing: %v", err)
	}
	code, stdout, _ := run([]string{"new", "--template", "acme", filepath.Join(h.root, "proj")}, h.env)
	if code != ExitOK {
		t.Fatalf("new exit = %d (stdout: %s)", code, stdout)
	}
	data, err := os.ReadFile(filepath.Join(h.root, "proj", "a.txt"))
	if err != nil || string(data) != "hello\n" {
		t.Errorf("scaffolded a.txt = %q, %v", data, err)
	}
}

func TestTemplateAddCommunityRequiresReview(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{
		"guppy.toml":     manifestFor("acme", "1.0.0", ""),
		"template/a.txt": "hello\n",
	})

	code, stdout, stderr := run([]string{"template", "add", "community:registry:acme@1.0.0"}, h.env)
	if code != ExitInput {
		t.Fatalf("gate exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, "review: acme@1.0.0") || !strings.Contains(stderr, "template/a.txt") {
		t.Errorf("review output missing manifest/file list on stderr:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout must carry no review content, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("gate error should mention --yes: %s", stderr)
	}

	code, _, _ = run([]string{"new", "--template", "acme", filepath.Join(h.root, "proj1")}, h.env)
	if code != ExitInput {
		t.Fatalf("untrusted resolve exit = %d, want %d", code, ExitInput)
	}

	code, _, stderr = run([]string{"template", "add", "community:registry:acme@1.0.0", "--yes"}, h.env)
	if code != ExitOK {
		t.Fatalf("add --yes exit = %d (stderr: %s)", code, stderr)
	}
	communityDir := filepath.Join(h.root, "data", "guppy", "community", "acme@1.0.0")
	if _, err := os.Stat(filepath.Join(communityDir, "guppy.toml")); err != nil {
		t.Fatalf("community install missing: %v", err)
	}
	code, _, stderr = run([]string{"new", "--template", "acme", filepath.Join(h.root, "proj2")}, h.env)
	if code != ExitOK {
		t.Fatalf("trusted resolve exit = %d (stderr: %s)", code, stderr)
	}
}

func TestTemplateUpdateCommunityShowsDiffAndGates(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{
		"guppy.toml":     manifestFor("acme", "1.0.0", ""),
		"template/a.txt": "v1\n",
	})
	if code, _, stderr := run([]string{"template", "add", "community:acme", "--yes"}, h.env); code != ExitOK {
		t.Fatalf("add exit = %d (stderr: %s)", code, stderr)
	}

	h.publish("acme", "1.1.0", map[string]string{
		"guppy.toml":     manifestFor("acme", "1.1.0", ""),
		"template/a.txt": "v2\n",
	})

	code, stdout, stderr := run([]string{"template", "update", "acme"}, h.env)
	if code != ExitInput {
		t.Fatalf("update gate exit = %d, want %d (stderr: %s)", code, ExitInput, stderr)
	}
	if !strings.Contains(stderr, "-v1") || !strings.Contains(stderr, "+v2") {
		t.Errorf("update review should show the diff on stderr:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout must carry no review content, got:\n%s", stdout)
	}

	code, _, stderr = run([]string{"template", "update", "--all", "--yes"}, h.env)
	if code != ExitOK {
		t.Fatalf("update --yes exit = %d (stderr: %s)", code, stderr)
	}
	old := filepath.Join(h.root, "data", "guppy", "community", "acme@1.0.0")
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old version dir not cleaned up")
	}
	data, err := os.ReadFile(filepath.Join(h.root, "data", "guppy", "community", "acme@1.1.0", "template", "a.txt"))
	if err != nil || string(data) != "v2\n" {
		t.Errorf("updated content = %q, %v", data, err)
	}

	code, _, stderr = run([]string{"template", "update", "--all"}, h.env)
	if code != ExitOK {
		t.Fatalf("re-update exit = %d (stderr: %s)", code, stderr)
	}
}

func TestTemplateRemoveScopes(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{"guppy.toml": manifestFor("acme", "1.0.0", "")})
	if code, _, stderr := run([]string{"template", "add", "acme"}, h.env); code != ExitOK {
		t.Fatalf("add user exit = %d (%s)", code, stderr)
	}
	if code, _, _ := run([]string{"template", "remove", "acme"}, h.env); code != ExitOK {
		t.Fatal("remove user failed")
	}
	if _, err := os.Stat(filepath.Join(h.root, "data", "guppy", "templates", "acme")); !os.IsNotExist(err) {
		t.Error("user install not removed")
	}

	if code, _, stderr := run([]string{"template", "add", "community:acme", "--yes"}, h.env); code != ExitOK {
		t.Fatalf("add community exit = %d (%s)", code, stderr)
	}
	if code, _, _ := run([]string{"template", "remove", "community:acme"}, h.env); code != ExitOK {
		t.Fatal("remove community failed")
	}
	if _, err := os.Stat(filepath.Join(h.root, "data", "guppy", "community", "acme@1.0.0")); !os.IsNotExist(err) {
		t.Error("community install not removed")
	}

	code, _, stderr := run([]string{"new", "--template", "acme", filepath.Join(h.root, "proj")}, h.env)
	if code != ExitInput || !strings.Contains(stderr, "unknown template") {
		t.Errorf("after remove: exit = %d, stderr = %s; want unknown template", code, stderr)
	}
}

func TestTemplateListShowsScopesAndSources(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{"guppy.toml": manifestFor("acme", "1.0.0", "")})
	run([]string{"template", "add", "acme"}, h.env)
	run([]string{"template", "add", "community:acme", "--yes"}, h.env)

	code, stdout, stderr := run([]string{"template", "list"}, h.env)
	if code != ExitOK {
		t.Fatalf("list exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "go-cli") || !strings.Contains(stdout, "acme") {
		t.Errorf("list missing builtin/user ids:\n%s", stdout)
	}
	if !strings.Contains(stdout, "registry:acme@1.0.0") {
		t.Errorf("list should show recorded provenance:\n%s", stdout)
	}

	code, stdout, _ = run([]string{"template", "list", "--all-scopes"}, h.env)
	if code != ExitOK {
		t.Fatal("list --all-scopes failed")
	}
	if !strings.Contains(stdout, "community") {
		t.Errorf("all-scopes list should include community scope:\n%s", stdout)
	}
}

func TestTemplateSearchQueriesRegistry(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme-cli", "1.0.0", map[string]string{"guppy.toml": manifestFor("acme-cli", "1.0.0", "")})

	code, stdout, stderr := run([]string{"template", "search", "acme"}, h.env)
	if code != ExitOK {
		t.Fatalf("search exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "acme-cli") || !strings.Contains(stdout, "1.0.0") {
		t.Errorf("search output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "community:") {
		t.Errorf("search should advertise the community add form:\n%s", stdout)
	}

	code, _, stderr = run([]string{"template", "search", "acme", "--offline"}, h.env)
	if code != ExitNetwork || !strings.Contains(stderr, "offline") {
		t.Errorf("offline search: exit = %d, stderr = %s; want network error", code, stderr)
	}
}

func TestTemplateAddOfflineIsNetworkError(t *testing.T) {
	h := newDistHarness(t)
	code, _, stderr := run([]string{"template", "add", "acme", "--offline"}, h.env)
	if code != ExitNetwork || !strings.Contains(stderr, "offline") {
		t.Errorf("offline add: exit = %d, stderr = %s; want exit 3", code, stderr)
	}
}

func TestTemplateAddChecksumMismatchIsNetworkError(t *testing.T) {
	h := newDistHarness(t)
	h.publish("acme", "1.0.0", map[string]string{"guppy.toml": manifestFor("acme", "1.0.0", "")})

	h.tarballs["acme-1.0.0.tgz"] = []byte("not a tarball")
	code, _, stderr := run([]string{"template", "add", "acme"}, h.env)
	if code != ExitFailure && code != ExitNetwork {
		t.Errorf("corrupt tarball: exit = %d, want failure", code)
	}
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("corrupt tarball should fail the checksum: %s", stderr)
	}
}

func TestTemplateAddLocalPath(t *testing.T) {
	h := newDistHarness(t)
	src := filepath.Join(h.root, "local-template")
	_ = os.MkdirAll(filepath.Join(src, "template"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "guppy.toml"), []byte(manifestFor("localish", "0.3.0", "")), 0o644)
	_ = os.WriteFile(filepath.Join(src, "template", "x.txt"), []byte("x\n"), 0o644)

	code, _, stderr := run([]string{"template", "add", src}, h.env)
	if code != ExitOK {
		t.Fatalf("local add exit = %d (%s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.root, "data", "guppy", "templates", "localish", "template", "x.txt")); err != nil {
		t.Errorf("local copy missing: %v", err)
	}
}

func TestTemplateClone(t *testing.T) {
	h := newDistHarness(t)
	into := filepath.Join(h.root, "clone-out")
	code, _, stderr := run([]string{"template", "clone", "go-cli", "--into", into}, h.env)
	if code != ExitOK {
		t.Fatalf("clone exit = %d (%s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(into, "guppy.toml")); err != nil {
		t.Errorf("cloned manifest missing: %v", err)
	}

	code, _, _ = run([]string{"template", "clone", "go-cli", "--into", into}, h.env)
	if code != ExitInput {
		t.Errorf("clone into existing dir: exit = %d, want %d", code, ExitInput)
	}
}

func TestParseTemplateSpec(t *testing.T) {
	cases := []struct {
		in        string
		community bool
		kind      dist.SourceKind
		id        string
		version   string
		url       string
		ref       string
		wantErr   bool
	}{
		{in: "acme", kind: "registry", id: "acme"},
		{in: "registry:acme@1.2.3", kind: "registry", id: "acme", version: "1.2.3"},
		{in: "community:acme", community: true, kind: "registry", id: "acme"},
		{in: "community:registry:acme@2.0.0", community: true, kind: "registry", id: "acme", version: "2.0.0"},
		{in: "git:https://example.com/r.git@main", kind: "git", url: "https://example.com/r.git", ref: "main"},
		{in: "git:https://example.com/r.git", kind: "git", url: "https://example.com/r.git"},
		{in: "community:git:https://example.com/r.git@dev", community: true, kind: "git", url: "https://example.com/r.git", ref: "dev"},
		{in: "", wantErr: true},
		{in: "community:", wantErr: true},
		{in: "wat:thing", wantErr: true},
		{in: "registry:", wantErr: true},
	}
	for _, c := range cases {
		got, err := parseTemplateSpec(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseTemplateSpec(%q): expected error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTemplateSpec(%q): %v", c.in, err)
			continue
		}
		if got.Community != c.community || got.Kind != c.kind || got.ID != c.id || got.Version != c.version || got.URL != c.url || got.Ref != c.ref {
			t.Errorf("parseTemplateSpec(%q) = %+v, want community=%v kind=%s id=%s ver=%s url=%s ref=%s",
				c.in, got, c.community, c.kind, c.id, c.version, c.url, c.ref)
		}
	}
}

func TestSplitGitRef(t *testing.T) {
	cases := []struct{ in, url, ref string }{
		{"https://x/repo.git@main", "https://x/repo.git", "main"},
		{"https://x/repo.git", "https://x/repo.git", ""},
		{"git@github.com:org/repo.git", "git@github.com:org/repo.git", ""},
		{"https://u@host/repo.git@v1", "https://u@host/repo.git", "v1"},
	}
	for _, c := range cases {
		url, ref := splitGitRef(c.in)
		if url != c.url || ref != c.ref {
			t.Errorf("splitGitRef(%q) = %q,%q; want %q,%q", c.in, url, ref, c.url, c.ref)
		}
	}
}
