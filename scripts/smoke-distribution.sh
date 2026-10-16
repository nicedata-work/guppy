#!/usr/bin/env bash
# Distribution smoke test, run in a bare container:
#
#   - static-JSON registry served over HTTP (python http.server — any
#     static file server works, that's the point);
#   - `template search` consults the index; `template add` without the
#     gate installs first-party, `community:` adds are review-gated
#     (refused non-interactively without --yes, untrusted until reviewed);
#   - `new --template` uses the community copy only after review;
#   - `template update` shows the upgrade diff and reconciles;
#   - `template remove` revokes install + trust;
#   - offline mode: registry verbs exit 3, local scopes still resolve.
#
# Usage: scripts/smoke-distribution.sh [image]   (default: debian:bookworm-slim)
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

# ---- build the fixture registry on the host -------------------------------
REG="$STAGE/registry"
mkdir -p "$REG/tarballs"

make_template() { # id version marker
  local id="$1" ver="$2" marker="$3" dir
  dir="$(mktemp -d)"
  mkdir -p "$dir/template"
  cat >"$dir/guppy.toml" <<EOF
id = "$id"
version = "$ver"
kind = "cli"
language = "go"
description = "smoke-test acme template"

[defaults]
git = false
bazel = false
direnv = false
linters = false
tests = false
EOF
  printf 'payload %s\n' "$marker" >"$dir/template/marker.txt"
  tar -czf "$REG/tarballs/$id-$ver.tgz" -C "$dir" .
  rm -rf "$dir"
}

sha256() { sha256sum "$1" | cut -d' ' -f1; }

make_template acme 1.0.0 v1
make_template acme 1.1.0 v2
cat >"$REG/index.json" <<EOF
{
  "schema": 1,
  "templates": [
    {
      "id": "acme",
      "version": "1.0.0",
      "kind": "cli",
      "language": "go",
      "description": "smoke-test acme template",
      "keywords": ["guppy-template", "cli"],
      "tarball": "/tarballs/acme-1.0.0.tgz",
      "sha256": "$(sha256 "$REG/tarballs/acme-1.0.0.tgz")"
    }
  ]
}
EOF

# A staged index that also lists 1.1.0 (the upgrade).
cat >"$STAGE/index-2.json" <<EOF
{
  "schema": 1,
  "templates": [
    {
      "id": "acme",
      "version": "1.0.0",
      "kind": "cli",
      "language": "go",
      "description": "smoke-test acme template",
      "keywords": ["guppy-template", "cli"],
      "tarball": "/tarballs/acme-1.0.0.tgz",
      "sha256": "$(sha256 "$REG/tarballs/acme-1.0.0.tgz")"
    },
    {
      "id": "acme",
      "version": "1.1.0",
      "kind": "cli",
      "language": "go",
      "description": "smoke-test acme template",
      "keywords": ["guppy-template", "cli"],
      "tarball": "/tarballs/acme-1.1.0.tgz",
      "sha256": "$(sha256 "$REG/tarballs/acme-1.1.0.tgz")"
    }
  ]
}
EOF

# ---- run the cycle in a bare container ------------------------------------
"$ENGINE" run --rm -i -v "$STAGE:/src:ro,Z" --entrypoint bash "$IMAGE" -s <<'CONTAINER'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update >/dev/null
apt-get -qq install -y python3 ca-certificates >/dev/null

install -m 0755 /src/guppy /usr/local/bin/guppy
cp -r /src/registry /tmp/registry
python3 -m http.server 8000 --directory /tmp/registry >/dev/null 2>&1 &
for _ in $(seq 50); do
  curl -sf http://127.0.0.1:8000/index.json >/dev/null && break
  sleep 0.1
done

export GUPPY_REGISTRY=http://127.0.0.1:8000
export GUPPY_DEFAULTS_GIT=false   # git deliberately absent in the image
cd /tmp

echo "== search consults the registry index =="
guppy template search acme | grep -q "acme" || { echo "search missed acme"; exit 1; }

echo "== community add is review-gated (no --yes, no TTY → exit 2) =="
if guppy template add community:acme </dev/null; then
  echo "community add should have been refused without --yes"; exit 1
fi
test ! -d "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/community/acme@1.0.0" \
  || { echo "refused add left an install"; exit 1; }

echo "== untrusted community template does not resolve =="
if guppy new --template acme /tmp/untrusted-proj </dev/null; then
  echo "untrusted template resolved"; exit 1
fi

echo "== community add --yes installs + records trust =="
guppy template add community:acme --yes
test -f "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/community/acme@1.0.0/template/marker.txt" \
  || { echo "community install missing"; exit 1; }

echo "== trusted community template scaffolds =="
guppy new --template acme /tmp/proj
grep -q "payload v1" /tmp/proj/marker.txt || { echo "wrong scaffold content"; exit 1; }

echo "== registry publishes 1.1.0: update shows diff, --yes applies =="
cp /src/index-2.json /tmp/registry/index.json
guppy template update acme --yes
grep -q "payload v2" "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/community/acme@1.1.0/template/marker.txt" \
  || { echo "update did not install 1.1.0"; exit 1; }
test ! -d "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/community/acme@1.0.0" \
  || { echo "old version dir not cleaned"; exit 1; }

echo "== update is idempotent =="
guppy template update --all 2>&1 | grep -q "up to date" || { echo "expected up-to-date"; exit 1; }

echo "== first-party add installs without any gate =="
guppy template add registry:acme@1.1.0
test -f "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/templates/acme/guppy.toml" \
  || { echo "user-scope install missing"; exit 1; }

echo "== remove revokes community install + trust =="
guppy template remove community:acme
test ! -d "${XDG_DATA_HOME:-$HOME/.local/share}/guppy/community/acme@1.1.0" \
  || { echo "community copy not removed"; exit 1; }
if guppy new --template acme /tmp/after-remove </dev/null; then
  # user-scope acme still exists — must succeed and come from user scope
  grep -q "payload v2" /tmp/after-remove/marker.txt || { echo "wrong scope won"; exit 1; }
fi

echo "== offline: registry verbs exit 3, local scopes still resolve =="
guppy --offline template search acme && { echo "offline search should fail"; exit 1; } || test $? -eq 3
guppy --offline template add acme && { echo "offline add should fail"; exit 1; } || test $? -eq 3
guppy --offline new --template acme /tmp/offline-proj \
  || { echo "offline resolve of local scope failed"; exit 1; }
grep -q "payload v2" /tmp/offline-proj/marker.txt || { echo "offline scaffold wrong"; exit 1; }

echo "SMOKE OK"
CONTAINER
