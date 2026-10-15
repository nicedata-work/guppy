package selfmanage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicedata-work/guppy/internal/paths"
)

// makeTarball builds a gzipped tar containing a single "guppy" entry with
// the given content — byte-identical to what scripts/release.sh produces.
func makeTarball(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
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

// serveRelease serves latest.txt + the assets map from an httptest server.
// assets maps filename → tarball bytes; sums are generated automatically.
func serveRelease(t *testing.T, latest string, assets map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	if latest != "" {
		mux.HandleFunc("/latest.txt", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, latest+"\n")
		})
	}
	for name, body := range assets {
		sum := sha256.Sum256(body)
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Write(body)
		})
		mux.HandleFunc("/"+name+".sha256", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateReplacesBinary(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "#!/bin/sh\necho guppy 9.9.9\n")
	srv := serveRelease(t, "9.9.9", map[string][]byte{AssetName("9.9.9", Platform{GOOS: "linux", GOARCH: "amd64"}): tarball})

	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	if err := os.WriteFile(current, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var progress strings.Builder
	res, err := Update(context.Background(), UpdateOptions{
		BaseURL:     srv.URL,
		Client:      srv.Client(),
		CurrentPath: current,
		Platform:    Platform{GOOS: "linux", GOARCH: "amd64"},
		Progress:    &progress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9", res.Version)
	}
	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho guppy 9.9.9\n" {
		t.Errorf("binary content = %q, want new release content", got)
	}

	if _, err := os.Stat(current + ".bak"); !os.IsNotExist(err) {
		t.Error("backup file should be removed after successful swap")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir should contain only the binary, got %v", entries)
	}
	if !strings.Contains(progress.String(), "9.9.9") {
		t.Errorf("progress should mention the version, got %q", progress.String())
	}
}

func TestUpdatePinnedVersion(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "v1.2.3-content")
	srv := serveRelease(t, "9.9.9", map[string][]byte{AssetName("1.2.3", Platform{GOOS: "linux", GOARCH: "amd64"}): tarball})

	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	os.WriteFile(current, []byte("old"), 0o755)

	res, err := Update(context.Background(), UpdateOptions{
		BaseURL:     srv.URL,
		Client:      srv.Client(),
		Version:     "1.2.3",
		CurrentPath: current,
		Platform:    Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Version != "1.2.3" {
		t.Errorf("Version = %q, want pinned 1.2.3", res.Version)
	}
}

func TestUpdateChecksumMismatchLeavesBinaryUntouched(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "evil content")

	mux := http.NewServeMux()
	name := AssetName("9.9.9", Platform{GOOS: "linux", GOARCH: "amd64"})
	mux.HandleFunc("/latest.txt", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "9.9.9\n") })
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(tarball) })
	mux.HandleFunc("/"+name+".sha256", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), name)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	os.WriteFile(current, []byte("trusted old binary"), 0o755)

	_, err := Update(context.Background(), UpdateOptions{
		BaseURL: srv.URL, Client: srv.Client(),
		CurrentPath: current, Platform: Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch error, got %v", err)
	}
	got, _ := os.ReadFile(current)
	if string(got) != "trusted old binary" {
		t.Errorf("binary was modified despite checksum failure: %q", got)
	}
}

// cancelOnEOF cancels a context once a response body is fully read — a
// seam for landing an interrupt in the post-download window.
type cancelOnEOF struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnEOF) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		r.cancel()
	}
	return n, err
}

// cancelAfterTarballTransport cancels the ctx once the tarball body has
// been completely read (the earliest interrupt that still leaves the
// download "complete", i.e. inside the staging/swap window).
type cancelAfterTarballTransport struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (t *cancelAfterTarballTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err == nil && strings.HasSuffix(req.URL.Path, ".tar.gz") {
		resp.Body = &cancelOnEOF{ReadCloser: resp.Body, cancel: t.cancel}
	}
	return resp, err
}

// TestUpdateInterruptedAfterDownloadLeavesBinaryUntouched pins refactor
// (styleguide T14/U14): an interrupt that lands after the bytes are
// fetched but before the swap must not touch the current binary — the
// swap is the point of no return and is guarded on ctx.
func TestUpdateInterruptedAfterDownloadLeavesBinaryUntouched(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "new content")
	srv := serveRelease(t, "9.9.9", map[string][]byte{AssetName("9.9.9", Platform{GOOS: "linux", GOARCH: "amd64"}): tarball})

	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	if err := os.WriteFile(current, []byte("trusted old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &http.Client{Transport: &cancelAfterTarballTransport{
		base:   srv.Client().Transport,
		cancel: cancel,
	}}

	_, err := Update(ctx, UpdateOptions{
		BaseURL: srv.URL, Client: client,
		CurrentPath: current, Platform: Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if !strings.Contains(err.Error(), "untouched") {
		t.Errorf("error should name the preserved binary, got %v", err)
	}
	got, _ := os.ReadFile(current)
	if string(got) != "trusted old binary" {
		t.Errorf("binary changed despite cancellation: %q", got)
	}
	if _, err := os.Stat(current + ".bak"); !os.IsNotExist(err) {
		t.Error("no backup should exist after a cancelled update")
	}
}

// TestUpdateCancelledBeforeFetchFailsFast: a ctx cancelled before the
// first network call aborts immediately — nothing is downloaded, staged,
// or swapped.
func TestUpdateCancelledBeforeFetchFailsFast(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	if err := os.WriteFile(current, []byte("trusted old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Update(ctx, UpdateOptions{
		BaseURL: "http://127.0.0.1:1", Client: &http.Client{},
		CurrentPath: current, Platform: Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	got, _ := os.ReadFile(current)
	if string(got) != "trusted old binary" {
		t.Errorf("binary changed despite cancellation: %q", got)
	}
}

func TestUpdateMissingAssetIsNetworkError(t *testing.T) {
	t.Parallel()
	srv := serveRelease(t, "9.9.9", nil)
	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	os.WriteFile(current, []byte("old"), 0o755)

	_, err := Update(context.Background(), UpdateOptions{
		BaseURL: srv.URL, Client: srv.Client(),
		CurrentPath: current, Platform: Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("want 404 download error, got %v", err)
	}
}

func TestSwapRestoresOnFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	os.WriteFile(current, []byte("old"), 0o755)

	staged := filepath.Join(dir, "no-such-subdir", "new")
	if err := swap(current, staged); err == nil {
		t.Fatal("swap should fail when staged is missing")
	}
	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("current binary must be restored after failed swap: %v", err)
	}
	if string(got) != "old" {
		t.Errorf("restored binary = %q, want original content", got)
	}
}

func TestUninstallRemovesBinaryAndRcBlocks(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	os.MkdirAll(binDir, 0o755)
	binary := filepath.Join(binDir, "guppy")
	os.WriteFile(binary, []byte("bin"), 0o755)

	rc := filepath.Join(home, ".bashrc")
	original := strings.Join([]string{
		"export EDITOR=vim",
		MarkerOpen,
		`export PATH="$HOME/.local/bin:$PATH"`,
		MarkerClose,
		"alias ll='ls -la'",
		"",
	}, "\n")
	if err := os.WriteFile(rc, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	zshrc := filepath.Join(home, ".zshrc")
	os.WriteFile(zshrc, []byte("# zsh config\n"), 0o644)

	layout := paths.Layout{
		ConfigHome: filepath.Join(home, ".config"),
		DataHome:   filepath.Join(home, ".local", "share"),
		CacheHome:  filepath.Join(home, ".cache"),
	}
	os.MkdirAll(layout.DataDir(), 0o755)
	os.WriteFile(filepath.Join(layout.DataDir(), "index.toml"), []byte("[]"), 0o644)

	res, err := Uninstall(context.Background(), UninstallOptions{
		BinaryPath: binary,
		Layout:     layout,
		HomeDir:    home,
		GOOS:       "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary); !os.IsNotExist(err) {
		t.Error("binary should be removed")
	}
	if len(res.CleanedRcs) != 1 || res.CleanedRcs[0] != rc {
		t.Errorf("CleanedRcs = %v, want [%s]", res.CleanedRcs, rc)
	}
	got, _ := os.ReadFile(rc)
	want := "export EDITOR=vim\nalias ll='ls -la'\n"
	if string(got) != want {
		t.Errorf("rc after uninstall = %q, want %q", got, want)
	}
	gotZsh, _ := os.ReadFile(zshrc)
	if string(gotZsh) != "# zsh config\n" {
		t.Error("zshrc without markers must be untouched")
	}

	if _, err := os.Stat(layout.DataDir()); err != nil {
		t.Error("data dir must be kept without --purge")
	}
	if res.KeptDataMsg != layout.DataDir() {
		t.Errorf("KeptDataMsg = %q, want %q", res.KeptDataMsg, layout.DataDir())
	}
}

func TestUninstallPurgeRemovesDataAndCache(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	layout := paths.Layout{
		ConfigHome: filepath.Join(home, ".config"),
		DataHome:   filepath.Join(home, ".local", "share"),
		CacheHome:  filepath.Join(home, ".cache"),
	}
	os.MkdirAll(layout.DataDir(), 0o755)
	os.MkdirAll(layout.CacheDir(), 0o755)
	os.MkdirAll(layout.ConfigHome, 0o755)
	os.WriteFile(filepath.Join(layout.ConfigHome, "guppy.toml"), []byte(""), 0o644)

	binary := filepath.Join(home, "guppy")
	os.WriteFile(binary, []byte("bin"), 0o755)

	res, err := Uninstall(context.Background(), UninstallOptions{
		BinaryPath: binary,
		Layout:     layout,
		HomeDir:    home,
		GOOS:       "linux",
		Purge:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{layout.DataDir(), layout.CacheDir()} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s should be purged", dir)
		}
	}
	if len(res.PurgedDirs) != 2 {
		t.Errorf("PurgedDirs = %v, want both dirs", res.PurgedDirs)
	}

	if _, err := os.Stat(layout.ConfigHome); err != nil {
		t.Error("config dir must never be purged")
	}
}

func TestUninstallUnterminatedMarkerIsWarning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	rc := filepath.Join(home, ".bashrc")
	os.WriteFile(rc, []byte(MarkerOpen+"\nexport PATH=x\n"), 0o644)

	binary := filepath.Join(home, "guppy")
	os.WriteFile(binary, []byte("bin"), 0o755)

	res, err := Uninstall(context.Background(), UninstallOptions{
		BinaryPath: binary,
		HomeDir:    home,
		GOOS:       "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "unterminated") {
		t.Errorf("want unterminated-marker warning, got %v", res.Warnings)
	}
	got, _ := os.ReadFile(rc)
	if !strings.Contains(string(got), MarkerOpen) {
		t.Error("unterminated block must be left untouched, not half-removed")
	}
}

// TestUpdateRefusesOversizedAsset pins the size cap: a hostile
// or misconfigured release server cannot make guppy buffer an unbounded
// body (MaxAssetBytes is shrunk here instead of serving 256 MiB).
func TestUpdateRefusesOversizedAsset(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "some binary content long enough to trip the cap")
	srv := serveRelease(t, "9.9.9", map[string][]byte{AssetName("9.9.9", Platform{GOOS: "linux", GOARCH: "amd64"}): tarball})

	dir := t.TempDir()
	current := filepath.Join(dir, "guppy")
	os.WriteFile(current, []byte("trusted old binary"), 0o755)

	_, err := Update(context.Background(), UpdateOptions{
		BaseURL: srv.URL, Client: srv.Client(),
		CurrentPath: current, Platform: Platform{GOOS: "linux", GOARCH: "amd64"},
		MaxAssetBytes: 64,
	})
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("want size-cap error, got %v", err)
	}
	got, _ := os.ReadFile(current)
	if string(got) != "trusted old binary" {
		t.Errorf("binary was modified despite refused download: %q", got)
	}
}

// TestStageBinaryRefusesOversizedEntry: even if a tarball passed the
// download cap, extraction enforces its own.
func TestStageBinaryRefusesOversizedEntry(t *testing.T) {
	t.Parallel()
	tarball := makeTarball(t, "guppy", "0123456789abcdef")
	dir := t.TempDir()
	if _, err := stageBinary(context.Background(), tarball, dir, Platform{GOOS: "linux"}, 8); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("want size-cap error, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("staging dir not clean after refusal: %v", entries)
	}
}
