// Built-in template library validity: every embedded template must load,
// validate (§3.2 schema), resolve its extends chain (§5), and pass the
// payload linter (§3.3) — the regression net that catches a corrupted
// built-in at `bazel test` time, before `guppy test` snapshots are even
// consulted.
package templates_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/nicedata-work/guppy/internal/manifest"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/templates"
)

func TestBuiltinLibraryIsLintClean(t *testing.T) {
	ids, err := templates.BuiltinIDs()
	if err != nil {
		t.Fatalf("BuiltinIDs: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("no built-in templates embedded")
	}
	r := templates.NewStandard(templates.Builtin(), nil)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			raw, issues, err := templates.Load(templates.BuiltinTemplateFS(id), templates.LoadOptions{
				Scope: templates.ScopeBuiltin, Kind: templates.SourceEmbedded,
			})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if manifest.HasErrors(issues) {
				t.Fatalf("schema issues: %v", issues)
			}
			chain, err := templates.ResolveChain(context.Background(), r, raw)
			if err != nil {
				t.Fatalf("ResolveChain: %v", err)
			}
			for _, i := range render.Lint(chain.Payload, chain.Manifest) {
				if i.Level == manifest.LevelError {
					t.Errorf("lint error: %s", i)
				}
			}

			if _, err := fs.ReadFile(templates.BuiltinTemplateFS(id), "testdata/snapshot/default.toml"); err != nil {
				t.Errorf("missing golden snapshot testdata/snapshot/default.toml (run guppy test -u on the template directory): %v", err)
			}
		})
	}
}
