// Package templates owns template identity, sources, and scope resolution
// (design §4). Embedded built-ins ship in the binary; registry and git
// sources resolve at add time.
package templates

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Scope is the trust/precedence layer a template belongs to (design §4.1).
// Resolution order is built-in → user → community, with built-ins winning
// collisions; the precedence logic lives only in Resolver implementations.
type Scope int

const (
	// ScopeBuiltin is the templates compiled into the binary (go:embed).
	ScopeBuiltin Scope = iota
	// ScopeUser is ~/.local/share/guppy/templates (user-authored or `template add`).
	ScopeUser
	// ScopeLocal is any path passed in place; loaded, never copied.
	ScopeLocal
	// ScopeCommunity is ~/.local/share/guppy/community/<id>@<version>; per-template trust gate.
	ScopeCommunity
)

// String returns the lowercase scope name used in CLI output.
func (s Scope) String() string {
	switch s {
	case ScopeBuiltin:
		return "builtin"
	case ScopeUser:
		return "user"
	case ScopeLocal:
		return "local"
	case ScopeCommunity:
		return "community"
	default:
		return fmt.Sprintf("scope(%d)", int(s))
	}
}

// SourceKind records how a template's payload bytes arrive. Every kind must
// be expressible as an fs.FS (the unifying payload abstraction).
type SourceKind int

const (
	// SourceEmbedded is compiled into the binary via go:embed.
	SourceEmbedded SourceKind = iota
	// SourceRegistry is a version-pinned registry tarball.
	SourceRegistry
	// SourceGit is a git checkout pinned to a commit SHA.
	SourceGit
	// SourceLocal is a directory on disk, loaded in place.
	SourceLocal
)

// String returns the lowercase kind name used in identity keys.
func (k SourceKind) String() string {
	switch k {
	case SourceEmbedded:
		return "embedded"
	case SourceRegistry:
		return "registry"
	case SourceGit:
		return "git"
	case SourceLocal:
		return "local"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Identity is the canonical, comparable, hashable identity of one template
// (design §4.3). One template == one Identity; Identity is the single dedup
// key across scopes.
type Identity struct {
	Scope   Scope
	Kind    SourceKind
	ID      string
	Version semver.Version

	Pin string
}

// Key returns the dedup key: the same identity encountered through two
// routes must produce the same Key. The Pin, when resolved, is the
// authoritative discriminator (registry id@version, git URL@SHA, abs path);
// before resolution, kind + id + version is the best canonical form.
func (id Identity) Key() string {
	if id.Pin != "" {
		return id.Kind.String() + ":" + id.Pin
	}
	var b strings.Builder
	b.WriteString(id.Kind.String())
	b.WriteString(":")
	b.WriteString(id.ID)
	if v := id.Version.String(); v != "" {
		b.WriteString("@")
		b.WriteString(v)
	}
	return b.String()
}

// String renders the identity for human-readable output (list/show tables).
func (id Identity) String() string {
	s := id.Scope.String() + "/" + id.ID
	if v := id.Version.String(); v != "" {
		s += "@" + v
	}
	if id.Pin != "" {
		s += " (" + id.Pin + ")"
	}
	return s
}
