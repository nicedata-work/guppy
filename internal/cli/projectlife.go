// `guppy project jump` and `guppy project clear` — lifecycle verbs over
// the global project index (design §9).
//
//	jump [NAME]    resolve a known project to its root directory; a
//	               child process cannot cd the shell, so the output is
//	               meant for the shellhook or `cd $(guppy project jump …)`
//	clear          forget every known project (index + seen log)
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/catalog"
)

func newJumpCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "jump [NAME]",
		Short: "Print a known project's root directory (cd via the shellhook)",
		Long: `Resolve a guppy-managed project to its root directory.

NAME matches the project directory's base name as recorded in the
global index. With no argument, exactly one known project is required —
ambiguity is an error listing the candidates.

A child process cannot change its parent shell's directory, so jump
prints the path; the shellhook (guppy shell-env) or an explicit
` + "`cd $(guppy project jump NAME)`" + ` does the actual cd.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  cd "$(guppy project jump dbt-project)"
  eval "$(guppy shell-env zsh)"   # installs the jump-aware hook`,
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runJump(rt, out, name)
		},
	}
}

func runJump(rt *runtime, out *output, name string) error {
	entries, err := loadIndex(rt, out)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return InputError(fmt.Errorf("no guppy-managed projects known (scaffold one with \"guppy new\")"))
	}

	var matches []catalog.Entry
	for _, e := range entries {
		if name == "" || filepath.Base(e.Path) == name {
			matches = append(matches, e)
		}
	}
	switch {
	case len(matches) == 1:
		_, _ = fmt.Fprintln(out.stdout, matches[0].Path)
		return nil
	case len(matches) == 0:
		return InputError(fmt.Errorf("no project named %q — known projects:\n%s",
			name, entryNames(entries)))
	default:
		return InputError(fmt.Errorf("multiple projects named %q — disambiguate:\n%s",
			name, pathsOf(matches)))
	}
}

// noArgJumpName is the error text for `jump` with several projects known.
func entryNames(entries []catalog.Entry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s\n", filepath.Base(e.Path))
	}
	return strings.TrimRight(b.String(), "\n")
}

func pathsOf(entries []catalog.Entry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s\n", e.Path)
	}
	return strings.TrimRight(b.String(), "\n")
}

func newClearCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Forget all known projects (clears the global index)",
		Long: `Clear the global project index: every known project is forgotten and
the shellhook's seen log is truncated.

The index is only a cache of where .guppy/project.toml markers have
been seen — clearing it never touches the projects themselves, and any
project you scaffold or enter is re-recorded automatically.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runClear(rt, out)
		},
	}
}

func runClear(rt *runtime, out *output) error {
	entries, err := loadIndex(rt, out)
	if err != nil {
		return err
	}
	// Fold+prune happened on read; clearing now drops exactly the live
	// entries plus the seen log that would re-fold them.
	for _, path := range []string{rt.layout.IndexFile(), rt.layout.SeenLog()} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return Fail(err)
		}
	}
	out.statusf(green, "OK", "cleared %d known project(s)", len(entries))
	return nil
}

// loadIndex reads the global index with its self-healing semantics:
// fold the shellhook's seen log, prune vanished projects (list, jump,
// and clear all observe the same view).
func loadIndex(rt *runtime, out *output) ([]catalog.Entry, error) {
	idx := &catalog.FileIndex{Path: rt.layout.IndexFile()}
	if n, err := idx.FoldSeen(rt.layout.SeenLog()); err != nil && !os.IsNotExist(err) {
		out.warnf("%v", err)
	} else if n > 0 && out.verbose() {
		out.infof("recorded %d project sighting(s) from the shellhook", n)
	}
	entries, err := idx.Entries()
	if err != nil {
		return nil, Fail(err)
	}
	return entries, nil
}
