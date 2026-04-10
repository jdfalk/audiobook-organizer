// file: internal/server/dedup_engine_test.go
// version: 1.1.0
// guid: 2a7e4d91-c538-4f06-b1d3-9e8c5a6f0d72

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
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
	engine := NewDedupEngine(es, mock, nil, nil, ms)

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

// --- Layer 3 LLM review tests ---

// seedCandidate inserts a pending embedding-layer candidate directly into the store.
func seedCandidate(t *testing.T, es *database.EmbeddingStore, entityType, aID, bID string, similarity float64) {
	t.Helper()
	if err := es.UpsertCandidate(database.DedupCandidate{
		EntityType: entityType,
		EntityAID:  aID,
		EntityBID:  bID,
		Layer:      "embedding",
		Similarity: &similarity,
		Status:     "pending",
	}); err != nil {
		t.Fatalf("UpsertCandidate: %v", err)
	}
}

func TestListAmbiguousCandidates_FiltersByRange(t *testing.T) {
	engine, _, es := setupTestEngine(t)

	// Seed candidates across the whole book similarity spectrum.
	seedCandidate(t, es, "book", "B1", "B2", 0.70) // below zone
	seedCandidate(t, es, "book", "B3", "B4", 0.82) // in zone
	seedCandidate(t, es, "book", "B5", "B6", 0.88) // in zone
	seedCandidate(t, es, "book", "B7", "B8", 0.95) // above zone
	// Same-range authors should be ignored when we query for books.
	seedCandidate(t, es, "author", "1", "2", 0.85)

	got, err := engine.listAmbiguousCandidates("book", 0.80, 0.92)
	if err != nil {
		t.Fatalf("listAmbiguousCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 book candidates in zone, got %d", len(got))
	}
	for _, c := range got {
		if c.EntityType != "book" {
			t.Errorf("unexpected entity_type %q", c.EntityType)
		}
		if c.Similarity == nil || *c.Similarity < 0.80 || *c.Similarity > 0.92 {
			t.Errorf("candidate %d similarity %v outside [0.80, 0.92]", c.ID, c.Similarity)
		}
	}
}

func TestBuildPairInput_Book(t *testing.T) {
	engine, mock, _ := setupTestEngine(t)

	authorID := 42
	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		switch id {
		case "BOOK_A":
			return &database.Book{ID: "BOOK_A", Title: "Dune", AuthorID: &authorID,
				Narrator: strPtr("Scott Brick"), ISBN13: strPtr("9780441013593")}, nil
		case "BOOK_B":
			return &database.Book{ID: "BOOK_B", Title: "Dune (Unabridged)", AuthorID: &authorID,
				ASIN: strPtr("B002V1OHSU")}, nil
		}
		return nil, nil
	}
	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		return &database.Author{ID: 42, Name: "Frank Herbert"}, nil
	}

	sim := 0.87
	c := database.DedupCandidate{
		ID: 1, EntityType: "book", EntityAID: "BOOK_A", EntityBID: "BOOK_B",
		Similarity: &sim,
	}
	input, ok := engine.buildPairInput(0, c)
	if !ok {
		t.Fatal("buildPairInput returned !ok")
	}
	if input.EntityType != "book" {
		t.Errorf("entity_type = %q", input.EntityType)
	}
	if input.A.Title != "Dune" || input.A.Author != "Frank Herbert" || input.A.Narrator != "Scott Brick" {
		t.Errorf("unexpected A: %+v", input.A)
	}
	if input.A.ISBN != "9780441013593" {
		t.Errorf("ISBN13 should populate ISBN, got %q", input.A.ISBN)
	}
	if input.B.Title != "Dune (Unabridged)" || input.B.ASIN != "B002V1OHSU" {
		t.Errorf("unexpected B: %+v", input.B)
	}
	if input.Similarity != 0.87 {
		t.Errorf("similarity = %v", input.Similarity)
	}
}

func TestBuildPairInput_Author(t *testing.T) {
	engine, mock, _ := setupTestEngine(t)

	mock.GetAuthorByIDFunc = func(id int) (*database.Author, error) {
		switch id {
		case 1:
			return &database.Author{ID: 1, Name: "J.R.R. Tolkien"}, nil
		case 2:
			return &database.Author{ID: 2, Name: "J. R. R. Tolkien"}, nil
		}
		return nil, nil
	}

	c := database.DedupCandidate{
		ID: 5, EntityType: "author", EntityAID: "1", EntityBID: "2",
	}
	input, ok := engine.buildPairInput(3, c)
	if !ok {
		t.Fatal("buildPairInput returned !ok")
	}
	if input.Index != 3 {
		t.Errorf("index = %d, want 3", input.Index)
	}
	if input.A.Title != "J.R.R. Tolkien" || input.B.Title != "J. R. R. Tolkien" {
		t.Errorf("unexpected entities: A=%q B=%q", input.A.Title, input.B.Title)
	}
}

func TestBuildPairInput_MissingEntityReturnsFalse(t *testing.T) {
	engine, mock, _ := setupTestEngine(t)

	mock.GetBookByIDFunc = func(id string) (*database.Book, error) {
		if id == "GOOD" {
			return &database.Book{ID: "GOOD", Title: "Good Book"}, nil
		}
		return nil, nil // Missing book
	}

	c := database.DedupCandidate{
		EntityType: "book", EntityAID: "GOOD", EntityBID: "MISSING",
	}
	if _, ok := engine.buildPairInput(0, c); ok {
		t.Error("expected !ok when one entity is missing")
	}
}

func TestApplyVerdicts_PersistsAndRoutes(t *testing.T) {
	engine, _, es := setupTestEngine(t)

	// Seed two candidates and get their IDs back.
	sim := 0.85
	_ = es.UpsertCandidate(database.DedupCandidate{
		EntityType: "book", EntityAID: "A1", EntityBID: "A2",
		Layer: "embedding", Similarity: &sim, Status: "pending",
	})
	_ = es.UpsertCandidate(database.DedupCandidate{
		EntityType: "book", EntityAID: "B1", EntityBID: "B2",
		Layer: "embedding", Similarity: &sim, Status: "pending",
	})
	candidates, _, _ := es.ListCandidates(database.CandidateFilter{EntityType: "book"})
	if len(candidates) != 2 {
		t.Fatalf("expected 2 seeded candidates, got %d", len(candidates))
	}

	byIndex := map[int]database.DedupCandidate{
		0: candidates[0],
		1: candidates[1],
	}
	verdicts := []ai.DedupPairVerdict{
		{Index: 0, IsDuplicate: true, Confidence: "high", Reason: "identical"},
		{Index: 1, IsDuplicate: false, Confidence: "medium", Reason: "different editions"},
		{Index: 99, IsDuplicate: true, Reason: "unknown index — should be ignored"},
	}

	applied := engine.applyVerdicts(verdicts, byIndex)
	if applied != 2 {
		t.Errorf("applied = %d, want 2", applied)
	}

	got, _, _ := es.ListCandidates(database.CandidateFilter{EntityType: "book"})
	for _, c := range got {
		if c.Layer != "llm" {
			t.Errorf("candidate %d layer = %q, want 'llm'", c.ID, c.Layer)
		}
		if c.LLMVerdict != "duplicate" && c.LLMVerdict != "not_duplicate" {
			t.Errorf("candidate %d verdict = %q", c.ID, c.LLMVerdict)
		}
		if c.LLMReason == "" {
			t.Errorf("candidate %d has empty reason", c.ID)
		}
	}
}

func TestRunLLMReview_NilParserSkipsGracefully(t *testing.T) {
	engine, _, es := setupTestEngine(t)
	// Leave engine.llmParser = nil (setupTestEngine constructs without a parser).

	seedCandidate(t, es, "book", "X", "Y", 0.85)

	if err := engine.RunLLMReview(context.Background()); err != nil {
		t.Fatalf("RunLLMReview with nil parser: %v", err)
	}
	// Candidate should remain unchanged at layer='embedding'.
	got, _, _ := es.ListCandidates(database.CandidateFilter{EntityType: "book"})
	if len(got) != 1 || got[0].Layer != "embedding" {
		t.Errorf("candidate should be untouched when parser is nil: %+v", got)
	}
}
