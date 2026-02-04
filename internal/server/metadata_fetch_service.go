// file: internal/server/metadata_fetch_service.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"fmt"
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

type MetadataFetchService struct {
	db database.Store
}

func NewMetadataFetchService(db database.Store) *MetadataFetchService {
	return &MetadataFetchService{db: db}
}

type FetchMetadataResponse struct {
	Message      string
	Book         *database.Book
	Source       string
	FetchedCount int
}

// FetchMetadataForBook fetches and applies metadata for a single audiobook
func (mfs *MetadataFetchService) FetchMetadataForBook(id string) (*FetchMetadataResponse, error) {
	// Get the audiobook
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// Search for metadata using current title
	client := metadata.NewOpenLibraryClient()

	// Strip chapter/book numbers to improve search results
	searchTitle := stripChapterFromTitle(book.Title)

	// Try with cleaned title first
	results, err := client.SearchByTitle(searchTitle)

	// Fall back to original title if cleaned search fails
	if (err != nil || len(results) == 0) && searchTitle != book.Title {
		results, err = client.SearchByTitle(book.Title)
	}

	// Final fallback: try with author if we have one
	if (err != nil || len(results) == 0) && book.AuthorID != nil {
		author, authorErr := mfs.db.GetAuthorByID(*book.AuthorID)
		if authorErr == nil && author != nil && author.Name != "" {
			log.Printf("[INFO] FetchMetadataForBook: Trying fallback search with author: %s", author.Name)
			results, err = client.SearchByTitleAndAuthor(searchTitle, author.Name)

			// Also try with original title + author if cleaned title failed
			if (err != nil || len(results) == 0) && searchTitle != book.Title {
				results, err = client.SearchByTitleAndAuthor(book.Title, author.Name)
			}
		}
	}

	if err != nil || len(results) == 0 {
		if book.AuthorID != nil {
			author, _ := mfs.db.GetAuthorByID(*book.AuthorID)
			if author != nil {
				return nil, fmt.Errorf("no metadata found for '%s' by '%s' in Open Library", book.Title, author.Name)
			}
		}
		return nil, fmt.Errorf("no metadata found for this book in Open Library")
	}

	// Use the first result
	meta := results[0]

	// Update book with fetched metadata
	mfs.applyMetadataToBook(book, &meta)

	// Update in database
	updatedBook, err := mfs.db.UpdateBook(id, book)
	if err != nil {
		return nil, fmt.Errorf("failed to update book: %w", err)
	}

	// Persist fetched metadata state
	mfs.persistFetchedMetadata(id, &meta)

	return &FetchMetadataResponse{
		Message: "metadata fetched and applied",
		Book:    updatedBook,
		Source:  "Open Library",
	}, nil
}

func (mfs *MetadataFetchService) applyMetadataToBook(book *database.Book, meta *metadata.BookMetadata) {
	if meta.Title != "" {
		book.Title = meta.Title
	}
	if meta.Publisher != "" {
		book.Publisher = stringPtr(meta.Publisher)
	}
	if meta.Language != "" {
		book.Language = stringPtr(meta.Language)
	}
	if meta.PublishYear != 0 {
		book.AudiobookReleaseYear = intPtrHelper(meta.PublishYear)
	}
}

func (mfs *MetadataFetchService) persistFetchedMetadata(bookID string, meta *metadata.BookMetadata) {
	fetchedValues := map[string]any{}
	if meta.Title != "" {
		fetchedValues["title"] = meta.Title
	}
	if meta.Publisher != "" {
		fetchedValues["publisher"] = meta.Publisher
	}
	if meta.Language != "" {
		fetchedValues["language"] = meta.Language
	}
	if meta.PublishYear != 0 {
		fetchedValues["audiobook_release_year"] = meta.PublishYear
	}
	if meta.Author != "" {
		fetchedValues["author_name"] = meta.Author
	}
	if meta.ISBN != "" {
		if len(meta.ISBN) == 10 {
			fetchedValues["isbn10"] = meta.ISBN
		} else {
			fetchedValues["isbn13"] = meta.ISBN
		}
	}
	if len(fetchedValues) > 0 {
		if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
			log.Printf("[ERROR] FetchMetadataForBook: failed to persist fetched metadata state: %v", err)
		}
	}
}
