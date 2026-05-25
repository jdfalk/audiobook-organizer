// file: internal/database/chai_books_by_author_test.go
// version: 1.1.0
// guid: f1g2h3i4-j5k6-47l8-m9n0-o1p2q3r4s5t6
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetBooksByAuthorID_Chai_BasicFiltering tests basic author filtering
func TestGetBooksByAuthorID_Chai_BasicFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	store, err := NewChaiStore(db.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	// Create 5 books and assign authors
	author1ID := 1
	author2ID := 2

	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Book %02d", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert test book %d: %v", i, err)
		}

		// Assign odd books to author 1, even books to author 2
		if i%2 == 1 {
			if err := insertTestBookAuthor(db.DB(), book.ID, author1ID); err != nil {
				t.Fatalf("failed to insert author relation: %v", err)
			}
		} else {
			if err := insertTestBookAuthor(db.DB(), book.ID, author2ID); err != nil {
				t.Fatalf("failed to insert author relation: %v", err)
			}
		}
	}

	// Test 1: Get books by author 1 (should get 3 books: 1, 3, 5)
	books, err := store.GetBooksByAuthorID_Chai(ctx, author1ID, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books for author 1, got %d", len(books))
	}

	// Test 2: Get books by author 2 (should get 2 books: 2, 4)
	books, err = store.GetBooksByAuthorID_Chai(ctx, author2ID, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 2 {
		t.Errorf("expected 2 books for author 2, got %d", len(books))
	}

	// Test 3: Get books by non-existent author (should get 0 books)
	books, err = store.GetBooksByAuthorID_Chai(ctx, 999, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books for non-existent author, got %d", len(books))
	}

	t.Logf("✓ Author filtering tests passed")
}

// TestGetBooksByAuthorID_Chai_Pagination tests pagination with author filter
func TestGetBooksByAuthorID_Chai_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	store, err := NewChaiStore(db.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	authorID := 1

	// Create 10 books assigned to the same author
	for i := 1; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Book %02d", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert test book %d: %v", i, err)
		}
		if err := insertTestBookAuthor(db.DB(), book.ID, authorID); err != nil {
			t.Fatalf("failed to insert author relation: %v", err)
		}
	}

	// Test 1: Get first 3 books
	books, err := store.GetBooksByAuthorID_Chai(ctx, authorID, 3, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books (page 1), got %d", len(books))
	}

	// Test 2: Get next 3 books
	books, err = store.GetBooksByAuthorID_Chai(ctx, authorID, 3, 3)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books (page 2), got %d", len(books))
	}

	// Test 3: Offset beyond results
	books, err = store.GetBooksByAuthorID_Chai(ctx, authorID, 3, 20)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books for offset beyond end, got %d", len(books))
	}

	// Test 4: Limit larger than results
	books, err = store.GetBooksByAuthorID_Chai(ctx, authorID, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 10 {
		t.Errorf("expected 10 books total, got %d", len(books))
	}

	t.Logf("✓ Pagination tests passed")
}

// TestGetBooksByAuthorID_Chai_DeletedBookExclusion tests that deleted books are excluded
func TestGetBooksByAuthorID_Chai_DeletedBookExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	store, err := NewChaiStore(db.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	authorID := 1

	// Create 5 active and 5 deleted books all assigned to the same author
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Active %02d", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert active book: %v", err)
		}
		if err := insertTestBookAuthor(db.DB(), book.ID, authorID); err != nil {
			t.Fatalf("failed to insert author relation: %v", err)
		}
	}

	for i := 6; i <= 10; i++ {
		book := &Book{
			ID:                fmt.Sprintf("ULID%010d", i),
			Title:             fmt.Sprintf("Deleted %02d", i),
			IsPrimaryVersion:  boolPtr(true),
			MarkedForDeletion: boolPtr(true),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert deleted book: %v", err)
		}
		if err := insertTestBookAuthor(db.DB(), book.ID, authorID); err != nil {
			t.Fatalf("failed to insert author relation: %v", err)
		}
	}

	// Should only get 5 active books, not the 5 deleted ones
	books, err := store.GetBooksByAuthorID_Chai(ctx, authorID, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 active books (deleted excluded), got %d", len(books))
	}

	t.Logf("✓ Deleted book exclusion tests passed")
}

// TestGetBooksByAuthorID_Chai_NonPrimaryVersionExclusion tests that non-primary versions are excluded
func TestGetBooksByAuthorID_Chai_NonPrimaryVersionExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	store, err := NewChaiStore(db.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	authorID := 1

	// Create 5 primary books
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Primary %02d", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert primary book: %v", err)
		}
		if err := insertTestBookAuthor(db.DB(), book.ID, authorID); err != nil {
			t.Fatalf("failed to insert author relation: %v", err)
		}
	}

	// Create 5 non-primary versions
	for i := 6; i <= 10; i++ {
		book := &Book{
			ID:                fmt.Sprintf("ULID%010d", i),
			Title:             fmt.Sprintf("NonPrimary %02d", i),
			IsPrimaryVersion:  boolPtr(false),
			MarkedForDeletion: boolPtr(false),
		}
		if _, err := insertTestBook(db.DB(), book); err != nil {
			t.Fatalf("failed to insert non-primary book: %v", err)
		}
		if err := insertTestBookAuthor(db.DB(), book.ID, authorID); err != nil {
			t.Fatalf("failed to insert author relation: %v", err)
		}
	}

	// Should only get 5 primary books, not the 5 non-primary versions
	books, err := store.GetBooksByAuthorID_Chai(ctx, authorID, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 primary books (non-primary excluded), got %d", len(books))
	}

	t.Logf("✓ Non-primary version exclusion tests passed")
}

// BenchmarkGetBooksByAuthorID_Chai benchmarks the author lookup on 50K books
func BenchmarkGetBooksByAuthorID_Chai(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		b.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	store, err := NewChaiStore(db.DB())
	if err != nil {
		b.Fatalf("NewChaiStore failed: %v", err)
	}

	// Create benchmark data: 50K books, distributed across 100 authors
	const totalBooks = 50_000
	const totalAuthors = 100

	for bookNum := 0; bookNum < totalBooks; bookNum++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%08d", bookNum),
			Title:            fmt.Sprintf("Book %08d", bookNum),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, _ = insertTestBook(db.DB(), book)

		// Assign to author (round-robin across 100 authors)
		authorID := (bookNum % totalAuthors) + 1
		_ = insertTestBookAuthor(db.DB(), book.ID, authorID)
	}

	// Benchmark: lookup all books for a single author (should be ~500 books)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authorID := 1 + (i % totalAuthors)
		_, err := store.GetBooksByAuthorID_Chai(ctx, authorID, 1_000_000, 0)
		if err != nil {
			b.Fatalf("GetBooksByAuthorID_Chai failed: %v", err)
		}
	}
}

// (Duplicate insertTestBookAuthor removed; canonical version lives in
// chai_books_list_test_helper.go. This stub kept so unused imports above
// stay referenced.)
