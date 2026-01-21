// file: internal/sysinfo/memory.go
// version: 1.0.1
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d

package sysinfo

import (
	"runtime"
)

// totalMemoryProvider allows tests to override platform memory queries.
var totalMemoryProvider = getTotalMemoryPlatform

// availableMemoryProvider allows tests to override platform memory queries.
var availableMemoryProvider = getAvailableMemoryPlatform

// GetTotalMemory returns the total system memory in bytes.
// Returns 0 if unable to determine (will be implemented per-platform).
func GetTotalMemory() uint64 {
	return totalMemoryProvider()
}

// MemoryStats represents comprehensive memory statistics
type MemoryStats struct {
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// GetMemoryStats returns current system memory statistics
func GetMemoryStats() (*MemoryStats, error) {
	total := totalMemoryProvider()
	if total == 0 {
		// Fallback to runtime stats only
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return &MemoryStats{
			TotalBytes:     0,
			AvailableBytes: 0,
			UsedBytes:      m.Sys,
			UsedPercent:    0,
		}, nil
	}

	available := availableMemoryProvider()
	used := total - available
	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) / float64(total) * 100.0
	}

	return &MemoryStats{
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsedPercent:    usedPercent,
	}, nil
}
