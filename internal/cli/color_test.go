package cli

import (
	"os"
	"strings"
	"testing"
	"time"
)

// stripANSI removes SGR escape sequences (test helper mirroring
// visibleLen's notion of printable text).
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			if j := strings.IndexByte(s[i:], 'm'); j >= 0 {
				i += j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// TestColorAlwaysEmitsANSI pins the --color always contract: table
// headers carry the bold attribute even into a pipe.
func TestColorAlwaysEmitsANSI(t *testing.T) {
	code, stdout, _ := run([]string{"--color", "always", "template", "list"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "\x1b[1m") {
		t.Errorf("expected bold header under --color always, got %q", stdout)
	}
}

// TestColorNeverAndAutoArePlain pins the two off-ramps: --color never
// explicitly, and auto-with-non-TTY (the default in tests and pipes).
func TestColorNeverAndAutoArePlain(t *testing.T) {
	for _, args := range [][]string{
		{"--color", "never", "template", "list"},
		{"template", "list"},
	} {
		code, stdout, _ := run(args, nil)
		if code != ExitOK {
			t.Fatalf("%v: exit = %d, want 0", args, code)
		}
		if strings.Contains(stdout, "\x1b") {
			t.Errorf("%v: unexpected ANSI escapes in %q", args, stdout)
		}
	}
}

// TestColorOnlyDecorates is the stability contract: colour decorates
// but never changes the text. Stripping the escapes from the coloured
// output must yield exactly the uncoloured output (so scripts, golden
// files, and greps are unaffected).
func TestColorOnlyDecorates(t *testing.T) {
	_, colored, _ := run([]string{"--color", "always", "template", "list"}, nil)
	_, plain, _ := run([]string{"--color", "never", "template", "list"}, nil)
	if got := stripANSI(colored); got != plain {
		t.Errorf("stripped coloured output != plain output:\ncolored: %q\nplain:   %q", got, plain)
	}
}

// TestNoColorEnvDisablesAutoColor pins the NO_COLOR convention: a
// character device (which auto would colour) stays plain when
// NO_COLOR is set.
func TestNoColorEnvDisablesAutoColor(t *testing.T) {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s: %v", os.DevNull, err)
	}
	defer func() { _ = devnull.Close() }()
	info, err := devnull.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("%s is not a character device here", os.DevNull)
	}

	runWith := func(env map[string]string) string {
		o := &output{
			g:   &globals{Color: "auto"},
			env: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		}
		return o.painterFor(devnull).paint(bold, "x")
	}
	if got := runWith(nil); got != "\x1b[1mx\x1b[0m" {
		t.Errorf("auto on a char device: got %q, want bold", got)
	}
	if got := runWith(map[string]string{"NO_COLOR": "1"}); got != "x" {
		t.Errorf("NO_COLOR set: got %q, want plain", got)
	}
	if got := runWith(map[string]string{"TERM": "dumb"}); got != "x" {
		t.Errorf("TERM=dumb: got %q, want plain", got)
	}
}

// TestWarnStyleIsYellow: the warning marker is the one stderr style —
// check the dbt shape (stamp + [WARN] tag) renders yellow under
// --color always and stays greppable after stripping.
func TestWarnStyleIsYellow(t *testing.T) {
	fixed := time.Date(2026, 10, 14, 14, 23, 5, 0, time.UTC)
	var stderr strings.Builder
	o := &output{
		g:      &globals{Color: "always"},
		env:    func(string) (string, bool) { return "", false },
		now:    func() time.Time { return fixed },
		stderr: &stderr,
	}
	o.warnf("hello %d", 7)
	got := stderr.String()
	if want := "\x1b[2m14:23:05\x1b[0m  \x1b[33m[WARN]\x1b[0m hello 7\n"; got != want {
		t.Errorf("warnf = %q, want %q", got, want)
	}
	if stripANSI(got) != "14:23:05  [WARN] hello 7\n" {
		t.Errorf("stripped warnf = %q, want plain text", stripANSI(got))
	}
}

// TestSummaryLine pins dbt's completion signature: counts, plural
// forms, duration, and severity colouring (red on errors).
func TestSummaryLine(t *testing.T) {
	fixed := time.Date(2026, 10, 14, 14, 23, 5, 0, time.UTC)
	var stderr strings.Builder
	o := &output{
		g:      &globals{Color: "never"},
		env:    func(string) (string, bool) { return "", false },
		now:    func() time.Time { return fixed.Add(420 * time.Millisecond) },
		stderr: &stderr,
	}
	o.summary(fixed, 1, 2)
	if want := "14:23:05  Completed with 1 error, 2 warnings in 0.42s.\n"; stderr.String() != want {
		t.Errorf("summary = %q, want %q", stderr.String(), want)
	}
}
