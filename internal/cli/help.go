// Coloured help rendering (ADR 0009 update). Cobra's default help is
// plain text; this replaces the help/usage funcs with a painter-driven
// renderer covering every section: usage line, aliases, examples,
// available commands (grouped on the root, per rootHelpGroups), and
// flag blocks. Otherwise same layout as cobra's default, same
// decoration contract as the rest of the CLI — colours vanish
// under --color never / NO_COLOR / non-TTY, so piping help stays
// byte-stable.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// installHelp wires the coloured renderers onto the root command; the
// funcs are inherited by every subcommand.
func (a *app) installHelp(root *cobra.Command) {
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {

		_ = a.applyFlagEnv(c.Root().PersistentFlags())
		a.out.writeHelp(c, c.OutOrStdout())
	})
	root.SetUsageFunc(func(c *cobra.Command) error {
		_ = a.applyFlagEnv(c.Root().PersistentFlags())
		a.out.writeUsageTo(c, c.OutOrStderr(), a.out.errP())
		return nil
	})
}

// writeHelp renders the full help page: description + usage body.
func (o *output) writeHelp(c *cobra.Command, out io.Writer) {
	if desc := c.Long; desc != "" {
		_, _ = fmt.Fprintln(out, strings.TrimRight(desc, "\n"))
		_, _ = fmt.Fprintln(out)
	} else if desc := c.Short; desc != "" {
		_, _ = fmt.Fprintln(out, desc)
		_, _ = fmt.Fprintln(out)
	}
	o.writeUsageTo(c, out, o.outP())
}

// writeUsageTo renders cobra's usage body: usage line, aliases,
// examples, available commands, flag blocks, subcommand footer.
func (o *output) writeUsageTo(c *cobra.Command, out io.Writer, p painter) {
	// Flags print in registration order (the curated order from the
	// global flag table), not pflag's lexical order.
	c.Flags().SortFlags = false
	if c.HasAvailableInheritedFlags() {
		c.InheritedFlags().SortFlags = false
	}

	_, _ = fmt.Fprintf(out, "%s\n", p.paint(bold, "Usage:"))

	if c.Runnable() {
		_, _ = fmt.Fprintf(out, "  %s\n", paintUseLine(c, p))
	}
	if c.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(out, "  %s [command]\n", p.paint(bold, c.CommandPath()))
	}

	if len(c.Aliases) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s\n  %s\n", p.paint(bold, "Aliases:"), p.paint(cyan, c.NameAndAliases()))
	}

	if c.HasExample() {
		_, _ = fmt.Fprintf(out, "\n%s\n%s\n", p.paint(bold, "Examples:"),
			p.paint(dim, strings.TrimSpace(c.Example)))
	}

	if c.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(out, "\n%s\n", p.paint(bold, "Available Commands:"))
		pad := c.NamePadding()
		for i, group := range helpCommandGroups(c) {
			if i > 0 {
				_, _ = fmt.Fprintln(out)
			}
			for _, sub := range group {
				_, _ = fmt.Fprintf(out, "  %s %s\n", p.paint(cyan, padTo(sub.Name(), pad)), sub.Short)
			}
		}
	}

	if c.HasAvailableLocalFlags() {
		_, _ = fmt.Fprintf(out, "\n%s\n", p.paint(bold, "Flags:"))
		writeFlagBlock(out, p, c.LocalFlags())
	}
	if c.HasAvailableInheritedFlags() {
		_, _ = fmt.Fprintf(out, "\n%s\n", p.paint(bold, "Global Flags:"))
		writeFlagBlock(out, p, c.InheritedFlags())
	}

	if c.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(out, "\n%s\n", p.paint(dim,
			fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.", c.CommandPath())))
	}
}

// helpCommandGroups splits the displayed subcommands into the groups the
// help page prints. The root uses the curated layout in rootHelpGroups
// (order + blank-line separation); every other command lists its
// available subcommands as a single group in cobra's own (name-sorted)
// order.
func helpCommandGroups(c *cobra.Command) [][]*cobra.Command {
	subs := c.Commands()
	if !c.HasParent() {
		var groups [][]*cobra.Command
		for _, names := range rootHelpGroups {
			var group []*cobra.Command
			for _, name := range names {
				for _, sub := range subs {
					if sub.Name() == name && (sub.IsAvailableCommand() || sub.Name() == "help") {
						group = append(group, sub)
					}
				}
			}
			if len(group) > 0 {
				groups = append(groups, group)
			}
		}
		if len(groups) > 0 {
			return groups
		}
	}
	var group []*cobra.Command
	for _, sub := range subs {
		if !sub.IsAvailableCommand() && sub.Name() != "help" {
			continue
		}
		group = append(group, sub)
	}
	return [][]*cobra.Command{group}
}

// paintUseLine renders the usage line: command-path words bold, place-
// holder tokens ([flags], [PATH], <id>…) dim, everything else plain.
func paintUseLine(c *cobra.Command, p painter) string {
	parts := strings.Fields(c.UseLine())
	pathLen := len(strings.Fields(c.CommandPath()))
	out := make([]string, 0, len(parts))
	for i, part := range parts {
		switch {
		case i < pathLen:
			out = append(out, p.paint(bold, part))
		case strings.HasPrefix(part, "[") || strings.HasPrefix(part, "<") ||
			strings.HasPrefix(part, "||"):
			out = append(out, p.paint(dim, part))
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

// writeFlagBlock renders one pflag set aligned like pflag's own
// FlagUsages: flag names yellow, value types and defaults dim.
func writeFlagBlock(out io.Writer, p painter, fs *pflag.FlagSet) {
	type row struct{ name, usage, def string }
	var rows []row
	width := 0
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "      --" + f.Name
		if f.Shorthand != "" {
			name = "  -" + f.Shorthand + ", --" + f.Name
		}
		typ := f.Value.Type()
		if typ != "bool" {
			name += " " + typ
		}
		def := ""
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && typ != "bool" {
			if f.Value.Type() == "string" {
				def = fmt.Sprintf(" (default %q)", f.DefValue)
			} else {
				def = " (default " + f.DefValue + ")"
			}
		}
		if len(name) > width {
			width = len(name)
		}
		rows = append(rows, row{name, f.Usage, def})
	})
	for _, r := range rows {
		_, _ = fmt.Fprintf(out, "%s   %s%s\n", p.paint(yellow, padTo(r.name, width)), r.usage, p.paint(dim, r.def))
	}
}
