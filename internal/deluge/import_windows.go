// file: internal/deluge/import_windows.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-def0-234567890123
// last-edited: 2026-05-11

//go:build windows

package deluge

import "fmt"

// reflinkCopyOS always returns an error on Windows (no reflink support).
// The caller falls back to io.Copy.
func reflinkCopyOS(src, dest string) error {
	return fmt.Errorf("reflink not supported on Windows")
}
