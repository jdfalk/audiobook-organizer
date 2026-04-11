// file: internal/metadata/taglib_reader_test.go
// version: 1.0.0
// guid: b1a2c3d4-e5f6-7890-1234-567890abcdef

package metadata

import (
	"testing"
)

// TestBuildMetadataFromTaglibMap covers the common shapes TagLib emits
// when reading from an audiobook file. This is the fallback path used
// when dhowden/tag can't parse a file — we need it to produce a usable
// Metadata for every format the scanner actually cares about.
func TestBuildMetadataFromTaglibMap(t *testing.T) {
	tests := []struct {
		name     string
		tags     map[string][]string
		filePath string
		check    func(t *testing.T, m Metadata)
	}{
		{
			name: "basic m4b tags",
			tags: map[string][]string{
				"TITLE":       {"Foundation and Empire"},
				"ARTIST":      {"Isaac Asimov"},
				"ALBUMARTIST": {"Isaac Asimov"},
				"ALBUM":       {"Foundation 4 - Foundation and Empire"},
				"GENRE":       {"Science Fiction"},
				"DATE":        {"1952"},
			},
			filePath: "/tmp/foundation-and-empire.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Title != "Foundation and Empire" {
					t.Errorf("title = %q, want Foundation and Empire", m.Title)
				}
				if m.Artist != "Isaac Asimov" {
					t.Errorf("artist = %q, want Isaac Asimov", m.Artist)
				}
				if m.Year != 1952 {
					t.Errorf("year = %d, want 1952", m.Year)
				}
				if m.Genre != "Science Fiction" {
					t.Errorf("genre = %q", m.Genre)
				}
			},
		},
		{
			name: "case insensitive keys",
			tags: map[string][]string{
				// Vorbis comments can come through lowercase
				"title":  {"The Hobbit"},
				"artist": {"J.R.R. Tolkien"},
				"album":  {"The Hobbit"},
			},
			filePath: "/tmp/hobbit.ogg",
			check: func(t *testing.T, m Metadata) {
				if m.Title != "The Hobbit" {
					t.Errorf("title = %q, want The Hobbit", m.Title)
				}
				if m.Artist != "J.R.R. Tolkien" {
					t.Errorf("artist = %q", m.Artist)
				}
			},
		},
		{
			name: "album_artist beats artist",
			tags: map[string][]string{
				"TITLE":        {"Foundation"},
				"ARTIST":       {"Scott Brick"},  // narrator
				"ALBUMARTIST":  {"Isaac Asimov"}, // author
			},
			filePath: "/tmp/foundation.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Artist != "Isaac Asimov" {
					t.Errorf("expected ALBUMARTIST to win, got %q", m.Artist)
				}
				// ARTIST should fall through to Narrator since it wasn't used
				if m.Narrator != "Scott Brick" {
					t.Errorf("narrator = %q, want Scott Brick", m.Narrator)
				}
			},
		},
		{
			name: "composer fallback when neither artist nor album_artist set",
			tags: map[string][]string{
				"TITLE":    {"Mystery"},
				"COMPOSER": {"Fallback Author"},
			},
			filePath: "/tmp/mystery.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Artist != "Fallback Author" {
					t.Errorf("expected composer fallback, got %q", m.Artist)
				}
			},
		},
		{
			name: "explicit NARRATOR property",
			tags: map[string][]string{
				"TITLE":    {"Book"},
				"ARTIST":   {"Author Name"},
				"NARRATOR": {"Narrator Name"},
			},
			filePath: "/tmp/book.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Narrator != "Narrator Name" {
					t.Errorf("narrator = %q, want Narrator Name", m.Narrator)
				}
			},
		},
		{
			name: "series and series_index",
			tags: map[string][]string{
				"TITLE":        {"Foundation and Empire"},
				"ARTIST":       {"Isaac Asimov"},
				"SERIES":       {"Foundation"},
				"SERIES_INDEX": {"4"},
			},
			filePath: "/tmp/foundation-4.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Series != "Foundation" {
					t.Errorf("series = %q", m.Series)
				}
				if m.SeriesIndex != 4 {
					t.Errorf("series_index = %d, want 4", m.SeriesIndex)
				}
			},
		},
		{
			name: "AUDIOBOOK_ORGANIZER custom tags round-trip",
			tags: map[string][]string{
				"TITLE":                          {"Book"},
				"ARTIST":                         {"Author"},
				"AUDIOBOOK_ORGANIZER_BOOK_ID":    {"01KNDB000000000000000000"},
				"AUDIOBOOK_ORGANIZER_TAG_VERSION": {"3"},
				"EDITION":                        {"Unabridged"},
			},
			filePath: "/tmp/book.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.BookOrganizerID != "01KNDB000000000000000000" {
					t.Errorf("BookOrganizerID = %q", m.BookOrganizerID)
				}
				if m.OrganizerTagVersion != "3" {
					t.Errorf("OrganizerTagVersion = %q", m.OrganizerTagVersion)
				}
				if m.Edition != "Unabridged" {
					t.Errorf("Edition = %q", m.Edition)
				}
			},
		},
		{
			name: "ISBN normalization",
			tags: map[string][]string{
				"TITLE":  {"Book"},
				"ARTIST": {"Author"},
				"ISBN13": {"978-0-553-29335-0"},
				"ISBN10": {"0-553-29335-4"},
				"ASIN":   {"B00MW8EZJU"},
			},
			filePath: "/tmp/book.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.ISBN13 != "9780553293350" {
					t.Errorf("ISBN13 = %q, want 9780553293350", m.ISBN13)
				}
				if m.ISBN10 != "0553293354" {
					t.Errorf("ISBN10 = %q, want 0553293354", m.ISBN10)
				}
				if m.ASIN != "B00MW8EZJU" {
					t.Errorf("ASIN = %q", m.ASIN)
				}
			},
		},
		{
			name: "missing title falls back to album then filename",
			tags: map[string][]string{
				"ARTIST": {"Author"},
				"ALBUM":  {"Album Fallback"},
			},
			filePath: "/tmp/book.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Title != "Album Fallback" {
					t.Errorf("title = %q, want Album Fallback (album fallback)", m.Title)
				}
			},
		},
		{
			name:     "empty tag map falls back to filename basename for title",
			tags:     map[string][]string{},
			filePath: "/tmp/some-audiobook-file.m4b",
			check: func(t *testing.T, m Metadata) {
				if m.Title != "some-audiobook-file" {
					t.Errorf("title = %q, want basename", m.Title)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := BuildMetadataFromTaglibMap(tc.tags, tc.filePath, nil)
			tc.check(t, m)
		})
	}
}

// TestNormalizeISBN covers the hyphen/whitespace-stripping logic that
// turns raw tag values into canonical ISBN strings, plus the length
// validation that rejects anything that isn't 10 or 13 digits.
func TestNormalizeISBN(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"978-0-553-29335-0", "9780553293350"},
		{"9780553293350", "9780553293350"},
		{"0553293354", "0553293354"},
		{"0-553-29335-4", "0553293354"},
		{"  978 0 553 29335 0  ", "9780553293350"},
		{"12345", ""},                  // too short
		{"12345678901234567", ""},      // too long
		{"978055329335X", "978055329335X"},
		{"garbage-string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeISBN(tc.in); got != tc.want {
				t.Errorf("normalizeISBN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
