// file: internal/server/metadata_fetch_service_test.go
// version: 4.1.0
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
	// With precision+recall scoring, "Great Adventure" (2/2 result words match
	// query words → precision 1.0) beats "The Great Adventure Story" (2/4 words
	// match → precision 0.5). Both have recall 1.0; "Great Adventure" wins on F1.
	if got[0].Title != "Great Adventure" {
		t.Errorf("expected 'Great Adventure' (higher precision), got %q", got[0].Title)
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

func TestIsGarbageValue(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Unknown", true},
		{"unknown", true},
		{"UNKNOWN", true},
		{"narrator", true},
		{"Narrator", true},
		{"", true},
		{"n/a", true},
		{"Terry Pratchett", false},
		{"Stephen King", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := isGarbageValue(tc.input); got != tc.want {
				t.Errorf("isGarbageValue(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsBetterValue_NeverDowngrade(t *testing.T) {
	// Real author -> "Unknown" should NOT replace
	if isBetterValue("Terry Pratchett", "Unknown") {
		t.Error("should not replace real author with Unknown")
	}
	// Empty -> real value should replace
	if !isBetterValue("", "Terry Pratchett") {
		t.Error("should replace empty with real value")
	}
	// "Unknown" -> real value should replace
	if !isBetterValue("Unknown", "Terry Pratchett") {
		t.Error("should replace Unknown with real value")
	}
	// Real -> real is allowed
	if !isBetterValue("Old Title", "New Title") {
		t.Error("should allow real->real replacement")
	}
}

func TestIsBetterStringPtr_NeverDowngrade(t *testing.T) {
	real := "Real Narrator"
	// Real narrator -> "narrator" garbage should NOT replace
	if isBetterStringPtr(&real, "narrator") {
		t.Error("should not replace real narrator with garbage 'narrator'")
	}
	// nil -> real should replace
	if !isBetterStringPtr(nil, "John Smith") {
		t.Error("should replace nil with real value")
	}
	// garbage -> real should replace
	unknown := "Unknown"
	if !isBetterStringPtr(&unknown, "John Smith") {
		t.Error("should replace Unknown with real value")
	}
}

func TestParseSeriesFromTitle(t *testing.T) {
	tests := []struct {
		input       string
		wantSeries  string
		wantPos     string
		wantTitle   string
	}{
		{"(Long Earth 05) The Long Cosmos", "Long Earth", "5", "The Long Cosmos"},
		{"(Dresden Files 01) Storm Front", "Dresden Files", "1", "Storm Front"},
		{"(Discworld 41) The Shepherd's Crown", "Discworld", "41", "The Shepherd's Crown"},
		{"(Series #3) Title", "Series", "3", "Title"},
		{"Series Name, Book 7", "Series Name", "7", ""},
		{"Just A Regular Title", "", "", ""},
		{"", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			series, pos, title := parseSeriesFromTitle(tc.input)
			if series != tc.wantSeries {
				t.Errorf("series: got %q, want %q", series, tc.wantSeries)
			}
			if pos != tc.wantPos {
				t.Errorf("position: got %q, want %q", pos, tc.wantPos)
			}
			if title != tc.wantTitle {
				t.Errorf("title: got %q, want %q", title, tc.wantTitle)
			}
		})
	}
}

func TestApplyMetadataToBook_NoDowngrade(t *testing.T) {
	mockDB := &database.MockStore{}
	mfs := NewMetadataFetchService(mockDB)

	realAuthor := "Terry Pratchett & Stephen Baxter"
	realNarrator := "Michael Fenton Stevens"
	book := &database.Book{
		ID:       "1",
		Title:    "The Long Cosmos",
		Narrator: &realNarrator,
	}
	// Simulate AuthorID being set (author resolved separately)
	_ = realAuthor

	meta := metadata.BookMetadata{
		Title:    "Unknown",
		Narrator: "narrator",
		Author:   "Unknown",
	}

	mfs.applyMetadataToBook(book, meta)

	// Title should NOT be replaced with "Unknown"
	if book.Title != "The Long Cosmos" {
		t.Errorf("title was downgraded to %q", book.Title)
	}
	// Narrator should NOT be replaced with garbage "narrator"
	if book.Narrator == nil || *book.Narrator != "Michael Fenton Stevens" {
		t.Errorf("narrator was downgraded to %v", book.Narrator)
	}
}

func TestRecordChangeHistory(t *testing.T) {
	var recorded []*database.MetadataChangeRecord
	mockDB := &database.MockStore{
		GetAuthorByIDFunc: func(id int) (*database.Author, error) {
			return &database.Author{ID: id, Name: "Old Author"}, nil
		},
		GetSeriesByIDFunc: func(id int) (*database.Series, error) {
			return &database.Series{ID: id, Name: "Old Series"}, nil
		},
		RecordMetadataChangeFunc: func(record *database.MetadataChangeRecord) error {
			recorded = append(recorded, record)
			return nil
		},
	}
	mfs := NewMetadataFetchService(mockDB)

	authorID := 1
	seriesID := 2
	book := &database.Book{
		ID:       "book1",
		Title:    "Old Title",
		AuthorID: &authorID,
		SeriesID: &seriesID,
	}
	meta := metadata.BookMetadata{
		Title:     "New Title",
		Author:    "New Author",
		Publisher: "New Publisher",
	}

	mfs.recordChangeHistory(book, meta, "TestSource")

	if len(recorded) < 3 {
		t.Fatalf("expected at least 3 change records, got %d", len(recorded))
	}

	// Verify all records have correct source and type
	for _, r := range recorded {
		if r.Source != "TestSource" {
			t.Errorf("expected source 'TestSource', got %q", r.Source)
		}
		if r.ChangeType != "fetched" {
			t.Errorf("expected change_type 'fetched', got %q", r.ChangeType)
		}
		if r.BookID != "book1" {
			t.Errorf("expected book_id 'book1', got %q", r.BookID)
		}
	}
}

// --- scoreTitleMatch tests (Task 1) ---

func TestScoreTitleMatch_BoxSetPenalised(t *testing.T) {
	// The individual book should beat the box set even if the box set contains
	// all the query words plus a lot more.
	results := []metadata.BookMetadata{
		{Title: "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set"},
		{Title: "The Long Earth", Description: "A novel.", CoverURL: "https://example.com/cover.jpg"},
	}
	got := bestTitleMatch(results, "The Long Earth")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Title != "The Long Earth" {
		t.Errorf("expected individual book, got box set: %q", got[0].Title)
	}
}

func TestScoreTitleMatch_CollectionPenalised(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Discworld Collection: Books 1-5"},
		{Title: "The Colour of Magic", Description: "First Discworld novel.", CoverURL: "https://cdn.example.com/cover.jpg", Narrator: "Tony Robinson"},
	}
	got := bestTitleMatch(results, "The Colour of Magic")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "The Colour of Magic" {
		t.Errorf("expected individual book, got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_OmnibusPenalised(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Foundation Omnibus: Foundation, Foundation and Empire, Second Foundation"},
		{Title: "Foundation", Author: "Isaac Asimov", Description: "The galactic empire crumbles.", ISBN: "9780553293357"},
	}
	got := bestTitleMatch(results, "Foundation")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "Foundation" {
		t.Errorf("expected 'Foundation', got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_ExactMatchWins(t *testing.T) {
	// A result with an exact title match should always win.
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos and Other Stories Collection"},
		{Title: "The Long Cosmos", Description: "Book 5 of the Long Earth series.", CoverURL: "https://example.com/c.jpg"},
	}
	got := bestTitleMatch(results, "The Long Cosmos")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "The Long Cosmos" {
		t.Errorf("expected 'The Long Cosmos', got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_BelowThresholdReturnsNil(t *testing.T) {
	// A result that shares no significant words with the query should be
	// rejected (score below minimum threshold).
	results := []metadata.BookMetadata{
		{Title: "A Completely Unrelated Title About Cooking"},
	}
	got := bestTitleMatch(results, "The Long Cosmos")
	if got != nil {
		t.Errorf("expected nil (below quality threshold), got %v", got)
	}
}

func TestScoreTitleMatch_RichMetadataBonus(t *testing.T) {
	// When two results score similarly on title, the one with richer
	// metadata (description + cover) should win.
	results := []metadata.BookMetadata{
		{Title: "Dune"},
		{Title: "Dune", Description: "Paul Atreides travels to Arrakis.", CoverURL: "https://example.com/dune.jpg", ISBN: "9780441013593"},
	}
	got := bestTitleMatch(results, "Dune")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	// The richer result is at index 1; it should be preferred.
	if got[0].Description == "" {
		t.Errorf("expected the richer result (with description), got title-only result")
	}
}

func TestScoreTitleMatch_LengthPenalty(t *testing.T) {
	// A very long title with the search words buried inside should score
	// lower than a concise matching title.
	results := []metadata.BookMetadata{
		// 10-word title containing all query words
		{Title: "Ender Game Complete Guide Expanded Universe Fan Edition Deluxe Version"},
		// Concise exact match
		{Title: "Ender's Game", Description: "Military sci-fi classic.", CoverURL: "https://example.com/enders.jpg"},
	}
	got := bestTitleMatch(results, "Ender's Game")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got[0].Title != "Ender's Game" {
		t.Errorf("expected concise match, got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_NDigitBooksPenalised(t *testing.T) {
	// "5 books" pattern should trigger the compilation penalty.
	results := []metadata.BookMetadata{
		{Title: "Hitchhiker 5 Books Complete Collection Douglas Adams"},
		{Title: "The Hitchhiker's Guide to the Galaxy", Description: "Don't panic.", CoverURL: "https://example.com/h2g2.jpg"},
	}
	got := bestTitleMatch(results, "The Hitchhiker's Guide to the Galaxy")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if strings.Contains(strings.ToLower(got[0].Title), "books") {
		t.Errorf("compilation result should not win: got %q", got[0].Title)
	}
}

func TestScoreTitleMatch_MultipleVariants(t *testing.T) {
	// bestTitleMatch accepts multiple title variants; scoring should use
	// the union of words from all variants.
	results := []metadata.BookMetadata{
		{Title: "The Fellowship of the Ring Box Set"},
		{Title: "Fellowship of the Ring", Description: "Part one of LOTR.", CoverURL: "https://example.com/lotr.jpg"},
	}
	// Provide both a cleaned and raw title variant.
	got := bestTitleMatch(results, "Fellowship of the Ring", "The Fellowship of the Ring")
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if strings.Contains(strings.ToLower(got[0].Title), "box set") {
		t.Errorf("box set should not win: got %q", got[0].Title)
	}
}

func TestApplySeriesPositionFilter_RejectsWrongPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos", SeriesPosition: "3"}, // wrong — book is #5
	}
	got := applySeriesPositionFilter(results, 5)
	if got != nil {
		t.Errorf("expected nil (wrong position), got %v", got)
	}
}

func TestApplySeriesPositionFilter_AcceptsCorrectPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos", SeriesPosition: "5"},
	}
	got := applySeriesPositionFilter(results, 5)
	if got == nil {
		t.Fatal("expected result, got nil")
	}
	if got[0].SeriesPosition != "5" {
		t.Errorf("expected position 5, got %q", got[0].SeriesPosition)
	}
}

func TestApplySeriesPositionFilter_NoKnownPosition(t *testing.T) {
	results := []metadata.BookMetadata{
		{Title: "Some Book", SeriesPosition: "3"},
	}
	// knownPosition == 0 means "we don't know" — pass through unchanged
	got := applySeriesPositionFilter(results, 0)
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d", len(got))
	}
}

func TestApplySeriesPositionFilter_NoPositionInResult(t *testing.T) {
	// If the result has no SeriesPosition, we can't reject it on position grounds.
	results := []metadata.BookMetadata{
		{Title: "The Long Cosmos"},
	}
	got := applySeriesPositionFilter(results, 5)
	if len(got) != 1 {
		t.Errorf("expected 1 result (no position to reject), got %d", len(got))
	}
}

func TestFetchMetadataForBook_BoxSetRejected_IndividualBookApplied(t *testing.T) {
	setupGlobalStoreForTest(t)

	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{ID: id, Title: "The Long Cosmos"}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
		RecordMetadataChangeFunc: func(record *database.MetadataChangeRecord) error {
			return nil
		},
	}

	// Source returns a box set first, then the real book.
	src := &mockMetadataSource{
		name: "TestSource",
		searchByTitleFunc: func(title string) ([]metadata.BookMetadata, error) {
			return []metadata.BookMetadata{
				// Box set — should be penalised and rejected.
				{
					Title:  "The Long Earth Series 5 Books Collection Terry Pratchett and Stephen Baxter Box Set",
					Author: "Terry Pratchett",
				},
				// Individual book — should win.
				{
					Title:       "The Long Cosmos",
					Author:      "Terry Pratchett",
					Description: "The fifth book in the Long Earth series.",
					CoverURL:    "https://example.com/long-cosmos.jpg",
					PublishYear: 2016,
				},
			}, nil
		},
	}

	mfs := NewMetadataFetchService(mockDB)
	mfs.overrideSources = []metadata.MetadataSource{src}

	resp, err := mfs.FetchMetadataForBook("book1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// The applied book title should be from the individual book, not the box set.
	if resp.Book == nil {
		t.Fatal("expected non-nil Book in response")
	}
	if strings.Contains(strings.ToLower(resp.Book.Title), "collection") ||
		strings.Contains(strings.ToLower(resp.Book.Title), "box set") {
		t.Errorf("box set was applied to book: %q", resp.Book.Title)
	}
	if resp.Book.Title != "The Long Cosmos" {
		t.Errorf("expected title 'The Long Cosmos', got %q", resp.Book.Title)
	}
}
