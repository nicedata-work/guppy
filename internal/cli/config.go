// `guppy config` — view and set guppy's global configuration (§10.3),
// the archetype of the noun-with-verbs shape the CLI follows.
//
//	config get <key>    print one resolved value
//	config set <key> <value>
//	                    write one key into the settings file
//	config list         print every known key with its resolved value
//
// Values are resolved through the normal precedence chain (defaults <
// guppy.toml < env < flags), so `get` and `list` show what the CLI
// actually sees, while `set` edits the settings file — textually, so
// comments and key order survive.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/fileutil"
	"github.com/nicedata-work/guppy/internal/settings"
)

func newConfigCmd(rt *runtime, out *output) *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "View and set guppy global config",
		Long: `View and set guppy's global configuration.

Values resolve through the normal precedence chain — defaults, the
settings file (--config or <config>/guppy/guppy.toml), GUPPY_* env
vars, flags — so get/list always show what commands actually see, and
set writes the settings file (comments preserved). Every key has a
GUPPY_* env equivalent; see "guppy config list".`,
		Example: `  guppy config get template.registry
  guppy config set defaults.linters false
  guppy config set template.dirs "~/templates,/opt/team-templates"
  guppy config list`,
	}
	cfg.AddCommand(newConfigGetCmd(rt, out))
	cfg.AddCommand(newConfigSetCmd(rt, out))
	cfg.AddCommand(newConfigListCmd(rt, out))
	return cfg
}

// configKey is one settable setting: its dotted name, how to read the
// resolved value out of Settings, and how to write a raw value into the
// TOML (typed parse here, so the file never receives garbage).
type configKey struct {
	get func(s settings.Settings) string
	set func(raw string) (section, key string, line string, err error)
}

// boolLine/stringList are the typed writers shared by the key table.
func boolLine(raw string) (string, error) {
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return "", fmt.Errorf("want true or false, got %q", raw)
	}
	return strconv.FormatBool(b), nil
}

func stringListLine(raw string) (string, error) {
	var parts []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("want a comma-separated list, got %q", raw)
	}
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = strconv.Quote(p)
	}
	return "[" + strings.Join(quoted, ", ") + "]", nil
}

func quotedString(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("want a non-empty value")
	}
	return strconv.Quote(raw), nil
}

// configKeys is THE table of settable keys (one definition, nothing to
// drift with the settings schema).
var configKeys = map[string]configKey{
	"defaults.git": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.Git }),
		set: stringSet("defaults", "git", boolLine)},
	"defaults.bazel": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.Bazel }),
		set: stringSet("defaults", "bazel", boolLine)},
	"defaults.direnv": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.Direnv }),
		set: stringSet("defaults", "direnv", boolLine)},
	"defaults.linters": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.Linters }),
		set: stringSet("defaults", "linters", boolLine)},
	"defaults.tests": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.Tests }),
		set: stringSet("defaults", "tests", boolLine)},
	"defaults.agents_md": {get: boolGet(func(s *settings.Settings) *bool { return s.Defaults.AgentsMD }),
		set: stringSet("defaults", "agents_md", boolLine)},
	"template.registry": {get: func(s settings.Settings) string { return s.Template.Registry },
		set: stringSet("template", "registry", quotedString)},
	"template.dirs": {get: func(s settings.Settings) string { return listGet(s.Template.Dirs) },
		set: stringSet("template", "dirs", stringListLine)},
	"shell.hook": {get: func(s settings.Settings) string { return strconv.FormatBool(s.Shell.Hook) },
		set: stringSet("shell", "hook", boolLine)},
	"network.offline": {get: func(s settings.Settings) string { return strconv.FormatBool(s.Network.Offline) },
		set: stringSet("network", "offline", boolLine)},
	"network.insecure": {get: func(s settings.Settings) string { return listGet(s.Network.Insecure) },
		set: stringSet("network", "insecure", stringListLine)},
}

// boolGet renders an optional bool: unset (inherit template default) vs
// an explicit value are different answers and must print differently.
func boolGet(field func(*settings.Settings) *bool) func(settings.Settings) string {
	return func(s settings.Settings) string {
		if v := field(&s); v != nil {
			return strconv.FormatBool(*v)
		}
		return "(unset — template default)"
	}
}

func listGet(list []string) string {
	if len(list) == 0 {
		return "(unset)"
	}
	return strings.Join(list, ",")
}

// stringSet builds a typed setter bound to its [section] key; the
// parser validates and renders the TOML value line.
func stringSet(section, key string, parse func(string) (string, error)) func(string) (string, string, string, error) {
	return func(raw string) (string, string, string, error) {
		line, err := parse(raw)
		if err != nil {
			return "", "", "", fmt.Errorf("invalid value: %w", err)
		}
		return section, key, fmt.Sprintf("%s = %s", key, line), nil
	}
}

func newConfigGetCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one config value (resolved)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			key, ok := configKeys[args[0]]
			if !ok {
				return InputError(fmt.Errorf("unknown config key %q (see \"guppy config list\")", args[0]))
			}
			_, _ = fmt.Fprintln(out.stdout, key.get(rt.settings))
			return nil
		},
	}
}

func newConfigSetCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value in the settings file",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			spec, ok := configKeys[args[0]]
			if !ok {
				return InputError(fmt.Errorf("unknown config key %q (see \"guppy config list\")", args[0]))
			}
			section, key, line, err := spec.set(args[1])
			if err != nil {
				return InputError(fmt.Errorf("%s: %w", args[0], err))
			}
			path := rt.settings.SourceFile
			if path == "" {
				path = rt.layout.SettingsFile()
			}
			if err := writeSetting(path, section, key, line); err != nil {
				return Fail(err)
			}
			out.statusf(green, "OK", "set %s in %s", args[0], path)
			out.infof("env (GUPPY_*) and flags still override the file — precedence: defaults < file < env < flags")
			return nil
		},
	}
}

// writeSetting edits the settings file textually: replace the `key =`
// line under its [section] header, or append the section and line at the
// end. Textual editing keeps the user's comments and key order, which a
// marshal round-trip would destroy.
func writeSetting(path, section, key, line string) error {
	data, readErr := readFileOrEmpty(path)
	if readErr != nil {
		return fmt.Errorf("read %s: %w", path, readErr)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}

	sectionIdx, keyIdx := -1, -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "["+section+"]" {
			sectionIdx = i
		} else if sectionIdx >= 0 && keyIdx < 0 && strings.HasPrefix(strings.TrimSpace(l), key+" ") {
			keyIdx = i
		} else if sectionIdx >= 0 && keyIdx < 0 && strings.HasPrefix(strings.TrimSpace(l), "[") {
			break // next section started; key not present in it
		}
	}
	switch {
	case sectionIdx >= 0 && keyIdx >= 0:
		lines[keyIdx] = line
	case sectionIdx >= 0:
		lines = append(lines[:sectionIdx+1], append([]string{"\t" + line}, lines[sectionIdx+1:]...)...)
	default:
		lines = append(lines, "["+section+"]", "\t"+line, "")
	}
	if err := fileutil.WriteAtomic(path, 0o644, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return err
	}
	return nil
}

// readFileOrEmpty reads path, treating a missing file as empty (set
// creates the file on first write).
func readFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func newConfigListCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every config key with its resolved value",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			source := rt.settings.SourceFile
			if source == "" {
				source = "(no settings file)"
			}
			out.infof("settings file: %s", source)
			w := newTable(out.stdout, "KEY", "VALUE").styled(out.outP())
			for _, name := range sortedConfigKeys() {
				w.row(name, configKeys[name].get(rt.settings))
			}
			w.flush()
			return nil
		},
	}
}

// sortedConfigKeys returns the key names in table order (the map literal
// above is written in display order; Go map iteration is random).
func sortedConfigKeys() []string {
	order := []string{
		"defaults.git", "defaults.bazel", "defaults.direnv",
		"defaults.linters", "defaults.tests", "defaults.agents_md",
		"template.registry", "template.dirs",
		"shell.hook", "network.offline", "network.insecure",
	}
	return order
}
