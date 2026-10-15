#!/usr/bin/env bash
# Installer smoke test: proves the installer end-to-end in a bare
# container (AGENTS.md: sandbox-verified acceptance).
#
#   release artifacts → local tarball server → `curl | sh` install →
#   guppy --help → guppy update replaces the binary → uninstall
#
# Runs the whole flow inside a clean container when docker/podman is
# available; otherwise falls back to an isolated $HOME on the host.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVE_DIR="$(mktemp -d)"
CONTAINER_HOME=""
SERVER_PID=""
ENGINE=""

cleanup() {
    [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
    rm -rf "$SERVE_DIR"
}
trap cleanup EXIT

# --- 1. build release versions into one serve dir ---------------------------

# Three artifacts: A (pinned install), B (the update target), and C (a
# deliberately corrupt release used to prove the rollback path: checksum
# mismatch must leave the installed binary untouched).
echo "smoke: building release artifacts"
VERSION_A="0.1.0"
VERSION_B="0.1.1"
VERSION_C="0.1.2"
DIST_DIR="$SERVE_DIR/a" VERSION="$VERSION_A" bash "$ROOT/scripts/release.sh" "$VERSION_A" >/dev/null
DIST_DIR="$SERVE_DIR/b" VERSION="$VERSION_B" bash "$ROOT/scripts/release.sh" "$VERSION_B" >/dev/null
DIST_DIR="$SERVE_DIR/c" VERSION="$VERSION_C" bash "$ROOT/scripts/release.sh" "$VERSION_C" >/dev/null
cp "$SERVE_DIR/a"/* "$SERVE_DIR/b"/* "$SERVE_DIR/c"/* "$SERVE_DIR"/ 2>/dev/null || true
# latest.txt must point at the newest (uncorrupt) version.
echo "$VERSION_B" > "$SERVE_DIR/latest.txt"
# Corrupt C's detached checksums so `update --version C` must fail
# checksum verification (the rollback requirement).
for f in "$SERVE_DIR"/guppy-"$VERSION_C"-*.tar.gz.sha256; do
    printf '%064d  %s\n' 0 "$(basename "${f%.sha256}")" > "$f"
done
ls "$SERVE_DIR"

# --- 2. pick a runner ------------------------------------------------------

if command -v docker >/dev/null 2>&1; then ENGINE=docker
elif command -v podman >/dev/null 2>&1; then ENGINE=podman
else ENGINE=none; fi
echo "smoke: runner = $ENGINE"

# --- 3. the assertions, shared between container and host fallback ---------

smoke_body() {
    # Runs with: $1 = release base URL, $2 = pinned install version,
    # $3 = latest version, $4 = corrupt-checksum version; fresh HOME,
    # guppy not installed.
    base="$1"
    ver_a="$2"
    ver_b="$3"
    ver_c="$4"
    export GUPPY_INSTALL_URL="$base"
    export GUPPY_RELEASE_BASE="$base"

    echo "--- install pinned $ver_a (curl | sh) ---"
    curl -fsSL "$base/install.sh" | GUPPY_VERSION="$ver_a" sh
    guppy="$HOME/.local/bin/guppy"

    "$guppy" --help >/dev/null || { echo "FAIL: guppy --help"; exit 1; }
    "$guppy" --version | grep -q "$ver_a" || { echo "FAIL: version != $ver_a"; exit 1; }
    grep -qs "guppy initialize" "$HOME/.profile" || true  # PATH block per shell detect
    echo "--- guppy --version: $("$guppy" --version) ---"

    echo "--- update → $ver_b ---"
    "$guppy" update
    "$guppy" --version | grep -q "$ver_b" || { echo "FAIL: update did not replace binary"; exit 1; }

    echo "--- update rollback: corrupt checksum for $ver_c ---"
    set +e
    "$guppy" update --version "$ver_c"
    code=$?
    set -e
    [ "$code" -eq 3 ] || { echo "FAIL: corrupt update exited $code, want 3 (network/registry)"; exit 1; }
    "$guppy" --version | grep -q "$ver_b" || { echo "FAIL: binary changed despite checksum failure"; exit 1; }
    # No debris from the refused update: no backup, no staged temp file.
    leftovers="$(ls "$HOME/.local/bin" | grep -v '^guppy$' || true)"
    [ -z "$leftovers" ] || { echo "FAIL: leftover files after refused update: $leftovers"; exit 1; }

    echo "--- uninstall + cleanup verification ---"
    "$guppy" uninstall --yes --purge
    [ ! -e "$guppy" ] || { echo "FAIL: binary still present"; exit 1; }
    [ ! -e "${XDG_DATA_HOME:-$HOME/.local/share}/guppy" ] || { echo "FAIL: data dir still present"; exit 1; }
    [ ! -e "${XDG_CACHE_HOME:-$HOME/.cache}/guppy" ] || { echo "FAIL: cache dir still present"; exit 1; }
    # The installer's rc marker block must be gone from every rc file.
    for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile" "$HOME/.config/fish/config.fish"; do
        if [ -f "$rc" ] && grep -qF "guppy initialize" "$rc"; then
            echo "FAIL: installer marker block still present in $rc"; exit 1
        fi
    done
    echo "SMOKE OK"
}

# --- 4. run -----------------------------------------------------------------

if [ "$ENGINE" != "none" ]; then
    # Self-contained container: python http.server serves /dist inside it,
    # so no host networking or port publishing is needed.
    "$ENGINE" run --rm \
        -v "$SERVE_DIR:/dist:ro,Z" \
        docker.io/library/debian:bookworm-slim \
        bash -c '
            set -euo pipefail
            apt-get update -qq && apt-get install -y -qq curl ca-certificates python3 >/dev/null
            python3 -m http.server 8976 --directory /dist --bind 127.0.0.1 &
            server=$!
            trap "kill $server" EXIT
            sleep 1
            export HOME=/root
            '"$(declare -f smoke_body)"'
            smoke_body http://127.0.0.1:8976 '"$VERSION_A"' '"$VERSION_B"' '"$VERSION_C"'
        '
else
    echo "smoke: WARNING - no container engine; running on host with isolated HOME"
    command -v curl >/dev/null || { echo "FAIL: curl required"; exit 1; }
    PORT=$((20000 + RANDOM % 20000))
    python3 -m http.server "$PORT" --directory "$SERVE_DIR" --bind 127.0.0.1 >/dev/null 2>&1 &
    SERVER_PID=$!
    sleep 1
    CONTAINER_HOME="$(mktemp -d)"
    HOME="$CONTAINER_HOME" XDG_CONFIG_HOME="$CONTAINER_HOME/.config" \
        XDG_DATA_HOME="$CONTAINER_HOME/.local/share" XDG_CACHE_HOME="$CONTAINER_HOME/.cache" \
        smoke_body "http://127.0.0.1:$PORT" "$VERSION_A" "$VERSION_B" "$VERSION_C"
fi

echo "smoke: PASS"
