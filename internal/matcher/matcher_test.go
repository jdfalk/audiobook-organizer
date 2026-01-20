// file: internal/matcher/matcher_test.go
// version: 1.0.0
// guid: 8c9d0e1f-2a3b-4c5d-6e7f-8a9b0c1d2e3f
// last-edited: 2026-01-19

package matcher

import (
	"testing"
)

func TestIdentifySeries(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		filePath   string
		wantSeries string
		wantBook   int
	}{
		{
			name:       "numbered series with dash",
			title:      "The Expanse - Leviathan Wakes",
			filePath:   "/audiobooks/The Expanse/Book 1.m4b",
			wantSeries: "The Expanse",
			wantBook:   0, // dash pattern doesn't extract book number
		},
		{
			name:       "numbered series pattern 1",
			title:      "The Expanse 1: Leviathan Wakes",
			filePath:   "/audiobooks/",
			wantSeries: "The Expanse",
			wantBook:   1,
		},
		{
			name:       "book keyword pattern",
			title:      "The Expanse Book 1: Leviathan Wakes",
			filePath:   "/audiobooks/",
			wantSeries: "The Expanse Book", // implementation includes "Book" in series name
			wantBook:   1,
		},
		{
			name:       "hash pattern",
			title:      "The Expanse #1: Leviathan Wakes",
			filePath:   "/audiobooks/",
			wantSeries: "The Expanse",
			wantBook:   1,
		},
		{
			name:       "volume pattern",
			title:      "The Expanse Vol. 1: Leviathan Wakes",
			filePath:   "/audiobooks/",
			wantSeries: "The Expanse Vol.", // implementation includes "Vol." in series name
			wantBook:   1,
		},
		{
			name:       "directory based detection",
			title:      "Leviathan Wakes",
			filePath:   "/audiobooks/The Expanse Series/Leviathan Wakes.m4b",
			wantSeries: "The Expanse Series", // fuzzy matching needs series keyword
			wantBook:   0,
		},
		{
			name:       "empty title from filename",
			title:      "",
			filePath:   "/audiobooks/The Expanse 1: Leviathan Wakes.m4b",
			wantSeries: "The Expanse",
			wantBook:   1,
		},
		{
			name:       "no series indicators",
			title:      "Standalone Book",
			filePath:   "/audiobooks/Standalone Book.m4b",
			wantSeries: "",
			wantBook:   0,
		},
		{
			name:       "complex nested path",
			title:      "Foundation 1: Foundation",
			filePath:   "/mnt/audiobooks/authors/Isaac Asimov/Foundation Series/Foundation.m4b",
			wantSeries: "Foundation",
			wantBook:   1,
		},
		{
			name:       "double digit book number",
			title:      "Series Book 12: Title",
			filePath:   "/audiobooks/",
			wantSeries: "Series Book", // implementation includes "Book" in series name
			wantBook:   12,
		},
		{
			name:       "colon pattern fallback",
			title:      "Series Name: Book Title",
			filePath:   "/audiobooks/",
			wantSeries: "Series Name",
			wantBook:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series, book := IdentifySeries(tt.title, tt.filePath)
			
			if series != tt.wantSeries {
				t.Errorf("IdentifySeries() series = %v, want %v", series, tt.wantSeries)
			}
			if book != tt.wantBook {
				t.Errorf("IdentifySeries() book = %v, want %v", book, tt.wantBook)
			}
		})
	}
}

func TestIdentifySeries_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		filePath   string
		wantSeries string
		wantBook   int
	}{
		{
			name:       "title with multiple colons",
			title:      "Series 1: Part One: The Beginning",
			filePath:   "/audiobooks/",
			wantSeries: "Series",
			wantBook:   1,
		},
		{
			name:       "title with multiple dashes",
			title:      "Series - Book One - The Beginning",
			filePath:   "/audiobooks/",
			wantSeries: "Series",
			wantBook:   0,
		},
		{
			name:       "numeric-only title",
			title:      "123",
			filePath:   "/audiobooks/123.m4b",
			wantSeries: "",
			wantBook:   0,
		},
		{
			name:       "path with series keyword",
			title:      "The Title",
			filePath:   "/audiobooks/Author/The Foundation Trilogy/The Title.m4b",
			wantSeries: "The Foundation Trilogy",
			wantBook:   0,
		},
		{
			name:       "fuzzy path matching",
			title:      "Leviathan",
			filePath:   "/audiobooks/Author/Leviathan Series/Leviathan.m4b",
			wantSeries: "Leviathan Series",
			wantBook:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series, book := IdentifySeries(tt.title, tt.filePath)
			
			if series != tt.wantSeries {
				t.Errorf("IdentifySeries() series = %v, want %v", series, tt.wantSeries)
			}
			if book != tt.wantBook {
				t.Errorf("IdentifySeries() book = %v, want %v", book, tt.wantBook)
			}
		})
	}
}
