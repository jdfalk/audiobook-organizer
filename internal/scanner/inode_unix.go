// file: internal/scanner/inode_unix.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

//go:build !windows

package scanner

import (
	"os"
	"syscall"
)

// getInode returns the inode number for the given file info.
// Returns 0, false if the underlying syscall type is unavailable.
func getInode(info os.FileInfo) (uint64, bool) {
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(sys.Ino), true
}
