// file: internal/server/dedup_engine_test.go
// version: 1.0.0
// guid: 2a7e4d91-c538-4f06-b1d3-9e8c5a6f0d72

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// setupTestEngine creates a DedupEngine with an in-memory EmbeddingStore and MockStore.
func setupTestEngine(t *testing.T) (*DedupEngine, *database.MockStore, *database.EmbeddingStore) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_embeddings.db")

	es, err := database.NewEmbeddingStore(dbPath)
	if err != nil {
		t.Fatalf("NewEmbeddingStore: %v", err)
	}
	t.Cleanup(func() { _ = es.Close(); _ = os.RemoveAll(tmpDir) })

	mock := &database.MockStore{}
	ms := NewMergeService(mock)
	engine := NewDedupEngine(es, mock, nil, ms)

	return engine, mock, es
}

// strPtr, intPtr, boolPtr are defined in other test files in this package

func TestDedupEngine_ExactMatch_FileHash(t *testing.T) {
	engine, mock, es := setupTestEngine(t)
	engine.AutoMergeEnabled = false

	authorID := 1
	bookA := &database.Book{ID: "BOOK_A", Title: "My Great Book", AuthorID: &authorID, FileHash: strPtr("hash123")}
	bookB := &database.Book{ID: "BOOK_B", Title: "My Great Book", AuthorID: &authorID, FileHash: strPtr("hash123")}

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		switch id {
		case "BOOK_A":
			return bookA, nil
		case "BOOK_B":
			return bookB, nil
		}
		return nil, nil
	}
	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		return &database.Author{ID: 1, Name: "Test Author"}, nil
	}
	mock.GetBookByFileHashFunc = func(hash string) (*database.Book, error) {
		if hash == "hash123" {
			return bookB, nil // Returns bookB for the shared hash
		}
		return nil, nil
	}
	mock.GetBookFilesFunc = func(bookID string) ([]database.BookFile, error) {
		return nil, nil // No separate files
	}
	mock.GetBooksByAuthorIDFunc = func(authorID int) ([]database.Book, error) {
		return []database.Book{*bookA, *bookB}, nil
	}
	mock.GetAllBooksFunc = func(limit, offset int) ([]database.Book, error) {
		return nil, nil // No ISBN matching needed
	}

	merged, err := engine.CheckBook(context.Background(), "BOOK_A")
	if err != nil {
		t.Fatalf("CheckBook: %v", err)
	}
	if merged {
		t.Fatal("expected no auto-merge when AutoMergeEnabled=false")
	}

	// Should have created at least one candidate
	candidates, total, err := es.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if total == 0 {
		t.Fatal("expected at least one candidate from file hash match")
	}

	found := false
	for _, c := range candidates {
		if c.Layer == "exact" &&
			((c.EntityAID == "BOOK_A" && c.EntityBID == "BOOK_B") ||
				(c.EntityAID == "BOOK_B" && c.EntityBID == "BOOK_A")) {
			found = true
		}
	}
	if !found {
		t.Error("expected exact-layer candidate for BOOK_A <-> BOOK_B")
	}
}

func TestDedupEngine_ExactMatch_FileHash_AutoMerge(t *testing.T) {
	engine, mock, _ := setupTestEngine(t)
	engine.AutoMergeEnabled = true

	authorID := 1
	bookA := &database.Book{ID: "BOOK_A", Title: "Same Title", AuthorID: &authorID, FileHash: strPtr("hash999")}
	bookB := &database.Book{ID: "BOOK_B", Title: "Same Title", AuthorID: &authorID, FileHash: strPtr("hash999")}

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		switch id {
		case "BOOK_A":
			return bookA, nil
		case "BOOK_B":
			return bookB, nil
		}
		return nil, nil
	}
	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		return &database.Author{ID: 1, Name: "Author"}, nil
	}
	mock.GetBookByFileHashFunc = func(hash string) (*database.Book, error) {
		if hash == "hash999" {
			return bookB, nil
		}
		return nil, nil
	}
	mock.GetBookFilesFunc = func(bookID string) ([]database.BookFile, error) {
		return nil, nil
	}
	mock.GetBooksByAuthorIDFunc = func(authorID int) ([]database.Book, error) {
		return []database.Book{*bookA, *bookB}, nil
	}
	mock.GetAllBooksFunc = func(limit, offset int) ([]database.Book, error) {
		return nil, nil
	}

	updateCalled := false
	mock.UpdateBookFunc = func(id string, book *database.Book) (*database.Book, error) {
		updateCalled = true
		return book, nil
	}

	merged, err := engine.CheckBook(context.Background(), "BOOK_A")
	if err != nil {
		t.Fatalf("CheckBook: %v", err)
	}
	if !merged {
		t.Fatal("expected auto-merge when AutoMergeEnabled=true")
	}
	if !updateCalled {
		t.Fatal("expected MergeService to call UpdateBook")
	}
}

func TestDedupEngine_ExactMatch_ISBN(t *testing.T) {
	engine, mock, es := setupTestEngine(t)

	authorID := 1
	bookA := &database.Book{ID: "BOOK_A", Title: "Title A", AuthorID: &authorID, ISBN13: strPtr("9780134685991")}
	bookB := &database.Book{ID: "BOOK_B", Title: "Title B", AuthorID: &authorID, ISBN13: strPtr("9780134685991")}

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		if id == "BOOK_A" {
			return bookA, nil
		}
		return nil, nil
	}
	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		return &database.Author{ID: 1, Name: "Author"}, nil
	}
	mock.GetBookByFileHashFunc = func(hash string) (*database.Book, error) {
		return nil, nil
	}
	mock.GetBookFilesFunc = func(bookID string) ([]database.BookFile, error) {
		return nil, nil
	}
	mock.GetBooksByAuthorIDFunc = func(authorID int) ([]database.Book, error) {
		return nil, nil // No title matches
	}
	mock.GetAllBooksFunc = func(limit, offset int) ([]database.Book, error) {
		if offset == 0 {
			return []database.Book{*bookA, *bookB}, nil
		}
		return nil, nil
	}

	_, err := engine.CheckBook(context.Background(), "BOOK_A")
	if err != nil {
		t.Fatalf("CheckBook: %v", err)
	}

	candidates, total, err := es.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Layer:      "exact",
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if total == 0 {
		t.Fatal("expected ISBN candidate")
	}

	found := false
	for _, c := range candidates {
		if c.EntityAID == "BOOK_A" && c.EntityBID == "BOOK_B" {
			found = true
		}
	}
	if !found {
		t.Error("expected exact-layer ISBN candidate for BOOK_A -> BOOK_B")
	}
}

func TestDedupEngine_ExactMatch_NoMatch(t *testing.T) {
	engine, mock, es := setupTestEngine(t)

	authorID1 := 1
	authorID2 := 2
	bookA := &database.Book{ID: "BOOK_A", Title: "Completely Different Title", AuthorID: &authorID1}
	bookB := &database.Book{ID: "BOOK_B", Title: "Another Unrelated Book", AuthorID: &authorID2}

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		if id == "BOOK_A" {
			return bookA, nil
		}
		return nil, nil
	}
	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		if id == 1 {
			return &database.Author{ID: 1, Name: "Author One"}, nil
		}
		return &database.Author{ID: 2, Name: "Author Two"}, nil
	}
	mock.GetBookByFileHashFunc = func(hash string) (*database.Book, error) {
		return nil, nil
	}
	mock.GetBookFilesFunc = func(bookID string) ([]database.BookFile, error) {
		return nil, nil
	}
	mock.GetBooksByAuthorIDFunc = func(authorID int) ([]database.Book, error) {
		if authorID == 1 {
			return []database.Book{*bookA}, nil // Only the book itself, no others
		}
		return nil, nil
	}
	mock.GetAllBooksFunc = func(limit, offset int) ([]database.Book, error) {
		if offset == 0 {
			return []database.Book{*bookA, *bookB}, nil
		}
		return nil, nil
	}

	_, err := engine.CheckBook(context.Background(), "BOOK_A")
	if err != nil {
		t.Fatalf("CheckBook: %v", err)
	}

	_, total, err := es.ListCandidates(database.CandidateFilter{EntityType: "book"})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 candidates, got %d", total)
	}
}

func TestDedupEngine_EmbedBook_NilClient(t *testing.T) {
	engine, mock, _ := setupTestEngine(t)
	// embedClient is nil by default in setupTestEngine

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		return &database.Book{ID: "BOOK_1", Title: "Test Book"}, nil
	}

	err := engine.EmbedBook(context.Background(), "BOOK_1")
	if err == nil {
		t.Fatal("expected error when embedClient is nil")
	}
	if err.Error() != "no embedding client configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"book", "back", 2},
		{"flaw", "lawn", 2},
		{"a", "b", 1},
	}

	for _, tc := range tests {
		got := levenshteinDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"  Hello   World  ", "hello world"},
		{"UPPERCASE", "uppercase"},
		{"already normal", "already normal"},
		{"", ""},
		{"  multiple   spaces   here  ", "multiple spaces here"},
	}

	for _, tc := range tests {
		got := normalizeTitle(tc.input)
		if got != tc.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDerefStr(t *testing.T) {
	s := "hello"
	if got := derefStr(&s); got != "hello" {
		t.Errorf("derefStr(&%q) = %q", s, got)
	}
	if got := derefStr(nil); got != "" {
		t.Errorf("derefStr(nil) = %q, want empty", got)
	}
}
