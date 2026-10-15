// `guppy template add` — install a template from a source spec
// (design §6, §7). User-scope adds are full trust (you asked for them);
// community adds pass the review gate: manifest, file list, and the
// diff against the previously installed version, then an explicit
// confirm (or --yes in CI — §7).
package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/diffutil"
	"github.com/nicedata-work/guppy/internal/manifest"
)

func newTemplateAddCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Install a template from the registry, git, or a local path",
		Long: `Install a template. Sources:

  <id>                       latest version from the registry
  registry:<id>[@<version>]  version-pinned registry tarball (checksum-verified)
  registry:<name>:<id>[@<v>] same, from the named registry (see
                             [template.registries] in guppy.toml)
  git:<url>[@<ref>]          git checkout, pinned to the resolved commit SHA
  ./path                     a local template directory (copied into scope)
  community:<source>         any of the above into the community scope —
                             requires review + confirm before first use

User-scope installs are trusted immediately; community installs print
the manifest, file list, and (for upgrades) the diff, and require an
interactive confirmation. In non-interactive contexts pass --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := parseTemplateSpec(args[0])
			if err != nil {
				return InputError(err)
			}
			return runAdd(cmd.Context(), rt, out, p, spec, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the interactive review confirmation (CI)")
	return cmd
}

func runAdd(ctx context.Context, rt *runtime, out *output, p *prompter, spec templateSpec, yes bool) error {
	fsys, version, pin, fullTrust, err := rt.fetchSource(ctx, spec)
	if err != nil {
		return err
	}
	m, _, err := readManifest(fsys)
	if err != nil {
		return InputError(err)
	}
	if err := validateSource(spec, m); err != nil {
		return InputError(err)
	}
	if spec.Community && version == "" {
		return InputError(fmt.Errorf("community template %q has no version in guppy.toml", m.ID))
	}

	if spec.Community && !fullTrust {

		oldDir, oldVersion := rt.newestCommunityCopy(m.ID)
		if oldVersion == version {
			return InputError(fmt.Errorf("community template %s@%s is already installed (guppy template update %s for a newer release)", m.ID, version, m.ID))
		}
		approved, err := reviewCommunity(out, p, m, fsys, oldDir, oldVersion, version, yes)
		if err != nil {
			return err
		}
		if !approved {
			out.infof("aborted: %s not installed", m.ID)
			return nil
		}
	}

	dest, err := rt.commitInstall(spec, m.ID, version, pin, fsys)
	if err != nil {
		return Fail(err)
	}
	out.statusf(green, "OK", "installed %s@%s (%s scope, %s) → %s", m.ID, version, spec.scope(), spec.Kind, dest)
	if spec.Community {
		out.statusf(green, "OK", "trusted: recorded for %s@%s", m.ID, version)
	}
	out.infof("use it: guppy new --template %s <path>", m.ID)
	return nil
}

// reviewCommunity is the §7 trust gate: print the manifest, the file
// list, and the diff vs the previously installed version, then require
// an explicit confirmation. Returns whether the install may proceed.
//
// The review content is a diagnostic-by-policy (styleguide U7): it goes
// to stderr with the confirmation prompt, so piping stdout can never
// hide what the user is being asked to trust.
func reviewCommunity(out *output, p *prompter, m manifest.Manifest, fsys fs.FS, oldDir, oldVersion, version string, yes bool) (bool, error) {
	stderr := out.stderr
	fmt.Fprintf(stderr, "review: %s@%s (community)\n", m.ID, version)
	fmt.Fprintf(stderr, "  kind: %s  language: %s\n", orDash(m.Kind), orDash(m.Language))
	if m.Description != "" {
		fmt.Fprintf(stderr, "  description: %s\n", m.Description)
	}
	if len(m.Extends) > 0 {
		fmt.Fprintf(stderr, "  extends: %v\n", m.Extends)
	}

	files, err := listTree(fsys)
	if err != nil {
		return false, err
	}
	fmt.Fprintln(stderr, "files:")
	for _, f := range files {
		fmt.Fprintf(stderr, "  %s\n", f)
	}

	if oldDir != "" {
		fmt.Fprintf(stderr, "changes from %s@%s:\n", m.ID, oldVersion)
		diffs, err := diffTrees(os.DirFS(oldDir), fsys)
		if err != nil {
			return false, err
		}
		if len(diffs) == 0 {
			fmt.Fprintln(stderr, "  (identical content)")
		}
		printDiff(stderr, out.errP(), diffs)
	}

	if yes {
		fmt.Fprintln(stderr, "guppy: --yes: review accepted")
		return true, nil
	}
	if !p.interactive() {
		return false, InputError(fmt.Errorf("community template %s@%s requires review + confirm: inspect the output above and re-run with --yes to accept", m.ID, version))
	}
	answer, err := p.prompt(fmt.Sprintf("trust and install %s@%s?", m.ID, version), "n")
	if err != nil {
		return false, InputError(err)
	}
	switch answer {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// diffTrees renders the unified diff of two template trees.
func diffTrees(old, new fs.FS) ([]string, error) {
	changes, err := diffutil.Trees(old, new)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range changes {
		out = append(out, fmt.Sprintf("  %s", c.Path))
		if c.Status != diffutil.StatusModified {
			out = append(out, fmt.Sprintf("    (%s)", c.Status))
			continue
		}
		oldData, oldErr := fs.ReadFile(old, c.Path)
		newData, newErr := fs.ReadFile(new, c.Path)
		if oldErr != nil || newErr != nil {
			continue
		}
		if isProbablyBinary(oldData) || isProbablyBinary(newData) {
			out = append(out, "    (binary content differs)")
			continue
		}
		for _, line := range diffutil.Unified(c.Path, oldData, newData) {
			out = append(out, "    "+line)
		}
	}
	return out, nil
}

// listTree lists every file in an fs.FS (slash paths, sorted).
func listTree(fsys fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return out, err
}

// isProbablyBinary reports whether data contains a NUL byte in its
// first 8 KiB (the standard heuristic).
func isProbablyBinary(data []byte) bool {
	n := min(len(data), 8192)
	return slices.Contains(data[:n], 0)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
