// file: internal/server/diskstats_unix.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012

//go:build !windows

package server

import "syscall"

// getDiskStats returns total, free bytes for the given path.
func getDiskStats(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	blockSize := uint64(stat.Bsize)
	return stat.Blocks * blockSize, stat.Bavail * blockSize, nil
}
