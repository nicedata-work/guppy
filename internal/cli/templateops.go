// `guppy template remove`, `clone`, and `update` (design §6, §7).
//
//	remove <id> | community:<id>     uninstall and drop the record
//	clone <id> [--into DIR]          copy a template out for editing
//	update [<id>|community:<id>|--all]
//	                                 reconcile installs with their sources:
//	                                 registry → newest index version, git →
//	                                 latest commit of the same ref (the ref
//	                                 never moves — §4.3). Community updates
//	                                 pass the review + diff gate (§7).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/dist"
)

func newTemplateRemoveCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>|community:<id>",
		Short: "Uninstall a template and forget its provenance",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			community, id, err := parseScopeID(args[0])
			if err != nil {
				return InputError(err)
			}
			return runRemove(rt, out, community, id)
		},
	}
}

func runRemove(rt *runtime, out *output, community bool, id string) error {
	scope := dist.ScopeUser
	if community {
		scope = dist.ScopeCommunity
	}
	store := rt.distStore()
	records, err := store.Load()
	if err != nil {
		return Fail(err)
	}
	_, recorded := dist.Find(records, id, scope)

	var removed []string
	switch {
	case community:
		dir, version := rt.newestCommunityCopy(id)
		if dir == "" {
			return InputError(fmt.Errorf("community template %q is not installed", id))
		}
		if err := os.RemoveAll(dir); err != nil {
			return Fail(err)
		}
		removed = append(removed, filepath.Join(rt.layout.CommunityDir(), dist.CommunityDirName(id, version)))
	default:
		dir := filepath.Join(rt.layout.TemplatesDir(), id)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			if _, other := dist.Find(records, id, dist.ScopeCommunity); other {
				return InputError(fmt.Errorf("template %q is installed in the community scope — use guppy template remove community:%s", id, id))
			}
			return InputError(fmt.Errorf("template %q is not installed", id))
		}
		if err := os.RemoveAll(dir); err != nil {
			return Fail(err)
		}
		removed = append(removed, dir)
	}

	if recorded {
		rest, _ := dist.Delete(records, id, scope)
		if err := store.Save(rest); err != nil {
			return Fail(err)
		}
	}
	for _, dir := range removed {
		out.infof("removed %s", dir)
	}
	if community {
		out.infof("trust record for %s revoked", id)
	}
	return nil
}

func newTemplateCloneCmd(rt *runtime, out *output) *cobra.Command {
	var into string
	cmd := &cobra.Command{
		Use:   "clone <id>",
		Short: "Copy a template out to a local directory for editing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClone(cmd.Context(), rt, out, args[0], into)
		},
	}
	cmd.Flags().StringVar(&into, "into", "", "destination directory (default: ./<id>)")
	return cmd
}

func runClone(ctx context.Context, rt *runtime, out *output, spec, into string) error {

	lt, _, err := rt.loadTemplate(ctx, spec)
	if err != nil {
		return InputError(err)
	}
	id := lt.t.Manifest.ID
	if into == "" {
		into = id
	}
	if st, err := os.Stat(into); err == nil && st.IsDir() {
		entries, err := os.ReadDir(into)
		if err != nil {
			return Fail(err)
		}
		if len(entries) > 0 {
			return InputError(fmt.Errorf("destination %s is not empty", into))
		}
	}
	if err := dist.InstallTree(lt.rootFS(), into, dist.InstallOptions{}); err != nil {
		return Fail(err)
	}
	abs, _ := filepath.Abs(into)
	out.infof("cloned %s → %s", lt.t.Identity.String(), abs)
	out.infof("edit it there, then: guppy lint %s && guppy test %s", into, into)
	return nil
}

func newTemplateUpdateCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var all, yes bool
	cmd := &cobra.Command{
		Use:   "update [<id>|community:<id>]",
		Short: "Reconcile installed templates with their sources",
		Long: `Reconcile installed templates with their sources: registry templates
move to the newest indexed version, git templates re-resolve their ref
to the newest commit (the ref itself never moves). Local-source
templates are loaded in place and have nothing to update.

Community-scope updates show the diff against the installed version and
require review + confirm (--yes in CI); first-party updates apply
directly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id string
			if len(args) == 1 {
				var err error
				_, id, err = parseScopeID(args[0])
				if err != nil {
					return InputError(err)
				}
			} else {
				all = true
			}
			return runUpdate(cmd.Context(), rt, out, p, id, all, yes)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "update every installed template")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the interactive review confirmation (CI)")
	return cmd
}

func runUpdate(ctx context.Context, rt *runtime, out *output, pr *prompter, id string, all, yes bool) error {
	if !all && id == "" {
		return InputError(fmt.Errorf("specify a template id or --all"))
	}
	if all && id != "" {
		return InputError(fmt.Errorf("specify an id or --all, not both"))
	}
	store := rt.distStore()
	records, err := store.Load()
	if err != nil {
		return Fail(err)
	}
	if len(records) == 0 {
		out.infof("nothing installed")
		return nil
	}

	matched := 0
	hardFailure := false
	updated := false
	started := out.clock()
	for _, rec := range records {
		if id != "" && rec.ID != id {
			continue
		}
		matched++
		ok, err := updateOne(ctx, rt, out, pr, rec, yes)
		if err != nil {

			if _, ok := errors.AsType[*ExitError](err); ok {
				return err
			}
			out.warnf("%v", err)
			hardFailure = true
			continue
		}
		if ok {
			updated = true
		}
	}
	errs := 0
	if hardFailure {
		errs = 1
	}
	out.summary(started, errs, 0)
	if hardFailure {
		return Fail(fmt.Errorf("one or more updates failed"))
	}
	if id != "" && matched == 0 {
		return InputError(fmt.Errorf("no installed template %q to update", id))
	}
	if !updated {
		out.infof("everything up to date")
	}
	return nil
}

// updateOne reconciles a single record. Returns whether anything was
// installed (false = already up to date).
func updateOne(ctx context.Context, rt *runtime, out *output, pr *prompter, rec dist.Record, yes bool) (bool, error) {
	spec := templateSpec{Community: rec.Scope == dist.ScopeCommunity, Kind: rec.Kind}

	switch rec.Kind {
	case dist.KindLocal:
		out.statusf(dim, "NOTE", "%s: local source (%s) — nothing to update", rec.ID, rec.Pin)
		return false, nil
	case dist.KindRegistry:

		spec.ID, spec.Version = rec.ID, ""
	case dist.KindGit:
		gitSpec := rec.Source
		if after, ok := strings.CutPrefix(gitSpec, "community:"); ok {
			gitSpec = after
		}
		url, ref := splitGitRef(strings.TrimPrefix(gitSpec, "git:"))
		if url == "" {
			return false, InputError(fmt.Errorf("record %s: unparseable git source %q", rec.ID, rec.Source))
		}
		spec.URL, spec.Ref = url, ref
	default:
		return false, InputError(fmt.Errorf("record %s: unknown source kind %q", rec.ID, rec.Kind))
	}

	fsys, version, pin, fullTrust, err := rt.fetchSource(ctx, spec)
	if err != nil {
		return false, err
	}
	if pin == rec.Pin && version == rec.Version {
		out.statusf(green, "OK", "%s: up to date (%s)", rec.ID, rec.Version)
		return false, nil
	}

	oldDir := filepath.Join(rt.layout.TemplatesDir(), rec.ID)
	if rec.Scope == dist.ScopeCommunity {
		oldDir, _ = rt.newestCommunityCopy(rec.ID)
	}
	oldFS := fs.FS(nil)
	if oldDir != "" {
		if st, err := os.Stat(oldDir); err == nil && st.IsDir() {
			oldFS = os.DirFS(oldDir)
		}
	}

	if rec.Scope == dist.ScopeCommunity && !fullTrust {
		m, _, err := readManifest(fsys)
		if err != nil {
			return false, err
		}
		approved, err := reviewCommunity(out, pr, m, fsys, oldDir, rec.Version, version, yes)
		if err != nil {
			return false, err
		}
		if !approved {
			out.statusf(yellow, "WARN", "%s: update declined", rec.ID)
			return false, nil
		}
	} else if oldFS != nil && out.verbose() {

		changes, err := diffTrees(oldFS, fsys)
		if err == nil {
			printDiff(out.stdout, out.outP(), changes)
		}
	}

	if _, err := rt.commitInstall(spec, rec.ID, version, pin, fsys); err != nil {
		return false, Fail(err)
	}
	out.statusf(green, "OK", "updated %s %s → %s (%s)", rec.ID, rec.Version, version, rec.Kind)
	return true, nil
}
