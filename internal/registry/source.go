package registry

import (
	"context"
	"io/fs"
	"os"

	"github.com/nicedata-work/guppy/internal/templates"
)

// RegistrySource adapts a registry Entry to the templates.Source
// contract: Fetch returns the verified extracted payload as
// an fs.FS, Pin is the immutable id@version (§4.3).
type RegistrySource struct {
	Client *Client
	Entry  Entry
}

var _ templates.Source = (*RegistrySource)(nil)

// Fetch verifies + extracts the tarball (cached) and returns the
// template root as an fs.FS.
func (s *RegistrySource) Fetch(ctx context.Context) (fs.FS, error) {
	dir, err := s.Client.Fetch(ctx, s.Entry)
	if err != nil {
		return nil, err
	}
	return os.DirFS(dir), nil
}

// Pin is the registry version — immutable by construction.
func (s *RegistrySource) Pin(context.Context) (string, error) { return s.Entry.Version, nil }

// Identity is the pre-fetch identity: registry kind, id, version.
func (s *RegistrySource) Identity() templates.Identity {
	return templates.Identity{
		Scope: templates.ScopeUser,
		Kind:  templates.SourceRegistry,
		ID:    s.Entry.ID,
		Pin:   s.Entry.Version,
	}
}
