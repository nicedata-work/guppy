package manifest

import (
	"strings"
	"testing"
)

func TestGPCommandUnmarshal(t *testing.T) {
	t.Run("string form", func(t *testing.T) {
		m := mustParse(t, "id = \"x\"\n[gp]\nbuild = \"make\"\n")
		got := m.GP["build"]
		if got.Command != "make" || len(got.Requires) != 0 {
			t.Errorf("GP[build] = %+v, want command=make no requires", got)
		}
	})
	t.Run("table form", func(t *testing.T) {
		m := mustParse(t, "id = \"x\"\n[gp]\nbuild = { command = \"make all\", requires = [\"bazel\", \"git\"] }\n")
		got := m.GP["build"]
		if got.Command != "make all" {
			t.Errorf("command = %q, want %q", got.Command, "make all")
		}
		if len(got.Requires) != 2 || got.Requires[0] != "bazel" || got.Requires[1] != "git" {
			t.Errorf("requires = %v, want [bazel git]", got.Requires)
		}
	})
	t.Run("unknown key is an error", func(t *testing.T) {
		_, _, err := Parse([]byte("[gp]\nbuild = { command = \"x\", when = \"bazel\" }\n"))
		if err == nil || !strings.Contains(err.Error(), "unknown key") {
			t.Errorf("Parse error = %v, want unknown-key error", err)
		}
	})
	t.Run("wrong value type is an error", func(t *testing.T) {
		_, _, err := Parse([]byte("[gp]\nbuild = 42\n"))
		if err == nil || !strings.Contains(err.Error(), "gp entry") {
			t.Errorf("Parse error = %v, want gp-entry union error", err)
		}
	})
}

func TestGPValidation(t *testing.T) {
	base := func(gp string) Manifest {
		m := mustParse(t, "id = \"x\"\nversion = \"1.0.0\"\nkind = \"cli\"\nlanguage = \"go\"\n"+gp)
		return m
	}
	tests := []struct {
		name    string
		gp      string
		wantErr string
		want    []string
	}{
		{
			name: "baseline complete is clean",
			gp:   "[gp]\nbuild = \"b\"\ntest = \"t\"\nlint = \"l\"\n",
		},
		{
			name:    "doctor is reserved",
			gp:      "[gp]\nbuild = \"b\"\ntest = \"t\"\nlint = \"l\"\ndoctor = \"rm -rf /\"\n",
			wantErr: "reserved",
		},
		{
			name:    "invalid verb name",
			gp:      "[gp]\nbuild = \"b\"\ntest = \"t\"\n\"Build All\" = \"b\"\n",
			wantErr: "invalid [gp] verb",
		},
		{
			name:    "empty command",
			gp:      "[gp]\nbuild = \"\"\ntest = \"t\"\nlint = \"l\"\n",
			wantErr: "has no command",
		},
		{
			name:    "unknown requires axis",
			gp:      "[gp]\nbuild = { command = \"b\", requires = [\"widgets\"] }\ntest = \"t\"\nlint = \"l\"\n",
			wantErr: "unknown default axis",
		},
		{
			name: "missing lint is a warning for language templates",
			gp:   "[gp]\nbuild = \"b\"\ntest = \"t\"\n",
			want: []string{`missing baseline [gp] verb "lint"`},
		},
		{
			name: "missing lint is fine for infra templates",
			gp:   "",
			want: []string{`missing baseline [gp] verb "build"`, `missing baseline [gp] verb "test"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := Validate(base(tt.gp))
			if tt.wantErr == "" && HasErrors(issues) {
				t.Fatalf("unexpected errors: %v", issues)
			}
			if tt.wantErr != "" {
				for _, i := range issues {
					if i.Level == LevelError && strings.Contains(i.Message, tt.wantErr) {
						return
					}
				}
				t.Fatalf("no error containing %q in %v", tt.wantErr, issues)
			}
			for _, want := range tt.want {
				found := false
				for _, i := range issues {
					if i.Level == LevelWarning && strings.Contains(i.Message, want) {
						found = true
					}
				}
				if !found {
					t.Errorf("no warning containing %q in %v", want, issues)
				}
			}
		})
	}
}

func TestMergeChainMergesGP(t *testing.T) {
	parent := Manifest{
		ID: "base", Version: "1.0.0", Kind: "infra", Language: "none",
		GP: map[string]GPCommand{
			"build": {Command: "make"},
			"test":  {Command: "make test"},
		},
	}
	child := Manifest{
		ID: "go-cli", Version: "1.0.0", Kind: "cli", Language: "go",
		GP: map[string]GPCommand{
			"test": {Command: "gotest -short"},
			"lint": {Command: "golangci-lint"},
		},
	}
	out := MergeChain([]Manifest{parent}, child)
	want := map[string]string{
		"build": "make",
		"test":  "gotest -short",
		"lint":  "golangci-lint",
	}
	if len(out.GP) != len(want) {
		t.Fatalf("GP = %v, want %v", out.GP, want)
	}
	for verb, cmd := range want {
		if got := out.GP[verb].Command; got != cmd {
			t.Errorf("GP[%s] = %q, want %q", verb, got, cmd)
		}
	}
}

// mustParse parses a manifest or fails the test (schema issues allowed —
// these fixtures test decode, not validation).
func mustParse(t *testing.T, src string) Manifest {
	t.Helper()
	m, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return m
}
