// dbt-style log formatting (ADR 0009 update): every stderr status line
// carries a dim HH:MM:SS stamp, results carry a bracketed [TAG]
// marker, and multi-item commands end with a completion summary —
// `Completed with 1 error, 2 warnings in 0.42s.`
//
// Stamps and tags are decoration, never data: stdout report bodies are
// tagged but never stamped, so pipes, golden files, and greps stay
// stable (same contract as the colour layer in color.go).
package cli

import (
	"fmt"
	"time"
)

// clock returns the output's time source (injectable for tests).
func (o *output) clock() time.Time {
	if o.now != nil {
		return o.now()
	}
	return time.Now()
}

// stamp renders the dim HH:MM:SS log prefix (dbt's line head).
func (o *output) stamp() string {
	return o.errP().paint(dim, o.clock().Format("15:04:05"))
}

// tagMark renders a bracketed dbt status marker, padded like dbt's:
// "OK " → "[OK ]", "WARN" → "[WARN]".
func tagMark(p painter, s style, name string) string {
	return p.paint(s, fmt.Sprintf("[%-3s]", name))
}

// statusf prints a stamped, tagged status line to stderr:
// `14:23:05  [OK ] updated acme 1.0.0 → 1.1.0`.
func (o *output) statusf(s style, name, format string, args ...any) {
	if o.g.Quiet {
		return
	}
	_, _ = fmt.Fprintf(o.stderr, "%s  %s %s\n", o.stamp(),
		tagMark(o.errP(), s, name), fmt.Sprintf(format, args...))
}

// warnf prints a stamped [WARN] line to stderr — the single warning
// entry point, so style and shape stay uniform.
func (o *output) warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.stderr, "%s  %s %s\n", o.stamp(),
		tagMark(o.errP(), yellow, "WARN"), fmt.Sprintf(format, args...))
}

// summary prints dbt's completion line — the closing signature of
// multi-item commands (doctor, lint, test, template update). Green when
// clean, yellow for warnings, red for errors.
func (o *output) summary(started time.Time, errs, warns int) {
	if o.g.Quiet {
		return
	}
	s := green
	switch {
	case errs > 0:
		s = red
	case warns > 0:
		s = yellow
	}
	secs := o.clock().Sub(started).Seconds()
	_, _ = fmt.Fprintf(o.stderr, "%s  %s\n", o.stamp(), o.errP().paint(s,
		fmt.Sprintf("Completed with %s, %s in %.2fs.",
			plural(errs, "error"), plural(warns, "warning"), secs)))
}

// plural renders "1 error" / "2 errors".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
