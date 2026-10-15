// `guppy lint` — the authoring loop of design §7: statically verify a
// template (manifest schema, extends resolution, every substitution in the
// payload) without leaving the CLI. Errors fail with exit 1; warnings are
// printed and otherwise ignored (§2.2).
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
)

func newLintCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "lint [PATH|TEMPLATE]",
		Short: "Lint a template: manifest, extends chain, and payload substitutions",
		Long: `Lint a template without rendering it: guppy.toml schema,
extends resolution (unresolvable parents and cycles are errors), and every
{{ }} substitution in the payload (undeclared vars and toggles are errors,
never silently empty output).

PATH is a template directory (default "."), TEMPLATE a resolvable template
id such as a built-in. Exit code 1 if any error-level issue is found.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			started := out.clock()
			spec := ""
			if len(args) == 1 {
				spec = args[0]
			}
			lt, issues, err := rt.loadTemplate(cmd.Context(), spec)
			if err != nil {

				fmt.Fprintf(out.stderr, "%s %v\n", tagMark(out.errP(), red, "FAIL"), err)
				return Fail(fmt.Errorf("lint failed"))
			}
			issues = append(issues, render.Lint(lt.t.Payload, lt.t.Manifest)...)
			for _, i := range issues {
				fmt.Fprintln(out.stderr, lintLine(out.errP(), i))
			}
			var errs, warns int
			for _, i := range issues {
				if i.Level == manifest.LevelError {
					errs++
				} else {
					warns++
				}
			}
			if errs > 0 {
				out.summary(started, errs, warns)
				return Fail(fmt.Errorf("lint: %d errors, %d warnings", errs, warns))
			}
			out.summary(started, 0, warns)
			return nil
		},
	}
}

// lintLine renders one lint issue with its dbt-style status tag
// ([FAIL] error, [WARN] warning) followed by `path: message` — the
// level word moves into the tag, the rest stays greppable. Findings are
// diagnostics: they go to stderr with the rest of the report (U7).
func lintLine(p painter, i manifest.Issue) string {
	st, name := yellow, "WARN"
	if i.Level == manifest.LevelError {
		st, name = red, "FAIL"
	}
	msg := i.Message
	if i.Path != "" {
		msg = i.Path + ": " + msg
	}
	return tagMark(p, st, name) + " " + msg
}
