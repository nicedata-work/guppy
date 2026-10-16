// `guppy project` — the project-scoped face of the CLI (design §3.5),
// shimmed as `gup`: walk up to the nearest .guppy/project.toml, refuse to
// run outside one, and dispatch the verb through the marker's [gup] command
// map. Verbs run through direnv when the project is direnv-wired (the
// pinned toolchain environment, §3.4) and directly otherwise — the
// explicit fallback that must keep working without direnv (design §3.5).
//
// Flag parsing is pass-through after the verb (`gup test --test_filter=…`
// reaches bazel intact); global flags stay usable before it.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/project"
)

func newProjectCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project [VERB] [ARGS...]",
		Short: "Create and manage projects (shimmed as gup)",
		Long: `Create and manage projects, and run project verbs — the project-scoped
face of guppy, shimmed as "gup".

Built-in subcommands cover the project lifecycle: new, list, jump, and
clear. Inside a guppy-managed project, any other verb dispatches
through that project's [gup] command map (.guppy/project.toml), walking
up to the nearest marker first. Built-in names shadow map verbs: a
project defining its own "list" verb must invoke it as
gup's explicit fallback or rename it.

Map verbs execute from the project root through direnv when the
project is direnv-wired (the pinned toolchain environment); without
direnv they run directly — Bazel pins (.bazelversion) still give
version hermeticity. An unknown verb prints the resolved command map.
"doctor" is built in (project scope) and never appears in the map.`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: nil,
		Example: `  guppy project new dbt-project --template dbt-v2
  guppy project list
  gup build            # inside a project (direnv shim)
  guppy project test  # the explicit fallback, works without direnv
  guppy --directory path/to/proj project lint`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProject(cmd.Context(), rt, out, p, args)
		},
	}

	cmd.Flags().SetInterspersed(false)
	cmd.AddCommand(newNewCmd(rt, out, p))
	cmd.AddCommand(newListCmd(rt, out))
	cmd.AddCommand(newJumpCmd(rt, out))
	cmd.AddCommand(newClearCmd(rt, out))
	return cmd
}

func runProject(ctx context.Context, rt *runtime, out *output, p *prompter, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return Fail(err)
	}
	root, marker, err := project.FindUp(cwd)
	if err != nil {
		if errors.Is(err, project.ErrOutsideProject) {
			return InputError(err)
		}
		return Fail(err)
	}

	if len(args) == 0 || isHelpArg(args[0]) {
		_, _ = fmt.Fprintf(out.stdout, "guppy-managed project: %s (template %s@%s)\n",
			root, marker.Project.Template, marker.Project.Version)
		_, _ = fmt.Fprintf(out.stdout, "verbs:\n%s", project.FormatMap(marker.GUP))
		_, _ = fmt.Fprintf(out.stdout, "  doctor  audit this project (built in)\n")
		return nil
	}

	verb := args[0]

	warnMisplacedGlobals(out, args[1:])
	if verb == "doctor" {

		return runDoctorAt(ctx, rt, out, root, marker, slices.Contains(args[1:], "--fix"))
	}
	command, ok := marker.GUP[verb]
	if !ok {
		return InputError(fmt.Errorf("unknown verb %q for this project — verbs:\n%s  doctor  (built in)",
			verb, project.FormatMap(marker.GUP)))
	}

	direnvPath, _ := exec.LookPath("direnv")
	mode := project.SelectRunner(root, direnvPath, project.EnvrcExists(root))
	if mode == project.ModeDirect && direnvPath == "" && project.EnvrcExists(root) {
		out.infof("direnv not found: running verb %q outside the pinned environment (Bazel pins still apply)", verb)
	}

	code, err := project.Run(project.RunOptions{
		Ctx:    ctx,
		Root:   root,
		Mode:   mode,
		Direnv: direnvPath,
		Verb:   verb,
		Args:   args[1:],
		Env:    os.Environ(),
		Stdin:  p.stdin,
		Stdout: out.stdout,
		Stderr: out.stderr,
	}, command)
	if err != nil {
		return Fail(err)
	}
	if mode == project.ModeDirenv && code != 0 {
		out.infof("hint: if direnv reported an authorization error, run: direnv allow %s", root)
	}
	if code != 0 {
		return Fail(fmt.Errorf("verb %q failed (exit %d)", verb, code))
	}
	return nil
}

// isHelpArg reports whether the first arg selects help rather than a verb
// (DisableFlagParsing-style pass-through means -h/--help are ours to
// interpret when they appear in verb position).
func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

// warnMisplacedGlobals warns about known guppy global flags that landed
// in the pass-through region after the project verb: with
// SetInterspersed(false) they are sent to the verb's tool, not parsed by
// guppy, and the user believed guppy got them (styleguide U3). Each
// distinct flag is named once, with the fix.
func warnMisplacedGlobals(out *output, args []string) {
	seen := map[string]bool{}
	for _, arg := range args {
		name, ok := passthroughGlobalFlag(arg)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		out.warnf("%s reached the verb, not guppy — put global flags before the verb (gup %s <verb>)", "--"+name, "--"+name)
	}
}

// passthroughGlobalFlag reports whether arg is a known guppy global flag
// spelled in the pass-through region (long form, with or without
// =value; the two mnemonic shorthands), returning its canonical long
// name. --help is never one: after the verb it belongs to the invoked
// tool (U3).
func passthroughGlobalFlag(arg string) (string, bool) {
	if rest, ok := strings.CutPrefix(arg, "--"); ok {
		name, _, _ := strings.Cut(rest, "=")
		_, known := globalFlagByName(name)
		return name, known
	}
	switch arg {
	case "-q":
		return "quiet", true
	case "-v":
		return "verbose", true
	}
	return "", false
}

// InvokedAsGUP reports whether the binary was invoked under the `gup` name
// (design §3.5: one binary, two faces — argv[0] selects the face). The
// direnv setup links `gup` into .direnv/bin/, so the shim is ambient
// inside a project and absent outside one.
func InvokedAsGUP(argv0 string) bool {
	base := argv0
	if i := strings.LastIndexAny(base, `/\\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	return base == "gup"
}

// ArgsFor maps an invocation to CLI args: as `gup` the binary is guppy in
// project mode (design §3.5).
func ArgsFor(argv0 string, args []string) []string {
	if InvokedAsGUP(argv0) {
		return append([]string{"project"}, args...)
	}
	return args
}
