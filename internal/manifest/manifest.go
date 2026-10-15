// Package manifest parses and validates guppy.toml (design §3.2).
//
// Parse is decode-only: schema validation (the error/warning split,
// extends resolution) lives in validate.go.
package manifest

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
)

// VarSpec declares one interactive variable (design §3.3). Vars render as
// {{ .name }} and may have prompts/defaults; undeclared usage in the
// payload is a lint error, never silently empty.
type VarSpec struct {
	Prompt  string `toml:"prompt"`
	Default string `toml:"default"`
}

// GPCommand is one entry of the [gp] command map (§3.5): the shell
// command line a scaffolded project runs for a verb. TOML value is either
// a bare string (the command) or a table { command, requires } where
// requires lists the [defaults] axes the verb depends on — a verb whose
// required axis is off at scaffold time is dropped from the resolved
// project's map (e.g. `--no-bazel` yields no `bazel build` verb).
type GPCommand struct {
	Command  string
	Requires []Axis
}

// UnmarshalTOML decodes the string-or-table union of a [gp] entry.
func (c *GPCommand) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case string:
		c.Command = val
		return nil
	case map[string]any:
		for key, raw := range val {
			switch key {
			case "command":
				cmd, ok := raw.(string)
				if !ok {
					return fmt.Errorf("command must be a string")
				}
				c.Command = cmd
			case "requires":
				list, ok := raw.([]any)
				if !ok {
					return fmt.Errorf("requires must be a list of [defaults] axes")
				}
				for _, item := range list {
					axis, ok := item.(string)
					if !ok {
						return fmt.Errorf("requires must be a list of [defaults] axes")
					}

					c.Requires = append(c.Requires, Axis(axis))
				}
			default:
				return fmt.Errorf("unknown key %q (want command, requires)", key)
			}
		}
		return nil
	default:
		return fmt.Errorf("gp entry must be a command string or { command, requires }")
	}
}

// Manifest is the content of a template's guppy.toml — the only required
// file of a template.
type Manifest struct {
	ID          string             `toml:"id"`
	Version     string             `toml:"version"`
	Kind        string             `toml:"kind"`
	Language    string             `toml:"language"`
	Description string             `toml:"description"`
	Extends     []string           `toml:"extends"`
	Requires    map[string]string  `toml:"requires"`
	Vars        map[string]VarSpec `toml:"vars"`
	Defaults    map[Axis]bool      `toml:"defaults"`
	Includes    Includes           `toml:"includes"`

	GP map[string]GPCommand `toml:"gp"`
}

// Includes narrows the payload file set (pi's "filters narrow, never
// expand" rule): include patterns can only subtract from what the template
// directory contains.
type Includes struct {
	Include []string `toml:"include"`
	Exclude []string `toml:"exclude"`
}

// isZero reports whether no narrowing is declared (empty = everything
// included, the default).
func (in Includes) isZero() bool { return len(in.Include) == 0 && len(in.Exclude) == 0 }

// Parse decodes a guppy.toml. It returns the manifest plus warnings for
// keys present in the file but absent from the schema (warnings-not-failures,
// design §2.2); schema validation errors are a lint concern (validate.go).
func Parse(data []byte) (Manifest, []string, error) {
	var m Manifest
	md, err := toml.DecodeReader(bytes.NewReader(data), &m)
	if err != nil {
		return Manifest{}, nil, err
	}
	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, "unknown manifest key: "+key.String())
	}
	return m, warnings, nil
}
