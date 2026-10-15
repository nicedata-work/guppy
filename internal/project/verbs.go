// Package project is the project-scoped face of the CLI (design §3.5):
// `guppy project <verb>`, shimmed as `gp`, dispatched via the [gp] command
// map in .guppy/project.toml. Discovery (marker.go) and verb execution
// (runner.go).
package project

// Command is one shell command line the project runs for a verb.
type Command string

// VerbMap is the [gp] command map from .guppy/project.toml: verb name →
// command line. Templates write it at scaffold time; doctor is native and
// never appears here.
type VerbMap map[string]Command

// Baseline verb names every first-party template ships (§3.5). Templates
// may add verbs beyond these; `doctor` is built in and never overridden.
const (
	VerbBuild = "build"
	VerbTest  = "test"
	VerbLint  = "lint"
)

// BaselineVerbs lists the built-in-expected verb names in display order.
func BaselineVerbs() []string { return []string{VerbBuild, VerbTest, VerbLint} }

// NativeVerbs are implemented by the CLI itself, never dispatched through
// the command map (§3.5: "doctor is built in and never overridden").
func NativeVerbs() []string { return []string{"doctor"} }
