package manifest

import (
	"strings"
	"testing"
)

// FuzzParse targets the manifest parser (fuzz-tested). Invariants: Parse never panics on arbitrary
// bytes, never returns both a manifest and an error, and warnings are
// always well-formed messages.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"id = \"x\"\nversion = \"1.0.0\"\nkind = \"cli\"\nlanguage = \"go\"\n",
		"id = 'unterminated\n",
		"[vars]\nname = { prompt = \"Name\", default = \"{{ .name }}\" }\n",
		"[gp]\nbuild = \"bazel build //...\"\ntest = { command = \"bazel test //...\", requires = [\"bazel\"] }\n",
		"extends = [\"base-toolchain\"]\nrequires = { guppy = \">=1.2\" }\n",
		"[[case]]\nname = \"x\"\n",
		"\x00\x01\xff\xfe",
		"id = \"x\"\nbogus_key = 42\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, warnings, err := Parse(data)
		if err != nil {
			if m.ID != "" {
				t.Errorf("manifest returned alongside error: %+v", m)
			}
			return
		}
		for _, w := range warnings {
			if !strings.HasPrefix(w, "unknown manifest key: ") {
				t.Errorf("unexpected warning shape: %q", w)
			}
		}
	})
}
