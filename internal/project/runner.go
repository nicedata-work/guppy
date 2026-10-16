// Verb execution for `guppy project <verb>` / `gup` (§3.5): dispatch runs
// the verb's command line from the project root, through direnv when the
// project is direnv-wired (the pinned environment — design §3.4), and
// directly when it is not. The fallback is a contract, not a degraded
// mode: `guppy project test` must work on a machine without direnv,
// because Bazel pins (.bazelversion) already give version hermeticity.
//
// Runner selection is pure (testable without spawning shells); only Run
// touches os/exec.
package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Mode is how a verb command executes.
type Mode int

const (
	// ModeDirenv: `direnv exec <root> sh -c <cmd>` — the command runs in
	// the project's pinned environment (shims on PATH, exports set).
	ModeDirenv Mode = iota
	// ModeDirect: `sh -c <cmd>` with cwd = project root — the explicit
	// fallback when direnv is absent or the project has no .envrc.
	ModeDirect
)

// String names the mode for diagnostics.
func (m Mode) String() string {
	if m == ModeDirenv {
		return "direnv"
	}
	return "direct"
}

// SelectRunner is pure: it picks the execution mode for a project root
// from the facts on the ground (direnv installed? .envrc present?).
func SelectRunner(root string, direnvPath string, envrcExists bool) Mode {
	if direnvPath != "" && envrcExists {
		return ModeDirenv
	}
	return ModeDirect
}

// EnvrcExists reports whether root has a .envrc (the direnv wiring of the
// environment contract, §3.4).
func EnvrcExists(root string) bool {
	st, err := os.Stat(filepath.Join(root, ".envrc"))
	return err == nil && !st.IsDir()
}

// Shell is the interpreter verb command lines run under (POSIX sh: the
// [gup] map holds shell command lines, §3.5).
const Shell = "sh"

// RunOptions carries the per-invocation execution state into Run.
type RunOptions struct {
	Ctx    context.Context
	Root   string
	Mode   Mode
	Direnv string
	Verb   string
	Args   []string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes the verb command line and returns the child's exit code
// (0 on success). The command inherits stdio — `gup test` streams bazel's
// output like running it by hand. Cancellation (SIGINT/SIGTERM via the
// CLI's signal-aware ctx) kills the child and surfaces as an error
// wrapping context.Canceled so the CLI can exit 130.
func Run(opts RunOptions, command Command) (int, error) {

	cmdline := withArgs(string(command), opts.Args)
	av := argv(opts.Mode, opts.Direnv, opts.Root, cmdline)
	c := exec.CommandContext(opts.Ctx, av[0], av[1:]...)
	if opts.Mode != ModeDirenv {
		c.Dir = opts.Root
	}
	c.Env = opts.Env
	c.Stdin = opts.Stdin
	c.Stdout = opts.Stdout
	c.Stderr = opts.Stderr

	err := c.Run()
	if opts.Ctx.Err() != nil {

		return -1, fmt.Errorf("run %s verb %q: %w", opts.Mode, opts.Verb, opts.Ctx.Err())
	}
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), nil
	}
	return -1, fmt.Errorf("run %s verb %q: %w", opts.Mode, opts.Verb, err)
}

// argv is pure: the process argv for a verb run. The direnv form is
// exactly `direnv exec ROOT sh -c CMD…` — the shell is direnv's command,
// not an argument of the verb's.
func argv(mode Mode, direnv, root, cmdline string) []string {
	if mode == ModeDirenv {
		return []string{direnv, "exec", root, Shell, "-c", cmdline}
	}
	return []string{Shell, "-c", cmdline}
}

// withArgs appends pass-through args to a shell command line, each shell-
// quoted so `gup test --test_filter='a b'` reaches the verb intact.
func withArgs(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		if a != "" && strings.IndexFunc(a, func(r rune) bool {
			return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:", r)
		}) == -1 {
			quoted = append(quoted, a)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return command + " " + strings.Join(quoted, " ")
}
