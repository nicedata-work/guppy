// Output styling — the colour half of ADR 0009 ("ANSI color gated on
// --color/NO_COLOR/TTY detection", stdlib escapes only). Policy:
//
//   - --color always   force colour, even into pipes
//   - --color never    never colour
//   - auto (default)   colour only on a terminal, honouring the NO_COLOR
//     convention and refusing TERM=dumb
//
// Colour only ever decorates: every styled token is also identifiable by
// its text, so pipes, golden files, and scripts stay stable (tests pin
// this by diffing --color always output against --color never with the
// escapes stripped).
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// style is an ANSI SGR attribute set. The zero value is plain text.
type style struct{ code string }

// The palette — deliberately small: emphasis (bold), de-emphasis (dim),
// and the three severities. No underlines, no backgrounds.
var (
	plain  = style{}
	bold   = style{"1"}
	dim    = style{"2"}
	red    = style{"31"}
	green  = style{"32"}
	yellow = style{"33"}
	cyan   = style{"36"}
)

// validateColor is the single --color validity check (styleguide D4:
// one rule, one site — the flag path in validateGlobals and the env
// binding path in applyFlagEnv both call it). It returns why the value
// is invalid, or nil.
func validateColor(v string) error {
	switch v {
	case "auto", "always", "never":
		return nil
	default:
		return fmt.Errorf("must be auto, always, or never")
	}
}

// wrap surrounds text with the style's escape sequence; plain is the
// identity.
func (s style) wrap(text string) string {
	if s.code == "" || text == "" {
		return text
	}
	return "\x1b[" + s.code + "m" + text + "\x1b[0m"
}

// painter applies styles when colour output is enabled for the stream
// it was resolved for (stdout and stderr are decided independently —
// you can pipe data while keeping coloured diagnostics).
type painter struct{ enabled bool }

func (p painter) paint(s style, text string) string {
	if !p.enabled {
		return text
	}
	return s.wrap(text)
}

// painterFor resolves the colour policy for one stream. --color always
// and never are explicit and win outright; auto is the TTY path.
func (o *output) painterFor(w io.Writer) painter {
	switch o.g.Color {
	case "always":
		return painter{enabled: true}
	case "never":
		return painter{}
	}

	if v, ok := o.env("NO_COLOR"); ok && v != "" {
		return painter{}
	}
	if v, ok := o.env("TERM"); ok && v == "dumb" {
		return painter{}
	}
	f, ok := w.(*os.File)
	if !ok {
		return painter{}
	}
	info, err := f.Stat()
	return painter{enabled: err == nil && info.Mode()&os.ModeCharDevice != 0}
}

// outP/errP are the memoized painters for the output's two streams
// (resolved once per invocation).
func (o *output) outP() painter {
	if o.outPainter == nil {
		p := o.painterFor(o.stdout)
		o.outPainter = &p
	}
	return *o.outPainter
}

func (o *output) errP() painter {
	if o.errPainter == nil {
		p := o.painterFor(o.stderr)
		o.errPainter = &p
	}
	return *o.errPainter
}

// visibleLen is the printable width of s with SGR sequences removed
// (byte count, matching the table's historical len()-based widths).
func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			if j := strings.IndexByte(s[i:], 'm'); j >= 0 {
				i += j
				continue
			}
		}
		n++
	}
	return n
}

// padTo right-pads s (by visible width) to w columns.
func padTo(s string, w int) string {
	if n := w - visibleLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// printDiff renders unified-diff lines with the conventional colouring
// (green additions, red removals, cyan hunk/file headers) — used by the
// community review gate (stderr, diagnostic) and -v template updates
// (stdout, data). The stream and its painter are chosen by the caller.
func printDiff(out io.Writer, p painter, lines []string) {
	for _, l := range lines {
		var s style
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			s = bold
		case strings.HasPrefix(l, "@@"):
			s = cyan
		case strings.HasPrefix(l, "+"):
			s = green
		case strings.HasPrefix(l, "-"):
			s = red
		}
		_, _ = fmt.Fprintln(out, p.paint(s, l))
	}
}
