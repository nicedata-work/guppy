// `guppy dev` — the template authoring loop (design §7), gathered under
// one noun:
//
//	dev new <DIR>       scaffold a minimal template shell (guppy.toml +
//	                    template/) to start authoring from
//	dev list            list locally developed templates (ledger records
//	                    whose source is a local directory)
//	dev lint <PATH>     static verification: manifest schema, extends
//	                    chain, payload substitutions
//	dev test <PATH>     render snapshot tests (golden files, var matrix)
//	dev publish <PATH>  the registry-PR flow: lint + test, build the
//	                    versioned tarball, print the index.json entry
//
// The registry itself is static files with no upload API (§10.3), so
// publish prepares everything the PR needs and prints the manual steps —
// it never invents a push protocol.
package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/archive"
	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/registry"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/scaffold"
)

func newDevCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	dev := &cobra.Command{
		Use:   "dev",
		Short: "Develop guppy templates (new, list, lint, test, publish)",
		Long: `Develop guppy templates: the authoring loop of design §7.

guppy dev new scaffolds a minimal template shell to start from; dev
lint and dev test verify a template statically and by render snapshot;
dev publish prepares the registry contribution (tarball + index entry).
Local templates are developed in place — dev list shows the ones the
install ledger knows about.`,
	}
	dev.AddCommand(newDevNewCmd(rt, out))
	dev.AddCommand(newDevListCmd(rt, out))
	dev.AddCommand(newLintCmd(rt, out))
	dev.AddCommand(newTestCmd(rt, out))
	dev.AddCommand(newDevPublishCmd(rt, out, p))
	return dev
}

// newDevNewCmd scaffolds a minimal template shell: a manifest with one
// required var and a one-file payload — everything else is the author's.
func newDevNewCmd(rt *runtime, out *output) *cobra.Command {
	var kind, lang, id, description string
	cmd := &cobra.Command{
		Use:   "new [DIR]",
		Short: "Create a new template shell to start authoring from",
		Long: `Create a minimal template shell: a guppy.toml (id, version, kind,
language, one required "name" var) and a template/ payload with a
starter README.

DIR defaults to the template id (created in the cwd). An existing
non-empty directory is refused. Then iterate in place:

  guppy dev lint <dir> && guppy dev test <dir>`,
		Args: cobra.MaximumNArgs(1),
		Example: `  guppy dev new dbt-v2
  guppy dev new my-template --kind lib --lang python`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ""
			if len(args) == 1 {
				dir = args[0]
			}
			return runDevNew(cmd.Context(), rt, out, dir, id, kind, lang, description)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "template id (default: the directory's base name)")
	cmd.Flags().StringVar(&kind, "kind", "cli", "template kind: cli, service, lib, app, docs, infra")
	cmd.Flags().StringVar(&lang, "lang", "none", "template language: go, ts, python, rust, none")
	cmd.Flags().StringVar(&description, "description", "", "one-line description for the registry index")
	return cmd
}

func runDevNew(ctx context.Context, rt *runtime, out *output, dir, id, kind, lang, description string) error {
	if dir == "" {
		if id == "" {
			return InputError(fmt.Errorf("specify DIR or --id"))
		}
		dir = id
	}
	if id == "" {
		id = filepath.Base(dir)
	}
	if id == "." || id == ".." || id == "/" || strings.ContainsAny(id, "/\\") {
		return InputError(fmt.Errorf("invalid template id %q (pass --id)", id))
	}
	if description == "" {
		description = "A guppy template"
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return InputError(err)
	}
	if st, err := os.Stat(abs); err == nil {
		if !st.IsDir() {
			return InputError(fmt.Errorf("%s exists and is not a directory", abs))
		}
		if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
			return InputError(fmt.Errorf("directory %s already exists and is not empty", abs))
		}
	}

	manifest := fmt.Sprintf(`id          = %q
version     = "0.1.0"
kind        = %q
language    = %q
description = %q

[vars]
name = { prompt = "Project name" }
`, id, kind, lang, description)
	readme := "# {{ .name }}\n\nScaffolded from the " + id + " template — edit template/ to make it yours.\n"

	files := render.NewFileSet([]render.File{
		{Path: "guppy.toml", Mode: 0o644, Content: []byte(manifest)},
		{Path: "template/README.md", Mode: 0o644, Content: []byte(readme)},
	})
	if _, err := scaffold.Apply(ctx, files, abs, scaffold.ApplyOptions{}); err != nil {
		return Fail(err)
	}

	// Seed the default-case snapshot so the first `guppy dev test` is
	// green — the same goldens `dev test -u` would write (placeholder
	// vars, default toggles). A fresh shell must not start in the red.
	if lt, _, err := rt.loadTemplate(ctx, dir); err == nil {
		if cases, _, cerr := buildCases(lt.t.Manifest, lt.rootFS()); cerr == nil {
			engine := render.Engine{}
			for _, c := range cases {
				rendered, rerr := engine.Render(lt.t, c.vars, c.toggles)
				if rerr != nil {
					break
				}
				if werr := writeSnapshot(filepath.Join(abs, snapshotGoldenName(c.name)), rendered); werr != nil {
					break
				}
			}
		}
	}

	out.statusf(green, "OK", "created template shell %s", abs)
	out.infof("iterate: guppy dev lint %s && guppy dev test %s", dir, dir)
	out.infof("ship it: guppy dev publish %s", dir)
	return nil
}

// newDevListCmd lists ledger records whose source is a local directory —
// the templates developed in place.
func newDevListCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List locally developed templates",
		Long: `List the locally developed templates the install ledger knows about —
records whose source is a directory (added with "guppy template add
<dir>"). Developed templates live where you author them; guppy only
remembers where.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDevList(rt, out)
		},
	}
}

func runDevList(rt *runtime, out *output) error {
	records, err := rt.distStore().Load()
	if err != nil {
		return Fail(err)
	}
	var local []dist.Record
	for _, rec := range records {
		if rec.Kind == dist.KindLocal {
			local = append(local, rec)
		}
	}
	if len(local) == 0 {
		_, _ = fmt.Fprintln(out.stdout, "no locally developed templates (start one with \"guppy dev new\", register it with \"guppy template add <dir>\")")
		return nil
	}
	w := newTable(out.stdout, "ID", "VERSION", "SOURCE").styled(out.outP())
	for _, rec := range local {
		w.row(rec.ID, orDash(rec.Version), rec.Pin)
	}
	w.flush()
	return nil
}

// newDevPublishCmd implements the publish half of §7: validate, package,
// and emit exactly what a static-registry PR needs.
func newDevPublishCmd(rt *runtime, out *output, p *prompter) *cobra.Command {
	var outDir string
	var skipTests bool
	cmd := &cobra.Command{
		Use:   "publish [PATH]",
		Short: "Prepare a template's registry contribution (tarball + index entry)",
		Long: `Publish a template the way the registry accepts contributions: a
versioned tarball and one index.json entry per version.

Publish runs the same gates consumers' CI runs — guppy dev lint and
guppy dev test — then writes guppy-<id>-<version>.tar.gz (the template
wrapped in its id/ directory) plus its sha256 into --out, and prints
the index.json Entry to append. Uploading is manual by design: the
registry is static files, and first-party contributions are a PR to the
guppy-registry repository.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  guppy dev publish ./my-template
  guppy dev publish . --out ~/registry-staging`,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := ""
			if len(args) == 1 {
				spec = args[0]
			}
			return runDevPublish(cmd.Context(), rt, out, p, spec, outDir, skipTests)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "./dist", "directory for the built tarball")
	cmd.Flags().BoolVar(&skipTests, "skip-tests", false, "skip the snapshot-test gate (lint always runs)")
	return cmd
}

func runDevPublish(ctx context.Context, rt *runtime, out *output, p *prompter, spec, outDir string, skipTests bool) error {
	lt, _, err := rt.loadTemplate(ctx, spec)
	if err != nil {
		return InputError(err)
	}
	m := lt.t.Manifest

	if err := runLint(ctx, rt, out, spec); err != nil {
		return fmt.Errorf("publish gate failed: %w", err)
	}
	if !skipTests {
		if err := runTest(ctx, rt, out, spec, false); err != nil {
			return fmt.Errorf("publish gate failed: %w", err)
		}
	}

	// Package: the authoring tree wrapped in a single <id>/ top-level
	// directory (the wrapping the client strips — publishing guide).
	var files []archive.File
	err = fs.WalkDir(lt.rootFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(lt.rootFS(), path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, archive.File{Rel: m.ID + "/" + path, Mode: info.Mode(), Body: body})
		return nil
	})
	if err != nil {
		return Fail(err)
	}
	tarball, err := archive.WriteTarGz(files)
	if err != nil {
		return Fail(err)
	}
	sum := sha256.Sum256(tarball)
	sha := hex.EncodeToString(sum[:])

	name := fmt.Sprintf("guppy-%s-%s.tar.gz", m.ID, m.Version)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return InputError(fmt.Errorf("create %s: %w", outDir, err))
	}
	dest := filepath.Join(outDir, name)
	if err := os.WriteFile(dest, tarball, 0o644); err != nil {
		return Fail(err)
	}
	if err := os.WriteFile(dest+".sha256", []byte(sha+"\n"), 0o644); err != nil {
		return Fail(err)
	}
	out.statusf(green, "OK", "wrote %s (sha256 %s)", dest, sha)

	entry := registryEntry{
		ID:          m.ID,
		Version:     m.Version,
		Kind:        m.Kind,
		Language:    m.Language,
		Description: m.Description,
		Keywords:    []string{registry.CommunityKeyword, m.Kind, m.Language},
		Tarball:     name,
		SHA256:      sha,
	}
	encoded, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Fail(err)
	}
	_, _ = fmt.Fprintf(out.stdout, "append to index.json:\n%s\n", encoded)
	out.infof("then upload %s (+ .sha256) beside the index and open the registry PR", dest)
	return nil
}

// registryEntry mirrors one index.json entry (schema 1, publishing
// guide) — the client enforces these field rules on install.
type registryEntry struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Kind        string   `json:"kind"`
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Tarball     string   `json:"tarball"`
	SHA256      string   `json:"sha256"`
}
