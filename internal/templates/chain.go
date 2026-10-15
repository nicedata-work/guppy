// extends chain resolution (design §5): ordered, depth-first,
// cycle-checked; parents contribute payload, [defaults], and [vars], with
// the child winning. The chain is resolved through the Resolver — scope
// precedence (§4.2) is never duplicated here.
package templates

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// ResolveChain resolves t's extends parents through r, returning t with
// Chain populated (depth-first order) and Payload replaced by the merged
// overlay (earliest parent first, t last) and Manifest replaced by the
// merged manifest. Unresolvable parents are errors (§3.2); cycles are lint
// errors (§5).
func ResolveChain(ctx context.Context, r Resolver, t Template) (Template, error) {
	var (
		chain  []Identity
		loaded []Template
		path   []string
	)
	var walk func(Template) error
	walk = func(cur Template) error {
		id := cur.Manifest.ID
		if slices.Contains(path, id) {
			return fmt.Errorf("extends cycle detected: %s", fmtCycle(append(path, id)))
		}
		path = append(path, id)
		defer func() { path = path[:len(path)-1] }()

		for _, parentID := range cur.Manifest.Extends {
			parent, err := r.Resolve(ctx, parentID)
			if err != nil {
				return fmt.Errorf("extends %q: %w", parentID, err)
			}
			if err := walk(parent); err != nil {
				return err
			}
			loaded = append(loaded, parent)
		}
		chain = append(chain, cur.Identity)
		return nil
	}
	if err := walk(t); err != nil {
		return Template{}, err
	}

	parents := make([]manifest.Manifest, 0, len(loaded))
	layers := make([]fs.FS, 0, len(loaded)+1)
	for _, p := range loaded {
		parents = append(parents, p.Manifest)
		layers = append(layers, p.Payload)
	}
	layers = append(layers, t.Payload)

	t.Manifest = manifest.MergeChain(parents, t.Manifest)
	t.Payload = Overlay(layers)
	t.Chain = chain
	return t, nil
}

func fmtCycle(names []string) string {
	var out strings.Builder
	for i, n := range names {
		if i > 0 {
			out.WriteString(" -> ")
		}
		out.WriteString(n)
	}
	return out.String()
}
