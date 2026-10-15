// Interactive prompts (uv rule: non-TTY contexts never prompt — they get
// explicit flags or exit code 2).
//
// The prompter is one of the three capability structs commands receive
// instead of the whole app (styleguide T6): it owns the
// per-invocation stdin reader (U13) and the TTY probe (U12).
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// prompter is the interaction capability: the single stdin reader for
// the whole invocation plus the controlling-terminal probe. Prompts are
// diagnostics (U7), so questions print to stderr — injectable like
// every other stream (G3).
type prompter struct {
	stdin io.Reader

	in *bufio.Reader

	stderr io.Writer

	ttyProbe func() bool
}

// newPrompter wraps stdin in the invocation's single buffered reader
// with the default TTY probe installed.
func newPrompter(stdin io.Reader, stderr io.Writer) *prompter {
	return &prompter{stdin: stdin, in: bufio.NewReader(stdin), stderr: stderr, ttyProbe: devTTYProbe}
}

// devTTYProbe is the default controlling-terminal probe: /dev/tty must
// open. Unix-only; the prompter's ttyProbe field is where other
// platforms plug in their console equivalent.
func devTTYProbe() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	tty.Close()
	return true
}

// interactive reports whether stdin is a real terminal. Test harnesses
// pass non-file readers, which counts as non-interactive — exactly like
// CI. A char device alone is not enough (/dev/null is one), so a
// controlling terminal must be present too.
func (p *prompter) interactive() bool {
	f, ok := p.stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return p.ttyProbe()
}

// prompt asks one question on stdin, showing def as the default. An empty
// answer falls back to def; a still-empty answer is an error (required).
func (p *prompter) prompt(question, def string) (string, error) {
	if !p.interactive() {
		return "", fmt.Errorf("non-interactive context: pass --var to set values")
	}
	if def != "" {
		fmt.Fprintf(p.stderr, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.stderr, "%s: ", question)
	}
	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		answer = def
	}
	if answer == "" {
		return "", fmt.Errorf("%s is required", question)
	}
	return answer, nil
}

// confirm asks a yes/no question on stdin. Non-interactive contexts (no
// TTY) must pass --yes explicitly — uv-style CI behavior.
func (p *prompter) confirm(question string) (bool, error) {
	if !p.interactive() {
		return false, fmt.Errorf("non-interactive context: pass --yes to confirm")
	}
	fmt.Fprintf(p.stderr, "%s [y/N] ", question)
	line, err := p.readLine()
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// readLine reads one line from the per-invocation stdin reader. All
// prompts and confirms share the single bufio.Reader created in Execute
// (styleguide U13): input buffered ahead of one prompt — typed or pasted
// faster than prompts are issued — survives into the next prompt instead
// of being re-read from the fd and lost.
func (p *prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read answer: %w", err)
	}
	return line, nil
}
