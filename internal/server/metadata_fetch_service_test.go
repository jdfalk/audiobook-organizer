// file: internal/server/metadata_fetch_service_test.go
// version: 2.0.0
// guid: f6a7b8c9-d0e1-f2a3-b4c5-d6e7f8a9b0c1

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
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

	mfs.applyMetadataToBook(book, meta)

	if book.Title != "Fetched Title" {
		t.Errorf("expected title 'Fetched Title', got %q", book.Title)
	}
	if book.Publisher == nil || *book.Publisher != "Test Publisher" {
		t.Errorf("expected publisher 'Test Publisher', got %v", book.Publisher)
	}
}

func TestMetadataFetchService_BuildSourceChain(t *testing.T) {
	// Save and restore config
	origSources := config.AppConfig.MetadataSources
	defer func() { config.AppConfig.MetadataSources = origSources }()

	config.AppConfig.MetadataSources = []config.MetadataSource{
		{ID: "google-books", Name: "Google Books", Enabled: true, Priority: 3},
		{ID: "openlibrary", Name: "Open Library", Enabled: true, Priority: 1},
		{ID: "audnexus", Name: "Audnexus", Enabled: false, Priority: 2},
	}

	mockDB := &database.MockStore{}
	mfs := NewMetadataFetchService(mockDB)
	chain := mfs.buildSourceChain()

	if len(chain) != 2 {
		t.Fatalf("expected 2 enabled sources, got %d", len(chain))
	}
	if chain[0].Name() != "Open Library" {
		t.Errorf("expected first source 'Open Library', got %q", chain[0].Name())
	}
	if chain[1].Name() != "Google Books" {
		t.Errorf("expected second source 'Google Books', got %q", chain[1].Name())
	}
}

func TestMetadataFetchService_NoSourcesEnabled(t *testing.T) {
	origSources := config.AppConfig.MetadataSources
	defer func() { config.AppConfig.MetadataSources = origSources }()

	config.AppConfig.MetadataSources = []config.MetadataSource{
		{ID: "openlibrary", Enabled: false, Priority: 1},
	}

	authorID := 1
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "Test", AuthorID: &authorID}, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	_, err := mfs.FetchMetadataForBook("book1")
	if err == nil {
		t.Error("expected error when no sources enabled")
	}
}
