// file: internal/sysinfo/uptime_linux.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-f2a3b4c5d6e7

//go:build linux

package sysinfo

import (
	"os"
	"strconv"
	"strings"
)

func getSystemUptimePlatform() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return secs
}
