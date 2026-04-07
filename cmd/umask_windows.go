//go:build windows

package cmd

// setUmask is a no-op on Windows (no POSIX ACLs).
func setUmask() {}
