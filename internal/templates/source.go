package templates

import (
	"context"
	"io/fs"
)

// Source knows how a template's payload bytes arrive. Fetch must return an
// fs.FS rooted at the template (embed, disk, git checkout, and registry
// tarball all satisfy this — the unifying abstraction of the whole design).
type Source interface {
	Fetch(ctx context.Context) (fs.FS, error)

	Pin(ctx context.Context) (string, error)

	Identity() Identity
}
