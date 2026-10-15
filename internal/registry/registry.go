// Package registry is the static-JSON registry client (design §4, §6).
// The registry is a plain directory of static files — one
// index.json plus release tarballs — servable by any static file server;
// the client uses stdlib net/http only (ADR 0005), verifies sha256
// checksums on every tarball, and resolves everything to immutable pins
// (§4.3: registry source → id@version).
//
// Network failures are httpx.NetworkError values, which the CLI maps to
// exit code 3; offline mode is enforced here too, so no command can
// reach the network behind --offline's back.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/httpx"
	"github.com/nicedata-work/guppy/internal/version"
)

// IndexFile is the well-known index name served at the registry base URL.
const IndexFile = "index.json"

// ErrNotFound marks "this registry has no such id/version" — distinct
// from a network failure so a registry chain can skip to the next
// registry without masking a broken one.
var ErrNotFound = errors.New("not found in registry")

// CommunityKeyword marks entries published for guppy's registry in the
// search index (pi/npm parity: topic keywords). Only entries carrying it
// are visible to `guppy template search`.
const CommunityKeyword = "guppy-template"

// Entry is one version of one template in the registry index.
type Entry struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Kind        string   `json:"kind,omitempty"`
	Language    string   `json:"language,omitempty"`
	Description string   `json:"description,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`

	Tarball string `json:"tarball"`

	SHA256 string `json:"sha256"`
}

// Index is the decoded registry index.
type Index struct {
	Schema    int     `json:"schema"`
	Templates []Entry `json:"templates"`
}

// IsNetworkError reports whether err is (or wraps) a registry network
// failure — the CLI's exit-3 classifier.
func IsNetworkError(err error) bool { return httpx.IsNetworkError(err) }

// ErrOffline is the sentinel for refused network access behind --offline.
var ErrOffline = httpx.ErrOffline

// Client talks to one registry (a base URL + HTTP client + offline flag).
type Client struct {
	BaseURL string

	HTTP *http.Client

	Offline bool

	CacheDir string

	Local fs.FS

	CacheKey string

	Token string
}

// Index fetches and decodes the registry index (network-bound over
// HTTP; read straight from the checkout for git registries).
func (c *Client) Index(ctx context.Context) (Index, error) {
	if c.Local != nil {
		data, err := fs.ReadFile(c.Local, IndexFile)
		if err != nil {
			return Index{}, fmt.Errorf("git registry has no %s: %w", IndexFile, err)
		}
		return decodeIndex(bytes.NewReader(data))
	}
	if err := c.requireOnline(); err != nil {
		return Index{}, err
	}
	base, err := c.base()
	if err != nil {
		return Index{}, err
	}
	u := base.JoinPath(IndexFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Index{}, &httpx.NetworkError{Op: "registry index", Err: err}
	}
	c.authorize(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return Index{}, &httpx.NetworkError{Op: "registry index", Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Index{}, &httpx.NetworkError{Op: "registry index",
			Err: fmt.Errorf("%s: %s", u.Redacted(), resp.Status)}
	}
	idx, err := decodeIndex(resp.Body)
	if err != nil {
		return Index{}, fmt.Errorf("registry index %s: %w", u.Redacted(), err)
	}
	return idx, nil
}

// decodeIndex parses and validates an index document (schema 1).
func decodeIndex(r io.Reader) (Index, error) {
	var idx Index
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&idx); err != nil {
		return Index{}, err
	}
	if idx.Schema != 1 {
		return Index{}, fmt.Errorf("unsupported schema %d (want 1)", idx.Schema)
	}
	return idx, nil
}

// Versions returns all indexed versions of id, newest first (semver
// order; non-semver versions sort after semver ones, lexicographic
// among themselves).
func (c *Client) Versions(ctx context.Context, id string) ([]Entry, error) {
	idx, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range idx.Templates {
		if e.ID == id {
			out = append(out, e)
		}
	}

	version.SortDesc(out, func(e Entry) string { return e.Version })
	return out, nil
}

// Get returns the entry for id@version ("" = latest).
func (c *Client) Get(ctx context.Context, id, version string) (Entry, error) {
	versions, err := c.Versions(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	if len(versions) == 0 {
		return Entry{}, fmt.Errorf("template %q not found in registry: %w", id, ErrNotFound)
	}
	if version == "" {
		return versions[0], nil
	}
	for _, e := range versions {
		if e.Version == version {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("template %q has no version %s in registry (available: %s)",
		id, version, strings.Join(versionList(versions), ", "))
}

// Search filters the index on the guppy-template keyword (§6: search
// consults the registry index) with a case-insensitive substring match
// against id, description, and keywords.
func (c *Client) Search(ctx context.Context, query string) ([]Entry, error) {
	idx, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []Entry
	for _, e := range idx.Templates {
		if !slices.Contains(e.Keywords, CommunityKeyword) {
			continue
		}
		if strings.Contains(strings.ToLower(e.ID), q) ||
			strings.Contains(strings.ToLower(e.Description), q) ||
			containsSubstring(e.Keywords, q) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
		return version.Compare(b.Version, a.Version)
	})
	return out, nil
}

func (c *Client) requireOnline() error {
	if c.Offline {
		return &httpx.NetworkError{Op: "registry", Err: ErrOffline}
	}
	return nil
}

// authorize sets the bearer token on req when one is configured.
func (c *Client) authorize(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// base parses BaseURL (settings validation already checked it, but be
// defensive — this is the network edge).
func (c *Client) base() (*url.URL, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid registry URL %q: must be an absolute http(s) URL", c.BaseURL)
	}
	return u, nil
}

func containsSubstring(list []string, q string) bool {
	for _, s := range list {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

func versionList(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Version
	}
	return out
}
