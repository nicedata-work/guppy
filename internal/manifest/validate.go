// Validation and lint issues for guppy.toml (design §3.2).
//
// The error/warning split is the spec table of §3.2, faithfully: schema
// fields well-formed (error); undeclared payload vars and unresolvable/
// cyclic extends are errors too, but they need the payload or the resolver,
// so they are reported by internal/render.Lint and internal/templates
// respectively. Everything else is a warning — warnings-not-failures
// (design §2.2).
package manifest

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Level classifies an Issue: errors fail `guppy lint` (exit 1) and refuse
// `guppy new`; warnings are printed and otherwise ignored.
type Level int

const (
	// LevelWarning: report and proceed.
	LevelWarning Level = iota
	// LevelError: lint fails; templates with errors are unusable.
	LevelError
)

// String returns the lowercase level name used in lint output.
func (l Level) String() string {
	if l == LevelError {
		return "error"
	}
	return "warning"
}

// Issue is one lint finding. Path is the payload-relative file the issue
// is about, or "" for manifest-level findings.
type Issue struct {
	Level   Level
	Path    string
	Message string
}

// String renders the issue for CLI output: `error: path: message`.
func (i Issue) String() string {
	if i.Path != "" {
		return fmt.Sprintf("%s: %s: %s", i.Level, i.Path, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Level, i.Message)
}

// HasErrors reports whether any issue is error-level.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Level == LevelError {
			return true
		}
	}
	return false
}

// The enum sets of §3.2.
var (
	kinds     = map[string]bool{"cli": true, "service": true, "lib": true, "app": true, "docs": true, "infra": true}
	languages = map[string]bool{"go": true, "ts": true, "python": true, "rust": true, "none": true}
)

var (
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	verbPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// ReservedVerbs are never template-declared: they are built into the CLI
// (design §3.5 — `doctor` is native and cannot be overridden).
var ReservedVerbs = map[string]bool{"doctor": true}

// BaselineVerbs are the verb names every first-party template ships
// (design §3.5).
var BaselineVerbs = []string{"build", "test", "lint"}

// Validate checks the schema rules of design §3.2 — the lint error table,
// nothing more: payload-dependent rules live in the render package,
// resolution-dependent ones in the templates package.
func Validate(m Manifest) []Issue {
	var issues []Issue
	errf := func(format string, args ...any) {
		issues = append(issues, Issue{Level: LevelError, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(format string, args ...any) {
		issues = append(issues, Issue{Level: LevelWarning, Message: fmt.Sprintf(format, args...)})
	}

	if m.ID == "" {
		errf("id is required")
	} else if !idPattern.MatchString(m.ID) {
		errf("invalid id %q: must match [a-z0-9-]", m.ID)
	}

	if m.Version == "" {
		errf("version is required")
	} else if _, err := semver.StrictNewVersion(m.Version); err != nil {
		errf("invalid version %q: must be semver (e.g. 0.1.0)", m.Version)
	}

	if m.Kind == "" {
		errf("kind is required")
	} else if !kinds[m.Kind] {
		errf("invalid kind %q: must be one of cli, service, lib, app, docs, infra", m.Kind)
	}

	if m.Language == "" {
		errf("language is required")
	} else if !languages[m.Language] {
		errf("invalid language %q: must be one of go, ts, python, rust, none", m.Language)
	}

	for axis := range m.Defaults {
		if !KnownAxis(axis) {
			errf("unknown default axis %q (known axes: %s)", axis, strings.Join(AxisNames(allAxes()), ", "))
		}
	}

	for _, verb := range sortedKeys(m.GP) {
		entry := m.GP[verb]
		switch {
		case ReservedVerbs[verb]:
			errf("[gp] verb %q is reserved: doctor is built in and never overridden", verb)
		case !verbPattern.MatchString(verb):
			errf("invalid [gp] verb %q: must match [a-z][a-z0-9-]*", verb)
		}
		if entry.Command == "" {
			errf("[gp] verb %q has no command", verb)
		}
		for _, axis := range entry.Requires {
			if !KnownAxis(axis) {
				errf("[gp] verb %q requires unknown default axis %q (known axes: %s)", verb, axis, strings.Join(AxisNames(allAxes()), ", "))
			}
		}
	}
	for _, verb := range []string{"build", "test"} {
		if _, ok := m.GP[verb]; !ok {
			warnf("missing baseline [gp] verb %q (every first-party template ships build/test/lint)", verb)
		}
	}
	if m.Language != "none" {
		if _, ok := m.GP["lint"]; !ok {
			warnf("missing baseline [gp] verb %q (every first-party template ships build/test/lint)", "lint")
		}
	}

	return issues
}

// sortedKeys returns the map keys sorted (deterministic output).
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
