// tests.toml — the var-matrix input of `guppy test` (design §7): golden
// snapshot cases declared beside the manifest. The "default" case always
// exists (var defaults + default toggles); [[case]] entries add matrix
// rows that override vars and/or toggles.
package manifest

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/BurntSushi/toml"
)

// TestCase is one row of the render matrix: a name plus optional var and
// toggle overrides on top of the template's defaults.
type TestCase struct {
	Name    string            `toml:"name"`
	Vars    map[string]string `toml:"vars"`
	Toggles map[Axis]bool     `toml:"toggles"`
}

// TestSpec is the content of tests.toml.
type TestSpec struct {
	Case []TestCase `toml:"case"`
}

var caseNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseTestSpec decodes tests.toml. Unknown keys are warnings
// (warnings-not-failures); case-name problems are errors — a snapshot
// without a stable name has nowhere to live.
func ParseTestSpec(data []byte) (TestSpec, []string, error) {
	var spec TestSpec
	md, err := toml.DecodeReader(bytes.NewReader(data), &spec)
	if err != nil {
		return TestSpec{}, nil, err
	}
	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, "unknown tests.toml key: "+key.String())
	}
	seen := map[string]bool{"default": true}
	for i, c := range spec.Case {
		switch {
		case c.Name == "":
			return TestSpec{}, warnings, fmt.Errorf("case #%d: name is required", i+1)
		case !caseNamePattern.MatchString(c.Name):
			return TestSpec{}, warnings, fmt.Errorf("case %q: name must match [a-z0-9-]", c.Name)
		case seen[c.Name]:
			return TestSpec{}, warnings, fmt.Errorf("case %q: duplicate name", c.Name)
		}
		seen[c.Name] = true
	}
	return spec, warnings, nil
}
