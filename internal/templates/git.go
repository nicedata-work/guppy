package templates

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nicedata-work/guppy/internal/httpx"
)

// GitSource is a git-backed Source (design §4.3, ADR 0004: exec git,
// never a Go git library). The ref given at add time is pinned to a
// commit SHA; Pin resolves ref → SHA, Fetch materializes the checkout at
// that SHA. `guppy template update` reconciles the checkout to the
// latest commit of the same ref — it never moves the ref itself.
type GitSource struct {
	URL string

	Ref string

	Dir string

	Offline bool

	GitBin string
}

var _ Source = (*GitSource)(nil)

// Pin resolves the ref to a commit SHA. SHA refs pass through
// unchanged; named refs resolve against the source of truth: the
// remote when online (so `template update` sees new commits on the
// ref), the existing checkout when offline (cached pin).
func (g *GitSource) Pin(ctx context.Context) (string, error) {
	if isSHA(g.Ref) {
		return strings.ToLower(g.Ref), nil
	}
	ref := g.Ref
	if ref == "" {
		ref = "HEAD"
	}
	if !g.Offline {
		out, err := g.gitOutput(ctx, "", "ls-remote", g.URL, ref)
		if err != nil {
			return "", &httpx.NetworkError{Op: "git pin", Err: err}
		}
		if sha := parseLsRemote(out, ref); sha != "" {
			return sha, nil
		}

	}
	if g.Dir != "" {
		if sha, err := g.gitOutput(ctx, g.Dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil && sha != "" {
			return sha, nil
		}
	}
	if g.Offline {
		return "", &httpx.NetworkError{Op: "git pin", Err: httpx.ErrOffline}
	}
	return "", fmt.Errorf("git: ref %q not found in %s", ref, g.URL)
}

// Fetch materializes the checkout at the pinned SHA and returns it as an
// fs.FS. Existing checkouts are reused (fetch + checkout only), so the
// offline path works from cache.
func (g *GitSource) Fetch(ctx context.Context) (fs.FS, error) {
	if g.Dir == "" {
		return nil, fmt.Errorf("git source: no checkout directory")
	}
	pin, err := g.Pin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(g.Dir, ".git")); os.IsNotExist(err) {
		if g.Offline {
			return nil, &httpx.NetworkError{Op: "git clone", Err: httpx.ErrOffline}
		}
		if err := os.MkdirAll(filepath.Dir(g.Dir), 0o755); err != nil {
			return nil, err
		}
		if _, err := g.gitOutput(ctx, "", "clone", "--no-checkout", "--quiet", g.URL, g.Dir); err != nil {
			return nil, &httpx.NetworkError{Op: "git clone", Err: err}
		}
	}
	if !g.hasCommit(ctx, pin) {
		if g.Offline {
			return nil, &httpx.NetworkError{Op: "git fetch", Err: fmt.Errorf("%w: commit %s not in checkout", httpx.ErrOffline, pin)}
		}
		if _, err := g.gitOutput(ctx, g.Dir, "fetch", "--quiet", "origin"); err != nil {
			return nil, &httpx.NetworkError{Op: "git fetch", Err: err}
		}
		if !g.hasCommit(ctx, pin) {
			return nil, fmt.Errorf("git: commit %s not found in %s", pin, g.URL)
		}
	}
	if _, err := g.gitOutput(ctx, g.Dir, "checkout", "--quiet", "--force", "--detach", pin); err != nil {
		return nil, fmt.Errorf("git checkout %s: %w", pin, err)
	}
	return os.DirFS(g.Dir), nil
}

// Identity returns the pre-fetch identity (kind git, pin set after Pin).
func (g *GitSource) Identity() Identity {
	id := Identity{Scope: ScopeUser, Kind: SourceGit, ID: g.GitDirName()}
	if isSHA(g.Ref) {
		id.Pin = strings.ToLower(g.Ref)
	}
	return id
}

// GitDirName is a stable, filesystem-safe name for the URL (used by
// callers deriving checkout dir names).
func (g *GitSource) GitDirName() string {
	s := strings.TrimSuffix(g.URL, ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "git-template"
	}
	return b.String()
}

func (g *GitSource) hasCommit(ctx context.Context, sha string) bool {
	out, err := g.gitOutput(ctx, g.Dir, "cat-file", "-e", sha+"^{commit}")
	return err == nil && out == ""
}

func (g *GitSource) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	bin := g.GitBin
	if bin == "" {
		bin = "git"
	}
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, bin, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func isSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// parseLsRemote picks the commit for ref from `git ls-remote` output,
// preferring the peeled tag object (`^{}`) when both are listed.
func parseLsRemote(out, ref string) string {
	var plain, peeled string
	for line := range strings.SplitSeq(out, "\n") {
		sha, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		switch {
		case name == ref+"^{}":
			peeled = sha
		case name == ref || strings.HasSuffix(name, "/"+ref):
			if plain == "" {
				plain = sha
			}
		}
	}
	if peeled != "" {
		return peeled
	}
	return plain
}
