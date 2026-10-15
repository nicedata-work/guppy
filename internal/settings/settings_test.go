package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicedata-work/guppy/internal/paths"
)

// envMap adapts a map to the LookupEnv signature.
func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

// writeSettings writes a settings file into an isolated config home and
// returns a Layout pointing at it, so tests never touch the real ~/.config.
func writeSettings(t *testing.T, content string) (paths.Layout, string) {
	t.Helper()
	root := t.TempDir()
	layout := paths.Layout{
		ConfigHome: filepath.Join(root, "cfg"),
		DataHome:   filepath.Join(root, "data"),
		CacheHome:  filepath.Join(root, "cache"),
		BinDir:     filepath.Join(root, "bin"),
	}
	path := layout.SettingsFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return layout, path
}

func emptyEnv() func(string) (string, bool) { return envMap(nil) }

func TestDefaultsWhenNothingSet(t *testing.T) {
	layout := paths.Layout{ConfigHome: t.TempDir()}
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err != nil {
		t.Fatal(err)
	}
	if s.Template.Registry != DefaultRegistry {
		t.Errorf("Registry = %q, want default %q", s.Template.Registry, DefaultRegistry)
	}
	if !s.Shell.Hook {
		t.Error("Shell.Hook = false, want default true")
	}
	if s.Network.Offline {
		t.Error("Network.Offline = true, want default false")
	}
	if s.Defaults.Linters != nil {
		t.Error("Defaults.Linters should be unset (nil) by default")
	}
	if s.SourceFile != "" {
		t.Errorf("SourceFile = %q, want empty", s.SourceFile)
	}
}

func TestFileOverridesDefaults(t *testing.T) {
	layout, path := writeSettings(t, `
[template]
registry = "https://files.example/registry"
[shell]
hook = false
[defaults]
linters = false
`)
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err != nil {
		t.Fatal(err)
	}
	if s.Template.Registry != "https://files.example/registry" {
		t.Errorf("Registry = %q, want file value", s.Template.Registry)
	}
	if s.Shell.Hook {
		t.Error("Shell.Hook should be false from file")
	}
	if s.Defaults.Linters == nil || *s.Defaults.Linters {
		t.Error("Defaults.Linters should be explicit false from file")
	}
	if s.SourceFile != path {
		t.Errorf("SourceFile = %q, want %q", s.SourceFile, path)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	layout, _ := writeSettings(t, `
[template]
registry = "https://files.example/registry"
[network]
offline = false
[defaults]
linters = false
`)
	s, err := Load(LoadOptions{
		Layout: layout,
		LookupEnv: envMap(map[string]string{
			"GUPPY_REGISTRY":         "https://env.example/registry",
			"GUPPY_OFFLINE":          "true",
			"GUPPY_DEFAULTS_LINTERS": "true",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Template.Registry != "https://env.example/registry" {
		t.Errorf("Registry = %q, want env value", s.Template.Registry)
	}
	if !s.Network.Offline {
		t.Error("Network.Offline should be true from env, overriding file")
	}
	if s.Defaults.Linters == nil || !*s.Defaults.Linters {
		t.Error("Defaults.Linters should be true from env, overriding file")
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	layout, _ := writeSettings(t, "")
	offline := true
	s, err := Load(LoadOptions{
		Layout:    layout,
		LookupEnv: envMap(map[string]string{"GUPPY_OFFLINE": "false"}),
		Flags:     FlagOverrides{Offline: &offline},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Network.Offline {
		t.Error("flag (true) should override env (false)")
	}

	s, err = Load(LoadOptions{
		Layout:    layout,
		LookupEnv: envMap(map[string]string{"GUPPY_OFFLINE": "true"}),
		Flags:     FlagOverrides{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Network.Offline {
		t.Error("env should apply when flag is unset")
	}
}

func TestFullPrecedenceChain(t *testing.T) {

	layout, _ := writeSettings(t, `
[template]
registry = "https://file.example/registry"
`)

	offline := true
	s, err := Load(LoadOptions{
		Layout:    layout,
		LookupEnv: envMap(map[string]string{"GUPPY_REGISTRY": "https://env.example/registry", "GUPPY_OFFLINE": "false"}),
		Flags:     FlagOverrides{Offline: &offline},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Template.Registry != "https://env.example/registry" {
		t.Errorf("registry: env beat file — got %q", s.Template.Registry)
	}
	if !s.Network.Offline {
		t.Error("offline: flag beat env")
	}

	if s.Template.Dirs != nil {
		t.Errorf("Dirs should be unset, got %v", s.Template.Dirs)
	}
}

func TestExplicitConfigFlag(t *testing.T) {
	layout, _ := writeSettings(t, "[shell]\nhook = false\n")
	alt := filepath.Join(t.TempDir(), "alt.toml")
	if err := os.WriteFile(alt, []byte("[shell]\nhook = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(LoadOptions{
		Layout:    layout,
		LookupEnv: envMap(map[string]string{"GUPPY_CONFIG": alt}),
		Flags:     FlagOverrides{Config: alt},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Shell.Hook || s.SourceFile != alt {
		t.Errorf("--config file should win; hook=%v source=%q", s.Shell.Hook, s.SourceFile)
	}
}

func TestMissingExplicitConfigIsError(t *testing.T) {
	layout := paths.Layout{ConfigHome: t.TempDir()}
	_, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv(), Flags: FlagOverrides{Config: filepath.Join(t.TempDir(), "nope.toml")}})
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("want config-file-not-found error, got %v", err)
	}

	_, err = Load(LoadOptions{Layout: layout, LookupEnv: envMap(map[string]string{"GUPPY_CONFIG": "/nonexistent/guppy.toml"})})
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("want config-file-not-found error for GUPPY_CONFIG, got %v", err)
	}
}

func TestMissingDefaultConfigIsFine(t *testing.T) {
	layout := paths.Layout{ConfigHome: t.TempDir()}
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err != nil {
		t.Fatalf("absent default settings file must not error: %v", err)
	}
	if s.SourceFile != "" {
		t.Errorf("SourceFile = %q, want empty", s.SourceFile)
	}
}

func TestMalformedTOMLError(t *testing.T) {
	layout, _ := writeSettings(t, "[template\nregistry = 1\n")
	_, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err == nil {
		t.Fatal("malformed TOML must error")
	}
	if !strings.Contains(err.Error(), layout.SettingsFile()) {
		t.Errorf("error should name the file: %v", err)
	}
}

func TestUnknownKeyIsWarningNotError(t *testing.T) {
	layout, _ := writeSettings(t, "[template]\ntypo_key = true\n")
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err != nil {
		t.Fatalf("unknown key must not be fatal: %v", err)
	}
	if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0], "typo_key") {
		t.Errorf("want warning naming typo_key, got %v", s.Warnings)
	}
}

func TestInvalidEnvBoolIsError(t *testing.T) {
	layout := paths.Layout{ConfigHome: t.TempDir()}
	_, err := Load(LoadOptions{Layout: layout, LookupEnv: envMap(map[string]string{"GUPPY_OFFLINE": "yup"})})
	if err == nil || !strings.Contains(err.Error(), "GUPPY_OFFLINE") {
		t.Errorf("want error naming GUPPY_OFFLINE, got %v", err)
	}
}

func TestInvalidRegistryIsError(t *testing.T) {
	layout, _ := writeSettings(t, "[template]\nregistry = \"not a url\"\n")
	_, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Errorf("want registry validation error, got %v", err)
	}

	layout, _ = writeSettings(t, "[template]\nregistry = \"ftp://example/registry\"\n")
	_, err = Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err == nil || !strings.Contains(err.Error(), "http") {
		t.Errorf("want scheme validation error, got %v", err)
	}
}

func TestTemplateDirsPathList(t *testing.T) {
	layout := paths.Layout{ConfigHome: t.TempDir()}
	list := strings.Join([]string{"/a/templates", "/b/templates"}, string(os.PathListSeparator))
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: envMap(map[string]string{"GUPPY_TEMPLATE_DIRS": list})})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Template.Dirs) != 2 || s.Template.Dirs[0] != "/a/templates" || s.Template.Dirs[1] != "/b/templates" {
		t.Errorf("Template.Dirs = %v, want the two entries", s.Template.Dirs)
	}
}

func TestNamedRegistriesParseAndValidate(t *testing.T) {
	layout, _ := writeSettings(t, `
[[template.registries]]
name = "team"
url = "https://github.internal/acme/templates/raw/main"
trust = "full"
[[template.registries]]
name = "labs"
url = "https://labs.example/registry"
`)
	s, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Template.Registries) != 2 {
		t.Fatalf("got %d registries, want 2", len(s.Template.Registries))
	}
	if s.Template.Registries[0].Name != "team" || s.Template.Registries[0].Trust != "full" {
		t.Errorf("first registry = %+v, want team/full", s.Template.Registries[0])
	}
	if s.Template.Registries[1].Trust != "" {
		t.Errorf("unset trust should stay empty (defaults to review at use site)")
	}
}

func TestNamedRegistriesValidation(t *testing.T) {
	cases := map[string]string{
		"bad name": `
[[template.registries]]
name = "Team!"
url = "https://x.example"`,
		"duplicate name": `
[[template.registries]]
name = "team"
url = "https://a.example"
[[template.registries]]
name = "team"
url = "https://b.example"`,
		"bad url": `
[[template.registries]]
name = "team"
url = "not a url"`,
		"bad trust": `
[[template.registries]]
name = "team"
url = "https://x.example"
trust = "implicit"`,
	}
	for name, cfg := range cases {
		layout, _ := writeSettings(t, cfg)
		if _, err := Load(LoadOptions{Layout: layout, LookupEnv: emptyEnv()}); err == nil {
			t.Errorf("%s: Load should fail", name)
		}
	}
}

func TestTokenEnvName(t *testing.T) {
	for in, want := range map[string]string{
		"":         "GUPPY_REGISTRY_TOKEN",
		"team":     "GUPPY_REGISTRY_TEAM_TOKEN",
		"data-eng": "GUPPY_REGISTRY_DATA_ENG_TOKEN",
	} {
		if got := TokenEnvName(in); got != want {
			t.Errorf("TokenEnvName(%q) = %q, want %q", in, got, want)
		}
	}
}
