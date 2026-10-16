// Shellhook emission (design §9): `guppy shell-env` prints the hook
// snippet for eval in a zsh/bash rc file, or installs it into the rc file
// as a marked block (--install) so `guppy uninstall` can remove it.
//
// Stub-hot-path rule (§9): the hook does no path resolution, no registry
// reads, and never invokes guppy. Per prompt it compares $PWD to the last
// seen directory and, when the directory changed *and* contains a marker,
// appends the path to a seen-log. Every index reader (guppy list, doctor,
// new) folds that log into index.toml off the hot path. A broken hook
// degrades to a stale index — never to a broken shell.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicedata-work/guppy/internal/fileutil"
	"github.com/nicedata-work/guppy/internal/project"
)

// Hook marker lines delimit the block shell-env installs into rc files.
// `guppy uninstall` removes whole marked blocks, never guesses.
const (
	HookMarkerOpen  = "# >>> guppy shellhook >>>"
	HookMarkerClose = "# <<< guppy shellhook <<<"
)

// SeenLogName is the seen-log file name inside the catalog dir (§10.2).
const SeenLogName = "seen.log"

// Hook is the emitted shellhook snippet. It is identical for zsh and
// bash: it self-selects on ZSH_VERSION/BASH_VERSION, so one eval line in
// any rc file does the right thing. The only substitution is the seen-log
// path, baked in at emission time from the resolved layout.
type Hook struct {
	SeenLog string
}

// Script renders the hook. The seen-log path is single-quote embedded
// (with POSIX escaping) so spaces and quotes in the path cannot break
// the snippet.
func (h Hook) Script() string {
	return fmt.Sprintf(`# >>> guppy shellhook >>>
# Emitted by "guppy shell-env". Stub on the hot path: one
# directory comparison per prompt, one append when entering a managed
# project — no guppy invocation, no path resolution. A broken hook
# degrades to a stale index, never to a broken shell.
__guppy_hook() {
  if [ "$PWD" != "${__guppy_last_dir:-}" ]; then
    __guppy_last_dir="$PWD"
    if [ -f "$PWD/%s" ]; then
      printf '%%s\n' "$PWD" >> %s 2>/dev/null || :
    fi
  fi
}
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null && add-zsh-hook precmd __guppy_hook
elif [ -n "${BASH_VERSION:-}" ]; then
  case ";${PROMPT_COMMAND:-};" in
    *";__guppy_hook;"*) ;;
    *) PROMPT_COMMAND="__guppy_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
  esac
fi
# <<< guppy shellhook <<<`, project.MarkerRelPath, shellQuote(h.SeenLog))
}

// shellQuote embeds s as a single-quoted POSIX word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InstallHook writes the hook into rcFile as a marked block (idempotent:
// an existing block is replaced in place). The parent directory and the
// seen-log's directory are created so the first prompt append succeeds.
func InstallHook(rcFile string, h Hook) error {
	if err := os.MkdirAll(filepath.Dir(h.SeenLog), 0o755); err != nil {
		return fmt.Errorf("create catalog dir: %w", err)
	}
	block := h.Script() + "\n"

	existing, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", rcFile, err)
	}
	if idx := strings.Index(string(existing), HookMarkerOpen); idx >= 0 {

		out, err := replaceMarkedBlock(string(existing), HookMarkerOpen, HookMarkerClose, block)
		if err != nil {
			return fmt.Errorf("%s: %w", rcFile, err)
		}
		return writeNewFile(rcFile, out)
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(block)
	return writeNewFile(rcFile, b.String())
}

// replaceMarkedBlock removes the first open..close block and inserts
// replacement in its place. An unterminated open marker is an error —
// never a destructive guess.
func replaceMarkedBlock(content, open, close, replacement string) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	start, end := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r\n")
		switch trimmed {
		case open:
			if start == -1 {
				start = i
			}
		case close:
			if start != -1 && end == -1 {
				end = i
			}
		}
	}
	if start == -1 {
		return content, nil
	}
	if end == -1 {
		return "", fmt.Errorf("unterminated %s block (open at line %d); left untouched", open, start+1)
	}
	var b strings.Builder
	b.WriteString(strings.Join(lines[:start], ""))
	b.WriteString(replacement)
	b.WriteString(strings.Join(lines[end+1:], ""))
	return b.String(), nil
}

// writeNewFile writes atomically (fileutil.WriteAtomic — CreateTemp +
// rename) so an rc file is never truncated by a crash mid-write.
func writeNewFile(path, content string) error {
	return fileutil.WriteAtomic(path, 0o644, []byte(content))
}

// HookInstalled reports whether any of the given rc files carries the
// shellhook block (doctor's missing-hook check).
func HookInstalled(rcFiles []string) bool {
	for _, rc := range rcFiles {
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), HookMarkerOpen) {
			return true
		}
	}
	return false
}
