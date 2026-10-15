// `guppy test` — the snapshot half of the authoring loop (design §7):
// render the template into a FileSet per matrix case (tests.toml beside
// the manifest; the "default" case is implicit) and compare against
// golden snapshots under testdata/snapshot/<case>/. `guppy test -u`
// rewrites the snapshots. Rendered output is additionally sanity-checked:
// every declared default toggle must actually change the output.
//
// Style: the matrix building, comparison, and coverage logic are pure
// functions; only runTest orchestrates I/O.
package cli

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/templates"
)

const snapshotDir = "testdata/snapshot"

// snapshotGoldenName returns the golden file name for a case.
func snapshotGoldenName(caseName string) string {
	return filepath.Join(snapshotDir, caseName+".toml")
}

func newTestCmd(rt *runtime, out *output) *cobra.Command {
	var update bool
	cmd := &cobra.Command{
		Use:   "test [PATH|TEMPLATE]",
		Short: "Render snapshot tests for a template (golden files, var matrices)",
		Long: `Render the template once per case — the implicit "default" case (var
defaults + default toggles) plus every [[case]] in tests.toml beside the
manifest — and compare the FileSet against golden snapshots under
testdata/snapshot/<case>/. Snapshots are updated with -u.

Vars without a value render as "example-<var>" so snapshots are
deterministic. Exit code 1 on any mismatch — this is what catches a
corrupted built-in.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := ""
			if len(args) == 1 {
				spec = args[0]
			}
			return runTest(cmd.Context(), rt, out, spec, update)
		},
	}
	cmd.Flags().BoolVarP(&update, "update", "u", false, "rewrite golden snapshots instead of comparing")
	return cmd
}

func runTest(ctx context.Context, rt *runtime, out *output, spec string, update bool) error {
	started := out.clock()
	lt, _, err := rt.loadTemplate(ctx, spec)
	if err != nil {
		return InputError(err)
	}
	cases, warnings, err := buildCases(lt.t.Manifest, lt.rootFS())
	if err != nil {
		return Fail(err)
	}
	warns := len(warnings)
	for _, w := range warnings {
		out.warnf("%s", w)
	}

	engine := render.Engine{}
	var failures []string
	for _, c := range cases {
		files, err := engine.Render(lt.t, c.vars, c.toggles)
		if err != nil {
			return Fail(fmt.Errorf("case %q: %w", c.name, err))
		}
		switch {
		case update:
			if lt.rootDir == "" {
				return InputError(fmt.Errorf("cannot update snapshots of built-in template %q: point guppy at a directory copy instead", lt.t.Manifest.ID))
			}
			if err := writeSnapshot(filepath.Join(lt.rootDir, snapshotGoldenName(c.name)), files); err != nil {
				return Fail(err)
			}
			out.infof("case %s: wrote %d snapshot files", c.name, len(files.Files))
		default:
			diffs := compareSnapshot(lt.rootFS(), snapshotGoldenName(c.name), files)
			for _, d := range diffs {
				failures = append(failures, fmt.Sprintf("case %s: %s (run \"guppy test -u\" to update)", c.name, d))
			}
		}
	}

	for _, w := range toggleWarnings(engine, lt.t, cases) {
		out.warnf("%s", w)
		warns++
	}

	if len(failures) > 0 {
		for _, f := range failures {
			_, _ = fmt.Fprintln(out.stdout, tagMark(out.outP(), red, "FAIL")+" "+f)
		}
		out.summary(started, len(failures), warns)
		return Fail(fmt.Errorf("test: %d snapshot mismatches", len(failures)))
	}
	out.summary(started, 0, warns)
	return nil
}

// renderCase is one matrix row: a name plus fully resolved vars/toggles.
type renderCase struct {
	name    string
	vars    render.Vars
	toggles render.Toggles
}

// buildCases is pure: it derives the render matrix from the manifest and
// the authoring root (tests.toml). "default" comes first, then each
// [[case]] row. Case vars are variable expressions rendered against the
// template's resolved vars (so `module_path = "example.com/x/{{ .name }}"`
// works) and override them. Vars without any value get a deterministic
// placeholder so snapshots never depend on the machine or operator.
func buildCases(m manifest.Manifest, root fs.FS) ([]renderCase, []string, error) {
	base := render.Vars{}
	for _, name := range sortedKeys(m.Vars) {
		if m.Vars[name].Default == "" {
			base[name] = "example-" + name
		}
	}
	resolved, _, err := render.ResolveVars(m, base)
	if err != nil {
		return nil, nil, err
	}

	var spec manifest.TestSpec
	var warnings []string
	if data, rerr := fs.ReadFile(root, "tests.toml"); rerr == nil {
		parsed, warns, perr := manifest.ParseTestSpec(data)
		if perr != nil {
			return nil, nil, fmt.Errorf("tests.toml: %w", perr)
		}
		spec = parsed
		warnings = append(warnings, warns...)
	}

	defaultToggles := render.Toggles{}
	maps.Copy(defaultToggles, m.Defaults)

	var cases []renderCase
	add := func(name string, varExprs map[string]string, togOverrides map[manifest.Axis]bool) error {
		vars := render.Vars{}
		maps.Copy(vars, resolved)
		for _, k := range sortedKeys(varExprs) {
			v, err := render.Expr(varExprs[k], vars)
			if err != nil {
				return fmt.Errorf("case %q: var %q: %w", name, k, err)
			}
			vars[k] = v
		}
		toggles := render.Toggles{}
		maps.Copy(toggles, defaultToggles)
		maps.Copy(toggles, togOverrides)
		cases = append(cases, renderCase{name: name, vars: vars, toggles: toggles})
		return nil
	}

	if err := add("default", nil, nil); err != nil {
		return nil, nil, err
	}
	for _, c := range spec.Case {
		if err := add(c.Name, c.Vars, c.Toggles); err != nil {
			return nil, nil, err
		}
	}
	return cases, warnings, nil
}

// toggleWarnings is pure: for the default case, flipping any declared
// default toggle must change the rendered FileSet (§7: "declared default
// toggles actually changed the output"). Warnings, not failures —
// payloads may legitimately leave axes unwired.
func toggleWarnings(engine render.Engine, t templates.Template, cases []renderCase) []string {
	if len(cases) == 0 || len(t.Manifest.Defaults) == 0 {
		return nil
	}
	baseFiles, err := engine.Render(t, cases[0].vars, cases[0].toggles)
	if err != nil {
		return nil
	}
	var out []string
	for _, k := range sortedKeys(t.Manifest.Defaults) {
		flipped := render.Toggles{}
		maps.Copy(flipped, cases[0].toggles)
		flipped[k] = !cases[0].toggles[k]
		f2, err := engine.Render(t, cases[0].vars, flipped)
		if err != nil || sameFileSet(baseFiles, f2) {
			out = append(out, fmt.Sprintf("default toggle %q does not affect the rendered output", k))
		}
	}
	return out
}

func sameFileSet(a, b render.FileSet) bool {
	if len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path || string(a.Files[i].Content) != string(b.Files[i].Content) {
			return false
		}
	}
	return true
}

// snapshotFile is one golden file inside a case snapshot.
type snapshotFile struct {
	Path    string `toml:"path"`
	Mode    string `toml:"mode"`
	Content string `toml:"content"`
}

// caseSnapshot is the golden image of one render case: the whole FileSet
// in one TOML file under testdata/snapshot/ — a single data file per case
// keeps gazelle out (no stray Go packages from rendered main.go files),
// avoids embed dotfile pitfalls, and stays human-diffable in reviews.
type caseSnapshot struct {
	Files []snapshotFile `toml:"file"`
}

// writeSnapshot replaces the case's golden file (deterministic ordering).
func writeSnapshot(path string, files render.FileSet) error {
	snap := caseSnapshot{}
	for _, f := range files.Files {
		snap.Files = append(snap.Files, snapshotFile{
			Path:    f.Path,
			Mode:    fmt.Sprintf("%04o", f.Mode.Perm()),
			Content: string(f.Content),
		})
	}
	data, err := toml.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// compareSnapshot is pure: it diffs a FileSet against the golden file in
// root (embedded for built-ins, disk for local templates). A missing
// golden file yields "missing" diffs for every file.
func compareSnapshot(root fs.FS, name string, files render.FileSet) []string {
	want := map[string]snapshotFile{}
	if data, err := fs.ReadFile(root, name); err == nil {
		var snap caseSnapshot
		if err := toml.Unmarshal(data, &snap); err == nil {
			for _, f := range snap.Files {
				want[f.Path] = f
			}
		}
	}

	var diffs []string
	got := map[string]render.File{}
	for _, f := range files.Files {
		got[f.Path] = f
	}
	for _, p := range sortedKeys(got) {
		f := got[p]
		w, ok := want[p]
		switch {
		case !ok:
			diffs = append(diffs, fmt.Sprintf("missing snapshot file %s", p))
		case w.Content != string(f.Content):
			diffs = append(diffs, fmt.Sprintf("%s differs: %s", p, firstDiffLine([]byte(w.Content), f.Content)))
		case w.Mode != fmt.Sprintf("%04o", f.Mode.Perm()):
			diffs = append(diffs, fmt.Sprintf("%s mode differs: want %s, got %04o", p, w.Mode, f.Mode.Perm()))
		}
	}
	for _, p := range sortedKeys(want) {
		if _, ok := got[p]; !ok {
			diffs = append(diffs, fmt.Sprintf("snapshot file %s has no matching rendered file", p))
		}
	}
	slices.Sort(diffs)
	return diffs
}

// firstDiffLine summarizes the first differing line of two file contents.
func firstDiffLine(want, got []byte) string {
	wl := strings.Split(string(want), "\n")
	gl := strings.Split(string(got), "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, w, g)
		}
	}
	return "contents differ"
}

// sortedKeys returns the map keys sorted (deterministic output). Keyed
// by any ordered type so it serves both string-keyed maps and the
// closed Axis vocabulary.
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
