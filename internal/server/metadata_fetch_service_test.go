// file: internal/server/metadata_fetch_service_test.go
// version: 2.2.0
// guid: f6a7b8c9-d0e1-f2a3-b4c5-d6e7f8a9b0c1

package server

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// mockMetadataSource implements metadata.MetadataSource for unit tests.
type mockMetadataSource struct {
	name                   string
	searchByTitleFunc      func(title string) ([]metadata.BookMetadata, error)
	searchByTitleAndAuthor func(title, author string) ([]metadata.BookMetadata, error)
}

func (m *mockMetadataSource) Name() string { return m.name }
func (m *mockMetadataSource) SearchByTitle(title string) ([]metadata.BookMetadata, error) {
	if m.searchByTitleFunc != nil {
		return m.searchByTitleFunc(title)
	}
	return nil, nil
}
func (m *mockMetadataSource) SearchByTitleAndAuthor(title, author string) ([]metadata.BookMetadata, error) {
	if m.searchByTitleAndAuthor != nil {
		return m.searchByTitleAndAuthor(title, author)
	}
	return nil, nil
}

// setupGlobalStoreForTest sets database.GlobalStore to a MockStore that handles
// metadata state calls (needed by persistFetchedMetadata -> loadMetadataState).
func setupGlobalStoreForTest(t *testing.T) {
	t.Helper()
	old := database.GlobalStore
	t.Cleanup(func() { database.GlobalStore = old })
	database.GlobalStore = &database.MockStore{
		GetMetadataFieldStatesFunc: func(bookID string) ([]database.MetadataFieldState, error) {
			return nil, nil
		},
		UpsertMetadataFieldStateFunc: func(state *database.MetadataFieldState) error {
			return nil
		},
	}
}

// --- Existing tests ---

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
	chain := mfs.BuildSourceChain()

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

// --- New tests ---

func TestMetadataFetchService_Source1Fails_Source2Succeeds(t *testing.T) {
	setupGlobalStoreForTest(t)

	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "The Hobbit"}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	source1 := &mockMetadataSource{
		name: "FailSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}
	source2 := &mockMetadataSource{
		name: "GoodSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			return []metadata.BookMetadata{{Title: "The Hobbit", Author: "J.R.R. Tolkien"}}, nil
		},
	}
	mfs.overrideSources = []metadata.MetadataSource{source1, source2}

	resp, err := mfs.FetchMetadataForBook("book1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != "GoodSource" {
		t.Errorf("expected source 'GoodSource', got %q", resp.Source)
	}
}

func TestMetadataFetchService_TitleStrippingFallback(t *testing.T) {
	setupGlobalStoreForTest(t)

	authorID := 1
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "The Hobbit - Chapter 5", AuthorID: &authorID}, nil
		},
		GetAuthorByIDFunc: func(id int) (*database.Author, error) {
			return &database.Author{ID: id, Name: "Tolkien"}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	var searchCount int32
	src := &mockMetadataSource{
		name: "TestSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			atomic.AddInt32(&searchCount, 1)
			return nil, nil // no results for title-only searches
		},
		searchByTitleAndAuthor: func(title, author string) ([]metadata.BookMetadata, error) {
			atomic.AddInt32(&searchCount, 1)
			// Only the stripped title + author search succeeds
			if title == "The Hobbit" && author == "Tolkien" {
				return []metadata.BookMetadata{{Title: "The Hobbit", Author: "Tolkien"}}, nil
			}
			return nil, nil
		},
	}
	mfs.overrideSources = []metadata.MetadataSource{src}

	resp, err := mfs.FetchMetadataForBook("book1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	count := atomic.LoadInt32(&searchCount)
	if count < 3 {
		t.Errorf("expected at least 3 search calls, got %d", count)
	}
}

func TestMetadataFetchService_AllSourcesNoResults(t *testing.T) {
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "Nonexistent Book"}, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	emptySrc := func(name string) *mockMetadataSource {
		return &mockMetadataSource{
			name: name,
			searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
				return nil, nil
			},
		}
	}
	mfs.overrideSources = []metadata.MetadataSource{emptySrc("A"), emptySrc("B"), emptySrc("C")}

	_, err := mfs.FetchMetadataForBook("book1")
	if err == nil {
		t.Fatal("expected error when all sources return no results")
	}
	if !strings.Contains(err.Error(), "no metadata found") {
		t.Errorf("expected error to contain 'no metadata found', got %q", err.Error())
	}
}

func TestMetadataFetchService_AllSourcesError(t *testing.T) {
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "Some Book"}, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	errSrc := func(name string) *mockMetadataSource {
		return &mockMetadataSource{
			name: name,
			searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
				return nil, fmt.Errorf("%s error", name)
			},
		}
	}
	mfs.overrideSources = []metadata.MetadataSource{errSrc("SourceA"), errSrc("SourceB")}

	_, err := mfs.FetchMetadataForBook("book1")
	if err == nil {
		t.Fatal("expected error when all sources return errors")
	}
	if !strings.Contains(err.Error(), "last error") {
		t.Errorf("expected error to contain 'last error', got %q", err.Error())
	}
}

func TestMetadataFetchService_ApplyMetadata_AllFields(t *testing.T) {
	mockDB := &database.MockStore{}
	mfs := NewMetadataFetchService(mockDB)

	book := &database.Book{ID: "1", Title: "Old"}
	meta := metadata.BookMetadata{
		Title:       "New Title",
		Publisher:   "Acme Press",
		Language:    "en",
		PublishYear: 2023,
		CoverURL:    "https://example.com/cover.jpg",
	}

	mfs.applyMetadataToBook(book, meta)

	if book.Title != "New Title" {
		t.Errorf("Title: want 'New Title', got %q", book.Title)
	}
	if book.Publisher == nil || *book.Publisher != "Acme Press" {
		t.Errorf("Publisher: want 'Acme Press', got %v", book.Publisher)
	}
	if book.Language == nil || *book.Language != "en" {
		t.Errorf("Language: want 'en', got %v", book.Language)
	}
	if book.AudiobookReleaseYear == nil || *book.AudiobookReleaseYear != 2023 {
		t.Errorf("AudiobookReleaseYear: want 2023, got %v", book.AudiobookReleaseYear)
	}
	if book.CoverURL == nil || *book.CoverURL != "https://example.com/cover.jpg" {
		t.Errorf("CoverURL: want cover URL, got %v", book.CoverURL)
	}
}

func TestMetadataFetchService_ApplyMetadata_EmptyFieldsPreserved(t *testing.T) {
	mockDB := &database.MockStore{}
	mfs := NewMetadataFetchService(mockDB)

	origPublisher := "Original Publisher"
	origLang := "fr"
	origYear := 1999
	origCover := "https://example.com/old.jpg"
	book := &database.Book{
		ID:                   "1",
		Title:                "Original Title",
		Publisher:            &origPublisher,
		Language:             &origLang,
		AudiobookReleaseYear: &origYear,
		CoverURL:             &origCover,
	}

	// Empty metadata should not overwrite existing fields
	meta := metadata.BookMetadata{}
	mfs.applyMetadataToBook(book, meta)

	if book.Title != "Original Title" {
		t.Errorf("Title should be preserved, got %q", book.Title)
	}
	if *book.Publisher != "Original Publisher" {
		t.Errorf("Publisher should be preserved, got %q", *book.Publisher)
	}
	if *book.Language != "fr" {
		t.Errorf("Language should be preserved, got %q", *book.Language)
	}
	if *book.AudiobookReleaseYear != 1999 {
		t.Errorf("Year should be preserved, got %d", *book.AudiobookReleaseYear)
	}
	if *book.CoverURL != "https://example.com/old.jpg" {
		t.Errorf("CoverURL should be preserved, got %q", *book.CoverURL)
	}
}

func TestMetadataFetchService_AuthorLookupFails(t *testing.T) {
	setupGlobalStoreForTest(t)

	authorID := 99
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "Test Book", AuthorID: &authorID}, nil
		},
		GetAuthorByIDFunc: func(id int) (*database.Author, error) {
			return nil, fmt.Errorf("author DB error")
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	src := &mockMetadataSource{
		name: "TestSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			return []metadata.BookMetadata{{Title: "Test Book", Publisher: "Found Publisher"}}, nil
		},
	}
	mfs.overrideSources = []metadata.MetadataSource{src}

	resp, err := mfs.FetchMetadataForBook("book1")
	if err != nil {
		t.Fatalf("search should proceed despite author lookup failure: %v", err)
	}
	if resp.Source != "TestSource" {
		t.Errorf("expected source 'TestSource', got %q", resp.Source)
	}
}

func TestStripChapterFromTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Hobbit", "The Hobbit"},
		{"The Hobbit - Chapter 5", "The Hobbit"},
		{"The Hobbit: Book 2", "The Hobbit"},
		{"The Hobbit (Chapter 3)", "The Hobbit"},
		{"Series Name Book 1", "Series Name"},
		{"Already Clean Title", "Already Clean Title"},
		{"Title, Chapter 7", "Title"},
		{"Title (Unabridged)", "Title"},
		// Bracket series prefix patterns
		{"[The Expanse 9.0] Leviathan Falls", "Leviathan Falls"},
		{"[Series Name] The Book Title", "The Book Title"},
		{"[Dresden Files 1] Storm Front", "Storm Front"},
		// Trailing brackets
		{"Title [Unabridged]", "Title"},
		// Part/Volume patterns
		{"The Fellowship of the Ring Part 2", "The Fellowship of the Ring"},
		{"Dune Volume 1", "Dune"},
		{"Book Title Vol. 3", "Book Title"},
		// Hash number patterns
		{"Title #12", "Title"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := stripChapterFromTitle(tc.input)
			if got != tc.want {
				t.Errorf("stripChapterFromTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestStripSubtitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Title: Subtitle", "Title"},
		{"Title - Subtitle", "Title"},
		{"Title — Subtitle", "Title"},
		{"No Subtitle", "No Subtitle"},
		{"", ""},
		{"Colon:NoSpace", "Colon:NoSpace"},
		{"A: B - C", "A"},                     // colon takes priority
		{"Multi - Word Title - Sub", "Multi"},  // first dash wins
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := stripSubtitle(tc.input)
			if got != tc.want {
				t.Errorf("stripSubtitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBestTitleMatch(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Completely Different Book"},
		{Title: "The Great Adventure Story"},
		{Title: "Great Adventure"},
	}

	got := bestTitleMatch(results, "The Great Adventure")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	// "The Great Adventure Story" has 3 word overlaps (the, great, adventure)
	// but "the" is <=2 chars so skipped. "great" + "adventure" = 2 for both.
	// "The Great Adventure Story" and "Great Adventure" both score 2,
	// but the first one encountered wins (index 1).
	if got[0].Title != "The Great Adventure Story" {
		t.Errorf("expected 'The Great Adventure Story', got %q", got[0].Title)
	}
}

func TestBestTitleMatch_NoOverlap(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Completely Different Book"},
		{Title: "Another Unrelated Title"},
	}

	got := bestTitleMatch(results, "Xyz Qwerty")
	if got != nil {
		t.Errorf("expected nil for no overlap, got %v", got)
	}
}

func TestBestTitleMatch_EmptyResults(t *testing.T) {
	got := bestTitleMatch(nil, "Some Title")
	if got != nil {
		t.Errorf("expected nil for empty results, got %v", got)
	}

	got = bestTitleMatch([]metadata.BookMetadata{}, "Some Title")
	if got != nil {
		t.Errorf("expected nil for empty slice, got %v", got)
	}
}
