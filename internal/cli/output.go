// The output capability (styleguide T6): streams, the
// colour policy, and the dbt-style logging surface commands print
// through. Commands receive an *output instead of the whole app, so a
// per-command test constructs a writer and a clock — nothing else.
//
// Output policy (quiet/verbose/colour) is read through the parsed
// globals (g), which the flag machinery fills before any command runs;
// help binds GUPPY_COLOR itself since it renders before setup.
package cli

import (
	"fmt"
	"io"
	"time"
)

// output is the CLI's output capability: the two streams plus the
// decoration/logging policy derived from the parsed global flags.
type output struct {
	g   *globals
	env EnvFunc

	stdout io.Writer
	stderr io.Writer

	now func() time.Time

	outPainter *painter
	errPainter *painter
}

// infof prints a stamped status line to stderr unless quiet
// (diagnostics never pollute stdout — uv rule; dbt.go for the shape).
func (o *output) infof(format string, args ...any) {
	if o.g.Quiet {
		return
	}
	fmt.Fprintf(o.stderr, "%s  %s\n", o.stamp(), fmt.Sprintf(format, args...))
}

// verbose reports whether -v was passed (diff-on-update, extra notes).
func (o *output) verbose() bool { return o.g.Verbose > 0 }

// progressWriter returns the writer for progress/status output of
// long-running operations: stderr, silenced by -q or --no-progress (ADR 0009).
func (o *output) progressWriter() io.Writer {
	if o.g.Quiet || o.g.NoProgress {
		return io.Discard
	}
	return o.stderr
}
