#!/usr/bin/env bash
# Catalog smoke test, run in a bare container:
#
#   - shellhook emission: `guppy shell-env` installs a stub hook that
#     records project sightings without invoking guppy;
#   - `guppy project list` folds the hook's seen-log, prunes deleted projects
#     (create 3, delete 1 → list shows 2);
#   - `guppy doctor` flags a hand-corrupted project and `--fix` repairs
#     the safe subset (missing files only — user edits survive);
#   - batch mode (`doctor --all`) audits the whole index;
#   - offline: everything above is local-scope only, no network.
#
# Usage: scripts/smoke-catalog.sh [image]   (default: debian:bookworm-slim)
set -euo pipefail

IMAGE="${1:-debian:bookworm-slim}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GUPPY_BIN="${GUPPY_BIN:-$ROOT/bazel-bin/cmd/guppy/guppy_/guppy}"
ENGINE="${CONTAINER_ENGINE:-}"
if [[ -z "$ENGINE" ]]; then
  ENGINE="$(command -v podman || command -v docker || true)"
fi

if [[ -z "$ENGINE" ]]; then
  echo "no container engine found (need podman or docker)" >&2
  exit 1
fi

if [[ ! -x "$GUPPY_BIN" ]]; then
  echo "guppy binary not found: $GUPPY_BIN (bazel build //cmd/guppy first)" >&2
  exit 1
fi

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp "$GUPPY_BIN" "$STAGE/guppy"
chmod 0755 "$STAGE/guppy"

"$ENGINE" run --rm -v "$STAGE:/src:ro,Z" "$IMAGE" bash -c '
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

echo "== install guppy =="
install -m 0755 /src/guppy /usr/local/bin/guppy
export HOME=/root
cd /app 2>/dev/null || { mkdir -p /app && cd /app; }

echo "== scaffold 3 projects =="
for name in alpha beta gamma; do
  guppy new "$name" --template go-cli --var "name=$name" --no-git
done

echo "== guppy project list shows all 3 (scaffold-time recording, no hook yet) =="
# Capture listings before grepping: piping straight into `grep -q`
# closes the pipe early (SIGPIPE under pipefail) and masks the exit.
listing="$(guppy project list)"
echo "$listing" | grep -q alpha
echo "$listing" | grep -q beta
echo "$listing" | grep -q gamma

echo "== install the shellhook (marked rc block) =="
touch "$HOME/.bashrc"
guppy shell-env bash --install
grep -q ">>> guppy shellhook >>>" "$HOME/.bashrc"
# Idempotent: a second install does not duplicate the block.
guppy shell-env bash --install
test "$(grep -c ">>> guppy shellhook >>>" "$HOME/.bashrc")" -eq 1

echo "== the hook is a stub: no guppy invocation, one test + one append =="
# Simulate prompt traffic: source the hook and change directories the
# way an interactive shell would. The manual project has a marker guppy
# never scaffolded — only the hook can put it on the index.
mkdir -p manual/.guppy
printf "schema = 2\n\n[project]\ntemplate = \"go-cli\"\nversion = \"0.2.0\"\n" > manual/.guppy/project.toml
bash -c "
  source <(sed -n \"/>>> guppy shellhook >>>/,/<<< guppy shellhook <<</p\" \"\$HOME/.bashrc\")
  cd /app/alpha && __guppy_hook
  cd /app/manual && __guppy_hook
"
SEEN="$HOME/.local/share/guppy/catalog/seen.log"
test -s "$SEEN"

echo "== guppy project list folds the hook log (manual project appears) =="
listing="$(guppy project list)"
echo "$listing" | grep -q manual
test ! -s "$SEEN"   # log cleared after the fold

echo "== delete 1 of 4 → prune-on-read drops it =="
rm -rf /app/beta
listing="$(guppy project list)"
echo "$listing" | grep -q alpha
! echo "$listing" | grep -q beta
echo "$listing" | grep -q gamma
echo "$listing" | grep -q manual

echo "== doctor flags a hand-corrupted project =="
cd /app/alpha
rm README.md
echo "user edit, precious" >> main.go
# Capture the report first: piping straight into `grep -q` closes the
# pipe early (SIGPIPE under pipefail) and masks the real exit code.
set +e
report="$(guppy doctor 2>&1)"
code=$?
set -e
test "$code" -eq 1
echo "$report" | grep -q "missing"
echo "$report" | grep -q "README.md"
echo "$report" | grep -q "modified"
echo "$report" | grep -q "main.go"

echo "== --fix restores the safe subset, never the user edit =="
set +e
fixout="$(guppy doctor --fix 2>&1)"
set -e
echo "$fixout" | grep -q "fixed"
test -f README.md
grep -q "user edit, precious" main.go

echo "== re-run: only the (legitimate) modification remains =="
set +e
report="$(guppy doctor 2>&1)"
code=$?
set -e
test "$code" -eq 1                        # still drift: the user edit
! echo "$report" | grep -q "missing"       # nothing left to restore

echo "== batch mode audits the whole index =="
set +e
batch="$(guppy doctor --all 2>&1)"
code=$?
set -e
test "$code" -eq 1                        # alpha is still modified
echo "$batch" | grep -q "\[OK \]"        # the others are clean
guppy doctor --all --fix >/dev/null 2>&1 || true

echo "== offline: catalog + doctor are local-scope only =="
GUPPY_OFFLINE=true guppy list >/dev/null
set +e
GUPPY_OFFLINE=true guppy doctor >/dev/null 2>&1
code=$?
set -e
test "$code" -eq 1

echo "== uninstall removes the hook block =="
guppy uninstall --yes >/dev/null 2>&1 || true
! grep -q ">>> guppy shellhook >>>" "$HOME/.bashrc"

echo "SMOKE OK"
'
