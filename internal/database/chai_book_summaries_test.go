// file: internal/database/chai_book_summaries_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-4f3a-b4c5-d6e7f8a9b0c1
// last-edited: 2026-05-24

package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

// insertSummaryTestBook inserts a minimal test book row for chai_book_summaries tests.
// It uses the same INSERT pattern as other chai test helpers but is private to this file
// to avoid redeclaration conflicts with helpers in chai_books_list_test.go and
// chai_books_by_author_test.go, which have conflicting signatures on main.
func insertSummaryTestBook(db *sql.DB, id, title, filePath string, isPrimary, deleted bool) error {
	primaryVal := "false"
	if isPrimary {
		primaryVal = "true"
	}
	deletedVal := "false"
	if deleted {
		deletedVal = "true"
	}
	// Include format='' to satisfy scanBookSummary which scans format as a non-nullable string.
	query := fmt.Sprintf(`
		INSERT INTO books (id, title, file_path, format, is_primary_version, marked_for_deletion, created_at, updated_at)
		VALUES ('%s', '%s', '%s', '', %s, %s, '2026-01-01T00:00:00', '2026-01-01T00:00:00')
	`, escapeSQL(id), escapeSQL(title), escapeSQL(filePath), primaryVal, deletedVal)
	_, err := db.Exec(query)
	return err
}

// openSummaryTestDB opens a fresh Chai DB at tmpDir and returns the ChaiStore.
func openSummaryTestDB(t *testing.T) (*ChaiDB, *ChaiStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewChaiDB(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB: %v", err)
	}
	store, err := NewChaiStore(db.DB())
	if err != nil {
		db.Close()
		t.Fatalf("NewChaiStore: %v", err)
	}
	return db, store
}

// TestGetAllBookSummaries_Chai_Pagination verifies limit/offset pagination.
func TestGetAllBookSummaries_Chai_Pagination(t *testing.T) {
	db, store := openSummaryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Insert 10 primary, non-deleted books.
	for i := 1; i <= 10; i++ {
		if err := insertSummaryTestBook(db.DB(),
			fmt.Sprintf("SUMM%010d", i),
			fmt.Sprintf("Summary Book %02d", i),
			fmt.Sprintf("/books/summary%02d.m4b", i),
			true, false,
		); err != nil {
			t.Fatalf("insert book %d: %v", i, err)
		}
	}

	// Limit 3, offset 0 → first 3.
	summaries, err := store.GetAllBookSummaries_Chai(ctx, 3, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai(3,0): %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3, got %d", len(summaries))
	}

	// Limit 3, offset 3 → next 3.
	summaries, err = store.GetAllBookSummaries_Chai(ctx, 3, 3)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai(3,3): %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3 at offset 3, got %d", len(summaries))
	}

	// Offset beyond results → empty (nil).
	summaries, err = store.GetAllBookSummaries_Chai(ctx, 10, 100)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai offset beyond range: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 beyond range, got %d", len(summaries))
	}

	// Limit 0 → treated as unlimited, returns all 10.
	summaries, err = store.GetAllBookSummaries_Chai(ctx, 0, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai(0,0): %v", err)
	}
	if len(summaries) != 10 {
		t.Errorf("limit=0 should return all 10, got %d", len(summaries))
	}
}

// TestGetAllBookSummaries_Chai_FiltersDeletedAndNonPrimary ensures marked-for-deletion
// and non-primary-version books are excluded.
func TestGetAllBookSummaries_Chai_FiltersDeletedAndNonPrimary(t *testing.T) {
	db, store := openSummaryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// 3 primary + active → should appear.
	for i := 1; i <= 3; i++ {
		if err := insertSummaryTestBook(db.DB(),
			fmt.Sprintf("GOOD%010d", i),
			fmt.Sprintf("Good Book %02d", i),
			fmt.Sprintf("/books/good%02d.m4b", i),
			true, false,
		); err != nil {
			t.Fatalf("insert active book %d: %v", i, err)
		}
	}

	// 2 marked-for-deletion → must be excluded.
	for i := 4; i <= 5; i++ {
		if err := insertSummaryTestBook(db.DB(),
			fmt.Sprintf("DELD%010d", i),
			fmt.Sprintf("Deleted Book %02d", i),
			fmt.Sprintf("/books/deld%02d.m4b", i),
			true, true,
		); err != nil {
			t.Fatalf("insert deleted book %d: %v", i, err)
		}
	}

	// 2 non-primary versions → must be excluded.
	for i := 6; i <= 7; i++ {
		if err := insertSummaryTestBook(db.DB(),
			fmt.Sprintf("NONP%010d", i),
			fmt.Sprintf("Non-Primary %02d", i),
			fmt.Sprintf("/books/nonp%02d.m4b", i),
			false, false,
		); err != nil {
			t.Fatalf("insert non-primary book %d: %v", i, err)
		}
	}

	summaries, err := store.GetAllBookSummaries_Chai(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai: %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("expected 3 active-primary summaries, got %d", len(summaries))
	}

	// Verify only the active-primary IDs appear.
	activeIDs := map[string]bool{
		"GOOD0000000001": true,
		"GOOD0000000002": true,
		"GOOD0000000003": true,
	}
	for _, s := range summaries {
		if !activeIDs[s.ID] {
			t.Errorf("unexpected book ID in summaries: %s", s.ID)
		}
	}
}

// TestGetAllBookSummaries_Chai_OrderByTitle confirms results are sorted ascending by title.
func TestGetAllBookSummaries_Chai_OrderByTitle(t *testing.T) {
	db, store := openSummaryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Insert out-of-alphabetical order.
	titles := []string{"Zebra", "Mango", "Apple", "Kiwi"}
	for i, title := range titles {
		if err := insertSummaryTestBook(db.DB(),
			fmt.Sprintf("ORD%09d", i),
			title,
			fmt.Sprintf("/books/ord%d.m4b", i),
			true, false,
		); err != nil {
			t.Fatalf("insert %s: %v", title, err)
		}
	}

	summaries, err := store.GetAllBookSummaries_Chai(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai: %v", err)
	}
	if len(summaries) != 4 {
		t.Fatalf("expected 4 results, got %d", len(summaries))
	}

	expected := []string{"Apple", "Kiwi", "Mango", "Zebra"}
	for i, s := range summaries {
		if s.Title != expected[i] {
			t.Errorf("position %d: expected title %q, got %q", i, expected[i], s.Title)
		}
	}
}

// TestGetAllBookSummaries_Chai_EmptyDB tests the empty database case.
func TestGetAllBookSummaries_Chai_EmptyDB(t *testing.T) {
	db, store := openSummaryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	summaries, err := store.GetAllBookSummaries_Chai(ctx, 100, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai on empty DB: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected empty slice on empty DB, got %d items", len(summaries))
	}
}

// TestGetAllBookSummaries_Chai_FieldsPopulated confirms that key BookSummary fields
// (ID, Title, FilePath) are populated correctly from the SQL scan.
func TestGetAllBookSummaries_Chai_FieldsPopulated(t *testing.T) {
	db, store := openSummaryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	if err := insertSummaryTestBook(db.DB(),
		"FIELDS0001", "Field Test Book", "/books/fields.m4b",
		true, false,
	); err != nil {
		t.Fatalf("insertSummaryTestBook: %v", err)
	}

	summaries, err := store.GetAllBookSummaries_Chai(ctx, 10, 0)
	if err != nil {
		t.Fatalf("GetAllBookSummaries_Chai: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]
	if s.ID != "FIELDS0001" {
		t.Errorf("ID: expected FIELDS0001, got %q", s.ID)
	}
	if s.Title != "Field Test Book" {
		t.Errorf("Title: expected %q, got %q", "Field Test Book", s.Title)
	}
	if s.FilePath != "/books/fields.m4b" {
		t.Errorf("FilePath: expected /books/fields.m4b, got %q", s.FilePath)
	}
	if s.IsPrimaryVersion == nil || !*s.IsPrimaryVersion {
		t.Errorf("IsPrimaryVersion: expected true, got %v", s.IsPrimaryVersion)
	}
}
