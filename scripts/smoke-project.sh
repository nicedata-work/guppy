#!/usr/bin/env bash
# Project-mode smoke test, run in a bare container:
#
#   - `guppy new` scaffolds a project with .guppy/project.toml;
#   - `guppy project test` runs the pinned toolchain WITHOUT direnv
#     (the explicit fallback — bazelisk + .bazelversion give version
#     hermeticity on their own);
#   - `gup` outside a project refuses (exit 2) — the shim is project
#     context, never ambient workflow;
#   - `gup doctor` is recognized as built-in and reserved.
#
# Usage: scripts/smoke-project.sh [image]   (default: debian:bookworm-slim)
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

echo "== install runtime deps (direnv deliberately absent) =="
# gcc: rules_go builds runtime/cgo, so a C compiler is the one system
# dependency a Go scaffold needs — the pins own every *versioned* tool.
apt-get -qq update >/dev/null
apt-get -qq install -y curl ca-certificates gcc >/dev/null

echo "== install guppy and the gup shim name =="
install -m 0755 /src/guppy /usr/local/bin/guppy
ln -s /usr/local/bin/guppy /usr/local/bin/gup

echo "== install bazelisk (pinned bazel comes from .bazelversion) =="
curl -fsSL -o /usr/local/bin/bazel \
  https://github.com/bazelbuild/bazelisk/releases/download/v1.27.0/bazelisk-linux-amd64
chmod 0755 /usr/local/bin/bazel
command -v direnv && { echo "unexpected direnv in container"; exit 1; } || true

echo "== scaffold =="
guppy --version
cd /app 2>/dev/null || { mkdir -p /app && cd /app; }
guppy new widget --template go-cli --var name=widget
test -f widget/.guppy/project.toml
grep -q "^schema = 2" widget/.guppy/project.toml
grep -q "bazel test //" widget/.guppy/project.toml

echo "== gup test without direnv (the fallback is a contract) =="
cd widget
guppy project test

echo "== gup outside a project refuses (exit 2) =="
cd /
set +e
gup test
code=$?
set -e
test "$code" -eq 2

echo "== gup doctor is built in (inside the project) =="
# A clean scaffold audits clean: exit 0, no drift. Capture before grep:
# piping into `grep -q` closes the pipe early (SIGPIPE under pipefail).
cd /app/widget
report="$(gup doctor 2>&1)"
echo "$report" | grep -q "clean"

# Corrupt the tree: doctor flags the drift (exit 1) and --fix restores
# the missing file without touching a user edit.
rm README.md
echo "my precious edit" >> main.go
set +e
report="$(gup doctor 2>&1)"
code=$?
set -e
test "$code" -eq 1
set +e
fixout="$(gup doctor --fix 2>&1)"
set -e
echo "$fixout" | grep -q "fixed"
test -f README.md
grep -q "my precious edit" main.go

echo "SMOKE OK"
'
