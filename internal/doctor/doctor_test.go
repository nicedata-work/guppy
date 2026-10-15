package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/nicedata-work/guppy/internal/doctor"
	"github.com/nicedata-work/guppy/internal/project"
	"github.com/nicedata-work/guppy/internal/render"
	"github.com/nicedata-work/guppy/internal/scaffold"
	"github.com/nicedata-work/guppy/internal/templates"
)

// scaffoldGoCLI renders the built-in go-cli with the given toggles and
// scaffolds it into dir, returning the marker as `guppy new` writes it.
func scaffoldGoCLI(t *testing.T, dir string, toggles render.Toggles) project.Marker {
	t.Helper()
	r := templates.NewStandard(templates.Builtin(), nil)
	raw, _, err := templates.Load(templates.BuiltinTemplateFS("go-cli"), templates.LoadOptions{
		Scope: templates.ScopeBuiltin, Kind: templates.SourceEmbedded,
	})
	assert.NilError(t, err)
	chain, err := templates.ResolveChain(context.Background(), r, raw)
	assert.NilError(t, err)

	vars, missing, err := render.ResolveVars(chain.Manifest, render.Vars{"name": "widget"})
	assert.NilError(t, err)
	assert.Equal(t, len(missing), 0)

	files, err := (render.Engine{}).Render(chain, vars, toggles)
	assert.NilError(t, err)
	_, err = scaffold.Apply(context.Background(), files, dir, scaffold.ApplyOptions{})
	assert.NilError(t, err)

	return project.Marker{
		Schema:  project.SchemaVersion,
		Project: project.ProjectInfo{Template: "go-cli", Version: chain.Manifest.Version},
		Vars:    vars,
		Toggles: toggles,
	}
}

func defaultToggles() render.Toggles {
	return render.Toggles{"git": true, "bazel": true, "direnv": true, "agents_md": true, "linters": true, "tests": true}
}

func resolver() *templates.Standard { return templates.NewStandard(templates.Builtin(), nil) }

func TestAuditCleanProject(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(rep.Diffs), 0, "diffs: %v", rep.Diffs)

	for _, n := range rep.Notes {
		assert.Assert(t, n != "")
	}
}

func TestAuditFlagsHandCorruptedProject(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())

	assert.NilError(t, os.MkdirAll(filepath.Join(dir, ".guppy"), 0o755))
	assert.NilError(t, os.Remove(filepath.Join(dir, "README.md")))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package hacked\n"), 0o644))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".guppy", "stray"), []byte("x"), 0o644))

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	assert.DeepEqual(t, rep.Diffs, scaffold.Diffs{
		{Kind: scaffold.DiffMissing, Path: "README.md"},
		{Kind: scaffold.DiffUnexpected, Path: filepath.Join(".guppy", "stray")},
		{Kind: scaffold.DiffModified, Path: "main.go"},
	})
	assert.Equal(t, len(rep.Fixable()), 1)
}

func TestFixRestoresMissingButNeverUserEdits(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())
	assert.NilError(t, os.MkdirAll(filepath.Join(dir, ".guppy"), 0o755))
	assert.NilError(t, os.Remove(filepath.Join(dir, "README.md")))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package hacked\n"), 0o644))

	opts := doctor.Options{Root: dir, Marker: m, Resolver: resolver()}
	rep, err := opts.Audit(context.Background())
	assert.NilError(t, err)

	fixed, err := opts.Fix(context.Background(), rep)
	assert.NilError(t, err)
	assert.DeepEqual(t, fixed, []string{"README.md"})

	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	assert.NilError(t, err)
	assert.Assert(t, len(data) > 0)

	data, err = os.ReadFile(filepath.Join(dir, "main.go"))
	assert.NilError(t, err)
	assert.Equal(t, string(data), "package hacked\n")

	rep, err = opts.Audit(context.Background())
	assert.NilError(t, err)
	assert.DeepEqual(t, rep.Diffs, scaffold.Diffs{{Kind: scaffold.DiffModified, Path: "main.go"}})
}

func TestSchemaOneMarkerFallsBackToTemplateDefaults(t *testing.T) {
	dir := t.TempDir()

	m := scaffoldGoCLI(t, dir, defaultToggles())
	m.Schema = 1
	m.Toggles = nil

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	for _, d := range rep.Diffs {
		if d.Path == "AGENTS.md" {
			t.Errorf("schema-1 fallback should expect AGENTS.md (default on): %v", rep.Diffs)
		}
	}
}

func TestRecordedTogglesWinOverDefaults(t *testing.T) {
	dir := t.TempDir()

	toggles := defaultToggles()
	toggles["agents_md"] = false
	m := scaffoldGoCLI(t, dir, toggles)

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	for _, d := range rep.Diffs {
		assert.Assert(t, d.Path != "AGENTS.md", "AGENTS.md reported although agents_md was off: %v", rep.Diffs)
	}
}

func TestAuditNotesUnresolvableTemplate(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())
	m.Project.Template = "vanished"

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	assert.Equal(t, len(rep.Notes), 1)
	assert.Assert(t, strings.Contains(rep.Notes[0], "not resolvable"))
}

func TestAuditNotesStalePin(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())
	m.Project.Version = "0.0.1"

	rep, err := (doctor.Options{Root: dir, Marker: m, Resolver: resolver()}).Audit(context.Background())
	assert.NilError(t, err)
	assert.Assert(t, noteHasPrefix(rep.Notes, "stale pin:"), "notes: %v", rep.Notes)
}

func TestAuditNotesMissingHook(t *testing.T) {
	dir := t.TempDir()
	m := scaffoldGoCLI(t, dir, defaultToggles())
	rep, err := (doctor.Options{
		Root: dir, Marker: m, Resolver: resolver(),
		HookInstalled: func() bool { return false },
	}).Audit(context.Background())
	assert.NilError(t, err)
	assert.Assert(t, noteHasPrefix(rep.Notes, "shellhook not installed"), "notes: %v", rep.Notes)

	rep, err = (doctor.Options{
		Root: dir, Marker: m, Resolver: resolver(),
		HookInstalled: func() bool { return true },
	}).Audit(context.Background())
	assert.NilError(t, err)
	for _, n := range rep.Notes {
		assert.Assert(t, !strings.Contains(n, "shellhook"))
	}
}

func noteHasPrefix(notes []string, prefix string) bool {
	for _, n := range notes {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}
