package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// run executes the CLI in-process and returns (exitCode, stdout, stderr).
func run(args []string, env map[string]string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRootNoArgsPrintsHelp(t *testing.T) {
	code, stdout, stderr := run(nil, nil)
	if code != ExitOK {
		t.Errorf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "guppy") || !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout should be help text, got %q", stdout)
	}
}

func TestVersionOutput(t *testing.T) {
	code, stdout, _ := run([]string{"--version"}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout, "guppy ") {
		t.Errorf("version output = %q, want 'guppy <version>'", stdout)
	}
}

func TestUnknownCommandIsInputError(t *testing.T) {
	code, _, stderr := run([]string{"frobnicate"}, nil)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d", code, ExitInput)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want unknown-command error", stderr)
	}
}

func TestUnknownFlagIsInputError(t *testing.T) {
	code, _, _ := run([]string{"--bogus"}, nil)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d", code, ExitInput)
	}
}

func TestInvalidColorFlagIsInputError(t *testing.T) {
	code, _, stderr := run([]string{"--color", "rainbow", "update"}, nil)
	if code != ExitInput {
		t.Errorf("exit = %d, want %d", code, ExitInput)
	}
	if !strings.Contains(stderr, "--color") {
		t.Errorf("stderr = %q, want --color error", stderr)
	}
}

func TestColorEnvBinding(t *testing.T) {

	code, _, stderr := run([]string{"update"}, map[string]string{"GUPPY_COLOR": "rainbow"})
	if code != ExitInput || !strings.Contains(stderr, "GUPPY_COLOR") {
		t.Errorf("exit = %d, stderr = %q; want input error naming GUPPY_COLOR", code, stderr)
	}
}

func TestOfflineEnvBlocksUpdate(t *testing.T) {
	code, _, stderr := run([]string{"update"}, map[string]string{"GUPPY_OFFLINE": "true"})
	if code != ExitNetwork {
		t.Errorf("exit = %d, want %d (network)", code, ExitNetwork)
	}
	if !strings.Contains(stderr, "offline") {
		t.Errorf("stderr = %q, want offline explanation", stderr)
	}
}

func TestUpdateUnreachableServerIsNetworkError(t *testing.T) {

	code, _, stderr := run([]string{"update"},
		map[string]string{"GUPPY_RELEASE_BASE": "http://127.0.0.1:1"})
	if code != ExitNetwork {
		t.Errorf("exit = %d, want %d (network); stderr: %s", code, ExitNetwork, stderr)
	}
}

// TestSIGINTExits130 pins the signal contract (styleguide U14): the run
// starts from signal.NotifyContext, an in-flight network call honors it,
// and SIGINT exits 130 — no partial state, no success code.
func TestSIGINTExits130(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("self-signal is unix-only")
	}

	release := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer release.Close()

	exit := make(chan int, 1)
	go func() {
		code, _, _ := run([]string{"update"}, map[string]string{"GUPPY_RELEASE_BASE": release.URL})
		exit <- code
	}()

	time.Sleep(300 * time.Millisecond)
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("signal self: %v", err)
	}

	select {
	case code := <-exit:
		if code != ExitInterrupted {
			t.Errorf("exit = %d, want %d (interrupted)", code, ExitInterrupted)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("update did not abort on SIGINT — the request ignored its ctx")
	}
}

// TestPromptReaderPreservesBufferedInput pins (styleguide
// U13): one bufio.Reader per invocation, so answers buffered ahead of an
// earlier prompt survive into the next one instead of being discarded.
func TestPromptReaderPreservesBufferedInput(t *testing.T) {

	p := newPrompter(strings.NewReader("first answer\nsecond answer\n"), io.Discard)
	first, err := p.readLine()
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	second, err := p.readLine()
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if strings.TrimSpace(first) != "first answer" || strings.TrimSpace(second) != "second answer" {
		t.Errorf("answers = %q, %q; buffered lookahead was lost between prompts", first, second)
	}
}

// TestDocsGeneratesDeterministicReference pins the "docs
// generated" requirement: `guppy docs` writes the full cobra reference and
// is byte-deterministic (CI diffs it via scripts/gen-docs.sh --check).
func TestDocsGeneratesDeterministicReference(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := run([]string{"docs", "--out", dir}, nil)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr)
	}
	first := readDocs(t, dir)
	if _, ok := first["guppy.md"]; !ok {
		t.Fatalf("guppy.md missing; have %d files", len(first))
	}
	if !strings.Contains(first["guppy.md"], "guppy new") {
		t.Error("root page should reference the new verb")
	}
	if !strings.Contains(first["guppy_new.md"], "--template") {
		t.Error("guppy_new.md should document flags")
	}

	code, _, _ = run([]string{"docs", "--out", dir}, nil)
	if code != ExitOK {
		t.Fatalf("second run exit = %d", code)
	}
	second := readDocs(t, dir)
	if len(first) != len(second) {
		t.Fatalf("file count changed between runs: %d vs %d", len(first), len(second))
	}
	for name, body := range first {
		if second[name] != body {
			t.Errorf("%s not deterministic", name)
		}
	}

	code, stdout, _ := run([]string{"--help"}, nil)
	if code != ExitOK {
		t.Fatalf("help exit = %d", code)
	}
	if strings.Contains(stdout, "\n  docs") {
		t.Error("docs command should be hidden from root help")
	}
}

// TestHelpIsColoured pins the help-rendering contract (ADR 0009
// update): section headers bold, commands cyan, flags yellow, place-
// holders and defaults dim — and, like everything else, decoration
// only: stripped help equals plain help byte-for-byte.
func TestHelpIsColoured(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		wants []string
	}{
		// template: bold header, a cyan subcommand, a yellow global flag.
		{[]string{"--color", "always", "template"}, []string{
			"\x1b[1mUsage:\x1b[0m",
			"\x1b[36madd",
			"\x1b[33m      --interactive",
		}},
		// dev new: a yellow local flag with a dim default value.
		{[]string{"--color", "always", "dev", "new", "--help"}, []string{
			"\x1b[33m      --kind string",
			"\x1b[2m (default \"cli\")\x1b[0m",
		}},
	} {
		code, colored, _ := run(tc.args, nil)
		if code != ExitOK {
			t.Fatalf("exit = %d, want 0 (%v)", code, tc.args)
		}
		for _, want := range tc.wants {
			if !strings.Contains(colored, want) {
				t.Errorf("help missing %q (%v):\n%q", want, tc.args, colored)
			}
		}

		plainArgs := slices.Clone(tc.args)
		plainArgs[1] = "never"
		code, plain, _ := run(plainArgs, nil)
		if code != ExitOK {
			t.Fatalf("plain exit = %d, want 0", code)
		}
		if got := stripANSI(colored); got != plain {
			t.Errorf("stripped coloured help != plain help (%v):\ncolored: %q\nplain:   %q", tc.args, got, plain)
		}
	}
}

// TestRootHelpLayout pins the curated root help: the two command
// groups in the designed order (blank line between them), hidden
// commands absent, and the global flags in table order ending in
// --help / --version.
func TestRootHelpLayout(t *testing.T) {
	code, stdout, _ := run([]string{"--help"}, nil)
	if code != ExitOK {
		t.Fatalf("help exit = %d", code)
	}

	const commandsHeader = "Available Commands:\n"
	start := strings.Index(stdout, commandsHeader)
	end := strings.Index(stdout, "\nFlags:")
	if start < 0 || end < start {
		t.Fatalf("root help is missing its command/flag sections:\n%s", stdout)
	}

	var gotCommands []string
	for line := range strings.SplitSeq(strings.TrimSuffix(stdout[start+len(commandsHeader):end], "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			gotCommands = append(gotCommands, "") // group separator
			continue
		}
		gotCommands = append(gotCommands, strings.Fields(line)[0])
	}
	wantCommands := []string{"new", "project", "template", "dev", "doctor", "help", "",
		"config", "update", "completion", "uninstall"}
	if !slices.Equal(gotCommands, wantCommands) {
		t.Errorf("Available Commands = %q, want %q", gotCommands, wantCommands)
	}

	flagsStart := strings.Index(stdout[end+2:], "\n")
	flagsSection := stdout[end+2+flagsStart+1:]
	if stop := strings.Index(flagsSection, "\nUse "); stop >= 0 {
		flagsSection = flagsSection[:stop]
	}
	var gotFlags []string
	for line := range strings.SplitSeq(strings.TrimSpace(flagsSection), "\n") {
		for f := range strings.FieldsSeq(line) {
			if strings.HasPrefix(f, "--") {
				gotFlags = append(gotFlags, f)
			}
		}
	}
	wantFlags := []string{"--interactive", "--config", "--directory", "--offline", "--quiet", "--verbose", "--help", "--version"}
	if !slices.Equal(gotFlags, wantFlags) {
		t.Errorf("Flags = %q, want %q", gotFlags, wantFlags)
	}

	for _, hidden := range []string{"shell-env", "--color", "--no-progress"} {
		if strings.Contains(stdout, hidden) {
			t.Errorf("root help should not list %q:\n%s", hidden, stdout)
		}
	}
}

// TestHelpColouredWithEnv pins that GUPPY_COLOR reaches --help even
// though help renders before PersistentPreRunE.
func TestHelpColouredWithEnv(t *testing.T) {
	code, stdout, _ := run([]string{"template"}, map[string]string{"GUPPY_COLOR": "always"})
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "\x1b[1mUsage:\x1b[0m") {
		t.Errorf("GUPPY_COLOR=always should colour help:\n%q", stdout)
	}
}

func readDocs(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(body)
	}
	return out
}

// TestTTYProbeSeam pins the tty seam (styleguide U12/G3): interactive()
// consults the injectable ttyProbe after the char-device check, so a
// platform probe (or a test) can supply the controlling-terminal answer
// without opening /dev/tty.
func TestTTYProbeSeam(t *testing.T) {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s: %v", os.DevNull, err)
	}
	defer func() { _ = devnull.Close() }()
	info, err := devnull.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("%s is not a character device here", os.DevNull)
	}

	probe := func(answer bool) *prompter {
		p := newPrompter(devnull, io.Discard)
		p.ttyProbe = func() bool { return answer }
		p.setEnabled(true)
		return p
	}
	if !probe(true).interactive() {
		t.Error("char device + willing probe + --interactive must be interactive")
	}
	if probe(false).interactive() {
		t.Error("char device + refusing probe must be non-interactive")
	}

	// Prompts are opt-in: without the --interactive gate, even a willing
	// probe and a char-device stdin refuse (the sharpened uv rule).
	unenabled := probe(true)
	unenabled.enabled = false
	if unenabled.interactive() {
		t.Error("without --interactive, prompting must be refused")
	}

	nonTTY := newPrompter(strings.NewReader("x"), io.Discard)
	nonTTY.ttyProbe = func() bool { return true }
	if nonTTY.interactive() {
		t.Error("non-file stdin must be non-interactive regardless of the probe")
	}
}
