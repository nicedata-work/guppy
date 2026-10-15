package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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

	"github.com/nicedata-work/guppy/internal/httpx"
)

// buildTarGz creates a template tarball (optionally wrapped in a top
// dir, like `tar czf x.tgz dir/`).
func buildTarGz(t *testing.T, wrap bool, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		full := name
		if wrap {
			full = "wrapper/" + name
		}
		if err := tw.WriteHeader(&tar.Header{Name: full, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
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

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// newTestRegistry serves index.json + tarballs from memory.
type testRegistry struct {
	*httptest.Server
	index    string
	tarballs map[string][]byte
}

func newTestRegistry(t *testing.T, entries []Entry, tarballs map[string][]byte) *testRegistry {
	t.Helper()
	reg := &testRegistry{tarballs: tarballs}
	var b strings.Builder
	b.WriteString(`{"schema":1,"templates":[`)
	for i, e := range entries {
		if i > 0 {
			b.WriteString(",")
		}
		kws, _ := json.Marshal(e.Keywords)
		_, _ = fmt.Fprintf(&b, `{"id":%q,"version":%q,"kind":%q,"language":%q,"description":%q,"keywords":%s,"tarball":%q,"sha256":%q}`,
			e.ID, e.Version, e.Kind, e.Language, e.Description, kws, e.Tarball, e.SHA256)
	}
	b.WriteString(`]}`)
	reg.index = b.String()
	reg.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+IndexFile:
			_, _ = fmt.Fprint(w, reg.index)
		case strings.HasPrefix(r.URL.Path, "/tarballs/"):
			data, ok := reg.tarballs[strings.TrimPrefix(r.URL.Path, "/tarballs/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(reg.Close)
	return reg
}

func (r *testRegistry) client() *Client {
	return &Client{BaseURL: r.URL, HTTP: r.Client(), CacheDir: filepath.Join(os.TempDir(), "guppy-test-cache", sha([]byte(r.URL)))}
}

func TestIndexDecodeAndVersionOrdering(t *testing.T) {
	reg := newTestRegistry(t, []Entry{
		{ID: "acme", Version: "1.0.0"},
		{ID: "acme", Version: "1.10.0"},
		{ID: "acme", Version: "1.2.0"},
		{ID: "other", Version: "2.0.0"},
	}, nil)
	versions, err := reg.client().Versions(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range versions {
		got = append(got, v.Version)
	}
	want := []string{"1.10.0", "1.2.0", "1.0.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Versions order = %v, want %v", got, want)
	}
}

func TestGetLatestAndExact(t *testing.T) {
	reg := newTestRegistry(t, []Entry{
		{ID: "acme", Version: "1.0.0"},
		{ID: "acme", Version: "2.0.0"},
	}, nil)
	c := reg.client()
	e, err := c.Get(context.Background(), "acme", "")
	if err != nil || e.Version != "2.0.0" {
		t.Errorf("Get latest = %v, %v", e, err)
	}
	e, err = c.Get(context.Background(), "acme", "1.0.0")
	if err != nil || e.Version != "1.0.0" {
		t.Errorf("Get exact = %v, %v", e, err)
	}
	if _, err := c.Get(context.Background(), "acme", "9.9.9"); err == nil {
		t.Error("Get missing version: expected error")
	}
	if _, err := c.Get(context.Background(), "nope", ""); err == nil {
		t.Error("Get unknown id: expected error")
	}
}

func TestSearchRequiresCommunityKeyword(t *testing.T) {
	reg := newTestRegistry(t, []Entry{
		{ID: "acme-cli", Version: "1.0.0", Keywords: []string{"guppy-template", "cli"}, Description: "an acme command-line tool"},
		{ID: "internal-thing", Version: "1.0.0", Keywords: []string{"private"}},
	}, nil)
	got, err := reg.client().Search(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "acme-cli" {
		t.Errorf("Search = %+v, want only acme-cli (keyword gate)", got)
	}
	got, err = reg.client().Search(context.Background(), "internal")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("non-keyword entry leaked into search: %+v", got)
	}
}

func TestFetchVerifiesChecksumAndCaches(t *testing.T) {
	tarball := buildTarGz(t, true, map[string]string{
		"guppy.toml":     "id = \"acme\"\nversion = \"1.0.0\"\n",
		"template/a.txt": "hello",
	})
	reg := newTestRegistry(t, []Entry{{ID: "acme", Version: "1.0.0", Tarball: "/tarballs/acme-1.0.0.tgz", SHA256: sha(tarball)}},
		map[string][]byte{"acme-1.0.0.tgz": tarball})
	c := reg.client()
	entry, err := c.Get(context.Background(), "acme", "")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := c.Fetch(context.Background(), entry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "guppy.toml")); err != nil {
		t.Errorf("guppy.toml not extracted (wrapper dir not stripped?): %v", err)
	}

	reg.Close()
	dir2, err := c.Fetch(context.Background(), entry)
	if err != nil || dir2 != dir {
		t.Errorf("cached Fetch = %q, %v; want same dir, nil", dir2, err)
	}
}

func TestFetchChecksumMismatch(t *testing.T) {
	tarball := buildTarGz(t, false, map[string]string{"guppy.toml": "id = \"acme\"\n"})
	reg := newTestRegistry(t, []Entry{{ID: "acme", Version: "1.0.0", Tarball: "/tarballs/a.tgz", SHA256: strings.Repeat("ab", 32)}},
		map[string][]byte{"a.tgz": tarball})
	entry, _ := reg.client().Get(context.Background(), "acme", "")
	_, err := reg.client().Fetch(context.Background(), entry)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Fetch with bad checksum: %v, want checksum mismatch", err)
	}
}

func TestFetchRefusesMissingChecksum(t *testing.T) {
	reg := newTestRegistry(t, []Entry{{ID: "acme", Version: "1.0.0", Tarball: "/tarballs/a.tgz"}}, nil)
	_, err := reg.client().Fetch(context.Background(), Entry{ID: "acme", Version: "1.0.0"})
	if err == nil || !strings.Contains(err.Error(), "no sha256") {
		t.Errorf("Fetch without checksum: %v", err)
	}
}

func TestOfflineIsNetworkError(t *testing.T) {
	c := &Client{BaseURL: "https://example.invalid/registry", Offline: true}
	_, err := c.Index(context.Background())
	if err == nil || !httpx.IsNetworkError(err) {
		t.Fatalf("offline Index: %v, want network error", err)
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Errorf("offline error should say so: %v", err)
	}
	if _, err := c.Fetch(context.Background(), Entry{ID: "x", Version: "1", SHA256: "y"}); !httpx.IsNetworkError(err) {
		t.Errorf("offline Fetch: %v, want network error", err)
	}
}

func TestIndexHTTPErrorsAreNetworkErrors(t *testing.T) {
	reg := httptest.NewServer(http.NotFoundHandler())
	defer reg.Close()
	c := &Client{BaseURL: reg.URL, HTTP: reg.Client()}
	if _, err := c.Index(context.Background()); !httpx.IsNetworkError(err) {
		t.Errorf("404 index: %v, want network error", err)
	}
}
