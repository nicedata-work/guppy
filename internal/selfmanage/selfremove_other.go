//go:build !windows

package selfmanage

import "os"

// RemoveRunningBinary deletes path, which may be the running guppy
// executable: POSIX allows removing a mapped image (the inode lives on
// until the process exits). The Windows build stages a rename plus a
// detached cleanup instead.
func RemoveRunningBinary(path string) error {
	return os.Remove(path)
}
