// Template source specs and the install machinery shared by
// `guppy template add/remove/update/clone` (design §4, §6, §7).
//
// Spec grammar (one grammar for every verb — the `community:` prefix is
// a scope qualifier, not a flag):
//
//	<id>                        registry:<id>@latest        → user scope
//	registry:<id>[@<version>]                              → user scope
//	git:<url>[@<ref>]                                     → user scope
//	./path | /abs/path | ~path                            → user scope (copied)
//	community:<anything above>                            → community scope (trust-gated)
//
// User-scope installs are full trust (you asked for them); community
// installs pass the §7 review gate. Identity rules (§4.3): registry →
// id@version (checksum-verified), git → URL@commit-SHA, local → the
// copied bytes with the manifest's version.
package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/httpx"
	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/templates"
	"github.com/nicedata-work/guppy/internal/version"
)

// templateSpec is a parsed template source argument.
type templateSpec struct {
	Community bool

	Kind dist.SourceKind

	ID      string
	Version string

	RegistryName string

	URL string
	Ref string

	Path string
}

// String renders the original spec shape (for records and output).
func (s templateSpec) String() string {
	var b strings.Builder
	if s.Community {
		b.WriteString("community:")
	}
	switch s.Kind {
	case dist.KindRegistry:
		b.WriteString("registry:")
		if s.RegistryName != "" {
			b.WriteString(s.RegistryName + ":")
		}
		b.WriteString(s.ID)
		if s.Version != "" {
			b.WriteString("@" + s.Version)
		}
	case dist.KindGit:
		b.WriteString("git:")
		b.WriteString(s.URL)
		if s.Ref != "" {
			b.WriteString("@" + s.Ref)
		}
	case dist.KindLocal:
		b.WriteString(s.Path)
	}
	return b.String()
}

// scope is the install scope name for records (dist.Scope*).
func (s templateSpec) scope() dist.ScopeName {
	if s.Community {
		return dist.ScopeCommunity
	}
	return dist.ScopeUser
}

// parseTemplateSpec parses an add/update source spec (grammar above).
func parseTemplateSpec(raw string) (templateSpec, error) {
	spec := templateSpec{}
	rest := strings.TrimSpace(raw)
	if rest == "" {
		return spec, fmt.Errorf("empty template source")
	}
	if after, ok := strings.CutPrefix(rest, "community:"); ok {
		spec.Community = true
		rest = strings.TrimSpace(after)
		if rest == "" {
			return spec, fmt.Errorf("community: prefix needs a source (e.g. community:acme-widget)")
		}
	}

	switch {
	case strings.HasPrefix(rest, "registry:"):
		rest = strings.TrimPrefix(rest, "registry:")

		if name, after, found := strings.Cut(rest, ":"); found {
			spec.RegistryName, rest = name, after
		}
		id, version, err := parseRegistryRef(rest)
		if err != nil {
			return spec, err
		}
		spec.Kind, spec.ID, spec.Version = dist.KindRegistry, id, version
	case strings.HasPrefix(rest, "git:"):
		url, ref := splitGitRef(strings.TrimPrefix(rest, "git:"))
		if url == "" {
			return spec, fmt.Errorf("git: source needs a URL (e.g. git:https://example.com/repo.git@main)")
		}
		spec.Kind, spec.URL, spec.Ref = dist.KindGit, url, ref
	case looksLikePath(rest):
		abs, err := filepath.Abs(expandHome(rest))
		if err != nil {
			return spec, err
		}
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return spec, fmt.Errorf("local source %s: not a directory", rest)
		}
		spec.Kind, spec.Path = dist.KindLocal, abs
	default:
		if strings.ContainsAny(rest, "/\\:") {
			return spec, fmt.Errorf("unrecognized template source %q (want <id>, registry:<id>[@<ver>], git:<url>[@<ref>], ./path, or community:<source>)", raw)
		}
		id, version, err := parseRegistryRef(rest)
		if err != nil {
			return spec, err
		}
		spec.Kind, spec.ID, spec.Version = dist.KindRegistry, id, version
	}
	return spec, nil
}

// parseScopeID parses an installed-template reference: `<id>` or
// `community:<id>` (remove/update/clone/show targets).
func parseScopeID(raw string) (community bool, id string, err error) {
	rest := strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(rest, "community:"); ok {
		community = true
		rest = strings.TrimSpace(after)
	}
	if rest == "" || strings.ContainsAny(rest, "/\\:@") {
		return false, "", fmt.Errorf("invalid template reference %q (want <id> or community:<id>)", raw)
	}
	return community, rest, nil
}

func parseRegistryRef(s string) (id, version string, err error) {
	id, version, _ = strings.Cut(s, "@")
	if id == "" {
		return "", "", fmt.Errorf("registry source needs a template id")
	}
	if strings.ContainsAny(id, "/\\") {
		return "", "", fmt.Errorf("invalid template id %q", id)
	}
	return id, version, nil
}

// splitGitRef splits "url[@ref]" on the last @ of the final path segment
// (userinfo @s like git@host: are never the separator).
func splitGitRef(s string) (url, ref string) {
	seg := s
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		if j := strings.LastIndex(seg[i:], "@"); j >= 0 {
			return seg[:i+j], seg[i+j+1:]
		}
		return s, ""
	}
	if i := strings.LastIndex(s, "@"); i >= 0 && !strings.Contains(s[:i], ":") {

		if strings.Contains(s, "://") {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func looksLikePath(s string) bool {
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") {
		return true
	}
	st, err := os.Stat(s)
	return err == nil && st.IsDir()
}

func expandHome(s string) string {
	if !strings.HasPrefix(s, "~"+string(filepath.Separator)) && s != "~" {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	return filepath.Join(home, strings.TrimPrefix(s, "~"))
}

// networkFail maps registry/network errors to exit code 3.
func networkFail(err error) error {
	if httpx.IsNetworkError(err) {
		return NetworkError(err)
	}
	return err
}

// fetchSource materializes the spec's bytes as an fs.FS plus the
// resolved version, immutable pin (§4.3), and whether the source is
// full-trust (a trust = "full" named registry — the §7 review gate is
// org policy, not per-user). The returned fs.FS stays valid after the
// call (registry cache / git checkout / os.DirFS).
func (rt *runtime) fetchSource(ctx context.Context, spec templateSpec) (fs.FS, string, string, bool, error) {
	switch spec.Kind {
	case dist.KindRegistry:
		entry, reg, err := rt.resolveRegistry(ctx, spec.RegistryName, spec.ID, spec.Version)
		if err != nil {
			return nil, "", "", false, networkFail(err)
		}
		client, err := rt.registryClientFor(ctx, reg)
		if err != nil {
			return nil, "", "", false, networkFail(err)
		}
		dir, err := client.Fetch(ctx, entry)
		if err != nil {
			return nil, "", "", false, networkFail(err)
		}
		return os.DirFS(dir), entry.Version, entry.Version, reg.Trust == "full", nil
	case dist.KindGit:
		src := &templates.GitSource{
			URL:     spec.URL,
			Ref:     spec.Ref,
			Dir:     filepath.Join(rt.layout.CacheDir(), "git", gitDirName(spec.URL)),
			Offline: rt.settings.Network.Offline,
		}
		pin, err := src.Pin(ctx)
		if err != nil {
			return nil, "", "", false, networkFail(err)
		}
		fsys, err := src.Fetch(ctx)
		if err != nil {
			return nil, "", "", false, networkFail(err)
		}

		m, _, err := readManifest(fsys)
		if err != nil {
			return nil, "", "", false, err
		}
		if !spec.Community {
			return fsys, m.Version, pin, false, nil
		}
		if err := requireSemver(m.Version); err != nil {
			return nil, "", "", false, fmt.Errorf("community template needs a semver version in guppy.toml: %w", err)
		}
		return fsys, m.Version, pin, false, nil
	default:
		fsys := os.DirFS(spec.Path)
		m, _, err := readManifest(fsys)
		if err != nil {
			return nil, "", "", false, err
		}
		return fsys, m.Version, spec.Path, false, nil
	}
}

// validateSource checks the fetched template against the spec: the
// manifest must parse, carry its id, and (for registry sources) agree
// with the index about id and version — a mismatched registry is an
// integrity failure, not something to paper over.
func validateSource(spec templateSpec, m manifest.Manifest) error {
	if m.ID == "" {
		return fmt.Errorf("template manifest has no id")
	}
	switch spec.Kind {
	case dist.KindRegistry:
		if m.ID != spec.ID {
			return fmt.Errorf("registry index says %q but the tarball manifest says %q", spec.ID, m.ID)
		}
		if spec.Version != "" && m.Version != spec.Version {
			return fmt.Errorf("registry index says %s@%s but the tarball manifest says version %s", spec.ID, spec.Version, m.Version)
		}
	case dist.KindLocal:
		if spec.Community && m.Version == "" {
			return fmt.Errorf("community template needs a semver version in guppy.toml")
		}
	}
	return nil
}

func readManifest(fsys fs.FS) (manifest.Manifest, []string, error) {
	data, err := fs.ReadFile(fsys, "guppy.toml")
	if err != nil {
		return manifest.Manifest{}, nil, fmt.Errorf("not a template: no guppy.toml (%w)", err)
	}
	return manifest.Parse(data)
}

// installDir computes the destination directory for an install.
func (rt *runtime) installDir(spec templateSpec, id, version string) string {
	if spec.Community {
		return filepath.Join(rt.layout.CommunityDir(), dist.CommunityDirName(id, version))
	}
	return filepath.Join(rt.layout.TemplatesDir(), id)
}

// commitInstall publishes fsys as the installed copy of id and records
// provenance (the community record doubles as the trust ledger).
func (rt *runtime) commitInstall(spec templateSpec, id, version, pin string, fsys fs.FS) (string, error) {
	dest := rt.installDir(spec, id, version)
	if err := dist.InstallTree(fsys, dest, dist.InstallOptions{AllowExisting: spec.Community}); err != nil {
		return "", err
	}
	if spec.Community {

		if err := rt.removeOtherVersions(id, version); err != nil {
			return "", err
		}
	}
	store := rt.distStore()
	records, err := store.Load()
	if err != nil {
		return "", err
	}

	source := spec.String()
	if spec.Kind == dist.KindRegistry && spec.Version == "" {
		source = "registry:" + id + "@" + version
	}
	records = dist.Upsert(records, dist.Record{
		ID:      id,
		Scope:   spec.scope(),
		Kind:    spec.Kind,
		Source:  source,
		Version: version,
		Pin:     pin,
	})
	if err := store.Save(records); err != nil {
		return "", err
	}
	return dest, nil
}

// removeOtherVersions deletes every installed copy of id in the
// community dir except keepVersion.
func (rt *runtime) removeOtherVersions(id, keepVersion string) error {
	entries, err := os.ReadDir(rt.layout.CommunityDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cid, ver := dist.ParseCommunityDirName(e.Name())
		if cid == id && ver != keepVersion {
			if err := os.RemoveAll(filepath.Join(rt.layout.CommunityDir(), e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// newestCommunityCopy returns the newest installed community dir for id
// (any trust state — used for upgrade diffs and remove).
func (rt *runtime) newestCommunityCopy(id string) (dir, newest string) {
	entries, err := os.ReadDir(rt.layout.CommunityDir())
	if err != nil {
		return "", ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cid, ver := dist.ParseCommunityDirName(e.Name())
		if cid != id {
			continue
		}
		if newest == "" || version.Compare(newest, ver) < 0 {
			dir, newest = filepath.Join(rt.layout.CommunityDir(), e.Name()), ver
		}
	}
	return dir, newest
}

// requireSemver rejects versions that are not valid semver.
func requireSemver(v string) error {
	if v == "" {
		return fmt.Errorf("empty version")
	}
	if _, err := semver.StrictNewVersion(v); err != nil {
		return fmt.Errorf("%q is not a valid version: %w", v, err)
	}
	return nil
}

func gitDirName(url string) string {
	return (&templates.GitSource{URL: url}).GitDirName()
}
