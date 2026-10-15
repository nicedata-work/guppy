// The one payload analyzer (styleguide D3: one rule set, one
// implementation). Render and Lint historically walked the payload
// twice, each re-implementing the includes filter, the .hbs split,
// parsing, and collision detection — two rule sets that can disagree
// ("lint passes but render fails" is a bug by definition). analyze() is
// now the single rule set: Render is analyze + strict emit, Lint is
// analyze + report; they differ only in what they do with the findings.
package render

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// fileAnalysis is one payload file as the analyzer sees it: the output
// path it maps to, its mode and bytes, and — for .hbs files — the
// parsed nodes plus an already-validated single-toggle gate (zero Axis
// when the file is not a whole-file gate).
type fileAnalysis struct {
	rel      string
	outPath  string
	mode     fs.FileMode
	verbatim bool
	content  []byte
	nodes    []node
	gate     manifest.Axis
}

// analyze applies the §3.2/§3.3 payload rules once: the walk, the
// [includes] narrowing, the .hbs split, parsing, output-path collision
// detection, and the declaration checks (undeclared vars/toggles are
// errors, never silently empty — §3.2). Warnings it emits: `{{` in a
// verbatim file (copied without substitution). Files that fail to parse
// or are unreadable are reported and excluded from the analyses.
func analyze(payload fs.FS, m manifest.Manifest) ([]fileAnalysis, []manifest.Issue) {
	var (
		files  []fileAnalysis
		issues []manifest.Issue
	)
	errf := func(path, format string, args ...any) {
		issues = append(issues, manifest.Issue{Level: manifest.LevelError, Path: path, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(path, format string, args ...any) {
		issues = append(issues, manifest.Issue{Level: manifest.LevelWarning, Path: path, Message: fmt.Sprintf(format, args...)})
	}

	sources := map[string]string{}
	_ = fs.WalkDir(payload, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !Included(m.Includes, rel) {
			return nil
		}
		data, rerr := fs.ReadFile(payload, rel)
		if rerr != nil {
			errf(rel, "unreadable: %v", rerr)
			return nil
		}
		mode := fs.FileMode(0o644)
		if info, ierr := d.Info(); ierr == nil && info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		fa := fileAnalysis{rel: rel, outPath: rel, mode: mode, verbatim: true, content: data}

		if before, isHBS := strings.CutSuffix(rel, ".hbs"); isHBS {
			fa.outPath = before
			fa.verbatim = false
			nodes, perr := parseTemplate(string(data))
			if perr != nil {
				errf(rel, "%v", perr)
				return nil
			}
			fa.nodes = nodes
			if gate, isGate := singleToggleGate(nodes); isGate {
				fa.gate = manifest.Axis(gate)
			}

			walkNodes(nodes, func(n node) {
				switch n.kind {
				case varNode:
					if _, ok := m.Vars[n.name]; !ok {
						errf(rel, "undeclared variable {{ .%s }} (declare it in [vars])", n.name)
					}
				case ifNode:

					if _, ok := m.Defaults[manifest.Axis(n.name)]; !ok {
						errf(rel, "undeclared toggle {{ if .%s }} (declare it in [defaults])", n.name)
					}
				}
			})
		} else if strings.Contains(string(data), "{{") {
			warnf(rel, "contains {{ but is not a .hbs file — copied verbatim, no substitution applied")
		}

		if fa.outPath == "" || strings.HasSuffix(fa.outPath, "/") {
			errf(rel, "invalid output path")
			return nil
		}
		if prev, dup := sources[fa.outPath]; dup {
			errf(rel, "%s and %s would both write %s", prev, rel, fa.outPath)
		} else {
			sources[fa.outPath] = rel
		}
		files = append(files, fa)
		return nil
	})
	return files, issues
}

// issueError renders one error-level issue as the hard error Render
// fails with (same message lint reports, same path prefix).
func issueError(i manifest.Issue) error {
	if i.Path != "" {
		return fmt.Errorf("%s: %s", i.Path, i.Message)
	}
	return fmt.Errorf("%s", i.Message)
}
