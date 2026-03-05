// file: internal/server/author_dedup_test.go
// version: 1.3.0
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
	// With smart canonical selection, the shorter name (ID 2) becomes canonical
	found := false
	for _, g := range groups {
		ids := map[int]bool{g.Canonical.ID: true}
		for _, v := range g.Variants {
			ids[v.ID] = true
		}
		if ids[1] && ids[2] {
			found = true
		}
	}
	if !found {
		t.Error("expected to find James S. A. Corey duplicate group")
	}
}

func TestIsDirtyAuthorName(t *testing.T) {
	dirty := []string{
		"Neal Stephenson - Snow Crash",
		"Big Finish Production",
		"BBC Studios",
		"Penguin Random House",
	}
	for _, name := range dirty {
		if !isDirtyAuthorName(name) {
			t.Errorf("expected %q to be flagged as dirty", name)
		}
	}

	clean := []string{
		"Neal Stephenson",
		"James S. A. Corey",
		"Brandon Sanderson",
		"Natalie Maher (aka Thundamoo)",
	}
	for _, name := range clean {
		if isDirtyAuthorName(name) {
			t.Errorf("expected %q to NOT be flagged as dirty", name)
		}
	}
}

func TestIsCompositeAuthorName(t *testing.T) {
	composite := []string{
		"Orson Scott Card/A Johnston",
		"Mark Tufo, Sean Runnette",
	}
	for _, name := range composite {
		if !isCompositeAuthorName(name) {
			t.Errorf("expected %q to be composite", name)
		}
	}

	single := []string{
		"David Kushner",
		"Smith, John",
		"J. K. Rowling",
		"Natalie Maher (aka Thundamoo)",
	}
	for _, name := range single {
		if isCompositeAuthorName(name) {
			t.Errorf("expected %q to NOT be composite", name)
		}
	}
}

func TestAreAuthorsDuplicate(t *testing.T) {
	shouldMatch := []struct{ a, b string }{
		{"James S. A. Corey", "James S.A. Corey"},
		{"Brandon Sanderson", "Brandon  Sanderson"},
		{"David Kushner", "David Kushner/Wil Wheaton"},
		{"Stephen King", "Steven King"},
		{"J. K. Rowling", "J.K. Rowling"},
	}
	for _, tt := range shouldMatch {
		if !areAuthorsDuplicate(tt.a, tt.b) {
			t.Errorf("expected %q and %q to match", tt.a, tt.b)
		}
	}

	shouldNotMatch := []struct{ a, b string }{
		{"Michael Grant", "Michael Angel"},
		{"Michael Grant", "Michael Troughton"},
		{"Michael Grant", "Michael Langan"},
		{"Michael Grant", "Michael Braun"},
		{"Michael Grant", "Michael Dalton"},
		{"Alex Karne", "Alex Irvine"},
		{"Mark Tufo", "Mark Twain"},
		{"Neal Stephenson", "Neal Stephenson - Snow Crash"},
	}
	for _, tt := range shouldNotMatch {
		if areAuthorsDuplicate(tt.a, tt.b) {
			t.Errorf("expected %q and %q to NOT match", tt.a, tt.b)
		}
	}
}

func TestPickCanonicalAuthor(t *testing.T) {
	tests := []struct {
		name     string
		names    []database.Author
		counts   map[int]int
		expectID int
	}{
		{
			name:     "prefer no slash",
			names:    []database.Author{{ID: 1, Name: "David Kushner/Wil Wheaton"}, {ID: 2, Name: "David Kushner"}},
			counts:   map[int]int{1: 3, 2: 3},
			expectID: 2,
		},
		{
			name:     "prefer no parenthetical",
			names:    []database.Author{{ID: 1, Name: "Natalie Maher (aka Thundamoo)"}, {ID: 2, Name: "Natalie Maher"}},
			counts:   map[int]int{1: 1, 2: 1},
			expectID: 2,
		},
		{
			name:     "prefer cleaner + more books",
			names:    []database.Author{{ID: 1, Name: "Mark Tufo"}, {ID: 2, Name: "Mark Tufo (Sean Runnette)"}},
			counts:   map[int]int{1: 5, 2: 1},
			expectID: 1,
		},
		{
			name:     "prefer no dash",
			names:    []database.Author{{ID: 1, Name: "Neal Stephenson - Snow Crash"}, {ID: 2, Name: "Neal Stephenson"}},
			counts:   map[int]int{1: 1, 2: 1},
			expectID: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countFn := func(id int) int { return tt.counts[id] }
			canonical := pickCanonicalAuthor(tt.names, countFn)
			if canonical.ID != tt.expectID {
				t.Errorf("expected canonical ID %d, got %d (%s)", tt.expectID, canonical.ID, canonical.Name)
			}
		})
	}
}
