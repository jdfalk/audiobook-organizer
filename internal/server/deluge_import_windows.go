// file: internal/server/deluge_import_windows.go
// version: 1.0.0
// guid: d4e5f6a7-8b9c-0d1e-2f3a-4b5c6d7e8f9a

//go:build windows

package server

import "fmt"

// reflinkCopyOS always returns an error on Windows (no reflink support).
// The caller falls back to io.Copy.
func reflinkCopyOS(src, dest string) error {
	return fmt.Errorf("reflink not supported on Windows")
}
