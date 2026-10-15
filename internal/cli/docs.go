package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nicedata-work/guppy/internal/version"
)

// newDocsCmd implements `guppy docs` (the CLI reference is
// generated from the cobra tree, never hand-written). Hidden: it is a
// release-engineering tool, not part of the user-facing verb set (design
// §6).
//
// The generator is hand-rolled rather than cobra/doc: cobra/doc drags in
// go-md2man (a new non-stdlib dependency — AGENTS.md
// requires an ADR for that) and stamps a generation date, which would
// make the committed reference non-deterministic and CI-uncheckable.
// Output here is deterministic, so `scripts/gen-docs.sh --check` diffs
// it in CI.
func newDocsCmd(out *output) *cobra.Command {
	var argsOut string
	cmd := &cobra.Command{
		Use:    "docs",
		Short:  "Generate the markdown CLI reference",
		Hidden: true,
		Long: `Generate the markdown CLI reference for every command from the cobra
command tree. Output is deterministic (no timestamps), so generated
docs are committed and CI checks them for staleness via
scripts/gen-docs.sh --check.`,
		Example: "  guppy docs --out docs/cli",
		RunE: func(c *cobra.Command, _ []string) error {
			outDir := argsOut
			if outDir == "" {
				return InputError(fmt.Errorf("--out is required (e.g. --out docs/cli)"))
			}
			pages := genReference(c.Root())
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return InputError(fmt.Errorf("create %s: %w", outDir, err))
			}

			entries, err := os.ReadDir(outDir)
			if err != nil {
				return Fail(err)
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					if err := os.Remove(filepath.Join(outDir, e.Name())); err != nil {
						return Fail(err)
					}
				}
			}
			for _, name := range sortedKeys(pages) {
				if err := os.WriteFile(filepath.Join(outDir, name), pages[name], 0o644); err != nil {
					return Fail(err)
				}
			}
			out.infof("wrote %d pages for guppy %s to %s", len(pages), version.Version, outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&argsOut, "out", "", "output directory for the generated markdown")
	return cmd
}

// genReference renders one markdown page per available command, keyed by
// file name (e.g. "guppy_new.md"), plus the root page "guppy.md".
func genReference(root *cobra.Command) map[string][]byte {
	pages := map[string][]byte{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.IsAvailableCommand() || c == root {
			pages[refFileName(c)] = refPage(root, c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return pages
}

// refFileName maps a command to its page name: "guppy new" →
// "guppy_new.md".
func refFileName(c *cobra.Command) string {
	return strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
}

// refPage renders the reference page for one command.
func refPage(root, c *cobra.Command) []byte {
	var b strings.Builder
	path := c.CommandPath()
	_, _ = fmt.Fprintf(&b, "# %s\n\n", path)

	synopsis := c.Long
	if synopsis == "" {
		synopsis = c.Short
	}
	if synopsis != "" {
		b.WriteString("## Synopsis\n\n")
		b.WriteString(strings.TrimSpace(synopsis))
		b.WriteString("\n")
	}

	_, _ = fmt.Fprintf(&b, "\n## Usage\n\n```\n%s\n```\n", usageLine(c))

	if c.Example != "" {
		b.WriteString("\n## Examples\n\n```\n")
		b.WriteString(strings.TrimSpace(c.Example))
		b.WriteString("\n```\n")
	}

	if flags := flagSection("Flags", c.LocalFlags()); flags != "" {
		b.WriteString("\n" + flags)
	}

	if c != root {
		if flags := flagSection("Global flags", c.InheritedFlags()); flags != "" {
			b.WriteString("\n" + flags)
		}
	}

	var children []*cobra.Command
	for _, sub := range c.Commands() {
		if sub.IsAvailableCommand() {
			children = append(children, sub)
		}
	}
	if len(children) > 0 {
		b.WriteString("\n## See also\n\n")
		for _, sub := range children {
			_, _ = fmt.Fprintf(&b, "* [%s](%s)\t%s\n", sub.CommandPath(), refFileName(sub), sub.Short)
		}
	}
	return []byte(b.String())
}

// usageLine is the command's usage line with a stable program prefix
// (cobra's UseLine uses argv[0], which is not deterministic under `go
// test`).
func usageLine(c *cobra.Command) string {
	line := c.CommandPath()
	if c.Use != "" {
		if name := strings.Fields(c.Use); len(name) > 1 {
			line += " " + strings.Join(name[1:], " ")
		}
	}
	if c.HasAvailableFlags() && !strings.Contains(line, "[flags]") {
		line += " [flags]"
	}
	return line
}

// flagSection renders one "###"-free markdown flag table; "" when there
// are no visible flags. pflag sorts flags lexically by default, keeping
// output deterministic.
func flagSection(title string, fs *pflag.FlagSet) string {
	type row struct{ name, usage string }
	var rows []row
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			name += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		rows = append(rows, row{name, f.Usage})
	})
	if len(rows) == 0 {
		return ""
	}
	slices.SortStableFunc(rows, func(a, b row) int { return strings.Compare(a.name, b.name) })
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "## %s\n\n", title)
	b.WriteString("| Flag | Description |\n|------|-------------|\n")
	for _, r := range rows {
		_, _ = fmt.Fprintf(&b, "| %s | %s |\n", r.name, r.usage)
	}
	b.WriteString("\n")
	return b.String()
}
