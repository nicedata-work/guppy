package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nicedata-work/guppy/internal/paths"
	"github.com/nicedata-work/guppy/internal/settings"
	"github.com/nicedata-work/guppy/internal/version"
)

// EnvFunc resolves env vars (os.LookupEnv or a test map).
type EnvFunc func(string) (string, bool)

// app is the per-invocation CLI: configuration plus the three
// capability structs commands draw on (styleguide T6 —
// one struct, one concern). Commands receive only the capabilities they
// use, so a per-command test constructs a writer and a clock, not the
// whole runtime.
type app struct {
	globals globals

	ranRun bool

	out    *output
	prompt *prompter
	rt     *runtime
}

// globals are the uniform global flags — identical on every subcommand
// (design §6, uv rule). Every one has a GUPPY_* env equivalent.
type globals struct {
	Quiet       bool
	Verbose     int
	Color       string
	NoProgress  bool
	Offline     bool
	Directory   string
	Config      string
	Interactive bool
}

// globalFlagKind discriminates how a global flag is registered (the
// closed set of pflag types the uniform flag set uses).
type globalFlagKind int

const (
	flagBool globalFlagKind = iota
	flagCount
	flagString
)

// globalFlagSpec defines one uniform global flag completely: its
// registration, its derived GUPPY_* env binding, and (where the value
// has a closed vocabulary) its validator. The table is the single
// definition (styleguide D4/U4): flags, env names, misplaced-flag
// warnings, and validation all derive from it — nothing can drift.
//
// A hidden spec stays fully functional (flag, GUPPY_* env, validation)
// but is omitted from every help and docs listing.
type globalFlagSpec struct {
	name      string
	shorthand string
	usage     string
	kind      globalFlagKind
	def       string
	hidden    bool

	target func(*globals) any

	validate func(value string) error
}

// envVar derives the env binding from the flag name: GUPPY_ + name
// uppercased, - → _ (U4: derived, never hand-listed).
func (s globalFlagSpec) envVar() string {
	return "GUPPY_" + strings.ToUpper(strings.ReplaceAll(s.name, "-", "_"))
}

// globalFlagSpecs is THE table of uniform global flags (design §6).
// Registration order is help-page order (pflag sorting is off), and
// help appends --help / --version after these: interactive, config,
// directory, offline, quiet, verbose, help, version. The two hidden
// specs at the end are supported but unlisted.
var globalFlagSpecs = []globalFlagSpec{
	{name: "interactive", usage: "enable interactive prompts", kind: flagBool,
		target: func(g *globals) any { return &g.Interactive }},
	{name: "config", usage: "path to guppy.toml settings file", kind: flagString,
		target: func(g *globals) any { return &g.Config }},
	{name: "directory", usage: "run as if started in this directory", kind: flagString,
		target: func(g *globals) any { return &g.Directory }},
	{name: "offline", usage: "disable all network access", kind: flagBool,
		target: func(g *globals) any { return &g.Offline }},
	{name: "quiet", shorthand: "q", usage: "only print errors", kind: flagBool,
		target: func(g *globals) any { return &g.Quiet }},
	{name: "verbose", shorthand: "v", usage: "increase output verbosity", kind: flagCount,
		target: func(g *globals) any { return &g.Verbose }},
	{name: "color", usage: "color output: auto, always, never", kind: flagString, def: "auto",
		hidden:   true,
		target:   func(g *globals) any { return &g.Color },
		validate: validateColor},
	{name: "no-progress", usage: "disable progress output", kind: flagBool,
		hidden: true,
		target: func(g *globals) any { return &g.NoProgress }},
}

// rootHelpGroups is the curated root help layout: one group of
// subcommands per inner slice, printed in the listed order with a
// blank line between groups. A name that is absent from the tree or
// hidden is skipped, so the table can never render a phantom row.
var rootHelpGroups = [][]string{
	{"new", "project", "template", "dev", "doctor", "help"},
	{"config", "update", "completion", "uninstall"},
}

// globalFlagByName looks up a spec by long name (ok=false when unknown).
func globalFlagByName(name string) (globalFlagSpec, bool) {
	for _, s := range globalFlagSpecs {
		if s.name == name {
			return s, true
		}
	}
	return globalFlagSpec{}, false
}

// registerGlobalFlags registers every uniform global flag from the
// table. Registration order is preserved (pflag's lexical sorting is
// off) so the help page lists them in table order; hidden specs are
// marked hidden and disappear from every listing.
func (a *app) registerGlobalFlags(pf *pflag.FlagSet) {
	pf.SortFlags = false
	for _, s := range globalFlagSpecs {
		p := s.target(&a.globals)
		switch s.kind {
		case flagBool:
			pf.BoolVarP(p.(*bool), s.name, s.shorthand, false, s.usage)
		case flagCount:
			pf.CountVarP(p.(*int), s.name, s.shorthand, s.usage)
		case flagString:
			pf.StringVarP(p.(*string), s.name, s.shorthand, s.def, s.usage)
		}
		if s.hidden {
			_ = pf.MarkHidden(s.name)
		}
	}
}

// Execute runs the CLI and returns the process exit code. Everything is
// injectable (env, stdio) so the whole CLI is testable in-process.
//
// Every invocation starts from a signal-aware, cancellable context
// (styleguide T14/U14): SIGINT/SIGTERM cancel it, all network/exec paths
// observe it, and interruptible multi-step operations (scaffold, update)
// abort through their rollback paths. An interrupted run exits 130 with
// partial work undone. Execute is the context's only owner (F1): it
// passes it as the first argument to every command and helper that
// blocks — never through a struct field.
func Execute(args []string, env EnvFunc, stdin io.Reader, stdout, stderr io.Writer) int {
	if env == nil {
		env = os.LookupEnv
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := &app{}
	a.out = &output{
		g: &a.globals, env: env, stdout: stdout, stderr: stderr,
		now: time.Now,
	}
	a.prompt = newPrompter(stdin, stderr)
	a.rt = &runtime{env: env, out: a.out}

	root := newRootCmd(a)
	root.SetArgs(args)
	root.SilenceErrors = true
	root.SilenceUsage = true

	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	code := exitCodeOf(err)
	switch {
	case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):

		code = ExitInterrupted
	case code == ExitFailure && !a.ranRun:

		code = ExitInput
	}
	if code == ExitInterrupted {
		_, _ = fmt.Fprintf(stderr, "%s  %s interrupted\n", a.out.stamp(), tagMark(a.out.errP(), red, "FAIL"))
		return code
	}
	_, _ = fmt.Fprintf(stderr, "%s  %s %v\n", a.out.stamp(), tagMark(a.out.errP(), red, "FAIL"), err)
	return code
}

func newRootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "guppy",
		Short:         "guppy - coding project template manager",
		Version:       version.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate("guppy {{.Version}}\n")
	root.SetOut(a.out.stdout)
	root.SetErr(a.out.stderr)

	pf := root.PersistentFlags()
	a.registerGlobalFlags(pf)

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return a.setup(cmd)
	}
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}

	root.AddCommand(newUpdateCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newUninstallCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newNewCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newProjectCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newDevCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newTemplateCmd(a.rt, a.out, a.prompt))
	root.AddCommand(newConfigCmd(a.rt, a.out))
	root.AddCommand(newDoctorCmd(a.rt, a.out))
	root.AddCommand(newShellEnvCmd(a.rt, a.out))
	root.AddCommand(newDocsCmd(a.out))
	a.installHelp(root)
	return root
}

// setup resolves the runtime state in precedence order: env binding for
// flags not explicitly set, --directory chdir, then settings load
// (defaults < guppy.toml < env < flags — the settings layer applies env
// and the offline flag override).
func (a *app) setup(cmd *cobra.Command) error {
	a.ranRun = true
	if err := a.applyFlagEnv(cmd.Flags()); err != nil {
		return InputError(err)
	}
	if err := a.validateGlobals(); err != nil {
		return InputError(err)
	}
	// Prompts are opt-in (the uv rule sharpened): without --interactive
	// the prompter refuses, so every command must have a flag/env path.
	a.prompt.setEnabled(a.globals.Interactive)
	if a.globals.Directory != "" {
		if err := os.Chdir(a.globals.Directory); err != nil {
			return InputError(fmt.Errorf("--directory: %w", err))
		}
	}
	a.rt.layout = paths.Resolve()

	var offlineFlag *bool
	if cmd.Flags().Changed("offline") {
		offlineFlag = &a.globals.Offline
	}
	s, err := settings.Load(settings.LoadOptions{
		Flags:     settings.FlagOverrides{Offline: offlineFlag, Config: a.globals.Config},
		Layout:    a.rt.layout,
		LookupEnv: a.rt.env,
	})
	if err != nil {
		return InputError(err)
	}
	a.rt.settings = s
	for _, w := range s.Warnings {
		a.out.warnf("%s", w)
	}
	return nil
}

// applyFlagEnv applies GUPPY_* env vars to any global flag the user did not
// set explicitly (flags > env). Env names and validation derive from the
// spec table — one definition, nothing to drift (D4).
func (a *app) applyFlagEnv(flags *pflag.FlagSet) error {
	for _, s := range globalFlagSpecs {
		f := flags.Lookup(s.name)
		if f == nil || f.Changed {
			continue
		}
		envVar := s.envVar()
		raw, ok := a.rt.env(envVar)
		if !ok || raw == "" {
			continue
		}
		if s.validate != nil {
			if err := s.validate(raw); err != nil {
				return fmt.Errorf("invalid value %q in %s: %w", raw, envVar, err)
			}
		}
		if err := f.Value.Set(raw); err != nil {
			return fmt.Errorf("invalid value %q in %s: %w", raw, envVar, err)
		}
	}
	return nil
}

// validateGlobals checks flag values the rest of the CLI may assume valid
// (the per-spec validators, from the same table).
func (a *app) validateGlobals() error {
	for _, s := range globalFlagSpecs {
		if s.validate == nil {
			continue
		}
		if err := s.validate(*s.target(&a.globals).(*string)); err != nil {
			return fmt.Errorf("invalid --%s %q: %w", s.name, *s.target(&a.globals).(*string), err)
		}
	}
	return nil
}
