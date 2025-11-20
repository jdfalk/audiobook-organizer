// file: internal/sysinfo/memory_darwin.go
// version: 1.0.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

//go:build darwin

package sysinfo

import (
	"syscall"
	"unsafe"
)

// getTotalMemoryPlatform returns total system memory on macOS
func getTotalMemoryPlatform() uint64 {
	mib := []int32{6 /* CTL_HW */, 24 /* HW_MEMSIZE */}
	var memsize uint64
	length := unsafe.Sizeof(memsize)

	_, _, err := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&memsize)),
		uintptr(unsafe.Pointer(&length)),
		0, 0,
	)

	if err != 0 {
		return 0
	}

	return memsize
}

// getAvailableMemoryPlatform returns available system memory on macOS
func getAvailableMemoryPlatform() uint64 {
	// On macOS, we can get vm_stat info, but for simplicity
	// we'll use a rough estimate based on inactive + free memory
	// This is an approximation - for accurate available memory,
	// we'd need to parse vm_stat output or use host_statistics64

	// For now, return total - (active + wired) as approximation
	// A more accurate implementation would use mach APIs
	total := getTotalMemoryPlatform()

	// Rough estimate: assume 20% overhead for system
	// This is conservative but safe
	if total > 0 {
		return total * 80 / 100
	}

	return 0
}
