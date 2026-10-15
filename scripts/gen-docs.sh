#!/usr/bin/env bash
# Generates (or checks) the markdown CLI reference in docs/cli/ from the
# cobra command tree (`guppy docs`: docs generated, never
# hand-written). CI runs --check to catch staleness.
#
# Usage: scripts/gen-docs.sh [--check]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE="${1:-write}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

( cd "$ROOT" && go run ./cmd/guppy docs --out "$TMP/cli" >/dev/null )

if [ "$MODE" = "--check" ]; then
    if ! diff -r "$ROOT/docs/cli" "$TMP/cli"; then
        echo "gen-docs: docs/cli is stale — run scripts/gen-docs.sh and commit" >&2
        exit 1
    fi
    echo "gen-docs: docs/cli is up to date"
else
    rm -rf "$ROOT/docs/cli"
    cp -r "$TMP/cli" "$ROOT/docs/cli"
    echo "gen-docs: wrote $ROOT/docs/cli ($(ls "$ROOT/docs/cli" | wc -l) pages)"
fi
