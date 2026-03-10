// file: internal/server/reconcile_test.go
// version: 1.0.0
// guid: f8a9b0c1-d2e3-4f5a-6b7c-8d9e0f1a2b3c

package server

import (
	"testing"
)

func TestNormalizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Book Title.m4b", "book title"},
		{"Book-Title_2024.mp3", "book title 2024"},
		{"My.Book.Name.m4a", "my book name"},
		{"UPPER CASE.flac", "upper case"},
		{"  extra   spaces  .m4b", "extra spaces"},
		{"no-extension", "no extension"},
		{"", ""},
	}

	for _, tt := range tests {
		result := normalizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCountMatchType(t *testing.T) {
	matches := []ReconcileMatch{
		{MatchType: "hash"},
		{MatchType: "hash"},
		{MatchType: "filename"},
		{MatchType: "original_hash"},
	}

	if got := countMatchType(matches, "hash"); got != 2 {
		t.Errorf("countMatchType(hash) = %d, want 2", got)
	}
	if got := countMatchType(matches, "filename"); got != 1 {
		t.Errorf("countMatchType(filename) = %d, want 1", got)
	}
	if got := countMatchType(matches, "original_hash"); got != 1 {
		t.Errorf("countMatchType(original_hash) = %d, want 1", got)
	}
	if got := countMatchType(matches, "nonexistent"); got != 0 {
		t.Errorf("countMatchType(nonexistent) = %d, want 0", got)
	}
}
