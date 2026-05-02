// file: internal/server/reconcile_test.go
// version: 2.0.0
// guid: f8a9b0c1-d2e3-4f5a-6b7c-8d9e0f1a2b3c

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/reconcile"
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
		result := reconcile.NormalizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCountMatchType(t *testing.T) {
	matches := []reconcile.ReconcileMatch{
		{MatchType: "hash"},
		{MatchType: "hash"},
		{MatchType: "filename"},
		{MatchType: "original_hash"},
	}

	if got := reconcile.CountMatchType(matches, "hash"); got != 2 {
		t.Errorf("CountMatchType(hash) = %d, want 2", got)
	}
	if got := reconcile.CountMatchType(matches, "filename"); got != 1 {
		t.Errorf("CountMatchType(filename) = %d, want 1", got)
	}
	if got := reconcile.CountMatchType(matches, "original_hash"); got != 1 {
		t.Errorf("CountMatchType(original_hash) = %d, want 1", got)
	}
	if got := reconcile.CountMatchType(matches, "nonexistent"); got != 0 {
		t.Errorf("CountMatchType(nonexistent) = %d, want 0", got)
	}
}
