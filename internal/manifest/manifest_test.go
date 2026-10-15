package manifest

import (
	"strings"
	"testing"
)

// TestValidateMatchesSpecTable pins the lint error list to the §3.2 table:
// id/version/kind/language well-formed are errors, nothing else errors at
// the schema level.
func TestValidateMatchesSpecTable(t *testing.T) {
	valid := Manifest{
		ID: "go-cli", Version: "0.3.1", Kind: "cli", Language: "go",
	}
	tests := []struct {
		name    string
		mutate  func(*Manifest)
		wantErr bool
		wantMsg string
	}{
		{"valid manifest has no issues", func(*Manifest) {}, false, ""},
		{"missing id", func(m *Manifest) { m.ID = "" }, true, "id is required"},
		{"bad id", func(m *Manifest) { m.ID = "Go_Cli!" }, true, "invalid id"},
		{"missing version", func(m *Manifest) { m.Version = "" }, true, "version is required"},
		{"non-semver version", func(m *Manifest) { m.Version = "1.0" }, true, "invalid version"},
		{"missing kind", func(m *Manifest) { m.Kind = "" }, true, "kind is required"},
		{"unknown kind", func(m *Manifest) { m.Kind = "website" }, true, "invalid kind"},
		{"missing language", func(m *Manifest) { m.Language = "" }, true, "language is required"},
		{"unknown language", func(m *Manifest) { m.Language = "cobol" }, true, "invalid language"},
		{"unknown defaults axis is an error with candidates", func(m *Manifest) {
			m.Defaults = map[Axis]bool{"widgets": true}
		}, true, "unknown default axis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid
			m.Defaults = map[Axis]bool{"git": true}
			tt.mutate(&m)
			issues := Validate(m)
			if got := HasErrors(issues); got != tt.wantErr {
				t.Fatalf("HasErrors = %v, want %v (issues: %v)", got, tt.wantErr, issues)
			}
			if tt.wantErr {
				found := false
				for _, i := range issues {
					if i.Level == LevelError && strings.Contains(i.Message, tt.wantMsg) {
						found = true
					}
				}
				if !found {
					t.Errorf("no error mentioning %q in %v", tt.wantMsg, issues)
				}
			}
		})
	}
}

func TestUnknownDefaultsAxisIsErrorWithCandidates(t *testing.T) {
	m := Manifest{ID: "x", Version: "1.0.0", Kind: "lib", Language: "none",
		Defaults: map[Axis]bool{"widgets": true},
		GP:       map[string]GPCommand{"build": {Command: "make"}, "test": {Command: "make test"}}}
	issues := Validate(m)
	if !HasErrors(issues) {
		t.Fatalf("unknown axis must be an error (T1: closed vocabulary), got %v", issues)
	}
	var found bool
	for _, i := range issues {
		if i.Level == LevelError && strings.Contains(i.Message, `unknown default axis "widgets"`) {
			found = true

			for _, want := range []string{"agents_md", "bazel", "direnv", "git", "linters", "tests"} {
				if !strings.Contains(i.Message, want) {
					t.Errorf("candidate list missing %q: %s", want, i.Message)
				}
			}
		}
	}
	if !found {
		t.Fatalf("issues = %v, want one error about widgets with candidates", issues)
	}
}

func TestMergeChainChildWins(t *testing.T) {
	parent := Manifest{
		ID: "base", Version: "1.0.0", Kind: "infra", Language: "none",
		Requires: map[string]string{"guppy": ">=0.1"},
		Vars:     map[string]VarSpec{"name": {Prompt: "Project name"}, "extra": {Prompt: "Extra"}},
		Defaults: map[Axis]bool{"git": true, "direnv": true},
		Includes: Includes{Exclude: []string{"template/scratch/**"}},
	}
	mid := Manifest{ID: "mid", Version: "1.0.0", Kind: "lib", Language: "go",
		Defaults: map[Axis]bool{"direnv": false}}
	child := Manifest{
		ID: "go-cli", Version: "0.3.1", Kind: "cli", Language: "go",
		Extends:  []string{"base"},
		Vars:     map[string]VarSpec{"name": {Prompt: "Name of the CLI"}},
		Defaults: map[Axis]bool{"tests": true},
	}
	got := MergeChain([]Manifest{parent, mid}, child)

	if got.ID != "go-cli" || got.Version != "0.3.1" || got.Kind != "cli" {
		t.Errorf("identity must stay the child's, got %v", got)
	}
	if got.Requires["guppy"] != ">=0.1" {
		t.Errorf("requires must be inherited, got %v", got.Requires)
	}
	if got.Vars["name"].Prompt != "Name of the CLI" {
		t.Errorf("child var must win: %v", got.Vars["name"])
	}
	if _, ok := got.Vars["extra"]; !ok {
		t.Errorf("parent var must be inherited: %v", got.Vars)
	}
	if !got.Defaults["git"] || got.Defaults["direnv"] || !got.Defaults["tests"] {
		t.Errorf("defaults merge wrong: %v", got.Defaults)
	}
	if len(got.Includes.Exclude) != 1 || got.Includes.Exclude[0] != "template/scratch/**" {
		t.Errorf("includes must fall back to the nearest narrowing parent: %v", got.Includes)
	}
}

func TestMergeChainChildIncludesWin(t *testing.T) {
	parent := Manifest{ID: "p", Defaults: map[Axis]bool{"git": true},
		Includes: Includes{Exclude: []string{"a"}}}
	child := Manifest{ID: "c", Includes: Includes{Exclude: []string{"b"}}}
	got := MergeChain([]Manifest{parent}, child)
	if len(got.Includes.Exclude) != 1 || got.Includes.Exclude[0] != "b" {
		t.Errorf("child includes must win: %v", got.Includes)
	}
}

func TestParseTestSpec(t *testing.T) {
	spec, warns, err := ParseTestSpec([]byte(`
[[case]]
name = "minimal"
[case.vars]
module_path = "example.com/x/{{ .name }}"
[case.toggles]
agents_md = false
`))
	if err != nil {
		t.Fatalf("ParseTestSpec: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("warnings = %v", warns)
	}
	if len(spec.Case) != 1 || spec.Case[0].Name != "minimal" {
		t.Fatalf("cases = %v", spec.Case)
	}
	if spec.Case[0].Vars["module_path"] == "" || spec.Case[0].Toggles["agents_md"] {
		t.Errorf("case body wrong: %+v", spec.Case[0])
	}
}

func TestParseTestSpecRejectsBadNames(t *testing.T) {
	for _, body := range []string{
		"[[case]]\n",
		"[[case]]\nname = \"Bad Name\"\n",
		"[[case]]\nname = \"default\"\n",
		"[[case]]\nname = \"dup\"\n[[case]]\nname = \"dup\"\n",
	} {
		if _, _, err := ParseTestSpec([]byte(body)); err == nil {
			t.Errorf("ParseTestSpec(%q) should fail", body)
		}
	}
}
