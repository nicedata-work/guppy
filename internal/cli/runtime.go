// The runtime capability (styleguide T6): the
// signal-aware execution context, env lookups, the XDG layout, the
// layered settings, and the services derived from them (the template
// resolver and the installs/trust ledger). Commands receive a *runtime
// instead of the whole app, so a per-command test constructs a context
// and a layout — nothing else.
//
// The capability carries no context (review finding F1, Google Go
// style: "do not store a Context inside a struct"). Execute owns the
// invocation's signal-aware ctx and passes it explicitly to each
// command and helper that blocks; a struct field would hide a per-call
// flow value among once-per-invocation state and make cancellation
// unreviewable by reading signatures.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nicedata-work/guppy/internal/catalog"
	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/httpx"
	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/paths"
	"github.com/nicedata-work/guppy/internal/registry"
	"github.com/nicedata-work/guppy/internal/selfmanage"
	"github.com/nicedata-work/guppy/internal/settings"
	"github.com/nicedata-work/guppy/internal/templates"
	"github.com/nicedata-work/guppy/internal/version"
)

// runtime is the execution environment capability: everything a command
// resolves against but never prints. out is the diagnostics sink for
// runtime-level warnings (e.g. a corrupt ledger at resolver build).
type runtime struct {
	env      EnvFunc
	layout   paths.Layout
	settings settings.Settings

	out *output
}

// resolver builds the standard resolver: built-ins, the user-scope
// template dirs (default dir + settings [template].dirs), and the
// community scope — trust-gated on the dist records ledger (design §7:
// a record is written only by the review-and-confirm flow).
func (rt *runtime) resolver() *templates.Standard {
	dirs := append([]string{rt.layout.TemplatesDir()}, rt.settings.Template.Dirs...)
	r := templates.NewStandard(templates.Builtin(), dirs)
	r.CommunityDir = rt.layout.CommunityDir()
	records, err := rt.distStore().Load()
	if err != nil {

		rt.out.warnf("%s (community templates treated as untrusted)", err)
		records = nil
	}
	r.Trusted = func(id, version string) bool { return dist.Trusted(records, id, version) }
	r.Warn = func(format string, args ...any) {
		rt.out.warnf(format, args...)
	}
	return r
}

// distStore is the installed-template records file (provenance + the
// community trust ledger).
func (rt *runtime) distStore() dist.Store {
	return dist.Store{Path: filepath.Join(rt.layout.DataDir(), "installs.toml")}
}

// registryRef is one entry of the registry chain (design §10.3): named
// team registries in declaration order, then the default public one.
type registryRef struct {
	Name string

	URL   string
	Git   string
	Ref   string
	Trust string
}

// registryChain returns the ordered registry chain.
func (rt *runtime) registryChain() []registryRef {
	chain := make([]registryRef, 0, len(rt.settings.Template.Registries)+1)
	for _, r := range rt.settings.Template.Registries {
		chain = append(chain, registryRef{Name: r.Name, URL: r.URL, Git: r.Git, Ref: r.Ref, Trust: r.Trust})
	}

	if rt.settings.Template.Registry != "" {
		chain = append(chain, registryRef{URL: rt.settings.Template.Registry})
	}
	return chain
}

// registryClientFor builds the client for one chain entry. HTTP
// registries get the per-registry bearer token
// (GUPPY_REGISTRY_<NAME>_TOKEN); git registries are cloned with the
// user's own git credentials (SSH keys, credential managers — Azure
// DevOps and GitHub org repos just work) and read from the checkout,
// so no token configuration is needed.
func (rt *runtime) registryClientFor(ctx context.Context, r registryRef) (*registry.Client, error) {
	if r.Git != "" {
		src := &templates.GitSource{
			URL:     r.Git,
			Ref:     r.Ref,
			Dir:     filepath.Join(rt.layout.CacheDir(), "registry-git", gitDirName(r.Git)),
			Offline: rt.settings.Network.Offline,
		}
		fsys, err := src.Fetch(ctx)
		if err != nil {
			return nil, fmt.Errorf("git registry %q (%s): %w", r.Name, r.Git, err)
		}

		return &registry.Client{
			Local:    fsys,
			CacheKey: "git:" + r.Git + "@" + r.Ref,
			CacheDir: filepath.Join(rt.layout.CacheDir(), "registry"),
		}, nil
	}
	token, _ := rt.env(settings.TokenEnvName(r.Name))
	return &registry.Client{
		BaseURL:  r.URL,
		HTTP:     httpx.New(rt.settings.Network.Insecure),
		Offline:  rt.settings.Network.Offline,
		CacheDir: filepath.Join(rt.layout.CacheDir(), "registry"),
		Token:    token,
	}, nil
}

// resolveRegistry returns the entry for id@version from the registry
// chain: named registries in declaration order, then the default. The
// first registry carrying the id wins (an org can shadow a public id
// deliberately); a not-found falls through to the next registry, but a
// network failure aborts — a team registry being down must never
// silently fall back to the public one.
func (rt *runtime) resolveRegistry(ctx context.Context, name, id, version string) (registry.Entry, registryRef, error) {
	chain := rt.registryChain()
	if name != "" {
		filtered := make([]registryRef, 0, 1)
		for _, r := range chain {
			if r.Name == name {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			return registry.Entry{}, registryRef{}, InputError(fmt.Errorf("no registry named %q (configure it in [template.registries])", name))
		}
		chain = filtered
	}
	var lastErr error
	for _, r := range chain {
		client, err := rt.registryClientFor(ctx, r)
		if err != nil {
			return registry.Entry{}, registryRef{}, networkFail(err)
		}
		entry, err := client.Get(ctx, id, version)
		switch {
		case err == nil:
			return entry, r, nil
		case errors.Is(err, registry.ErrNotFound):
			lastErr = err
		default:
			return registry.Entry{}, registryRef{}, networkFail(err)
		}
	}
	return registry.Entry{}, registryRef{}, lastErr
}

// labeledEntry pairs a search hit with the registry it came from ("") =
// the default registry).
type labeledEntry struct {
	registry.Entry
	Registry string
}

// searchRegistries queries the whole chain — named registries first,
// then the default — deduplicating by id so the first registry carrying
// one wins, mirroring resolveRegistry's order.
func (rt *runtime) searchRegistries(ctx context.Context, query string) ([]labeledEntry, error) {
	var out []labeledEntry
	seen := map[string]bool{}
	for _, r := range rt.registryChain() {
		client, err := rt.registryClientFor(ctx, r)
		if err != nil {
			return nil, networkFail(err)
		}
		entries, err := client.Search(ctx, query)
		if err != nil {
			return nil, networkFail(err)
		}
		for _, e := range entries {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			out = append(out, labeledEntry{Entry: e, Registry: r.Name})
		}
	}
	return out, nil
}

// recordProject upserts a project into the global index — any command
// that "resolves my projects" also refreshes them (design §9), so the
// index is useful even without the shellhook.
func (rt *runtime) recordProject(root string) {
	idx := &catalog.FileIndex{Path: rt.layout.IndexFile()}
	e, err := catalog.Record(root, rt.out.clock().UTC())
	if err != nil {
		return
	}
	_ = idx.Upsert(e)
}

// communityRoot is the installed dir of the newest trusted community
// copy of id ("" when none).
func (rt *runtime) communityRoot(id string) string {
	entries, err := os.ReadDir(rt.layout.CommunityDir())
	if err != nil {
		return ""
	}
	var best, bestVer string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cid, ver := dist.ParseCommunityDirName(e.Name())
		if cid != id {
			continue
		}
		if best == "" || version.Compare(bestVer, ver) < 0 {
			best = filepath.Join(rt.layout.CommunityDir(), e.Name())
			bestVer = ver
		}
	}
	return best
}

// releaseBase returns the release base URL for guppy update: the
// GUPPY_RELEASE_BASE env override (installer parity, used by tests and
// mirrors) or the canonical default.
func (rt *runtime) releaseBase() string {
	if v, ok := rt.env("GUPPY_RELEASE_BASE"); ok && v != "" {
		return v
	}
	return selfmanage.DefaultReleaseBase
}

// loadedTemplate is a fully resolved template plus its authoring root
// (the directory containing guppy.toml, tests.toml, and testdata/ — used
// by `guppy test` for snapshots).
type loadedTemplate struct {
	t templates.Template

	rootDir string
}

// rootFS returns the authoring root (works for embedded and disk alike).
func (l loadedTemplate) rootFS() fs.FS {
	if l.rootDir != "" {
		return os.DirFS(l.rootDir)
	}
	return templates.BuiltinTemplateFS(l.t.Identity.ID)
}

// loadTemplate resolves spec — "" means ".", a directory is a local
// template, anything else is a template id. Schema-level error issues
// (§3.2) are returned so `guppy lint` can report them; only a missing or
// unparseable manifest is a hard error.
func (rt *runtime) loadTemplate(ctx context.Context, spec string) (loadedTemplate, []manifest.Issue, error) {
	if spec == "" {
		spec = "."
	}
	if st, err := os.Stat(spec); err == nil && st.IsDir() {
		abs, err := filepath.Abs(spec)
		if err != nil {
			return loadedTemplate{}, nil, err
		}
		t, issues, err := templates.Load(os.DirFS(abs), templates.LoadOptions{
			Scope: templates.ScopeLocal, Kind: templates.SourceLocal, Pin: abs,
		})
		if err != nil {
			return loadedTemplate{}, nil, err
		}
		chain, err := templates.ResolveChain(ctx, rt.resolver(), t)
		if err != nil {
			return loadedTemplate{}, nil, err
		}
		return loadedTemplate{t: chain, rootDir: abs}, issues, nil
	}

	t, err := rt.resolver().Resolve(ctx, spec)
	if err != nil {
		return loadedTemplate{}, nil, err
	}
	chain, err := templates.ResolveChain(ctx, rt.resolver(), t)
	if err != nil {
		return loadedTemplate{}, nil, err
	}
	lt := loadedTemplate{t: chain}
	switch chain.Identity.Scope {
	case templates.ScopeUser:
		for _, dir := range rt.resolver().UserDirs {
			if st, err := os.Stat(filepath.Join(dir, chain.Manifest.ID, "guppy.toml")); err == nil && !st.IsDir() {
				lt.rootDir = filepath.Join(dir, chain.Manifest.ID)
				break
			}
		}
	case templates.ScopeCommunity:
		if root := rt.communityRoot(chain.Manifest.ID); root != "" {
			lt.rootDir = root
		}
	}
	return lt, nil, nil
}

// isDir reports whether path is an existing directory.
func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
