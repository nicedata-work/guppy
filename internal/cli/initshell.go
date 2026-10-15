// `guppy init-shell` — shellhook emission .
//
//	guppy init-shell [zsh|bash]        print the hook (eval in your rc file)
//	guppy init-shell --install [SHELL] write the hook into the rc file as a
//	                                   marked block (removable by
//	                                   `guppy uninstall`)
//
// The hook is a stub on the hot path: one directory comparison per
// prompt, one append when entering a managed project. It never invokes
// guppy; guppy folds the seen-log into index.toml on the next read.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/catalog"
)

func newInitShellCmd(rt *runtime, out *output) *cobra.Command {
	var install bool
	cmd := &cobra.Command{
		Use:   "init-shell [zsh|bash]",
		Short: "Emit the shellhook that keeps the project index fresh",
		Long: `Emit the shellhook that keeps the global project index fresh.

Without --install the hook is printed for eval in your rc file:

    eval "$(guppy init-shell zsh)"

With --install it is written into the rc file as a marked block
(idempotent; ` + "`guppy uninstall`" + ` removes it). The hook itself is a
stub: one directory comparison per prompt and one append when you enter
a guppy-managed project — it never invokes guppy, so it cannot slow
down or break your shell. Guppy folds its observations into
index.toml whenever a command reads the index (guppy list, doctor).`,
		Args: cobra.MaximumNArgs(1),
		Example: `  eval "$(guppy init-shell zsh)"
  guppy init-shell bash --install`,
		RunE: func(_ *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			}
			return runInitShell(rt, out, shell, install)
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "write the hook into the rc file (marked block) instead of printing it")
	return cmd
}

func runInitShell(rt *runtime, out *output, shell string, install bool) error {
	if !rt.settings.Shell.Hook {
		return InputError(fmt.Errorf("shellhooks are disabled in your settings ([shell] hook = false); re-enable them or remove the setting"))
	}
	shell, err := resolveShell(rt.env, shell)
	if err != nil {
		return InputError(err)
	}
	h := catalog.Hook{SeenLog: rt.layout.SeenLog()}
	if !install {
		_, _ = fmt.Fprint(out.stdout, h.Script())
		return nil
	}
	rc := rcFile(shell)
	if rc == "" {
		return InputError(fmt.Errorf("don't know the rc file for shell %q on %s — run \"guppy init-shell %s\" and add it manually", shell, goruntime.GOOS, shell))
	}
	if err := catalog.InstallHook(rc, h); err != nil {
		return Fail(err)
	}
	out.statusf(green, "OK", "installed shellhook into %s (remove with \"guppy uninstall\")", rc)
	return nil
}

// resolveShell validates an explicit shell argument or detects one from
// $SHELL. Only zsh and bash are supported (the hook uses their prompt
// mechanisms; other shells can eval the printed snippet knowingly).
func resolveShell(env EnvFunc, arg string) (string, error) {
	if arg != "" {
		switch arg {
		case "zsh", "bash":
			return arg, nil
		default:
			return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash)", arg)
		}
	}
	if sh, ok := env("SHELL"); ok && sh != "" {
		switch base := filepath.Base(sh); base {
		case "zsh", "bash":
			return base, nil
		}
	}
	return "", fmt.Errorf("cannot detect your shell from $SHELL — pass one explicitly: guppy init-shell [zsh|bash]")
}

// rcFile is the rc file the hook is installed into per shell/OS.
func rcFile(shell string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	switch {
	case shell == "zsh":
		return filepath.Join(home, ".zshrc")
	case goruntime.GOOS == "windows":
		return ""
	default:
		return filepath.Join(home, ".bashrc")
	}
}

// hookInstalled reports whether the shellhook block is present in any
// rc file (doctor's missing-hook check). Best-effort: probes the same
// rc files `guppy uninstall` cleans.
func hookInstalled(home string) bool {
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return false
		}
	}
	var rcs []string
	for _, name := range []string{".bashrc", ".zshrc"} {
		if p := filepath.Join(home, name); fileExists(p) {
			rcs = append(rcs, p)
		}
	}
	return catalog.HookInstalled(rcs)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
