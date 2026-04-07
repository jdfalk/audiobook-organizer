//go:build !windows

package cmd

import "syscall"

// setUmask sets the process umask to 0002 on Unix systems so that
// os.Create yields 0664 and os.MkdirAll yields 0775, preserving
// group-write for POSIX ACL compatibility.
func setUmask() {
	syscall.Umask(0002)
}
