#!/bin/sh
# guppy installer — https://nicedata.work/guppy
#
#   curl -LsSf https://nicedata.work/guppy/install.sh | sh
#
# Downloads the checksummed release tarball, verifies it, and installs the
# single static binary to ~/.local/bin/guppy (uv model, design §10.1).
# Never runs as root as a *requirement* — it just needs a writable install
# dir. Unattended/CI safe: no prompts.
#
# Environment:
#   GUPPY_INSTALL_URL    release base URL (default https://github.com/nicedata-work/guppy/releases/latest/download)
#   GUPPY_INSTALL_DIR    install dir (default ~/.local/bin)
#   GUPPY_VERSION        pin a version instead of latest.txt
#   GUPPY_SHELL          rc file to add PATH to (default: detect from $SHELL)
#   GUPPY_NO_MODIFY_PATH set to 1 to skip PATH modification
#
# Flags: --version X, --install-dir DIR, --no-modify-path, --help

set -eu

GUPPY_INSTALL_URL="${GUPPY_INSTALL_URL:-https://github.com/nicedata-work/guppy/releases/latest/download}"
GUPPY_INSTALL_DIR="${GUPPY_INSTALL_DIR:-${HOME}/.local/bin}"
GUPPY_VERSION="${GUPPY_VERSION:-}"
GUPPY_NO_MODIFY_PATH="${GUPPY_NO_MODIFY_PATH:-}"

MARKER_OPEN="# >>> guppy initialize >>>"
MARKER_CLOSE="# <<< guppy initialize <<<"

log() { printf 'guppy-installer: %s\n' "$*"; }
warn() { printf 'guppy-installer: warning: %s\n' "$*" >&2; }
die() { printf 'guppy-installer: error: %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<'EOF'
Install guppy (https://nicedata.work/guppy).

Usage: install.sh [OPTIONS]

Options:
  --version X          install version X instead of latest
  --install-dir DIR    install into DIR (default ~/.local/bin)
  --no-modify-path     do not add DIR to PATH in your shell rc file
  -h, --help           show this help

Environment: GUPPY_INSTALL_URL, GUPPY_INSTALL_DIR, GUPPY_VERSION,
GUPPY_SHELL, GUPPY_NO_MODIFY_PATH.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version) [ $# -ge 2 ] || die "--version needs a value"; GUPPY_VERSION="$2"; shift 2 ;;
        --install-dir) [ $# -ge 2 ] || die "--install-dir needs a value"; GUPPY_INSTALL_DIR="$2"; shift 2 ;;
        --no-modify-path) GUPPY_NO_MODIFY_PATH=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) usage >&2; die "unknown argument: $1" ;;
    esac
done

# --- platform detection -----------------------------------------------------

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
    linux|darwin) ;;
    *) die "unsupported OS: $os (Windows: use install.ps1)" ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "unsupported architecture: $arch" ;;
esac

if [ "$(id -u)" = "0" ]; then
    warn "running as root; guppy is meant to be installed per-user"
fi

# --- download + verify ------------------------------------------------------

workdir="$(mktemp -d)"
cleanup() { rm -rf "$workdir"; }
trap cleanup EXIT

fetch() {
    url="$1"; dest="$2"
    curl -fsSL --retry 3 --retry-delay 1 -o "$dest" "$url" \
        || die "download failed: $url"
}

if [ -z "$GUPPY_VERSION" ]; then
    fetch "$GUPPY_INSTALL_URL/latest.txt" "$workdir/latest.txt"
    GUPPY_VERSION="$(tr -d '[:space:]' < "$workdir/latest.txt")"
    [ -n "$GUPPY_VERSION" ] || die "latest.txt at $GUPPY_INSTALL_URL is empty"
fi

name="guppy-$GUPPY_VERSION-$os-$arch.tar.gz"
log "downloading $name"
fetch "$GUPPY_INSTALL_URL/$name" "$workdir/$name"
fetch "$GUPPY_INSTALL_URL/$name.sha256" "$workdir/$name.sha256"

expected="$(cut -d ' ' -f 1 < "$workdir/$name.sha256" | tr '[:upper:]' '[:lower:]')"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$workdir/$name" | cut -d ' ' -f 1)"
elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$workdir/$name" | cut -d ' ' -f 1)"
else
    die "neither sha256sum nor shasum found; cannot verify download"
fi
[ "$actual" = "$expected" ] || die "checksum mismatch for $name (got $actual, want $expected)"
log "checksum verified"

tar -xzf "$workdir/$name" -C "$workdir" || die "unpack failed"
[ -f "$workdir/guppy" ] || die "tarball does not contain the guppy binary"

mkdir -p "$GUPPY_INSTALL_DIR" || die "cannot create $GUPPY_INSTALL_DIR"
if command -v install >/dev/null 2>&1; then
    install -m 0755 "$workdir/guppy" "$GUPPY_INSTALL_DIR/guppy" || die "install failed"
else
    cp "$workdir/guppy" "$GUPPY_INSTALL_DIR/guppy" || die "install failed"
    chmod 0755 "$GUPPY_INSTALL_DIR/guppy"
fi
log "guppy $GUPPY_VERSION installed to $GUPPY_INSTALL_DIR/guppy"

# --- PATH modification ------------------------------------------------------

rc_file_for_shell() {
    case "$1" in
        *fish*) echo "${HOME}/.config/fish/config.fish" ;;
        *zsh*)  echo "${HOME}/.zshrc" ;;
        *bash*) echo "${HOME}/.bashrc" ;;
        *)      echo "${HOME}/.profile" ;;
    esac
}

add_to_path() {
    rc="$1"
    if grep -qsF "$MARKER_OPEN" "$rc" 2>/dev/null; then
        return 0 # already present (idempotent re-install)
    fi
    mkdir -p "$(dirname "$rc")"
    touch "$rc"
    {
        printf '%s\n' "$MARKER_OPEN"
        case "$rc" in
            *config.fish) printf 'fish_add_path "%s"\n' "$GUPPY_INSTALL_DIR" ;;
            *)            printf 'export PATH="%s:$PATH"\n' "$GUPPY_INSTALL_DIR" ;;
        esac
        printf '%s\n' "$MARKER_CLOSE"
    } >> "$rc"
    log "added $GUPPY_INSTALL_DIR to PATH in $rc"
}

if [ -z "$GUPPY_NO_MODIFY_PATH" ]; then
    # Idempotent: add_to_path skips files that already carry the marker.
    rc="${GUPPY_SHELL:-$(rc_file_for_shell "${SHELL:-}")}"
    add_to_path "$rc"
    if ! echo ":$PATH:" | grep -qs ":$GUPPY_INSTALL_DIR:"; then
        log "restart your shell, or run: source $rc"
    fi
fi

log "next steps: guppy --help  ·  guppy new <project>"
