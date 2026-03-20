// file: internal/metadata/custom_tags.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package metadata

// Custom tag key constants for embedding audiobook organizer metadata in audio files.
// All keys use the AUDIOBOOK_ORGANIZER_ prefix to avoid collisions with standard tags.
const (
	TagBookID      = "AUDIOBOOK_ORGANIZER_ID"
	TagVersion     = "AUDIOBOOK_ORGANIZER_VERSION"
	TagISBN10      = "AUDIOBOOK_ORGANIZER_ISBN10"
	TagISBN13      = "AUDIOBOOK_ORGANIZER_ISBN13"
	TagASIN        = "AUDIOBOOK_ORGANIZER_ASIN"
	TagOpenLibrary = "AUDIOBOOK_ORGANIZER_OPENLIBRARY"
	TagHardcover   = "AUDIOBOOK_ORGANIZER_HARDCOVER"
	TagGoogleBooks = "AUDIOBOOK_ORGANIZER_GOOGLEBOOKS"
	TagEdition     = "AUDIOBOOK_ORGANIZER_EDITION"
	TagPrintYear   = "AUDIOBOOK_ORGANIZER_PRINT_YEAR"
	TagCoverURL    = "AUDIOBOOK_ORGANIZER_COVER_URL"

	// CustomTagVersion is the current schema version for custom tags.
	CustomTagVersion = "1"
)

// CustomTags holds the custom audiobook organizer tag values extracted from or
// destined for audio file metadata.
type CustomTags struct {
	BookID        string
	Version       string
	ISBN10        string
	ISBN13        string
	ASIN          string
	OpenLibraryID string
	HardcoverID   string
	GoogleBooksID string
}

// ToMap converts CustomTags to a map[string]string for writing to audio files.
// Only non-empty values are included.
func (ct CustomTags) ToMap() map[string]string {
	m := make(map[string]string)
	if ct.BookID != "" {
		m[TagBookID] = ct.BookID
	}
	m[TagVersion] = CustomTagVersion
	if ct.ISBN10 != "" {
		m[TagISBN10] = ct.ISBN10
	}
	if ct.ISBN13 != "" {
		m[TagISBN13] = ct.ISBN13
	}
	if ct.ASIN != "" {
		m[TagASIN] = ct.ASIN
	}
	if ct.OpenLibraryID != "" {
		m[TagOpenLibrary] = ct.OpenLibraryID
	}
	if ct.HardcoverID != "" {
		m[TagHardcover] = ct.HardcoverID
	}
	if ct.GoogleBooksID != "" {
		m[TagGoogleBooks] = ct.GoogleBooksID
	}
	return m
}
