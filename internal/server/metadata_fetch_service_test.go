// file: internal/server/metadata_fetch_service_test.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-f2a3-b4c5-d6e7f8a9b0c1

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func TestMetadataFetchService_FetchMetadataForBook_NotFound(t *testing.T) {
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return nil, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	_, err := mfs.FetchMetadataForBook("nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent book")
	}
}

func TestMetadataFetchService_ApplyMetadataToBook(t *testing.T) {
	mockDB := &database.MockStore{}
	mfs := NewMetadataFetchService(mockDB)

	book := &database.Book{ID: "1", Title: "Original Title"}
	meta := metadata.BookMetadata{
		Title:     "Fetched Title",
		Publisher: "Test Publisher",
	}

	mfs.applyMetadataToBook(book, &meta)

	if book.Title != "Fetched Title" {
		t.Errorf("expected title 'Fetched Title', got %q", book.Title)
	}
	if book.Publisher == nil || *book.Publisher != "Test Publisher" {
		t.Errorf("expected publisher 'Test Publisher', got %v", book.Publisher)
	}
}