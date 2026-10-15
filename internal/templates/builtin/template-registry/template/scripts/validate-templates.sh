#!/usr/bin/env bash
# Validates every template under templates/: guppy lint + guppy test.
# CI runs this on every PR; run it locally before pushing.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
fail=0
found=0
for d in "$root"/templates/*/; do
    [ -f "$d/guppy.toml" ] || continue
    found=1
    echo "== $(basename "$d")"
    guppy lint "$d" || fail=1
    guppy test "$d" || fail=1
done
[ "$found" = 1 ] || echo "no templates yet — drop the first one into templates/<id>/"
exit "$fail"
