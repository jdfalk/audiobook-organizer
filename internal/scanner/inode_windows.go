// file: internal/scanner/inode_windows.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

//go:build windows

package scanner

import "os"

// getInode is a no-op on Windows since inodes are not available.
func getInode(_ os.FileInfo) (uint64, bool) {
	return 0, false
}
