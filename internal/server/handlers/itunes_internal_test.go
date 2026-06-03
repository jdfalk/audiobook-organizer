// file: internal/server/handlers/itunes_internal_test.go
// version: 1.0.0
// guid: 5d8b2f0c-3a14-4e67-9b21-7c0e4a9f6d18
// last-edited: 2026-06-03

package handlers

import "testing"

// TestCalculatePercent tests the unexported percentage helper. Migrated from
// the server package alongside the helper itself; package-internal so it can
// call the unexported func directly. Covers the clamp and zero-total guards.
func TestCalculatePercent(t *testing.T) {
	tests := []struct {
		current, total, want int
	}{
		{0, 0, 0},
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{200, 100, 100}, // capped at 100
		{-1, 100, 0},    // negative capped at 0
		{5, 0, 0},       // zero total
	}
	for _, tt := range tests {
		got := calculatePercent(tt.current, tt.total)
		if got != tt.want {
			t.Errorf("calculatePercent(%d, %d) = %d, want %d", tt.current, tt.total, got, tt.want)
		}
	}
}
