// extends merge (design §5): declarative inheritance, never executable
// hooks. Parents contribute payload files, [defaults], [vars], and [gp]
// verbs; children override them per-verb. Payload merging happens in
// internal/templates (it needs the fs.FS layers); this file merges the
// manifest half.
package manifest

import "maps"

import "slices"

// MergeChain merges the manifests of a resolved extends chain — parents in
// order (earliest first, later parents win over earlier ones per §5), then
// the child, which wins overall. Identity fields (id, version, kind,
// language, description, extends) always stay the child's; parents only
// contribute requires, vars, defaults, and (as a fallback) includes.
func MergeChain(parents []Manifest, child Manifest) Manifest {
	out := child
	out.Requires = map[string]string{}
	out.Vars = map[string]VarSpec{}
	out.Defaults = map[Axis]bool{}
	out.GP = map[string]GPCommand{}

	for _, p := range parents {
		mergeInto(&out, p)
	}
	mergeInto(&out, child)

	if child.Includes.isZero() {
		for _, parent := range slices.Backward(parents) {
			if !parent.Includes.isZero() {
				out.Includes = parent.Includes
				break
			}
		}
	}
	return out
}

// mergeInto layers src's contribution over out (src's identity fields are
// ignored — only contributes requires/vars/defaults).
func mergeInto(out *Manifest, src Manifest) {
	maps.Copy(out.Requires, src.Requires)
	maps.Copy(out.Vars, src.Vars)
	maps.Copy(out.Defaults, src.Defaults)
	maps.Copy(out.GP, src.GP)
}
