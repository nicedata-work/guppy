// `guppy update` and `guppy uninstall` — the executable-management verbs
// (design §10.1) plus the project-update ladder:
//
//	guppy update        the executable only
//	guppy update --all  the executable and all installed templates
//	guppy update .      the current project to its template's latest
//
// Template reconciliation itself is `guppy template update`.
package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/httpx"
	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/scaffold"
	"github.com/nicedata-work/guppy/internal/selfmanage"
	"github.com/nicedata-work/guppy/internal/version"
)

func newUpdateCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var (
		pin string
		all bool
	)
	cmd := &cobra.Command{
		Use:   "update [PATH]",
		Short: "Update guppy, templates, or a project",
		Long: `Update things, in three composable forms:

  guppy update        update the guppy executable only
  guppy update --all  update the executable and all installed templates
  guppy update .      update the current project to its template's
                      latest version (requires .guppy/project.toml)

A PATH form re-renders the project's recorded vars and toggles against
the newest template version and merges the result (existing files are
never clobbered — they are kept and reported). Installed templates are
reconciled with "guppy template update"; a project update always uses
what is installed locally after that.`,
		Example: "  guppy update\n  guppy update --all\n  guppy update .",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if all {
					return InputError(fmt.Errorf("pass a project PATH or --all, not both"))
				}
				return runProjectUpdate(cmd.Context(), rt, out, args[0])
			}
			if rt.settings.Network.Offline {
				return NetworkError(fmt.Errorf("cannot update: offline mode is enabled"))
			}
			client := httpx.New(rt.settings.Network.Insecure)
			res, err := selfmanage.Update(cmd.Context(), selfmanage.UpdateOptions{
				BaseURL:  rt.releaseBase(),
				Version:  pin,
				Client:   client,
				Progress: out.progressWriter(),
			})
			if err != nil {
				if errors.Is(err, selfmanage.ErrSwap) {
					return Fail(err)
				}
				return NetworkError(err)
			}
			out.statusf(green, "OK", "guppy updated to %s", res.Version)
			if all {
				return runUpdate(cmd.Context(), rt, out, p, "", true, false)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pin, "version", "", "update to this exact version instead of latest")
	cmd.Flags().BoolVar(&all, "all", false, "also update all installed templates (executable update + guppy template update)")
	return cmd
}

// runProjectUpdate implements `guppy update [PATH]`: re-render the
// project from its template's newest resolvable version and merge.
// Never-clobber applies (§2.1) — user-modified files are kept and
// reported; the marker is rewritten directly because the merge must
// record the new provenance even when every payload file was kept.
func runProjectUpdate(ctx context.Context, rt *runtime, out *output, target string) error {
	root, marker, err := project.FindUp(target)
	if err != nil {
		if errors.Is(err, project.ErrOutsideProject) {
			return InputError(err)
		}
		return Fail(err)
	}

	lt, _, err := rt.loadTemplate(ctx, marker.Project.Template)
	if err != nil {
		return InputError(fmt.Errorf("project template %q: %w", marker.Project.Template, err))
	}
	latest := lt.t.Manifest.Version
	switch version.Compare(marker.Project.Version, latest) {
	case 0, 1:
		out.infof("%s: template %s is at the latest version (%s)", root, marker.Project.Template, latest)
		return nil
	}

	// Re-render from the marker's record — the same inputs doctor audits
	// with (recorded vars; recorded toggles over the template defaults).
	toggles := markerToggles(lt.t.Manifest, marker)
	files, err := (render.Engine{}).Render(lt.t, render.Vars(marker.Vars), toggles)
	if err != nil {
		return Fail(err)
	}
	var verbs project.VerbMap
	if len(lt.t.Manifest.GP) > 0 {
		verbs, err = project.ResolveVerbs(lt.t.Manifest.GP, toggles, marker.Vars)
		if err != nil {
			return Fail(err)
		}
	}
	markerFile, err := project.File(project.MarkerInput{
		TemplateID:   lt.t.Manifest.ID,
		TemplateVer:  latest,
		Chain:        chainRefs(lt.t.Chain),
		Vars:         marker.Vars,
		Toggles:      toggles,
		Verbs:        verbs,
		GuppyVersion: version.Version,
	})
	if err != nil {
		return Fail(err)
	}

	res, err := scaffold.Apply(ctx, files, root, scaffold.ApplyOptions{})
	if err != nil {
		return Fail(err)
	}
	markerPath := filepath.Join(root, project.MarkerRelPath)
	if err := os.WriteFile(markerPath, markerFile.Content, markerFile.Mode); err != nil {
		return Fail(err)
	}
	rt.recordProject(root)

	out.statusf(green, "OK", "updated %s: template %s %s → %s (%s)",
		root, marker.Project.Template, marker.Project.Version, latest, res.Summary())
	for _, kept := range res.Kept {
		out.warnf("kept %s (differs from the template, never clobbered)", kept)
	}
	if len(verbs) > 0 {
		out.infof("project verbs: %s", strings.Join(project.SortedVerbNames(verbs), ", "))
	}
	return nil
}

// markerToggles reconstructs the scaffold-time axes from the marker's
// record over the (new) template defaults — the same layering doctor
// audits with, mirrored here because doctor's helper is package-private.
func markerToggles(m manifest.Manifest, marker project.Marker) render.Toggles {
	t := render.Toggles{}
	maps.Copy(t, m.Defaults)
	maps.Copy(t, marker.Toggles)
	return t
}

func newUninstallCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var yes, purge bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove guppy from this machine",
		Long: `Remove the guppy binary and the shell-hook block the installer added to
your rc files. User settings (~/.config/guppy) are never touched; pass
--purge to additionally remove data (~/.local/share/guppy) and cache
(~/.cache/guppy).`,
		Example: "  guppy uninstall --yes --purge",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				confirmed, err := p.confirm(fmt.Sprintf("Remove guppy at %s?", currentBinary()))
				if err != nil {
					return InputError(err)
				}
				if !confirmed {
					out.infof("aborted")
					return nil
				}
			}
			res, err := selfmanage.Uninstall(cmd.Context(), selfmanage.UninstallOptions{
				Layout: rt.layout,
				Purge:  purge,
			})
			if err != nil {
				return Fail(err)
			}
			out.statusf(green, "OK", "removed %s", res.BinaryPath)
			for _, rc := range res.CleanedRcs {
				out.statusf(green, "OK", "cleaned %s", rc)
			}
			for _, dir := range res.PurgedDirs {
				out.statusf(green, "OK", "purged %s", dir)
			}
			for _, w := range res.Warnings {
				out.warnf("%s", w)
			}
			if res.KeptDataMsg != "" {
				out.infof("kept %s (use --purge to remove)", res.KeptDataMsg)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove data and cache dirs (never settings)")
	return cmd
}

// currentBinary is the resolved path of the running executable.
func currentBinary() string {
	p, err := os.Executable()
	if err != nil {
		return "guppy"
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}
