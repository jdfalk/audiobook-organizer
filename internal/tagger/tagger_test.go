// file: internal/tagger/tagger_test.go
// version: 1.0.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e
// last-edited: 2026-01-19

package tagger

import (
	"testing"
)

// Note: UpdateSeriesTags requires a database connection and is tested in integration tests
func TestPackageExists(t *testing.T) {
	// Verify package compiles and basic structure exists
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}
