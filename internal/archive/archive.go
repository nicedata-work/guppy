// Package archive reads and writes tar (optionally gzipped) archives
// with path-safety guarantees: no absolute paths, no `..` escapes, no
// symlinks/hardlinks, no special files. Both untrusted registry tarballs
// (internal/registry) and trusted `git archive` output (internal/templates
// git sources) go through here, so the extraction rules exist once.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// File is one buffered regular file of an archive.
type File struct {
	Rel  string
	Mode fs.FileMode
	Body []byte
}

// ReadTarGz parses gzipped tar bytes into validated files.
func ReadTarGz(data []byte) ([]File, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	return ReadTar(gz)
}

// ReadTar parses tar bytes into validated files (regular files only;
// directories are implicit, links and special entries are refused).
// Total body size is capped — callers pass already size-capped streams.
func ReadTar(r io.Reader) ([]File, error) {
	tr := tar.NewReader(r)
	var files []File
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name := path.Clean(h.Name)
		if name == "." || name == "" {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("archive entry escapes root: %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			return nil, fmt.Errorf("archive contains a link (%c: %s) — refusing", h.Typeflag, h.Name)
		case tar.TypeReg:
			body, err := io.ReadAll(io.LimitReader(tr, 32<<20))
			if err != nil {
				return nil, err
			}
			files = append(files, File{Rel: name, Mode: h.FileInfo().Mode().Perm(), Body: body})
		default:
			return nil, fmt.Errorf("archive contains an unsupported entry (%c: %s)", h.Typeflag, h.Name)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("archive is empty")
	}
	return files, nil
}

// TopDir reports the archive's single wrapping directory: the common
// first path component of every file, when every file has one. Archives
// with files at the top level have none.
func TopDir(files []File) (string, bool) {
	root, _, hasSlash := strings.Cut(files[0].Rel, "/")
	if !hasSlash {
		return "", false
	}
	for _, f := range files[1:] {
		r, _, ok := strings.Cut(f.Rel, "/")
		if !ok || r != root {
			return "", false
		}
	}
	return root, true
}

// Write extracts files into dest, dropping the first strip path
// components of each entry. Extraction is path-checked again at write
// time (defense in depth against a confused parser upstream), and no
// write may travel through a symlinked directory (security
// pass): a hostile or stale dest tree cannot redirect extraction.
func Write(files []File, dest string, strip int) error {
	for _, f := range files {
		rel := f.Rel
		for range strip {
			var ok bool
			_, rel, ok = strings.Cut(rel, "/")
			if !ok {
				rel = ""
				break
			}
		}
		if rel == "" {
			continue
		}
		cleanRel := filepath.Clean(filepath.FromSlash(rel))
		target := filepath.Join(dest, cleanRel)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(filepath.Separator)) {
			return fmt.Errorf("archive entry escapes root: %q", f.Rel)
		}
		if err := checkNoSymlink(dest, cleanRel); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := f.Mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, f.Body, mode); err != nil {
			return err
		}
	}
	return nil
}

// checkNoSymlink walks rel under root and refuses if any existing
// component is a symlink — the same rule scaffold.Apply enforces on
// FileSet materialization (security pass).
func checkNoSymlink(root, rel string) error {
	parts := strings.Split(rel, string(filepath.Separator))
	cur := root
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("inspect %s: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			kind := "directory"
			if i == len(parts)-1 {
				kind = "file"
			}
			return fmt.Errorf("%s %s is a symlink — refusing to write through it", kind, cur)
		}
	}
	return nil
}
