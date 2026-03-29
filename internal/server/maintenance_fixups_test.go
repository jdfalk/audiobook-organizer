// file: internal/server/maintenance_fixups_test.go
// version: 1.0.0
// guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ---------------------------------------------------------------------------
// parsePattern1 tests
// ---------------------------------------------------------------------------

// TestParseReadByNarrator_Pattern1 covers the case where:
//   - title = "read by <narrator>"
//   - author = "<real title>" (or "<real title>_" with trailing underscore)
func TestParseReadByNarrator_Pattern1(t *testing.T) {
	book := &database.Book{
		ID:       "01JKTEST0000000000000001",
		Title:    "read by Nick Podehl",
		FilePath: "/audio/janitor.m4b",
	}
	authorName := "The Janitor"

	result := parsePattern1(book, authorName)
	if result == nil {
		t.Fatal("parsePattern1 returned nil, expected a fix")
	}

	if result.NewTitle != "The Janitor" {
		t.Errorf("NewTitle = %q, want %q", result.NewTitle, "The Janitor")
	}
	if result.NewNarrator != "Nick Podehl" {
		t.Errorf("NewNarrator = %q, want %q", result.NewNarrator, "Nick Podehl")
	}
	if result.Pattern != "read_by_swap" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "read_by_swap")
	}
	if result.OldTitle != book.Title {
		t.Errorf("OldTitle = %q, want %q", result.OldTitle, book.Title)
	}
	if result.OldAuthor != authorName {
		t.Errorf("OldAuthor = %q, want %q", result.OldAuthor, authorName)
	}
}

// TestParseReadByNarrator_TrailingUnderscore verifies that a trailing
// underscore on the author name is stripped when it becomes the new title.
func TestParseReadByNarrator_TrailingUnderscore(t *testing.T) {
	book := &database.Book{
		ID:       "01JKTEST0000000000000002",
		Title:    "read by Some Narrator",
		FilePath: "/audio/victoria_falling.m4b",
	}
	authorName := "Victoria Falling_"

	result := parsePattern1(book, authorName)
	if result == nil {
		t.Fatal("parsePattern1 returned nil, expected a fix")
	}

	if result.NewTitle != "Victoria Falling" {
		t.Errorf("NewTitle = %q, want %q", result.NewTitle, "Victoria Falling")
	}
}

// TestParseReadByNarrator_Pattern1_EmptyNarrator verifies nil is returned
// when the narrator portion is empty.
func TestParseReadByNarrator_Pattern1_EmptyNarrator(t *testing.T) {
	book := &database.Book{
		ID:    "01JKTEST0000000000000003",
		Title: "read by ",
	}
	result := parsePattern1(book, "Some Author")
	if result != nil {
		t.Errorf("expected nil for empty narrator, got %+v", result)
	}
}

// TestParseReadByNarrator_Pattern1_EmptyAuthor verifies nil is returned
// when the author (real title) is empty after stripping underscores.
func TestParseReadByNarrator_Pattern1_EmptyAuthor(t *testing.T) {
	book := &database.Book{
		ID:    "01JKTEST0000000000000004",
		Title: "read by Some Narrator",
	}
	result := parsePattern1(book, "")
	if result != nil {
		t.Errorf("expected nil for empty author, got %+v", result)
	}
}

// ---------------------------------------------------------------------------
// parsePattern2 tests
// ---------------------------------------------------------------------------

// TestParseReadByNarrator_Pattern2 covers the case where:
//   - title = "<Real Title> - <Narrator> - read by <Author>"
func TestParseReadByNarrator_Pattern2(t *testing.T) {
	book := &database.Book{
		ID:       "01JKTEST0000000000000005",
		Title:    "Old Family - Matt Hicks - read by Alvin",
		FilePath: "/audio/old_family.m4b",
	}
	authorName := "Alvin"

	result := parsePattern2(book, authorName)
	if result == nil {
		t.Fatal("parsePattern2 returned nil, expected a fix")
	}

	if result.NewTitle != "Old Family" {
		t.Errorf("NewTitle = %q, want %q", result.NewTitle, "Old Family")
	}
	if result.NewNarrator != "Matt Hicks" {
		t.Errorf("NewNarrator = %q, want %q", result.NewNarrator, "Matt Hicks")
	}
	if result.Pattern != "title_dash_read_by" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "title_dash_read_by")
	}
}

// TestParseReadByNarrator_Pattern2_NoNarratorDash covers the case where the
// title before " - read by " has no dash (narrator field will be empty).
func TestParseReadByNarrator_Pattern2_NoNarratorDash(t *testing.T) {
	book := &database.Book{
		ID:    "01JKTEST0000000000000006",
		Title: "Standalone Title - read by Narrator Name",
	}
	result := parsePattern2(book, "Narrator Name")
	if result == nil {
		t.Fatal("parsePattern2 returned nil, expected a fix")
	}

	if result.NewTitle != "Standalone Title" {
		t.Errorf("NewTitle = %q, want %q", result.NewTitle, "Standalone Title")
	}
	// No dash before "read by" means narrator is extracted as empty
	if result.NewNarrator != "" {
		t.Errorf("NewNarrator = %q, want empty string when no dash in title portion", result.NewNarrator)
	}
}

// TestParseReadByNarrator_Pattern2_CaseInsensitive verifies the " - read by "
// match is case-insensitive.
func TestParseReadByNarrator_Pattern2_CaseInsensitive(t *testing.T) {
	book := &database.Book{
		ID:    "01JKTEST0000000000000007",
		Title: "Cool Book - Narrator Guy - Read By Author Person",
	}
	result := parsePattern2(book, "Author Person")
	if result == nil {
		t.Fatal("parsePattern2 returned nil for mixed-case 'Read By'")
	}
	if result.NewTitle != "Cool Book" {
		t.Errorf("NewTitle = %q, want %q", result.NewTitle, "Cool Book")
	}
	if result.NewNarrator != "Narrator Guy" {
		t.Errorf("NewNarrator = %q, want %q", result.NewNarrator, "Narrator Guy")
	}
}

// TestParseReadByNarrator_Pattern2_EmptyTitle verifies nil is returned when
// the title portion before " - read by " is empty.
func TestParseReadByNarrator_Pattern2_EmptyTitle(t *testing.T) {
	book := &database.Book{
		ID:    "01JKTEST0000000000000008",
		Title: " - read by Nobody",
	}
	result := parsePattern2(book, "Nobody")
	if result != nil {
		t.Errorf("expected nil for empty title, got %+v", result)
	}
}

// ---------------------------------------------------------------------------
// normalizeSeriesName tests
// ---------------------------------------------------------------------------

// TestNormalizeSeriesName exercises the canonical key generation used for
// duplicate-series detection.
func TestNormalizeSeriesName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Strip leading "the"
		{"The Dark Tower", "dark tower"},
		// Strip trailing "series"
		{"Wheel of Time Series", "wheel of time"},
		// Strip trailing "trilogy"
		{"Lord of the Rings Trilogy", "lord of the rings"},
		// Collapse extra whitespace
		{"  Extra  Spaces  ", "extra spaces"},
		// Strip trailing "saga"
		{"Star Wars Saga", "star wars"},
		// Strip trailing "duology"
		{"Some Book Duology", "some book"},
		// Strip trailing "quartet"
		{"Some Book Quartet", "some book"},
		// No special suffix — pass-through (lowercased)
		{"Foundation", "foundation"},
		// Leading "the" + trailing "series"
		{"The Stormlight Archive Series", "stormlight archive"},
		// Punctuation removal
		{"Harry Potter's Adventures", "harry potter s adventures"},
		// Empty string
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeSeriesName(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeSeriesName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// helper / utility tests
// ---------------------------------------------------------------------------

// TestCaseInsensitiveIndex verifies the helper finds substrings
// regardless of casing.
func TestCaseInsensitiveIndex(t *testing.T) {
	tests := []struct {
		s, sub string
		want   int
	}{
		{"Hello World", "world", 6},
		{"Hello World", "HELLO", 0},
		{"no match here", "xyz", -1},
		{"", "abc", -1},
		{"abc", "", 0},
	}
	for _, tt := range tests {
		got := caseInsensitiveIndex(tt.s, tt.sub)
		if got != tt.want {
			t.Errorf("caseInsensitiveIndex(%q, %q) = %d, want %d", tt.s, tt.sub, got, tt.want)
		}
	}
}

// TestStringDeref verifies stringDeref handles nil and non-nil pointers.
func TestStringDeref(t *testing.T) {
	if got := stringDeref(nil); got != "" {
		t.Errorf("stringDeref(nil) = %q, want %q", got, "")
	}

	v := "hello"
	if got := stringDeref(&v); got != "hello" {
		t.Errorf("stringDeref(&%q) = %q, want %q", v, got, "hello")
	}
}

// TestTitleFromFilePath verifies the path-parsing helper.
func TestTitleFromFilePath(t *testing.T) {
	tests := []struct {
		fp   string
		want string
	}{
		{"/audio/Author Name/Book Title/chapter1.m4b", "Book Title"},
		{"/single/file.m4b", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := titleFromFilePath(tt.fp)
		if got != tt.want {
			t.Errorf("titleFromFilePath(%q) = %q, want %q", tt.fp, got, tt.want)
		}
	}
}

// TestCountApplied and TestCountErrors verify the summary counters.
func TestCountApplied(t *testing.T) {
	results := []readByFixResult{
		{Applied: true},
		{Applied: false},
		{Applied: true},
	}
	if got := countApplied(results); got != 2 {
		t.Errorf("countApplied = %d, want 2", got)
	}
}

func TestCountErrors(t *testing.T) {
	results := []readByFixResult{
		{Error: "oops"},
		{Error: ""},
		{Error: "another error"},
	}
	if got := countErrors(results); got != 2 {
		t.Errorf("countErrors = %d, want 2", got)
	}
}
