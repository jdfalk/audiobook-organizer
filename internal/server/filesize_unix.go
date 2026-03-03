// file: internal/server/filesize_unix.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

//go:build !windows

package server

import (
	"os"
	"syscall"
)

// filePhysicalSize returns the on-disk size using block count (physical), falling
// back to logical size if syscall info is unavailable.
func filePhysicalSize(info os.FileInfo) int64 {
	if sys := info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			return stat.Blocks * 512
		}
	}
	return info.Size()
}
