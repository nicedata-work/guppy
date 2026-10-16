// `guppy template` — template management verbs (design §6): show the
// merged view of a template (§5 — inheritance is never invisible), and
// the registry-backed distribution verbs (list/search/add/remove/
// clone/update/refresh with the §7 community trust gate). The authoring
// loop lives under `guppy dev` (lint/test/publish).
package cli

import (
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
)

func newTemplateCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	tmpl := &cobra.Command{
		Use:   "template",
		Short: "Inspect and manage templates",
		Long:  "Inspect and manage templates: show merged views, list/search the catalog, install (add), uninstall (remove), copy out for editing (clone), reconcile installs with their sources (update), and drop the download cache (refresh).",
	}
	tmpl.AddCommand(newTemplateShowCmd(rt, out))
	tmpl.AddCommand(newTemplateListCmd(rt, out))
	tmpl.AddCommand(newTemplateSearchCmd(rt, out))
	tmpl.AddCommand(newTemplateAddCmd(rt, out, p))
	tmpl.AddCommand(newTemplateRemoveCmd(rt, out))
	tmpl.AddCommand(newTemplateCloneCmd(rt, out))
	tmpl.AddCommand(newTemplateUpdateCmd(rt, out, p))
	tmpl.AddCommand(newTemplateRefreshCmd(rt, out))
	return tmpl
}

func newTemplateShowCmd(rt *runtime, out *output) *cobra.Command {
	var contents bool
	cmd := &cobra.Command{
		Use:   "show <id|PATH>",
		Short: "Show a template's merged view: chain, vars, defaults, and payload",
		Long: `Show what a template will actually be written as: the resolved extends
chain, the merged [vars] and [defaults], and the merged payload file
list (parents overlaid, child wins whole-file). --contents dumps the
merged payload files verbatim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lt, _, err := rt.loadTemplate(cmd.Context(), args[0])
			if err != nil {
				return InputError(err)
			}
			if err := showTemplate(out.stdout, lt, contents); err != nil {
				return Fail(err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&contents, "contents", false, "also print the merged payload file contents")
	return cmd
}

// showTemplate prints the merged view. Pure-ish: writes only to out.
func showTemplate(out io.Writer, lt loadedTemplate, contents bool) error {
	t := lt.t
	_, _ = fmt.Fprintf(out, "%s (%s)\n", t.Identity.String(), t.Identity.Scope)

	_, _ = fmt.Fprintln(out, "chain:")
	for _, id := range t.Chain {
		_, _ = fmt.Fprintf(out, "  %s\n", id)
	}

	_, _ = fmt.Fprintln(out, "vars:")
	if len(t.Manifest.Vars) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	}
	for _, name := range sortedKeys(t.Manifest.Vars) {
		v := t.Manifest.Vars[name]
		line := "  " + name
		if v.Prompt != "" {
			line += fmt.Sprintf("  prompt=%q", v.Prompt)
		}
		if v.Default != "" {
			line += fmt.Sprintf("  default=%q", v.Default)
		}
		_, _ = fmt.Fprintln(out, line)
	}

	_, _ = fmt.Fprintln(out, "defaults:")
	if len(t.Manifest.Defaults) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	} else {
		axes := make([]string, 0, len(t.Manifest.Defaults))
		for k, v := range t.Manifest.Defaults {
			axes = append(axes, fmt.Sprintf("%s=%t", k, v))
		}
		slices.Sort(axes)
		for _, ax := range axes {
			_, _ = fmt.Fprintf(out, "  %s\n", ax)
		}
	}

	_, _ = fmt.Fprintln(out, "verbs:")
	if len(t.Manifest.GP) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	} else {
		for _, verb := range sortedKeys(t.Manifest.GP) {
			entry := t.Manifest.GP[verb]
			line := fmt.Sprintf("  %-6s %s", verb, entry.Command)
			if len(entry.Requires) > 0 {
				line += fmt.Sprintf("  (requires: %s)", strings.Join(manifest.AxisNames(entry.Requires), ", "))
			}
			_, _ = fmt.Fprintln(out, line)
		}
	}

	files, err := payloadFiles(t.Payload, t.Manifest)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "files:")
	for _, f := range files {
		_, _ = fmt.Fprintf(out, "  %s\n", f)
	}

	if contents {
		for _, f := range files {
			_, _ = fmt.Fprintf(out, "\n--- %s ---\n", f)
			data, err := fs.ReadFile(t.Payload, f)
			if err != nil {
				return err
			}
			_, _ = out.Write(data)
		}
	}
	return nil
}

// payloadFiles lists the merged payload paths (include/exclude applied),
// sorted.
func payloadFiles(payload fs.FS, m manifest.Manifest) ([]string, error) {
	var out []string
	err := fs.WalkDir(payload, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if render.Included(m.Includes, rel) {
			out = append(out, rel)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}
