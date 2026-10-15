package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/paths"
	"github.com/nicedata-work/guppy/internal/registry"
	"github.com/nicedata-work/guppy/internal/settings"
)

// fakeRegistry serves an index (and 404s for anything else — resolve
// tests only touch the index; tarball integrity is the client's job and
// covered by the distribution tests).
func fakeRegistry(t *testing.T, entries ...registry.Entry) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"schema": 1, "templates": entries})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func chainRuntime(t *testing.T, s settings.Settings, env map[string]string) *runtime {
	t.Helper()
	base := t.TempDir()
	return &runtime{
		env: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		layout: paths.Layout{
			ConfigHome: filepath.Join(base, "config"),
			DataHome:   filepath.Join(base, "data"),
			CacheHome:  filepath.Join(base, "cache"),
			BinDir:     filepath.Join(base, "bin"),
		},
		settings: s,
		out:      &output{g: &globals{}, env: func(string) (string, bool) { return "", false }},
	}
}

func entry(id, version string) registry.Entry {
	return registry.Entry{ID: id, Version: version, Tarball: id + "-" + version + ".tar.gz",
		SHA256: strings.Repeat("a", 64), Keywords: []string{"guppy-template"}}
}

func TestResolveRegistryChainOrder(t *testing.T) {
	team := fakeRegistry(t, entry("alpha", "1.0.0"))
	public := fakeRegistry(t, entry("alpha", "2.0.0"), entry("beta", "0.1.0"))
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", URL: team.URL}},
			Registry:   public.URL,
		},
	}, nil)

	e, reg, err := rt.resolveRegistry(context.Background(), "", "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Version != "1.0.0" || reg.Name != "team" {
		t.Errorf("alpha = %s@%s via %q, want 1.0.0 via team", e.ID, e.Version, reg.Name)
	}

	e, reg, err = rt.resolveRegistry(context.Background(), "", "beta", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Version != "0.1.0" || reg.Name != "" {
		t.Errorf("beta = %s@%s via %q, want 0.1.0 via default", e.ID, e.Version, reg.Name)
	}
}

func TestResolveRegistryNetworkFailureAborts(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)
	public := fakeRegistry(t, entry("alpha", "2.0.0"))
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", URL: broken.URL}},
			Registry:   public.URL,
		},
	}, nil)

	_, _, err := rt.resolveRegistry(context.Background(), "", "alpha", "")
	if err == nil {
		t.Fatal("want error when an earlier registry is down")
	}
	if !registry.IsNetworkError(err) {
		t.Errorf("err = %v, want a network error (no silent fall-through)", err)
	}
}

func TestResolveRegistryNamedDisambiguation(t *testing.T) {
	team := fakeRegistry(t, entry("gamma", "0.2.0"))
	public := fakeRegistry(t)
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", URL: team.URL}},
			Registry:   public.URL,
		},
	}, nil)

	e, reg, err := rt.resolveRegistry(context.Background(), "team", "gamma", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if reg.Name != "team" || e.Version != "0.2.0" {
		t.Errorf("got %s@%s via %q, want gamma@0.2.0 via team", e.ID, e.Version, reg.Name)
	}
	if _, _, err := rt.resolveRegistry(context.Background(), "nope", "gamma", ""); err == nil {
		t.Error("unknown registry name must be an error")
	}
}

func TestSearchRegistriesDeduplicates(t *testing.T) {
	team := fakeRegistry(t, entry("alpha", "1.0.0"), entry("team-only", "0.1.0"))
	public := fakeRegistry(t, entry("alpha", "2.0.0"), entry("public-only", "0.3.0"))
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", URL: team.URL}},
			Registry:   public.URL,
		},
	}, nil)

	hits, err := rt.searchRegistries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range hits {
		got[h.ID] = h.Registry
	}
	if got["alpha"] != "team" {
		t.Errorf("alpha from %q, want team (chain order)", got["alpha"])
	}
	if got["team-only"] != "team" || got["public-only"] != "" {
		t.Errorf("unexpected aggregation: %v", got)
	}
}

func TestRegistryChainTokenFromEnv(t *testing.T) {
	srv := fakeRegistry(t, entry("alpha", "1.0.0"))
	var gotAuth string
	authed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/index.json" {
			json.NewEncoder(w).Encode(map[string]any{"schema": 1, "templates": []registry.Entry{}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(authed.Close)
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", URL: authed.URL}},
			Registry:   srv.URL,
		},
	}, map[string]string{"GUPPY_REGISTRY_TEAM_TOKEN": "sekrit"})

	if _, err := rt.searchRegistries(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sekrit" {
		t.Errorf("Authorization = %q, want the per-registry bearer token", gotAuth)
	}
}

func TestOfflineRegistryChainIsRefused(t *testing.T) {
	s := settings.Settings{Template: settings.Template{Registry: "https://files.example/registry"}}
	rt := chainRuntime(t, s, nil)
	rt.settings.Network.Offline = true
	if _, err := rt.searchRegistries(context.Background(), ""); !registry.IsNetworkError(err) {
		t.Errorf("err = %v, want ErrOffline network error", err)
	}
}

func TestParseRegistrySpecNamedRegistry(t *testing.T) {

	spec, err := parseTemplateSpec("registry:team:foo@1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != dist.KindRegistry || spec.RegistryName != "team" || spec.ID != "foo" || spec.Version != "1.2.0" {
		t.Errorf("spec = %+v, want team/foo/1.2.0", spec)
	}
	if got := spec.String(); got != "registry:team:foo@1.2.0" {
		t.Errorf("String() = %q, want a round-trippable spec", got)
	}

	spec, err = parseTemplateSpec("registry:foo")
	if err != nil {
		t.Fatal(err)
	}
	if spec.RegistryName != "" || spec.ID != "foo" {
		t.Errorf("spec = %+v, want plain registry id", spec)
	}
}

// gitRegistryRepo builds a real local git repository whose checkout IS
// the registry: index.json at the root, tarballs/ beside it (the
// paved-road layout for a private org repo or Azure DevOps git repo).
func gitRegistryRepo(t *testing.T, tarballName string, tarball []byte, entries ...registry.Entry) string {
	t.Helper()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tarballs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tarballName), tarball, 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := json.Marshal(map[string]any{"schema": 1, "templates": entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), idx, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("-C", dir, "init", "-q", "-b", "main")
	runGit("-C", dir, "add", ".")
	runGit("-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "registry")
	return dir
}

// tarGz builds an in-memory tarball wrapping one top-level directory.
func tarGz(t *testing.T, root, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: root + "/", Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	body := content
	if err := tw.WriteHeader(&tar.Header{Name: root + "/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGitRegistryResolveFetchAndOffline(t *testing.T) {
	tarball := tarGz(t, "alpha", "guppy.toml", "id = \"alpha\"\nversion = \"1.0.0\"\n")
	sum := sha256.Sum256(tarball)
	repo := gitRegistryRepo(t, "tarballs/guppy-alpha-1.0.0.tar.gz", tarball,
		registry.Entry{ID: "alpha", Version: "1.0.0",
			Tarball: "tarballs/guppy-alpha-1.0.0.tar.gz",
			SHA256:  hex.EncodeToString(sum[:]), Keywords: []string{"guppy-template"}})
	rt := chainRuntime(t, settings.Settings{
		Template: settings.Template{
			Registries: []settings.NamedRegistry{{Name: "team", Git: repo}},
		},
	}, nil)
	ctx := context.Background()

	e, reg, err := rt.resolveRegistry(ctx, "", "alpha", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if reg.Name != "team" || e.Version != "1.0.0" {
		t.Fatalf("got %s@%s via %q", e.ID, e.Version, reg.Name)
	}

	client, err := rt.registryClientFor(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := client.Fetch(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "guppy.toml")); err != nil {
		t.Errorf("extracted template: %v", err)
	}

	rt.settings.Network.Offline = true
	if _, _, err := rt.resolveRegistry(ctx, "", "alpha", "1.0.0"); err != nil {
		t.Errorf("offline resolve with warm clone: %v", err)
	}
}
