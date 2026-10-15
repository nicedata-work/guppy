// The standard Resolver implementation (design §4.2): scope order
// built-in → user → community, first match wins, built-ins always win
// collisions so a stale or malicious user/community template cannot
// shadow a first-party one — collisions warn, the winner is
// deterministic. This is the only place scope precedence is defined
// (§4.3).
//
// Community scope is trust-gated (§4.1, §7): the Trusted callback says
// whether an installed community id@version has passed review (the
// dist records file is the ledger); an untrusted template is refused
// with the review hint, never silently loaded. Local templates are not
// resolved here at all — a path passed in place is loaded directly by
// the caller (ScopeLocal, "loaded, never copied").
package templates

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/version"
)

// Standard is the production Resolver. Builtin is the embedded library
// root (see Builtin()); UserDirs are directories containing one template
// directory per id (the default user scope dir plus any extra
// [template].dirs from settings — searched in order, later dirs shadow
// earlier user dirs but never a built-in). CommunityDir holds
// <id>@<version> subdirs (design §4.1); Trusted gates community
// templates on review (nil trusts everything — tests and user-scope
// resolution don't need the gate).
type Standard struct {
	Builtin      fs.FS
	UserDirs     []string
	CommunityDir string

	Trusted func(id, version string) bool

	Warn func(format string, args ...any)
}

// NewStandard builds the resolver for the given layout.
func NewStandard(builtin fs.FS, userDirs []string) *Standard {
	var dirs []string
	for _, d := range userDirs {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return &Standard{Builtin: builtin, UserDirs: dirs}
}

func (s *Standard) warnf(format string, args ...any) {
	if s.Warn != nil {
		s.Warn(format, args...)
	}
}

// Resolve implements Resolver: find id in scope order, load it raw, and
// refuse templates with schema-level errors (§3.2). Absence is an error
// listing candidates; an untrusted community copy is an error with the
// review hint even when a shadowed-by situation exists.
func (s *Standard) Resolve(ctx context.Context, id string) (Template, error) {
	if t, issues, err := s.resolveBuiltin(id); err == nil {
		if manifest.HasErrors(issues) {
			return Template{}, fmt.Errorf("built-in template %q is invalid: %s", id, firstError(issues))
		}
		s.warnShadowed(id, ScopeBuiltin)
		return t, nil
	} else if !isNotFound(err) {
		return Template{}, err
	}
	for _, dir := range s.UserDirs {
		if t, issues, err := s.resolveDir(dir, id); err == nil {
			if manifest.HasErrors(issues) {
				return Template{}, fmt.Errorf("template %q in %s is invalid: %s", id, dir, firstError(issues))
			}
			s.warnShadowed(id, ScopeUser)
			return t, nil
		} else if !isNotFound(err) {
			return Template{}, err
		}
	}
	if s.CommunityDir != "" {
		t, err := s.resolveCommunity(id)
		if err == nil {
			return t, nil
		} else if !isNotFound(err) {
			return Template{}, err
		}
	}

	candidates, _ := s.List(ctx, true)
	var ids []string
	for _, c := range candidates {
		ids = append(ids, c.Identity.String())
	}
	return Template{}, fmt.Errorf("unknown template %q (available: %s)", id, strings.Join(ids, ", "))
}

// List implements Resolver: every resolvable template, built-ins first.
// allScopes=false restricts to the non-gated scopes (built-in + user);
// allScopes=true adds the trusted community scope — untrusted community
// copies never appear (they are not resolvable).
func (s *Standard) List(_ context.Context, allScopes bool) ([]Template, error) {
	var out []Template
	ids, err := BuiltinIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if t, issues, err := s.resolveBuiltin(id); err == nil && !manifest.HasErrors(issues) {
			out = append(out, t)
		}
	}
	for _, dir := range s.UserDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if t, issues, err := s.resolveDir(dir, e.Name()); err == nil && !manifest.HasErrors(issues) {
				out = append(out, t)
			}
		}
	}
	if allScopes && s.CommunityDir != "" {
		out = append(out, s.listCommunity()...)
	}
	slices.SortFunc(out, func(a, b Template) int { return strings.Compare(a.Manifest.ID, b.Manifest.ID) })
	return out, nil
}

func (s *Standard) resolveBuiltin(id string) (Template, []manifest.Issue, error) {
	fsys := BuiltinTemplateFS(id)
	if fsys == nil {
		return Template{}, nil, errNotFound{id: id}
	}
	t, issues, err := Load(fsys, LoadOptions{Scope: ScopeBuiltin, Kind: SourceEmbedded})
	if err != nil {
		return Template{}, nil, err
	}
	if t.Manifest.ID != id {
		return Template{}, nil, errNotFound{id: id}
	}
	return t, issues, nil
}

func (s *Standard) resolveDir(dir, id string) (Template, []manifest.Issue, error) {
	root := filepath.Join(dir, id)
	abs, err := filepath.Abs(root)
	if err != nil {
		return Template{}, nil, err
	}
	if st, err := os.Stat(filepath.Join(abs, "guppy.toml")); err != nil || st.IsDir() {
		return Template{}, nil, errNotFound{id: id}
	}
	t, issues, err := Load(os.DirFS(abs), LoadOptions{Scope: ScopeUser, Kind: SourceLocal, Pin: abs})
	if err != nil {
		return Template{}, nil, err
	}
	if t.Manifest.ID != id {
		return Template{}, nil, errNotFound{id: id}
	}
	return t, issues, nil
}

// communityCopy is one installed community template directory.
type communityCopy struct {
	dir     string
	version string
}

// communityCopies lists <id>@<version> dirs for id, newest version first.
func (s *Standard) communityCopies(id string) []communityCopy {
	entries, err := os.ReadDir(s.CommunityDir)
	if err != nil {
		return nil
	}
	var out []communityCopy
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cid, ver := parseCommunityName(e.Name())
		if cid != id {
			continue
		}
		out = append(out, communityCopy{dir: filepath.Join(s.CommunityDir, e.Name()), version: ver})
	}

	version.SortDesc(out, func(c communityCopy) string { return c.version })
	return out
}

// resolveCommunity loads the newest trusted installed copy of id.
func (s *Standard) resolveCommunity(id string) (Template, error) {
	copies := s.communityCopies(id)
	var untrusted []string
	for _, c := range copies {
		if s.Trusted != nil && !s.Trusted(id, c.version) {
			untrusted = append(untrusted, c.version)
			continue
		}
		abs, err := filepath.Abs(c.dir)
		if err != nil {
			return Template{}, err
		}
		t, issues, err := Load(os.DirFS(abs), LoadOptions{Scope: ScopeCommunity, Kind: SourceLocal, Pin: abs})
		if err != nil {
			return Template{}, err
		}
		if t.Manifest.ID != id {
			continue
		}
		if manifest.HasErrors(issues) {
			return Template{}, fmt.Errorf("community template %s@%s is invalid: %s", id, c.version, firstError(issues))
		}
		s.warnShadowed(id, ScopeCommunity)
		return t, nil
	}
	if len(untrusted) > 0 {
		return Template{}, &UntrustedError{ID: id, Versions: untrusted}
	}
	return Template{}, errNotFound{id: id}
}

// listCommunity returns every trusted community template (newest copy
// per id).
func (s *Standard) listCommunity() []Template {
	entries, err := os.ReadDir(s.CommunityDir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Template

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, _ := parseCommunityName(e.Name())
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		for _, c := range s.communityCopies(id) {
			if s.Trusted != nil && !s.Trusted(id, c.version) {
				continue
			}
			abs, err := filepath.Abs(c.dir)
			if err != nil {
				continue
			}
			t, issues, err := Load(os.DirFS(abs), LoadOptions{Scope: ScopeCommunity, Kind: SourceLocal, Pin: abs})
			if err != nil || t.Manifest.ID != id || manifest.HasErrors(issues) {
				continue
			}
			out = append(out, t)
			break
		}
	}
	return out
}

// warnShadowed emits the §4.2 collision warning when id also exists in
// a scope below winner (built-ins win for free — they're checked first).
func (s *Standard) warnShadowed(id string, winner Scope) {
	for _, below := range scopesBelow(winner) {
		switch below {
		case ScopeUser:
			for _, dir := range s.UserDirs {
				if _, err := os.Stat(filepath.Join(dir, id, "guppy.toml")); err == nil {
					s.warnf("template %q: user copy in %s is shadowed by the %s template (higher scope wins)", id, dir, winner)
					return
				}
			}
		case ScopeCommunity:
			if len(s.communityCopies(id)) > 0 {
				s.warnf("template %q: community copy is shadowed by the %s template (higher scope wins)", id, winner)
			}
		}
	}
}

// scopesBelow returns the scopes lower precedence than scope.
func scopesBelow(scope Scope) []Scope {
	switch scope {
	case ScopeBuiltin:
		return []Scope{ScopeUser, ScopeCommunity}
	case ScopeUser:
		return []Scope{ScopeCommunity}
	default:
		return nil
	}
}

// parseCommunityName splits "<id>@<version>"; no "@" means no version.
func parseCommunityName(name string) (id, version string) {
	if i := strings.LastIndex(name, "@"); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

// UntrustedError says a community template exists but has not passed
// the review gate (design §7). It is an input error at the CLI layer
// (exit 2) with the exact command that runs the gate.
type UntrustedError struct {
	ID       string
	Versions []string
}

func (e *UntrustedError) Error() string {
	return fmt.Sprintf("community template %s@%s is not trusted: review it before first use (guppy template add community:%s), or remove it (guppy template remove community:%s)",
		e.ID, strings.Join(e.Versions, ", "), e.ID, e.ID)
}

// errNotFound distinguishes "no such template here" (try the next scope)
// from real failures.
type errNotFound struct{ id string }

func (e errNotFound) Error() string { return "template not found: " + e.id }

func isNotFound(err error) bool {
	_, ok := err.(errNotFound)
	return ok
}

func firstError(issues []manifest.Issue) string {
	for _, i := range issues {
		if i.Level == manifest.LevelError {
			return i.Message
		}
	}
	return ""
}
