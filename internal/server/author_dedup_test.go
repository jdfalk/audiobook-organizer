// file: internal/server/author_dedup_test.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestNormalizeAuthorName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"James S. A. Corey", "James S. A. Corey"},
		{"James S.A. Corey", "James S. A. Corey"},
		{"James  S.A.  Corey", "James S. A. Corey"},
		{"  John Smith  ", "John Smith"},
		{"", ""},
		{"A.B.C. Author", "A. B. C. Author"},
	}

	for _, tt := range tests {
		got := NormalizeAuthorName(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeAuthorName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestJaroWinklerSimilarity(t *testing.T) {
	// Identical strings
	if s := jaroWinklerSimilarity("hello", "hello"); s != 1.0 {
		t.Errorf("identical strings should be 1.0, got %f", s)
	}

	// Very similar
	s := jaroWinklerSimilarity("James S. A. Corey", "James S.A. Corey")
	if s < 0.9 {
		t.Errorf("similar author names should be >= 0.9, got %f", s)
	}

	// Different
	s = jaroWinklerSimilarity("John Smith", "Jane Doe")
	if s > 0.7 {
		t.Errorf("different names should be < 0.7, got %f", s)
	}
}

func TestIsMultiAuthorString(t *testing.T) {
	if !isMultiAuthorString("Author1, Author2, Author3, Author4") {
		t.Error("should be multi-author")
	}
	if isMultiAuthorString("James S. A. Corey") {
		t.Error("should not be multi-author")
	}
	if isMultiAuthorString("Smith, John") {
		t.Error("last, first should not be multi-author")
	}
}

func TestFindDuplicateAuthors(t *testing.T) {
	authors := []database.Author{
		{ID: 1, Name: "James S. A. Corey"},
		{ID: 2, Name: "James S.A. Corey"},
		{ID: 3, Name: "Brandon Sanderson"},
		{ID: 4, Name: "Brandon  Sanderson"},
	}

	bookCountFn := func(id int) int { return 1 }

	groups := FindDuplicateAuthors(authors, 0.9, bookCountFn)
	if len(groups) < 1 {
		t.Fatalf("expected at least 1 duplicate group, got %d", len(groups))
	}

	// Should find "James S. A. Corey" / "James S.A. Corey" as a group
	found := false
	for _, g := range groups {
		if g.Canonical.ID == 1 && len(g.Variants) > 0 && g.Variants[0].ID == 2 {
			found = true
		}
	}
	if !found {
		t.Error("expected to find James S. A. Corey duplicate group")
	}
}
