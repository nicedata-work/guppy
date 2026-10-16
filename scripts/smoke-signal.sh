#!/usr/bin/env bash
# Signal-handling smoke test, run in a bare container with real signals —
# the in-process tests can't
# prove what a kernel-delivered signal does:
#
#   - SIGINT to a running project verb exits 130, the child is killed
#     (no "sleep 60" ever completes), stderr says "interrupted";
#   - SIGTERM does the same (exit 130);
#   - SIGINT to `guppy new` mid-scaffold leaves no half-scaffolded tree:
#     either the scaffold finished (exit 0, complete tree) or it was
#     interrupted (exit 130, target directory absent — rollback ran).
#
# Usage: scripts/smoke-signal.sh [image]   (default: debian:bookworm-slim)
set -euo pipefail

IMAGE="${1:-debian:bookworm-slim}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GUPPY_BIN="${GUPPY_BIN:-$ROOT/.bazel/bin/cmd/guppy/guppy_/guppy}"
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
ln -sf /usr/local/bin/guppy /usr/local/bin/gp
export HOME=/root
cd /app 2>/dev/null || { mkdir -p /app && cd /app; }

echo "== scaffold a project and add a deliberately slow verb =="
# --no-direnv: the interrupt test is about signal handling, not the
# pinned environment — keep the runner selection deterministic.
guppy new widget --template go-cli --var name=widget --no-git --no-direnv
cd widget
sed -i "/^\[gp\]$/a\\  slow = \"sleep 60\"" .guppy/project.toml
grep -q "slow" .guppy/project.toml

echo "== SIGINT to a running verb: exit 130, child killed =="
gp slow 2>interrupt.err &
pid=$!
sleep 1
kill -INT "$pid"
set +e
wait "$pid"
code=$?
set -e
test "$code" -eq 130
grep -q "interrupted" interrupt.err
rm -f interrupt.err

echo "== SIGTERM to a running verb: exit 130 =="
gp slow 2>interrupt.err &
pid=$!
sleep 1
kill -TERM "$pid"
set +e
wait "$pid"
code=$?
set -e
test "$code" -eq 130
grep -q "interrupted" interrupt.err
rm -f interrupt.err

echo "== SIGINT to guppy new: finished or rolled back, never half-scaffolded =="
cd /app
guppy new interrupted --template go-cli --var name=interrupted --no-git --no-direnv &
pid=$!
# Land the interrupt somewhere inside the run (startup, mid-write, or —
# if guppy is fast — just after completion); every landing spot must be
# consistent: exit 0 with a complete tree, or exit 130 with no tree at all.
sleep 0.05
kill -INT "$pid" 2>/dev/null || true
set +e
wait "$pid"
code=$?
set -e
case "$code" in
  0)
    test -f interrupted/.guppy/project.toml
    test -f interrupted/main.go
    ;;
  130)
    test ! -e interrupted
    ;;
  *)
    echo "unexpected exit $code for interrupted scaffold" >&2
    exit 1
    ;;
esac

echo "SMOKE OK"
'
