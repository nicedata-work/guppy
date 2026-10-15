package templates

import (
	"context"
	"io/fs"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// Template is a manifest + payload + identity: the unit Resolver hands to
// the renderer. Payload is an fs.FS — never a tree of []byte.
type Template struct {
	Identity Identity
	Manifest manifest.Manifest

	Chain   []Identity
	Payload fs.FS
}

// Resolver owns scope precedence (§4.2) — the only place in the codebase
// where scope ordering is defined. Collisions between scopes are warnings
// with a deterministic winner (built-in wins), never silent duplication.
type Resolver interface {
	Resolve(ctx context.Context, id string) (Template, error)

	List(ctx context.Context, allScopes bool) ([]Template, error)
}
