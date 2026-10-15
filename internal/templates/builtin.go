// First-party templates compiled into the binary (design §2.1: `guppy new`
// must work offline and be instant — uv model, go:embed). Adding a
// built-in is an embed + a test, reviewed like code; never fetched at
// runtime.
package templates

import (
	"embed"
	"io/fs"
)

// builtinFS holds the first-party template library: one directory per
// template (guppy.toml + template/ payload + tests.toml + testdata golden
// snapshots). `all:` so dotfile payload (.gitignore.hbs) is embedded —
// dotfiles are ordinary payload files (§3.1).
//
//go:embed all:builtin
var builtinFS embed.FS

// Builtin returns the embedded first-party template library: an fs.FS
// whose root contains one directory per built-in template id.
func Builtin() fs.FS {
	return builtinFS
}

// BuiltinIDs lists the embedded template ids (sorted by WalkDir order).
func BuiltinIDs() ([]string, error) {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(builtinFS, "builtin/"+e.Name()+"/guppy.toml"); err != nil {
			continue
		}
		ids = append(ids, e.Name())
	}
	return ids, nil
}

// BuiltinTemplateFS returns the root fs.FS of one embedded template (or
// nil if there is no such built-in).
func BuiltinTemplateFS(id string) fs.FS {
	sub, err := fs.Sub(builtinFS, "builtin/"+id)
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "guppy.toml"); err != nil {
		return nil
	}
	return sub
}
