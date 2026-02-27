// file: internal/server/diskstats_windows.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-234567890123

//go:build windows

package server

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getDiskStats returns total, free bytes for the given path using Windows API.
func getDiskStats(path string) (total, free uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid path: %w", err)
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, e1 := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceExW failed: %w", e1)
	}
	return totalBytes, freeBytesAvailable, nil
}
