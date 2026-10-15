// Package doctor implements design §8: the audit of an existing project
// against its template's expected file set, and the safe `--fix` subset.
//
// Doctor is manifest-driven and read-only except for --fix: it takes the
// .guppy/project.toml record (written at scaffold time — the uv
// pin-at-creation rule applied to provenance), re-renders the template
// with the recorded vars and toggles, diffs the tree against that
// FileSet, and reports where the tree drifted. Restoring a missing file
// can never destroy user content, which is why that — and only that —
// is in the fixable subset; modified files are reported, never touched.
package doctor

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/scaffold"
	"github.com/nicedata-work/guppy/internal/templates"
	"github.com/nicedata-work/guppy/internal/version"
)

// Report is the result of auditing one project.
type Report struct {
	Root string

	Template string

	Diffs scaffold.Diffs

	Notes []string
}

// Clean reports whether the project matches its template exactly.
func (r Report) Clean() bool { return len(r.Diffs) == 0 && len(r.Notes) == 0 }

// Fixable returns the safe subset: files --fix may restore.
func (r Report) Fixable() scaffold.Diffs {
	var out scaffold.Diffs
	for _, d := range r.Diffs {
		if d.Kind == scaffold.DiffMissing {
			out = append(out, d)
		}
	}
	return out
}

// Options parameterizes Audit and Fix.
type Options struct {
	Root string

	Marker project.Marker

	Resolver templates.Resolver

	HookInstalled func() bool
}

// expectedSet is the re-rendered template view an audit diffs against:
// the resolved template (for its current version — the stale-pin
// comparison) plus the file set rendered from the marker's recorded
// vars and toggles. A named result type, not a flag-return tuple
// (styleguide/arch review M-P4).
type expectedSet struct {
	t     templates.Template
	files render.FileSet

	note string
	ok   bool
}

// expected re-renders the template's file set from the marker's recorded
// vars and toggles. "Unrenderable" conditions (unresolvable template,
// newly required vars) become the returned note instead of an error:
// doctor must still report stale-pin and hook findings for those
// projects. ok=false means there is no expected set to diff against.
func (o Options) expected(ctx context.Context) expectedSet {
	t, err := o.Resolver.Resolve(ctx, o.Marker.Project.Template)
	if err != nil {
		return expectedSet{note: fmt.Sprintf(
			"template %s is not resolvable (%v) — the project cannot be audited against it; reinstall it with \"guppy template add\", or remove stale copies with \"guppy template remove\"",
			o.Marker.Project.Template, err)}
	}

	t, err = templates.ResolveChain(ctx, o.Resolver, t)
	if err != nil {
		return expectedSet{note: fmt.Sprintf("cannot resolve the extends chain: %v", err)}
	}

	vars, missing, err := render.ResolveVars(t.Manifest, render.Vars(o.Marker.Vars))
	if err != nil {
		return expectedSet{t: t, note: fmt.Sprintf("cannot re-render the expected file set: %v", err)}
	}
	if len(missing) > 0 {
		return expectedSet{t: t, note: fmt.Sprintf(
			"cannot re-render the expected file set: template now requires vars with no recorded value: %s (re-scaffold the project to record them)",
			strings.Join(missing, ", "))}
	}
	files, err := (render.Engine{}).Render(t, vars, resolveToggles(t.Manifest, o.Marker))
	if err != nil {
		return expectedSet{t: t, note: fmt.Sprintf("cannot re-render the expected file set: %v", err)}
	}
	return expectedSet{t: t, files: files, ok: true}
}

// Audit diffs the project tree against the template's expected file set
// (design §8). It never writes.
func (o Options) Audit(ctx context.Context) (Report, error) {
	m := o.Marker
	rep := Report{
		Root:     o.Root,
		Template: m.Project.Template + "@" + m.Project.Version,
	}

	exp := o.expected(ctx)
	if !exp.ok {
		rep.Notes = append(rep.Notes, exp.note)
	} else {
		diffs, err := (scaffold.Local{Files: exp.files}).Audit(o.Root)
		if err != nil {
			return Report{}, err
		}
		rep.Diffs = diffs
	}

	if exp.ok && m.Project.Version != "" && exp.t.Manifest.Version != "" &&
		m.Project.Version != exp.t.Manifest.Version &&
		version.Compare(m.Project.Version, exp.t.Manifest.Version) < 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"stale pin: scaffolded from %s@%s, installed template is %s@%s — run \"guppy template update %s\" then \"guppy doctor --fix\" to reconcile",
			m.Project.Template, m.Project.Version, m.Project.Template, exp.t.Manifest.Version, m.Project.Template))
	}

	if o.HookInstalled != nil && !o.HookInstalled() {
		rep.Notes = append(rep.Notes, "shellhook not installed in your rc file — run \"guppy init-shell --install\" so `guppy list` can track this project ")
	}
	return rep, nil
}

// Fix applies the safe subset: restore the report's missing files from
// the recorded template render. Modified and unexpected files are never
// touched (§8 — user content survives every fix).
func (o Options) Fix(ctx context.Context, rep Report) (fixed []string, err error) {
	safe := rep.Fixable()
	if len(safe) == 0 {
		return nil, nil
	}
	exp := o.expected(ctx)
	if !exp.ok {
		return nil, fmt.Errorf("cannot fix: %s", exp.note)
	}
	if err := (scaffold.Local{Files: exp.files}).Repair(o.Root, safe); err != nil {
		return nil, err
	}
	for _, d := range safe {
		fixed = append(fixed, d.Path)
	}
	return fixed, nil
}

// resolveToggles reconstructs the scaffold-time axes: recorded toggles
// (marker schema ≥ 2) win; schema-1 markers fall back to the template's
// [defaults], which is what `guppy new` would have used with no --no-*
// flags.
func resolveToggles(m manifest.Manifest, marker project.Marker) render.Toggles {
	t := render.Toggles{}
	maps.Copy(t, m.Defaults)
	if marker.Schema >= 2 {
		maps.Copy(t, marker.Toggles)
	}
	return t
}
