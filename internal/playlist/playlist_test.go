// file: internal/playlist/playlist_test.go
// version: 1.0.0
// guid: 9c0d1e2f-3a4b-5c6d-7e8f-9a0b1c2d3e4f
// last-edited: 2026-01-19

package playlist

import (
	"testing"
)

// Note: GeneratePlaylistsForSeries requires a database connection and is tested in integration tests
func TestPackageExists(t *testing.T) {
	// Verify package compiles
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}
