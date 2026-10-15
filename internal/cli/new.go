// `guppy new` — uv-init-style scaffolding (design §6): composable axes
// over built-in templates, every default with a --no-* inverse, existing
// directories refused unless --force (merge, never clobber — §2.1), and
// vars resolved from [vars] prompts/defaults.
package cli

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/scaffold"
	"github.com/nicedata-work/guppy/internal/settings"
	"github.com/nicedata-work/guppy/internal/templates"
	"github.com/nicedata-work/guppy/internal/version"
)

func newNewCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var (
		templateID string
		kind, lang string
		bare       bool
		force      bool
		dryRun     bool
		varFlags   []string
		noGit      bool
		noBazel    bool
		noDirenv   bool
		noLinters  bool
		noTests    bool
		noAgentsMD bool
	)
	cmd := &cobra.Command{
		Use:   "new [PATH]",
		Short: "Scaffold a new project from a template",
		Long: `Scaffold a new project from a template.

Without --template, the template is picked from the kind × language axes
(--kind/--lang); ambiguity is an error listing candidates. Every default
axis has a --no-* inverse, and --bare turns all of them off.

An existing non-empty directory is refused; pass --force to merge into it
(existing files are never clobbered — they are kept and reported, and
re-runs are idempotent).

Vars are prompted interactively when they have no value; in non-interactive
contexts pass --var name=value (missing vars are exit code 2).`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: nil,
		Example: `  guppy new my-cli --template go-cli
  guppy new . --kind cli --lang go --var module_path=example.com/x/y
  guppy new existing-dir --force --no-direnv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd.Context(), rt, out, p, newOptions{
				TemplateID: templateID,
				Kind:       kind,
				Lang:       lang,
				Bare:       bare,
				Force:      force,
				DryRun:     dryRun,
				Vars:       varFlags,
				Dir:        ".",
				NoToggles: map[manifest.Axis]bool{
					manifest.AxisGit:      noGit,
					manifest.AxisBazel:    noBazel,
					manifest.AxisDirenv:   noDirenv,
					manifest.AxisLinters:  noLinters,
					manifest.AxisTests:    noTests,
					manifest.AxisAgentsMD: noAgentsMD,
				},
			}, args)
		},
	}
	cmd.Flags().StringVarP(&templateID, "template", "t", "", "template to use (id or directory); GUPPY_TEMPLATE")
	cmd.Flags().StringVar(&kind, "kind", "", "pick the template by kind: cli, service, lib, app, docs, infra")
	cmd.Flags().StringVar(&lang, "lang", "", "pick the template by language: go, ts, python, rust, none")
	cmd.Flags().BoolVar(&bare, "bare", false, "scaffold minimally: turn every default axis off")
	cmd.Flags().BoolVar(&force, "force", false, "merge into an existing non-empty directory (never clobbers files)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the files that would be created, write nothing")
	cmd.Flags().StringArrayVarP(&varFlags, "var", "x", nil, "set a template var (key=value, repeatable); GUPPY_VARS")
	cmd.Flags().BoolVar(&noGit, "no-git", false, "disable the git default (no .git init, no git ignores)")
	cmd.Flags().BoolVar(&noBazel, "no-bazel", false, "disable the bazel default")
	cmd.Flags().BoolVar(&noDirenv, "no-direnv", false, "disable the direnv default")
	cmd.Flags().BoolVar(&noLinters, "no-linters", false, "disable the linters default")
	cmd.Flags().BoolVar(&noTests, "no-tests", false, "disable the tests default")
	cmd.Flags().BoolVar(&noAgentsMD, "no-agents-md", false, "disable the agents_md default (no AGENTS.md)")
	return cmd
}

// newOptions carries resolved flag state into runNew (testable core).
type newOptions struct {
	TemplateID string
	Kind       string
	Lang       string
	Bare       bool
	Force      bool
	DryRun     bool
	Vars       []string
	Dir        string
	NoToggles  map[manifest.Axis]bool
}

func runNew(ctx context.Context, rt *runtime, out *output, p *prompter, opts newOptions, args []string) error {
	dir := opts.Dir
	if len(args) == 1 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return InputError(err)
	}

	if st, err := os.Stat(abs); err == nil {
		if !st.IsDir() {
			return InputError(fmt.Errorf("%s exists and is not a directory", abs))
		}
		if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 && !opts.Force {
			return InputError(fmt.Errorf("directory %s already exists and is not empty (pass --force to merge into it)", abs))
		}
	}

	spec := opts.TemplateID
	if spec == "" {
		if v, ok := rt.env("GUPPY_TEMPLATE"); ok && v != "" {
			spec = v
		}
	}
	if spec == "" {
		list, lerr := rt.resolver().List(ctx, false)
		if lerr != nil {
			return InputError(lerr)
		}
		picked, perr := pickTemplate(opts.Kind, opts.Lang, list)
		if perr != nil {
			return InputError(perr)
		}
		spec = picked
	}

	lt, issues, err := rt.loadTemplate(ctx, spec)
	if err != nil {
		return InputError(err)
	}
	for _, w := range issues {
		out.warnf("%s", w)
	}
	if manifest.HasErrors(issues) {
		var msgs []string
		for _, i := range issues {
			if i.Level == manifest.LevelError {
				msgs = append(msgs, i.String())
			}
		}
		return Fail(fmt.Errorf("template %q has lint errors:\n  %s", spec, strings.Join(msgs, "\n  ")))
	}
	out.infof("using template %s", lt.t.Identity.String())

	toggles := resolveToggles(lt.t.Manifest, rt.settings.Defaults, opts.Bare, opts.NoToggles)

	var envVars string
	if v, ok := rt.env("GUPPY_VARS"); ok {
		envVars = v
	}
	provided, err := providedVars(opts.Vars, envVars, abs, lt.t.Manifest)
	if err != nil {
		return InputError(err)
	}
	resolved, missing, err := render.ResolveVars(lt.t.Manifest, provided)
	if err != nil {
		return Fail(err)
	}
	if len(missing) > 0 {
		resolvedMore, err := resolveMissingVars(p, lt.t.Manifest, missing)
		if err != nil {
			return InputError(err)
		}
		maps.Copy(resolved, resolvedMore)
	}

	files, err := (render.Engine{}).Render(lt.t, resolved, toggles)
	if err != nil {
		return Fail(err)
	}

	var verbs project.VerbMap
	if len(lt.t.Manifest.GP) > 0 {
		verbs, err = project.ResolveVerbs(lt.t.Manifest.GP, toggles, resolved)
		if err != nil {
			return Fail(err)
		}
		marker, merr := project.File(project.MarkerInput{
			TemplateID:   lt.t.Manifest.ID,
			TemplateVer:  lt.t.Manifest.Version,
			Chain:        chainRefs(lt.t.Chain),
			Vars:         resolved,
			Toggles:      toggles,
			Verbs:        verbs,
			GuppyVersion: version.Version,
		})
		if merr != nil {
			return Fail(merr)
		}
		files.Add(marker)
	}

	if opts.DryRun {
		for _, f := range files.Files {
			_, _ = fmt.Fprintln(out.stdout, f.Path)
		}
		out.infof("dry run: would create %d files in %s", len(files.Files), abs)
		return nil
	}

	res, err := scaffold.Apply(ctx, files, abs, scaffold.ApplyOptions{})
	if err != nil {
		return Fail(err)
	}
	out.statusf(green, "OK", "scaffolded %s: %s", abs, res.Summary())
	for _, p := range res.Kept {
		out.warnf("kept %s (exists with different content, never clobbered)", p)
	}

	rt.recordProject(abs)

	if len(verbs) > 0 {
		out.infof("project verbs: %s", strings.Join(project.SortedVerbNames(verbs), ", "))
	}
	if toggles[manifest.AxisDirenv] {
		out.infof("activate the pinned environment: cd %s && direnv allow . && gp test", abs)
	}

	if toggles[manifest.AxisGit] {
		gitInit(ctx, out, abs)
	}
	return nil
}

// chainRefs renders the resolved extends chain as identity refs
// (id@version) for the project marker's provenance record (§9).
func chainRefs(chain []templates.Identity) []string {
	out := make([]string, 0, len(chain))
	for _, id := range chain {
		ref := id.ID
		if v := id.Version.String(); v != "" {
			ref += "@" + v
		}
		out = append(out, ref)
	}
	return out
}

// pickTemplate is pure: it resolves the kind × language matrix (§4.2)
// over the candidate list — exactly one match or an error listing the
// candidates.
func pickTemplate(kind, lang string, list []templates.Template) (string, error) {
	var candidates []string
	var matches []string
	for _, t := range list {
		candidates = append(candidates, fmt.Sprintf("%s (%s/%s)", t.Manifest.ID, t.Manifest.Kind, t.Manifest.Language))
		if (kind == "" || t.Manifest.Kind == kind) && (lang == "" || t.Manifest.Language == lang) {
			matches = append(matches, t.Manifest.ID)
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) == 0:
		return "", fmt.Errorf("no template matches kind=%q language=%q (candidates: %s)", kind, lang, strings.Join(candidates, ", "))
	default:
		slices.Sort(matches)
		return "", fmt.Errorf("ambiguous template for kind=%q language=%q: %s (pass --template)",
			kind, lang, strings.Join(matches, ", "))
	}
}

// resolveToggles is pure: it layers the precedence chain for toggle
// axes — template [defaults] < settings [defaults] < --bare < --no-*
// flags. Only axes the template declares survive. The settings bridge
// is the one typed conversion from settings.Defaults' named fields to
// the Axis vocabulary (styleguide T1 — no string keys here).
func resolveToggles(m manifest.Manifest, user settings.Defaults, bare bool, noFlags map[manifest.Axis]bool) render.Toggles {
	t := render.Toggles{}
	maps.Copy(t, m.Defaults)
	for _, o := range settingsAxisOverrides(user) {
		if o.value != nil {
			if _, declared := m.Defaults[o.axis]; declared {
				t[o.axis] = *o.value
			}
		}
	}
	if bare {
		for k := range t {
			t[k] = false
		}
	}
	for k, off := range noFlags {
		if off {
			if _, declared := m.Defaults[k]; declared {
				t[k] = false
			}
		}
	}
	return t
}

// axisOverride is one user-settings default override bound to its axis.
type axisOverride struct {
	axis  manifest.Axis
	value *bool
}

// settingsAxisOverrides is the single mapping of settings.Defaults'
// named fields onto the Axis vocabulary — the only place the two
// vocabularies meet (D4: one definition, nothing to drift).
func settingsAxisOverrides(d settings.Defaults) []axisOverride {
	return []axisOverride{
		{manifest.AxisGit, d.Git},
		{manifest.AxisBazel, d.Bazel},
		{manifest.AxisDirenv, d.Direnv},
		{manifest.AxisLinters, d.Linters},
		{manifest.AxisTests, d.Tests},
		{manifest.AxisAgentsMD, d.AgentsMD},
	}
}

// providedVars is pure: it merges --var flags and GUPPY_VARS
// (comma-separated) and seeds `name` from the target directory when the
// template declares it.
func providedVars(varFlags []string, envVars, abs string, m manifest.Manifest) (render.Vars, error) {
	pairs := append([]string(nil), varFlags...)
	if envVars != "" {
		pairs = append(pairs, strings.Split(envVars, ",")...)
	}
	provided := render.Vars{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --var %q: expected key=value", p)
		}
		provided[k] = v
	}
	if _, ok := provided["name"]; !ok {
		if _, declared := m.Vars["name"]; declared {
			provided["name"] = filepath.Base(abs)
		}
	}
	return provided, nil
}

// resolveMissingVars prompts interactively for vars with no value; in
// non-interactive contexts it is an input error (exit 2, §6).
func resolveMissingVars(p *prompter, m manifest.Manifest, missing []string) (render.Vars, error) {
	if !p.interactive() {
		var kv []string
		for _, name := range missing {
			kv = append(kv, name+"=…")
		}
		return nil, fmt.Errorf("missing required vars: %s (pass --var %s)",
			strings.Join(missing, ", "), strings.Join(kv, " --var "))
	}
	out := render.Vars{}
	for _, name := range missing {
		spec := m.Vars[name]
		question := spec.Prompt
		if question == "" {
			question = "Value for " + name
		}
		v, err := p.prompt(question, "")
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, nil
}

// gitInit best-effort initializes a git repo (§1: scaffolds want git);
// git availability is bootstrap plumbing, never a hard requirement.
// Runs through the invocation's ctx (T14): an interrupt during
// `git init` aborts it like any other exec path.
func gitInit(ctx context.Context, out *output, dir string) {
	if isDir(filepath.Join(dir, ".git")) {
		return
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return
	}
	cmd := exec.CommandContext(ctx, git, "init")
	cmd.Dir = dir
	if combined, err := cmd.CombinedOutput(); err != nil {
		out.warnf("git init failed: %v: %s", err, strings.TrimSpace(string(combined)))
	}
}
