// `guppy update` and `guppy uninstall` — the executable-management verbs
// (design §10.1). Template updates are a different verb: `guppy template
// update` reconciles installed templates; `guppy update` never touches
// them.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/httpx"
	"github.com/nicedata-work/guppy/internal/selfmanage"
)

func newUpdateCmd(rt *runtime, out *output) *cobra.Command {
	var pin string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the guppy executable in place",
		Long: `Download the latest (or a pinned) release, verify its checksum, and
atomically replace the running binary. The previous binary is kept as a
backup until the new one is in place, and restored if installation fails.

Updates the guppy executable only; installed templates are reconciled
with "guppy template update".`,
		Example: "  guppy update\n  guppy update --version 1.2.3",
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			return nil
		},
	}
	cmd.Flags().StringVar(&pin, "version", "", "update to this exact version instead of latest")
	return cmd
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
