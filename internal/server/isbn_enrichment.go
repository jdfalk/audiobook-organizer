// file: internal/server/isbn_enrichment.go
// version: 1.0.0
// guid: 34290bd0-745e-4509-ad2d-e237785bb7ef

package server

import (
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// ISBNEnrichmentService searches external metadata sources for ISBN (and ASIN)
// when a book is missing those identifiers after a metadata fetch/apply.
type ISBNEnrichmentService struct {
	db      database.Store
	sources []metadata.MetadataSource
}

// NewISBNEnrichmentService creates an enrichment service that will search the
// given metadata sources for ISBN/ASIN data.
func NewISBNEnrichmentService(db database.Store, sources []metadata.MetadataSource) *ISBNEnrichmentService {
	return &ISBNEnrichmentService{db: db, sources: sources}
}

// EnrichBookISBN searches external sources for ISBN if the book doesn't have one.
// It also back-fills ASIN from Audible when missing. Returns true if any
// identifier was found and saved.
func (s *ISBNEnrichmentService) EnrichBookISBN(bookID string) (bool, error) {
	book, err := s.db.GetBookByID(bookID)
	if err != nil || book == nil {
		return false, err
	}

	hasISBN := (book.ISBN10 != nil && *book.ISBN10 != "") || (book.ISBN13 != nil && *book.ISBN13 != "")
	hasASIN := book.ASIN != nil && *book.ASIN != ""

	// Nothing to do if both are already present.
	if hasISBN && hasASIN {
		return false, nil
	}

	// Build search query from book title + author.
	title := book.Title
	author := s.resolveAuthor(book)

	updated := false

	// --- ISBN enrichment ---
	if !hasISBN {
		for _, src := range s.sources {
			isbn, isbnLen := s.searchSourceForISBN(src, title, author)
			if isbn == "" {
				continue
			}
			if isbnLen == 13 {
				book.ISBN13 = &isbn
			} else {
				book.ISBN10 = &isbn
			}
			if _, err := s.db.UpdateBook(bookID, book); err != nil {
				return false, err
			}
			log.Printf("[INFO] ISBN enrichment: found %s for %q (%s)", isbn, title, src.Name())
			updated = true
			break
		}
	}

	// --- ASIN enrichment ---
	if !hasASIN {
		for _, src := range s.sources {
			if src.Name() != "Audible" {
				continue
			}
			asin := s.searchSourceForASIN(src, title, author)
			if asin == "" {
				break
			}
			book.ASIN = &asin
			if _, err := s.db.UpdateBook(bookID, book); err != nil {
				return updated, err
			}
			log.Printf("[INFO] ASIN enrichment: found %s for %q", asin, title)
			updated = true
			break
		}
	}

	return updated, nil
}

// resolveAuthor returns the author name for the book, or "" if unknown.
func (s *ISBNEnrichmentService) resolveAuthor(book *database.Book) string {
	if book.AuthorID == nil {
		return ""
	}
	a, err := s.db.GetAuthorByID(*book.AuthorID)
	if err != nil || a == nil {
		return ""
	}
	return a.Name
}

// searchSourceForISBN queries a single metadata source and returns the first
// ISBN that matches the title strictly, along with its length (10 or 13).
func (s *ISBNEnrichmentService) searchSourceForISBN(src metadata.MetadataSource, title, author string) (string, int) {
	var results []metadata.BookMetadata

	if author != "" {
		results, _ = src.SearchByTitleAndAuthor(title, author)
	}
	if len(results) == 0 {
		results, _ = src.SearchByTitle(title)
	}

	for _, r := range results {
		if !isStrictTitleMatch(title, r.Title) {
			continue
		}
		if r.ISBN != "" {
			return r.ISBN, len(r.ISBN)
		}
	}
	return "", 0
}

// searchSourceForASIN queries a single metadata source and returns the first
// ASIN that matches the title strictly.
func (s *ISBNEnrichmentService) searchSourceForASIN(src metadata.MetadataSource, title, author string) string {
	var results []metadata.BookMetadata

	if author != "" {
		results, _ = src.SearchByTitleAndAuthor(title, author)
	}
	if len(results) == 0 {
		results, _ = src.SearchByTitle(title)
	}

	for _, r := range results {
		if isStrictTitleMatch(title, r.Title) && r.ASIN != "" {
			return r.ASIN
		}
	}
	return ""
}

// isStrictTitleMatch returns true if titles are close enough to be the same book.
func isStrictTitleMatch(dbTitle, searchTitle string) bool {
	a := strings.ToLower(strings.TrimSpace(dbTitle))
	b := strings.ToLower(strings.TrimSpace(searchTitle))
	if a == "" || b == "" {
		return false
	}
	// Exact match
	if a == b {
		return true
	}
	// One is a prefix of the other (e.g., "Shadows of Self" matches "Shadows of Self: A Mistborn Novel")
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		// But only if the shorter one is at least 60% of the longer one's length
		shorter := len(a)
		longer := len(b)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return float64(shorter)/float64(longer) >= 0.6
	}
	return false
}
