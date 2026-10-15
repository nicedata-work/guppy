package version

import "testing"

// TestCompare pins the ordering policy:
// semver-aware, non-semver sorts last, lexicographic among themselves.
func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0-rc1", "1.0.0", -1},
		{"2.0.0", "10.0.0", -1},
		{"zzz", "1.0.0", 1},
		{"1.0.0", "zzz", -1},
		{"zzz", "aaa", 1},
	}
	for _, c := range cases {
		got := Compare(c.a, c.b)
		if sign := func() int {
			switch {
			case got < 0:
				return -1
			case got > 0:
				return 1
			default:
				return 0
			}
		}(); sign != c.want {
			t.Errorf("Compare(%q,%q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

// TestSortDesc pins the shared descending-sort entry point : the
// newest version comes first; non-semver never outranks semver.
func TestSortDesc(t *testing.T) {
	versions := []string{"1.0.0", "zzz", "2.3.0", "10.0.0", "1.10.0"}
	type pair struct{ id, ver string }
	items := []pair{{"a", "1.0.0"}, {"b", "zzz"}, {"c", "2.3.0"}, {"d", "10.0.0"}, {"e", "1.10.0"}}
	SortDesc(items, func(p pair) string { return p.ver })
	want := []string{"10.0.0", "2.3.0", "1.10.0", "1.0.0", "zzz"}
	for i, p := range items {
		if p.ver != want[i] {
			t.Fatalf("SortDesc[%d] = %q, want %q (full: %v)", i, p.ver, want[i], versions)
		}
	}
}
