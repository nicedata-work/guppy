#!/usr/bin/env bash
# BM25-prioritized fuzzing driver (hardening:
# fuzz/property tests on manifest parse + renderer green).
#
# For each fuzz target: distill the accumulated go-fuzz cache
# ($GOCACHE/fuzz) into the top-N seeds ranked by BM25 against the
# package's own source (tools/fuzzrank), stage them beside the built-in
# seed corpus, run the time-boxed fuzz, then unstage. The ranking spends
# the fixed time budget on inputs lexically closest to what the target
# actually parses, instead of re-mining boilerplate.
#
# Env: GO (go binary, default "go"), FUZZTIME (default 30s),
#      FUZZTOP (seeds kept per target, default 64).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-go}"
FUZZTIME="${FUZZTIME:-30s}"
FUZZTOP="${FUZZTOP:-64}"

TARGETS=(
    "internal/manifest:FuzzParse"
    "internal/render:FuzzParseTemplate"
    "internal/render:FuzzRender"
    "internal/render:FuzzExpr"
    "internal/archive:FuzzReadTarGz"
)

cd "$ROOT"
gocache="$("$GO" env GOCACHE)"
modpath="$("$GO" list -m -f '{{.Path}}')"
cache_root="$gocache/fuzz/$modpath"

staged="$(mktemp)"
target_staged="$(mktemp)"
cleanup() {
    while IFS= read -r f; do
        [ -n "$f" ] && rm -f "$f"
    done < "$staged"
    rm -f "$staged" "$target_staged"
}
trap cleanup EXIT

for spec in "${TARGETS[@]}"; do
    pkg="${spec%%:*}"
    fn="${spec##*:}"
    seeddir="$ROOT/$pkg/testdata/fuzz/$fn"
    mkdir -p "$seeddir"

    # Rank this module's cache for THIS target and stage the top seeds.
    # (Cache layout: $GOCACHE/fuzz/<module>/<pkg>/<FuzzTarget>/<hash>.)
    # Seeds already in the repo's testdata are the reviewed baseline and
    # stay untouched; staged files carry a .staged suffix so cleanup is
    # exact.
    target_cache="$cache_root/$pkg/$fn"
    : > "$target_staged"
    if [ -d "$target_cache" ]; then
        "$GO" run ./tools/fuzzrank -paths -top "$FUZZTOP" \
            -query "$ROOT/$pkg" -corpus "$target_cache" |
        while IFS= read -r src; do
            dest="$seeddir/$(basename "$src").staged"
            if [ -e "$dest" ]; then continue; fi
            cp "$src" "$dest"
            echo "$dest" >> "$staged"
            echo "$dest" >> "$target_staged"
        done
    fi

    echo "fuzz: $pkg $fn ($FUZZTIME, $(ls "$seeddir" | wc -l) seeds)"
    "$GO" test -run=NONE -fuzz="^$fn$" -fuzztime="$FUZZTIME" "./$pkg"

    # Leave the tree pristine: unstage this target's seeds and remove the
    # now-empty seed dir chain (stopping at the first dir with content,
    # e.g. a committed baseline corpus or snapshots).
    while IFS= read -r f; do
        [ -n "$f" ] && rm -f "$f"
    done < "$target_staged"
    d="$seeddir"
    while [ "$d" != "$ROOT" ] && rmdir "$d" 2>/dev/null; do
        d="$(dirname "$d")"
    done
done
echo "fuzz: all targets green"
