package manifest

import "slices"

// The axis vocabulary: the closed set of composable template defaults
// (design §3.2 [defaults]). Typed per styleguide T1 — axis keys are
// named values, never bare strings, so a typo can't compile its way
// past every check. The TOML/flag boundaries convert explicitly.
type Axis string

// The known [defaults] axes — the complete vocabulary. Manifest lint
// rejects anything else (with the candidate list), so this set is the
// single source of truth (D4).
const (
	AxisGit      Axis = "git"
	AxisBazel    Axis = "bazel"
	AxisDirenv   Axis = "direnv"
	AxisLinters  Axis = "linters"
	AxisTests    Axis = "tests"
	AxisAgentsMD Axis = "agents_md"
)

// knownAxes is the closed axis set (styleguide T1).
var knownAxes = map[Axis]bool{
	AxisGit:      true,
	AxisBazel:    true,
	AxisDirenv:   true,
	AxisLinters:  true,
	AxisTests:    true,
	AxisAgentsMD: true,
}

// KnownAxis reports whether a is in the axis vocabulary.
func KnownAxis(a Axis) bool { return knownAxes[a] }

// AxisNames converts axes to their wire names — the explicit boundary
// conversion for string APIs like strings.Join (styleguide T1).
func AxisNames(axes []Axis) []string {
	out := make([]string, len(axes))
	for i, a := range axes {
		out[i] = string(a)
	}
	return out
}

// allAxes returns the known axes sorted (deterministic messages and
// candidate lists).
func allAxes() []Axis {
	out := make([]Axis, 0, len(knownAxes))
	for a := range knownAxes {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}
