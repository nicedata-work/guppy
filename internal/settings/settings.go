// Package settings loads and layers guppy's user settings (design §10.3).
//
// One TOML file (<config>/guppy/guppy.toml), uv-style: flat, commented,
// every key env-overridable. Precedence, uv convention:
//
//	defaults < guppy.toml < env (GUPPY_*) < flags
//
// Project settings live only in .guppy/project.toml and affect the project
// face (gp) — the global CLI never reads project state, keeping the trust
// boundary of §4.1 intact.
package settings

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/nicedata-work/guppy/internal/paths"
)

// DefaultRegistry is the canonical registry endpoint (§10.3).
const DefaultRegistry = "https://raw.githubusercontent.com/nicedata-work/guppy-registry/main"

// Defaults holds the user-level overrides of template [defaults] (§10.3).
// Pointers distinguish "unset" (inherit template default) from an explicit
// false — every one of these has a --no-* flag inverse per the uv rule.
type Defaults struct {
	Git      *bool `toml:"git"`
	Bazel    *bool `toml:"bazel"`
	Direnv   *bool `toml:"direnv"`
	Linters  *bool `toml:"linters"`
	Tests    *bool `toml:"tests"`
	AgentsMD *bool `toml:"agents_md"`
}

// Template holds template-sourcing settings.
type Template struct {
	Dirs []string `toml:"dirs"`

	Registry string `toml:"registry"`

	Registries []NamedRegistry `toml:"registries"`
}

// NamedRegistry is one team registry entry.
type NamedRegistry struct {
	Name string `toml:"name"`

	URL string `toml:"url"`

	Git string `toml:"git"`

	Ref string `toml:"ref"`

	Trust string `toml:"trust"`
}

// Shell holds shellhook settings (§9).
type Shell struct {
	Hook bool `toml:"hook"`
}

// Network holds network policy settings.
type Network struct {
	Offline  bool     `toml:"offline"`
	Insecure []string `toml:"insecure"`
}

// Settings is the fully resolved configuration: defaults, layered with the
// settings file, then env, then flags.
type Settings struct {
	Defaults Defaults
	Template Template
	Shell    Shell
	Network  Network

	SourceFile string

	Warnings []string
}

// Default returns the built-in defaults (the bottom of the precedence chain).
func Default() Settings {
	return Settings{
		Template: Template{Registry: DefaultRegistry},
		Shell:    Shell{Hook: true},
	}
}

// FlagOverrides carries the values of precedence-top flags, but only for
// flags the user actually set (nil = not set = inherit from env/file).
type FlagOverrides struct {
	Offline *bool
	Config  string
}

// LoadOptions parameterizes Load for the CLI and for tests.
type LoadOptions struct {
	Flags FlagOverrides

	Layout paths.Layout

	LookupEnv func(string) (string, bool)
}

func (o LoadOptions) layout() paths.Layout {
	if o.Layout != (paths.Layout{}) {
		return o.Layout
	}
	return paths.Resolve()
}

func (o LoadOptions) env(key string) (string, bool) {
	if o.LookupEnv != nil {
		return o.LookupEnv(key)
	}
	return os.LookupEnv(key)
}

// Load resolves settings through the precedence chain:
// defaults < guppy.toml < env < flags.
//
// A missing default settings file is not an error (defaults win). A missing
// *explicitly requested* file (--config / GUPPY_CONFIG) is an error: the
// user asked for a file that isn't there. Malformed TOML is an error with
// line/column context.
func Load(opts LoadOptions) (Settings, error) {
	s := Default()

	path, explicit := settingsFilePath(opts)
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			warns, err := applyFile(&s, data)
			if err != nil {
				return Settings{}, fmt.Errorf("%s: %w", path, err)
			}
			s.Warnings = append(s.Warnings, warns...)
			s.SourceFile = path
		case os.IsNotExist(err) && !explicit:

		case os.IsNotExist(err):
			return Settings{}, fmt.Errorf("config file not found: %s", path)
		default:
			return Settings{}, fmt.Errorf("read %s: %w", path, err)
		}
	}

	if err := applyEnv(&s, opts.env); err != nil {
		return Settings{}, err
	}

	if opts.Flags.Offline != nil {
		s.Network.Offline = *opts.Flags.Offline
	}

	if err := validate(&s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// settingsFilePath resolves which file to read and whether it was requested
// explicitly (flag > env > default location).
func settingsFilePath(opts LoadOptions) (path string, explicit bool) {
	if opts.Flags.Config != "" {
		return opts.Flags.Config, true
	}
	if v, ok := opts.env("GUPPY_CONFIG"); ok && v != "" {
		return v, true
	}
	return opts.layout().SettingsFile(), false
}

// applyFile layers the TOML file over s and returns warnings for unknown
// keys (warnings-not-failures).
func applyFile(s *Settings, data []byte) (warnings []string, err error) {
	md, err := toml.Decode(string(data), s)
	if err != nil {
		return nil, err
	}
	for _, key := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("unknown settings key: %s", key.String()))
	}
	return warnings, nil
}

// envSpec maps each GUPPY_* env var to its effect. This table is the single
// source of truth for env binding of settings (§6: every option has a
// GUPPY_* equivalent).
func applyEnv(s *Settings, lookup func(string) (string, bool)) error {

	boolVars := []struct {
		key   string
		field *bool
	}{
		{"GUPPY_OFFLINE", &s.Network.Offline},
		{"GUPPY_SHELL_HOOK", &s.Shell.Hook},
	}
	for _, v := range boolVars {
		if err := applyBoolEnv(v.key, lookup, v.field); err != nil {
			return err
		}
	}

	if err := applyDefaultsEnv(s, lookup); err != nil {
		return err
	}

	if raw, ok := lookup("GUPPY_REGISTRY"); ok && raw != "" {
		s.Template.Registry = raw
	}
	if raw, ok := lookup("GUPPY_TEMPLATE_DIRS"); ok && raw != "" {
		s.Template.Dirs = splitPathList(raw)
	}
	if raw, ok := lookup("GUPPY_INSECURE_HOSTS"); ok && raw != "" {
		s.Network.Insecure = splitPathList(raw)
	}
	return nil
}

func applyDefaultsEnv(s *Settings, lookup func(string) (string, bool)) error {
	for _, v := range []struct {
		key string
		set func(bool)
	}{
		{"GUPPY_DEFAULTS_GIT", func(b bool) { s.Defaults.Git = &b }},
		{"GUPPY_DEFAULTS_BAZEL", func(b bool) { s.Defaults.Bazel = &b }},
		{"GUPPY_DEFAULTS_DIRENV", func(b bool) { s.Defaults.Direnv = &b }},
		{"GUPPY_DEFAULTS_LINTERS", func(b bool) { s.Defaults.Linters = &b }},
		{"GUPPY_DEFAULTS_TESTS", func(b bool) { s.Defaults.Tests = &b }},
		{"GUPPY_DEFAULTS_AGENTS_MD", func(b bool) { s.Defaults.AgentsMD = &b }},
	} {
		raw, ok := lookup(v.key)
		if !ok || raw == "" {
			continue
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid boolean %q in %s: must be true or false", raw, v.key)
		}
		v.set(b)
	}
	return nil
}

func applyBoolEnv(key string, lookup func(string) (string, bool), field *bool) error {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("invalid boolean %q in %s: must be true or false", raw, key)
	}
	*field = b
	return nil
}

// splitPathList splits on the OS path list separator (: on unix, ; on
// windows) and drops empties.
func splitPathList(raw string) []string {
	var out []string
	for _, p := range filepath.SplitList(raw) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// TokenEnvName is the environment variable holding the bearer token for
// a named registry (GUPPY_REGISTRY_<NAME>_TOKEN, dashes → underscores;
// GUPPY_REGISTRY_TOKEN covers the default registry).
func TokenEnvName(name string) string {
	if name == "" {
		return "GUPPY_REGISTRY_TOKEN"
	}
	return "GUPPY_REGISTRY_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_TOKEN"
}

// nameRe constrains registry names: labels, search qualifiers, and
// registry:<name>:<id> disambiguators all share this vocabulary.
var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// validate checks settings the loader must refuse to proceed with (exit
// code 2 — unresolved input — at the CLI layer).
func validate(s *Settings) error {
	if s.Template.Registry != "" {
		u, err := url.Parse(s.Template.Registry)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid registry URL %q: must be an absolute http(s) URL", s.Template.Registry)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("invalid registry URL %q: scheme must be http or https", s.Template.Registry)
		}
	}
	names := map[string]bool{}
	for i, r := range s.Template.Registries {
		where := fmt.Sprintf("template.registries[%d]", i)
		if !nameRe.MatchString(r.Name) {
			return fmt.Errorf("%s: invalid name %q (want [a-z0-9-]+)", where, r.Name)
		}
		if names[r.Name] {
			return fmt.Errorf("%s: duplicate registry name %q", where, r.Name)
		}
		names[r.Name] = true
		switch {
		case r.URL == "" && r.Git == "":
			return fmt.Errorf("%s: needs either url or git", where)
		case r.URL != "" && r.Git != "":
			return fmt.Errorf("%s: url and git are mutually exclusive", where)
		case r.URL != "":
			u, err := url.Parse(r.URL)
			if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("%s: invalid url %q: must be an absolute http(s) URL", where, r.URL)
			}
		}
		switch r.Trust {
		case "", "review", "full":
		default:
			return fmt.Errorf("%s: invalid trust %q (want \"review\" or \"full\")", where, r.Trust)
		}
	}
	return nil
}
