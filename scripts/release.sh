#!/usr/bin/env bash
# Builds the release artifacts the installer and `guppy update` consume:
#
#   dist/latest.txt                                  <- version pointer
#   dist/guppy-<version>-<os>-<arch>.tar.gz         <- binary tarball
#   dist/guppy-<version>-<os>-<arch>.tar.gz.sha256  <- detached checksum
#   dist/install.sh                                  <- installer copy
#
# Usage: scripts/release.sh [VERSION]              one platform (host by default)
#        scripts/release.sh --matrix [VERSION]     the full release matrix:
#                                                  linux/darwin/windows × amd64/arm64
# Env:   GOOS/GOARCH (default: host), DIST_DIR (default dist/)
#
# Note: artifacts are cross-compiled with the Go toolchain (CGO_ENABLED=0,
# static binary) — release packaging bootstraps outside Bazel; Bazel remains
# the canonical dev/CI build. This script builds the full release matrix.

set -euo pipefail

MODULE="github.com/nicedata-work/guppy"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# --- matrix mode: build every release cell ---------------------------------

if [ "${1:-}" = "--matrix" ]; then
    shift
    VERSION="${1:-${VERSION:-}}"
    if [ -z "$VERSION" ]; then
        echo "release.sh: VERSION required for --matrix (arg or env)" >&2
        exit 1
    fi
    DIST="${DIST_DIR:-$ROOT/dist}"
    for cell in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
        GOOS="${cell%/*}" GOARCH="${cell#*/}" DIST_DIR="$DIST" "$0" "$VERSION"
    done
    echo "release.sh: matrix complete → $DIST ($(ls "$DIST" | wc -l) files)"
    exit 0
fi

VERSION="${1:-${VERSION:-}}"
if [ -z "$VERSION" ]; then
    echo "release.sh: VERSION required (arg or env)" >&2
    exit 1
fi
case "$VERSION" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) echo "release.sh: VERSION must be semver (got $VERSION)" >&2; exit 1 ;;
esac

GOOS="${GOOS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
case "${GOARCH:-$(uname -m)}" in
    x86_64|amd64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) echo "release.sh: unsupported architecture" >&2; exit 1 ;;
esac

DIST_DIR="${DIST_DIR:-$ROOT/dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "release.sh: building guppy $VERSION ($GOOS/$GOARCH)"
# Windows assets carry the .exe suffix inside the tarball — install.ps1
# and `guppy update` both expect guppy.exe there.
BIN="guppy"
if [ "$GOOS" = "windows" ]; then
    BIN="guppy.exe"
fi
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
    -trimpath \
    -ldflags "-s -w -X $MODULE/internal/version.Version=$VERSION" \
    -o "$STAGE/$BIN" "$ROOT/cmd/guppy"

ASSET="guppy-$VERSION-$GOOS-$GOARCH.tar.gz"
mkdir -p "$DIST_DIR"
tar -C "$STAGE" -czf "$DIST_DIR/$ASSET" "$BIN"

if command -v sha256sum >/dev/null 2>&1; then
    (cd "$DIST_DIR" && sha256sum "$ASSET" > "$ASSET.sha256")
else
    (cd "$DIST_DIR" && shasum -a 256 "$ASSET" > "$ASSET.sha256")
fi

# latest.txt + installer copy (only when this asset is the newest built).
echo "$VERSION" > "$DIST_DIR/latest.txt"
cp "$ROOT/install/install.sh" "$DIST_DIR/install.sh"

echo "release.sh: wrote $DIST_DIR/$ASSET"
