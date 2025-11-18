// file: internal/organizer/reflink_windows.go
// version: 1.0.0
// guid: 7c6d5e4f-3a2b-1c0d-9e8f-7a6b5c4d3e2f

//go:build windows

package organizer

import (
	"fmt"
)

// reflinkFilePlatform creates a CoW reflink on Windows (not supported)
func (o *Organizer) reflinkFilePlatform(sourcePath, targetPath string) error {
	// Windows doesn't support reflinks in the same way
	// Return error so auto mode falls back to hardlink or copy
	return fmt.Errorf("reflink not supported on Windows")
}
