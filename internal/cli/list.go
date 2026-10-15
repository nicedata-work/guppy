// `guppy list` — the global project index .
//
// Reads index.toml with prune-on-read: the shellhook's seen-log is
// folded in first (each logged path's marker read for template id and
// mtime), then entries whose project or marker has vanished are pruned.
// The index self-heals on every read — no daemon, no manual GC.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/catalog"
)

func newListCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List guppy-managed projects on this machine",
		Long: `List the guppy-managed projects known on this machine.

The list comes from the global index (index.toml), which is maintained
by the shellhook ("guppy init-shell") and pruned on every read: entries
whose project directory or .guppy/project.toml no longer exists are
removed automatically. Projects scaffolded or audited by guppy are
recorded even without the hook.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runListProjects(rt, out)
		},
	}
}

func runListProjects(rt *runtime, out *output) error {
	idx := &catalog.FileIndex{Path: rt.layout.IndexFile()}

	if n, err := idx.FoldSeen(rt.layout.SeenLog()); err != nil && !os.IsNotExist(err) {
		out.warnf("%v", err)
	} else if n > 0 && out.verbose() {
		out.infof("recorded %d project sighting(s) from the shellhook", n)
	}
	entries, err := idx.Entries()
	if err != nil {
		return Fail(err)
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out.stdout, "no guppy-managed projects found (scaffold one with \"guppy new\")")
		return nil
	}

	w := newTable(out.stdout, "PROJECT", "TEMPLATE", "LAST SEEN").styled(out.outP())
	for _, e := range entries {
		w.row(e.Path, orDash(e.TemplateID), out.outP().paint(dim, lastSeen(e.LastSeen, out.clock())))
	}
	w.flush()
	return nil
}

// lastSeen renders a relative timestamp ("just now", "3h ago", "2d ago")
// with the absolute time in parens — human first, machine second.
func lastSeen(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	var rel string
	switch {
	case d < time.Minute:
		rel = "just now"
	case d < time.Hour:
		rel = fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		rel = fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		rel = fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return fmt.Sprintf("%s (%s)", rel, t.Local().Format("2006-01-02"))
}
