// `guppy doctor` — the audit half of the environment contract (§8).
//
//	guppy doctor [--fix]          audit this project (cwd or --directory)
//	guppy doctor --all [--fix]    batch mode: audit every indexed project
//
// Doctor is manifest-driven: the marker's recorded template, vars, and
// toggles re-render the expected file set, the tree is diffed against
// it, and --fix restores the safe subset (missing files only — user
// edits are reported, never touched).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/catalog"
	"github.com/nicedata-work/guppy/internal/doctor"
	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/scaffold"
)

func newDoctorCmd(rt *runtime, out *output) *cobra.Command {
	var fix, all bool
	cmd := &cobra.Command{
		Use:   "doctor [--fix]",
		Short: "Audit a guppy-managed project against its template ",
		Long: `Audit a guppy-managed project against its template.

Doctor re-renders the template with the vars and toggles recorded in
.guppy/project.toml, diffs the project tree against that expected file
set, and reports:

  missing      files the template expects but the tree lacks
               (safe to restore — the --fix subset)
  modified     files that differ from the template; may be your edits —
               reported, never overwritten
  unexpected   extra files in the .guppy/ metadata dir
  note         project-level findings: stale pin (the installed template
               is newer than the scaffold), missing shellhook, or an
               unresolvable template

With --all, every project in the global index is audited (batch mode);
deleted projects are pruned from the index first .`,
		Args: cobra.NoArgs,
		Example: `  guppy doctor              # audit the project containing cwd
  guppy doctor --fix        # restore missing scaffolded files
  guppy doctor --all        # audit every indexed project`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all {
				return runDoctorAll(cmd.Context(), rt, out, fix)
			}
			return runDoctor(cmd.Context(), rt, out, fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "restore missing scaffolded files (never touches modified files)")
	cmd.Flags().BoolVar(&all, "all", false, "audit every project in the global index")
	return cmd
}

// doctorOptions builds the audit options for a project root+marker,
// wiring the resolver and the shellhook check.
func doctorOptions(rt *runtime, root string, m project.Marker) doctor.Options {
	return doctor.Options{
		Root:          root,
		Marker:        m,
		Resolver:      rt.resolver(),
		HookInstalled: func() bool { return hookInstalled("") },
	}
}

func runDoctor(ctx context.Context, rt *runtime, out *output, fix bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return Fail(err)
	}
	root, m, err := project.FindUp(cwd)
	if err != nil {
		if errors.Is(err, project.ErrOutsideProject) {
			return InputError(err)
		}
		return Fail(err)
	}
	return runDoctorAt(ctx, rt, out, root, m, fix)
}

// runDoctorAt audits one project (shared by `guppy doctor` and the
// built-in `gp doctor` verb — §3.5: one audit, two faces).
func runDoctorAt(ctx context.Context, rt *runtime, out *output, root string, m project.Marker, fix bool) error {
	started := out.clock()
	rt.recordProject(root)

	opts := doctorOptions(rt, root, m)
	rep, err := opts.Audit(ctx)
	if err != nil {
		return Fail(err)
	}
	if fix && !rep.Clean() {
		fixed, ferr := opts.Fix(ctx, rep)
		if ferr != nil {
			return Fail(ferr)
		}
		for _, p := range fixed {
			_, _ = fmt.Fprintf(out.stdout, "%s fixed %s (restored)\n", tagMark(out.outP(), green, "OK"), p)
		}

		if rep, err = opts.Audit(ctx); err != nil {
			return Fail(err)
		}
	}
	printReport(out.stdout, rep, out.outP())
	errs, warns := reportCounts(rep)
	out.summary(started, errs, warns)
	if len(rep.Diffs) > 0 {
		return Fail(fmt.Errorf("project drifted from its template (%d finding(s))", len(rep.Diffs)))
	}
	return nil
}

// runDoctorAll audits every project in the index (batch mode, §9).
func runDoctorAll(ctx context.Context, rt *runtime, out *output, fix bool) error {
	idx := &catalog.FileIndex{Path: rt.layout.IndexFile()}
	if _, err := idx.FoldSeen(rt.layout.SeenLog()); err != nil && !os.IsNotExist(err) {
		out.warnf("%v", err)
	}
	entries, err := idx.Entries()
	if err != nil {
		return Fail(err)
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out.stdout, "no guppy-managed projects found (scaffold one with \"guppy new\")")
		return nil
	}

	drifted, errs, warns := 0, 0, 0
	started := out.clock()
	for _, e := range entries {
		m, err := readMarker(e.Path)
		if err != nil {

			_, _ = fmt.Fprintf(out.stderr, "%s %s: %v\n", tagMark(out.errP(), red, "FAIL"), e.Path, err)
			drifted++
			errs++
			continue
		}
		opts := doctorOptions(rt, e.Path, m)
		rep, err := opts.Audit(ctx)
		if err != nil {
			return Fail(err)
		}
		if fix && !rep.Clean() {
			if _, ferr := opts.Fix(ctx, rep); ferr != nil {
				return Fail(ferr)
			}
			if rep, err = opts.Audit(ctx); err != nil {
				return Fail(err)
			}
		}
		if len(rep.Diffs) == 0 {
			_, _ = fmt.Fprintf(out.stdout, "%s %s (%s)\n", tagMark(out.outP(), green, "OK"), rep.Root, rep.Template)
			for _, n := range rep.Notes {
				_, _ = fmt.Fprintf(out.stdout, "  %s %s\n", tagMark(out.outP(), dim, "NOTE"), n)
				warns++
			}
			continue
		}
		drifted++
		e, w := reportCounts(rep)
		errs += e
		warns += w
		printReport(out.stdout, rep, out.outP())
	}
	out.summary(started, errs, warns)
	if drifted > 0 {
		return Fail(fmt.Errorf("%d of %d project(s) need attention", drifted, len(entries)))
	}
	return nil
}

// readMarker reads and decodes the marker at projectDir.
func readMarker(projectDir string) (project.Marker, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(project.MarkerRelPath)))
	if err != nil {
		return project.Marker{}, err
	}
	return project.Decode(data)
}

// printReport renders one audit report. The finding column is
// fixed-width (greppable, diff-able) and tagged+coloured by severity:
// missing is restorable drift (red, [FAIL]), modified is a user edit
// (yellow, [WARN], never touched), notes are advisory (dim, [NOTE]).
func printReport(out io.Writer, rep doctor.Report, p painter) {
	_, _ = fmt.Fprintf(out, "%s (template %s)\n", rep.Root, rep.Template)
	for _, d := range rep.Diffs {
		_, _ = fmt.Fprintf(out, "  %s %s %s\n", tagMark(p, findingStyle(d.Kind), findingTag(d.Kind)),
			padTo(findingLabel(d.Kind), 10), d.Path)
	}
	if len(rep.Diffs) == 0 {
		_, _ = fmt.Fprintf(out, "  %s clean — matches the template exactly\n", tagMark(p, green, "OK"))
	}
	for _, n := range rep.Notes {
		_, _ = fmt.Fprintf(out, "  %s %s\n", tagMark(p, dim, "NOTE"), n)
	}
	if fixable := len(rep.Fixable()); fixable > 0 {
		_, _ = fmt.Fprintf(out, "  %d missing file(s) can be restored with \"guppy doctor --fix\"\n", fixable)
	}
}

// reportCounts splits a report into summary errors and warnings so the
// completion line matches the tags: [FAIL] findings (missing,
// unexpected) count as errors, [WARN] (modified, stale) and [NOTE]
// findings as warnings.
func reportCounts(rep doctor.Report) (errs, warns int) {
	for _, d := range rep.Diffs {
		if findingTag(d.Kind) == "FAIL" {
			errs++
		} else {
			warns++
		}
	}
	warns += len(rep.Notes)
	return errs, warns
}

// findingTag maps a finding kind to its dbt-style status tag.
func findingTag(k scaffold.DiffKind) string {
	switch k {
	case scaffold.DiffMissing:
		return "FAIL"
	case scaffold.DiffModified, scaffold.DiffUnexpected:
		return "WARN"
	default:
		return "NOTE"
	}
}

// findingStyle maps a finding kind to its severity colour.
func findingStyle(k scaffold.DiffKind) style {
	switch k {
	case scaffold.DiffMissing:
		return red
	case scaffold.DiffModified, scaffold.DiffUnexpected:
		return yellow
	default:
		return plain
	}
}

func findingLabel(k scaffold.DiffKind) string {
	switch k {
	case scaffold.DiffMissing:
		return "missing"
	case scaffold.DiffModified:
		return "modified"
	case scaffold.DiffUnexpected:
		return "unexpected"
	case scaffold.DiffStale:
		return "stale"
	default:
		return "diff"
	}
}
