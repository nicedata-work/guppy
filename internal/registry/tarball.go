package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicedata-work/guppy/internal/archive"
	"github.com/nicedata-work/guppy/internal/httpx"
)

// Fetch downloads the tarball for entry, verifies its sha256 checksum
// (always enforced), safely extracts it into the cache, and returns the
// root directory of the extracted template. The result is an immutable
// pin: entry.ID@entry.Version with verified bytes (§4.3).
//
// Extraction refuses absolute paths, `..` escapes, and symlinks/hardlinks
// (path-traversal audit — enforced
// where the untrusted bytes land).
func (c *Client) Fetch(ctx context.Context, entry Entry) (string, error) {
	if err := c.requireOnline(); err != nil {
		return "", err
	}
	if entry.SHA256 == "" {
		return "", fmt.Errorf("registry entry %s@%s has no sha256 checksum", entry.ID, entry.Version)
	}

	dest := c.extractDir(entry)
	if sum, err := os.ReadFile(dest + ".sha256"); err == nil && string(sum) == strings.ToLower(entry.SHA256) {
		if _, err := os.Stat(filepath.Join(dest, "guppy.toml")); err == nil {
			return dest, nil
		}
	}

	data, err := c.download(ctx, entry)
	if err != nil {
		return "", err
	}
	if sum := sha256hex(data); sum != strings.ToLower(entry.SHA256) {
		return "", fmt.Errorf("checksum mismatch for %s@%s: index says %s, got %s",
			entry.ID, entry.Version, entry.SHA256, sum)
	}

	staging := dest + ".staging"
	if err := os.RemoveAll(staging); err != nil {
		return "", err
	}
	files, err := archive.ReadTarGz(data)
	if err != nil {
		return "", fmt.Errorf("extract %s@%s: %w", entry.ID, entry.Version, err)
	}

	strip := 0
	if _, ok := archive.TopDir(files); ok {
		strip = 1
	}
	if err := archive.Write(files, staging, strip); err != nil {
		os.RemoveAll(staging)
		return "", fmt.Errorf("extract %s@%s: %w", entry.ID, entry.Version, err)
	}
	if _, err := os.Stat(filepath.Join(staging, "guppy.toml")); err != nil {
		os.RemoveAll(staging)
		return "", fmt.Errorf("registry tarball for %s@%s has no guppy.toml at its root", entry.ID, entry.Version)
	}
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.Rename(staging, dest); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest+".sha256", []byte(strings.ToLower(entry.SHA256)), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

func (c *Client) download(ctx context.Context, entry Entry) ([]byte, error) {
	if c.Local != nil {
		return c.downloadLocal(entry)
	}
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(entry.Tarball)
	if err != nil {
		return nil, fmt.Errorf("invalid tarball path %q: %w", entry.Tarball, err)
	}
	resolved := base.ResolveReference(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.String(), nil)
	if err != nil {
		return nil, &httpx.NetworkError{Op: "tarball fetch", Err: err}
	}
	c.authorize(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, &httpx.NetworkError{Op: "tarball fetch", Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpx.NetworkError{Op: "tarball fetch",
			Err: fmt.Errorf("%s: %s", resolved.Redacted(), resp.Status)}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, &httpx.NetworkError{Op: "tarball fetch", Err: err}
	}
	return data, nil
}

// downloadLocal reads the tarball from a git-registry checkout: the
// repo is cloned with the user's own git credentials, so tarball paths
// are repo-relative (an absolute URL in a git registry is a config
// error, not something to fetch anonymously).
func (c *Client) downloadLocal(entry Entry) ([]byte, error) {
	if strings.HasPrefix(entry.Tarball, "http://") || strings.HasPrefix(entry.Tarball, "https://") || strings.HasPrefix(entry.Tarball, "/") {
		return nil, fmt.Errorf("git registry tarball for %s@%s must be a repo-relative path, got %q", entry.ID, entry.Version, entry.Tarball)
	}
	data, err := fs.ReadFile(c.Local, entry.Tarball)
	if err != nil {
		return nil, fmt.Errorf("git registry has no tarball %q: %w", entry.Tarball, err)
	}
	return data, nil
}

// extractDir is where entry's verified tarball lives in the cache:
// <cache>/<urlhash>/<id>/<version>/ with the checksum marker beside it.
func (c *Client) extractDir(entry Entry) string {
	host := "default"
	if c.CacheKey != "" {
		host = hex.EncodeToString(sha256bytes([]byte(c.CacheKey)))[:16]
	} else if u, err := c.base(); err == nil {
		host = hex.EncodeToString(sha256bytes([]byte(u.Scheme + "://" + u.Host + u.Path)))[:16]
	}
	return filepath.Join(c.CacheDir, host, sanitize(entry.ID), sanitize(entry.Version))
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func sha256hex(data []byte) string { return hex.EncodeToString(sha256bytes(data)) }

func sha256bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
