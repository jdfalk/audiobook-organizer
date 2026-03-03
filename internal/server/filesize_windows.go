// file: internal/server/filesize_windows.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

//go:build windows

package server

import "os"

// filePhysicalSize returns the file size. On Windows, block-based size is not
// available via syscall.Stat_t, so we return the logical size.
func filePhysicalSize(info os.FileInfo) int64 {
	return info.Size()
}
