//go:build windows

package selfmanage

import (
	"os"
	"os/exec"
	"syscall"
)

// RemoveRunningBinary deletes path, which may be the running guppy
// executable. Windows locks the image file of a running process, so a
// plain remove fails with "Access is denied" — the delete is staged
// instead: the binary is renamed aside (renaming a running image is
// allowed, and frees the original path immediately) and a detached cmd
// script deletes the staged copy once the lock clears when guppy exits,
// then removes itself. The POSIX build is a plain os.Remove.
func RemoveRunningBinary(path string) error {
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return err
	}
	aside := path + ".guppy-old"
	if err := os.Rename(path, aside); err != nil {
		return err
	}
	script := aside + ".cleanup.cmd"
	cmdText := "@echo off\r\n" +
		":retry\r\n" +
		"ping -n 2 127.0.0.1 > NUL\r\n" +
		"del /f \"" + aside + "\" 2> NUL\r\n" +
		"if exist \"" + aside + "\" goto retry\r\n" +
		"del \"" + script + "\"\r\n"
	if err := os.WriteFile(script, []byte(cmdText), 0o644); err != nil {
		return nil // path is already free; the staged copy stays for manual cleanup
	}
	cmd := exec.Command("cmd", "/C", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		// DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP — no console
		// window, and the cleanup survives guppy's own exit.
		CreationFlags: 0x00000008 | 0x00000200,
	}
	_ = cmd.Start()
	return nil
}
