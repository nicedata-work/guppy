// Version ordering (styleguide T12: a comparator used across packages
// lives once, in the package whose concern it is — five packages used to
// reach into `dist` for this, each re-deriving the same descending-sort
// closure).
//
// Policy: semver-aware; non-semver strings sort AFTER all semver ones
// (lexicographic among themselves) — an unparseable version is never
// treated as newest (T9: failure direction is the safe side).
package version

import (
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Compare orders two version strings: -1 when a < b, 0 when equal (or
// both non-semver and equal), 1 when a > b. Semver compares by
// semver rules; any non-semver side sorts after every semver one.
func Compare(a, b string) int {
	sa, ea := semver.NewVersion(a)
	sb, eb := semver.NewVersion(b)
	switch {
	case ea != nil && eb != nil:
		return strings.Compare(a, b)
	case ea != nil:
		return 1
	case eb != nil:
		return -1
	default:
		return sa.Compare(sb)
	}
}

// SortDesc sorts slice newest-first by the version each element carries
// (the single descending-sort entry point — call sites pass only how to
// read the version out of their element type).
//
// Non-semver versions sort LAST even here (T9: an unparseable version
// must never win "newest" — a plain reversal of Compare would put them
// first, which is how a corrupt registry entry could shadow a real
// release).
func SortDesc[T any](slice []T, ver func(T) string) {
	slices.SortFunc(slice, func(a, b T) int {
		va, vb := ver(a), ver(b)
		_, ea := semver.NewVersion(va)
		_, eb := semver.NewVersion(vb)
		if (ea == nil) != (eb == nil) {
			if ea == nil {
				return -1
			}
			return 1
		}
		return Compare(vb, va)
	})
}
