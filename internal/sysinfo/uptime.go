// file: internal/sysinfo/uptime.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-e1f2a3b4c5d6

package sysinfo

// systemUptimeProvider allows tests to override platform uptime queries.
var systemUptimeProvider = getSystemUptimePlatform

// GetSystemUptimeSeconds returns the number of seconds the OS has been running.
// Returns 0 if unable to determine.
func GetSystemUptimeSeconds() float64 {
	return systemUptimeProvider()
}
