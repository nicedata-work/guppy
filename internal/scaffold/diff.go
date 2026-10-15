// The audit-diff vocabulary of the environment contract (design §8): what
// Local.Audit produces and Local.Repair consumes. The types live here —
// with the only code that produces them — not in a standalone package.
package scaffold

// DiffKind classifies one expected-vs-actual difference found by Audit.
type DiffKind int

const (
	// DiffMissing: a payload file the template expects is absent.
	DiffMissing DiffKind = iota
	// DiffUnexpected: a file the template never produced exists in a
	// contract-managed location.
	DiffUnexpected
	// DiffModified: file exists but content differs from a fresh render
	// (may be a user edit — never auto-clobbered).
	DiffModified
	// DiffStale: pin/config drift (e.g. .bazelversion vs template default).
	DiffStale
)

// Diff is one audit finding: a path and what is wrong with it.
type Diff struct {
	Kind DiffKind
	Path string
}

// Diffs is an ordered set of audit findings.
type Diffs []Diff
