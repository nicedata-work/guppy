// Package httpx is the single place guppy creates HTTP clients (ADR 0005).
// It applies the network policy from settings: the allowed-insecure-host
// list (§10.3, uv parity — TLS verification disabled only for hosts the
// user explicitly listed). Offline mode never constructs a client at all;
// that gate lives in the calling commands.
package httpx

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
)

// New returns an http.Client honoring the insecure-host policy. All other
// hosts get default verification and timeouts.
func New(insecureHosts []string) *http.Client {
	if len(insecureHosts) == 0 {
		return &http.Client{}
	}
	insecure := make(map[string]bool, len(insecureHosts))
	for _, h := range insecureHosts {
		insecure[h] = true
	}
	return &http.Client{Transport: &insecureAwareTransport{
		base:     http.DefaultTransport,
		insecure: insecure,
	}}
}

type insecureAwareTransport struct {
	base     http.RoundTripper
	insecure map[string]bool
}

func (t *insecureAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.insecure[req.URL.Hostname()] {
		//nolint:gosec // the user explicitly listed this host as insecure (§10.3).
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		return tr.RoundTrip(req)
	}
	return t.base.RoundTrip(req)
}

// NetworkError marks network/offline failures — the CLI's exit-code-3
// classifier (design §6). Op gives the failing operation context.
type NetworkError struct {
	Op  string
	Err error
}

func (e *NetworkError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *NetworkError) Unwrap() error { return e.Err }

// IsNetworkError reports whether err is (or wraps) a network failure.
func IsNetworkError(err error) bool {
	var ne *NetworkError
	return errors.As(err, &ne)
}

// ErrOffline is the sentinel for refused network access behind --offline
// (global flag / GUPPY_OFFLINE / settings).
var ErrOffline = errors.New("offline: network access is disabled (--offline/GUPPY_OFFLINE)")
