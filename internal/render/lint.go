// Static verification of a template payload (design §3.2, §3.3): the
// rules `guppy lint` enforces before a template is ever rendered. Every
// substitution must resolve — statically checkable because the grammar is
// tiny (§3.3), and that is the whole point.
//
// The payload rules themselves live once, in analyze (styleguide D3) —
// shared with the renderer, so lint and render can never disagree.
// Lint adds the manifest-level checks (var defaults) and the advisory
// warnings on top of the analyzer's findings.
package render

import (
	"fmt"
	"io/fs"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// Lint statically verifies payload against manifest: syntax errors,
// undeclared variables, undeclared toggles, output-path collisions,
// unusable var defaults — all errors (§3.2: "undeclared {{ .X }} is a lint
// error, never silently empty") — plus declared-but-unused vars as
// warnings. It applies the same include/exclude narrowing the renderer
// does: excluded files are never rendered, so their contents don't matter.
//
// The manifest is the merged one (extends applied, §5): parent vars are
// declared vars for the child payload.
func Lint(payload fs.FS, m manifest.Manifest) []manifest.Issue {
	analyses, issues := analyze(payload, m)

	used := map[string]bool{}
	if derr := checkVarDefaults(m); derr != nil {
		issues = append(issues, manifest.Issue{Level: manifest.LevelError, Message: fmt.Sprintf("%v", derr)})
	} else {
		for _, name := range sortedVarNames(m.Vars) {
			def := m.Vars[name].Default
			if def == "" {
				continue
			}
			if nodes, perr := parseTemplate(def); perr == nil {
				walkNodes(nodes, func(n node) {
					if n.kind == varNode {
						used[n.name] = true
					}
				})
			}
		}
	}

	for _, fa := range analyses {
		walkNodes(fa.nodes, func(n node) {
			if n.kind == varNode {
				if _, ok := m.Vars[n.name]; ok {
					used[n.name] = true
				}
			}
		})
	}

	for _, name := range sortedVarNames(m.Vars) {
		if !used[name] {
			issues = append(issues, manifest.Issue{
				Level:   manifest.LevelWarning,
				Message: fmt.Sprintf("declared var %q is never used", name),
			})
		}
	}
	return issues
}
