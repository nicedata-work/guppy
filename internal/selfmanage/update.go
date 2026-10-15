// Package selfmanage implements `guppy update` and `guppy uninstall`
// (design §10.1). The atomic-replace primitive is hand-rolled
// (ADR 0006): verify the checksum before touching anything, keep the old
// binary as a backup until the new one is in place, restore on failure.
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
	"os"
	"path/filepath"
	"strings"
)

// Error classes, so the CLI can map failures to exit codes (§6): artifact
// failures are network/registry problems (exit 3), swap failures are local
// operation failures (exit 1).
var (
	// ErrArtifact marks download/checksum/unpack failures (remote side).
	ErrArtifact = errors.New("release artifact failure")
	// ErrSwap marks the local binary-replace failure.
	ErrSwap = errors.New("binary swap failure")
)

// defaultMaxAssetBytes caps every download and staged extraction (security
// pass: a hostile or misconfigured release server cannot make
// guppy fill the disk). A static guppy binary is ~15 MiB; 256 MiB is
// orders of headroom. The zero value of UpdateOptions.MaxAssetBytes
// selects it (styleguide T10/T11: no package-global test seam).
const defaultMaxAssetBytes int64 = 256 << 20

// DefaultReleaseBase is where release artifacts live: latest.txt plus
// per-platform tarballs with detached checksums (see scripts/release.sh).
const DefaultReleaseBase = "https://github.com/nicedata-work/guppy/releases/latest/download"

// UpdateOptions parameterizes Update for the CLI and for tests.
type UpdateOptions struct {
	BaseURL string

	Version string

	Client *http.Client

	CurrentPath string

	Platform Platform

	Progress io.Writer

	MaxAssetBytes int64
}

// UpdateResult reports what the update did.
type UpdateResult struct {
	Version string
	Path    string
}

// Platform identifies a release asset's platform. A value type so the
// pair travels together — "GOOS alone" is not an asset name (styleguide
// M-T5).
type Platform struct {
	GOOS   string
	GOARCH string
}

// Current is the platform of the running binary.
func Current() Platform {
	return Platform{GOOS: hostGOOS(), GOARCH: hostGOARCH()}
}

// AssetName returns the release asset file name for a version/platform.
// Must stay in lockstep with scripts/release.sh.
func AssetName(version string, p Platform) string {
	return fmt.Sprintf("guppy-%s-%s-%s.tar.gz", version, p.GOOS, p.GOARCH)
}

// Update downloads the release tarball, verifies its sha256 against the
// detached checksum file, and swaps the current binary in place. Any error
// before the swap leaves the current binary untouched.
func Update(ctx context.Context, opts UpdateOptions) (UpdateResult, error) {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = DefaultReleaseBase
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{}
	}
	progress := opts.Progress
	if progress == nil {
		progress = io.Discard
	}
	capBytes := opts.MaxAssetBytes
	if capBytes <= 0 {
		capBytes = defaultMaxAssetBytes
	}

	current := opts.CurrentPath
	if current == "" {
		var err error
		current, err = os.Executable()
		if err != nil {
			return UpdateResult{}, fmt.Errorf("locate current executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			current = resolved
		}
	}

	plat := opts.Platform
	if plat == (Platform{}) {
		plat = Current()
	}

	version := strings.TrimSpace(opts.Version)
	if version == "" {
		raw, err := fetchSize(ctx, client, base+"/latest.txt", capBytes)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("resolve latest version: %w: %w", ErrArtifact, err)
		}
		version = strings.TrimSpace(string(raw))
		if version == "" {
			return UpdateResult{}, fmt.Errorf("latest.txt at %s is empty", base)
		}
		_, _ = fmt.Fprintf(progress, "latest version: %s\n", version)
	}

	name := AssetName(version, plat)
	tarballURL := base + "/" + name
	sumURL := tarballURL + ".sha256"

	wantSum, err := fetchChecksum(ctx, client, sumURL)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("fetch checksum: %w: %w", ErrArtifact, err)
	}

	_, _ = fmt.Fprintf(progress, "downloading %s\n", tarballURL)
	tarball, err := fetchSize(ctx, client, tarballURL, capBytes)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("download release: %w: %w", ErrArtifact, err)
	}
	gotSum := sha256.Sum256(tarball)
	if hex.EncodeToString(gotSum[:]) != wantSum {
		return UpdateResult{}, fmt.Errorf("checksum mismatch for %s: got %s, want %s: %w", name, hex.EncodeToString(gotSum[:]), wantSum, ErrArtifact)
	}

	dir := filepath.Dir(current)
	staged, err := stageBinary(ctx, tarball, dir, plat, capBytes)
	if err != nil {
		if ctx.Err() != nil {

			return UpdateResult{}, fmt.Errorf("update interrupted (current binary untouched): %w", err)
		}
		return UpdateResult{}, fmt.Errorf("unpack release: %w: %w", ErrArtifact, err)
	}
	defer func() { _ = os.Remove(staged) }()

	if err := ctx.Err(); err != nil {
		return UpdateResult{}, fmt.Errorf("update interrupted (current binary untouched): %w", err)
	}

	if err := swap(current, staged); err != nil {
		return UpdateResult{}, fmt.Errorf("swap binary: %w: %w", ErrSwap, err)
	}
	_, _ = fmt.Fprintf(progress, "guppy %s installed at %s\n", version, current)
	return UpdateResult{Version: version, Path: current}, nil
}

// swap replaces current with staged: current → backup, staged → current,
// unlink backup. On failure the backup is restored, so the binary path is
// valid before and after every step (ADR 0006; works on Windows too,
// where renaming over a running exe is not allowed).
func swap(current, staged string) error {
	backup := current + ".bak"
	_ = os.Remove(backup)

	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("move current binary aside: %w", err)
	}
	if err := os.Rename(staged, current); err != nil {

		if rerr := os.Rename(backup, current); rerr != nil {
			return fmt.Errorf("install new binary: %v (and restore failed: %v; backup at %s)", err, rerr, backup)
		}
		return err
	}
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {

		return nil
	}
	return nil
}

// stageBinary extracts the guppy binary from the tarball into dir under a
// temp name and returns its path. capBytes bounds the extraction (the
// download cap already passed; this is defense in depth).
func stageBinary(ctx context.Context, tarball []byte, dir string, plat Platform, capBytes int64) (string, error) {
	want := "guppy"
	if plat.GOOS == "windows" {
		want = "guppy.exe"
	}
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()

	out, err := os.CreateTemp(dir, ".guppy-new-*")
	if err != nil {
		return "", err
	}
	name := out.Name()

	tr := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			_ = out.Close()
			_ = os.Remove(name)
			return "", err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			_ = out.Close()
			_ = os.Remove(name)
			return "", fmt.Errorf("tarball has no %s entry", want)
		}
		if err != nil {
			_ = out.Close()
			_ = os.Remove(name)
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != want {
			continue
		}
		if hdr.Size > capBytes {
			_ = out.Close()
			_ = os.Remove(name)
			return "", fmt.Errorf("tarball entry %s is %d bytes — over the %d byte cap", want, hdr.Size, capBytes)
		}
		n, err := io.Copy(out, tr)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(name)
			return "", err
		}
		if n > capBytes {
			_ = out.Close()
			_ = os.Remove(name)
			return "", fmt.Errorf("tarball entry %s is over the %d byte cap", want, capBytes)
		}
		if err := out.Sync(); err != nil {
			_ = out.Close()
			_ = os.Remove(name)
			return "", err
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(name)
			return "", err
		}
		mode := os.FileMode(0o755)
		if hdr.Mode != 0 {
			mode = os.FileMode(hdr.Mode) & 0o777
		}
		if err := os.Chmod(name, mode); err != nil {
			_ = os.Remove(name)
			return "", err
		}
		return name, nil
	}
}

// fetchChecksum parses a "<hex>  <filename>" sha256sum-style file.
func fetchChecksum(ctx context.Context, client *http.Client, url string) (string, error) {
	raw, err := fetchSize(ctx, client, url, 1<<20)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return "", fmt.Errorf("checksum file %s is empty", url)
	}
	sum := strings.ToLower(fields[0])
	if len(sum) != sha256.Size*2 {
		return "", fmt.Errorf("checksum file %s: %q is not a sha256 hex digest", url, fields[0])
	}
	return sum, nil
}

// fetchSize GETs url with a hard response-size cap (security pass: never
// buffer an unbounded remote body).
func fetchSize(ctx context.Context, client *http.Client, url string, cap int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, clientErr := client.Do(req)
	if clientErr != nil {
		return nil, clientErr
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, cap+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > cap {
		return nil, fmt.Errorf("GET %s: response exceeds the %d byte cap", url, cap)
	}
	return raw, nil
}
