// [includes] narrowing (design §3.2, pi's "filters narrow, never expand"
// rule). Patterns are matched against the file path relative to the
// template root — both payload-relative ("scratch/x") and root-relative
// ("template/scratch/x", as written in the §3.2 example) forms count, so
// manifests read naturally in both layout modes.
package render

import (
	"path"
	"strings"

	"github.com/nicedata-work/guppy/internal/manifest"
)

// matchGlob matches name (slash-separated) against pattern, supporting
// `*`, `?`, character classes (via path.Match per segment) and `**` as a
// full segment meaning "zero or more directories".
func matchGlob(pattern, name string) bool {
	return segMatch(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func segMatch(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {

			for i := 0; i <= len(name); i++ {
				if segMatch(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// included reports whether rel (a payload-relative path) survives the
// template's include/exclude filters. Empty include list means everything.
func Included(in manifest.Includes, rel string) bool {
	if len(in.Include) > 0 && !anyMatch(in.Include, rel) {
		return false
	}
	return !anyMatch(in.Exclude, rel)
}

func anyMatch(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchGlob(p, rel) || matchGlob(p, "template/"+rel) {
			return true
		}
	}
	return false
}
