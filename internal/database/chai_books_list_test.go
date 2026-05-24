// file: internal/database/chai_books_list_test.go
// version: 1.0.1
// guid: a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllBooks_Chai_BasicPagination tests pagination with default filters
func TestGetAllBooks_Chai_BasicPagination(t *testing.T) {
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

	// Create 10 test books
	for i := 1; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Book %02d", i),
			FilePath:         fmt.Sprintf("/books/book%02d.m4b", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBook(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert test book %d: %v", i, err)
		}
	}

	// Test 1: Get first 3 books
	books, err := store.GetAllBooks_Chai(ctx, 3, 0, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books, got %d", len(books))
	}

	// Test 2: Get next 3 books
	books, err = store.GetAllBooks_Chai(ctx, 3, 3, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books, got %d", len(books))
	}

	// Test 3: Offset beyond results
	books, err = store.GetAllBooks_Chai(ctx, 3, 20, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books for offset beyond end, got %d", len(books))
	}

	// Test 4: Limit larger than results
	books, err = store.GetAllBooks_Chai(ctx, 100, 0, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 10 {
		t.Errorf("expected 10 books, got %d", len(books))
	}

	t.Logf("✓ Pagination tests passed")
}

// TestGetAllBooks_Chai_MarkedForDeletion tests deleted book filtering
func TestGetAllBooks_Chai_MarkedForDeletion(t *testing.T) {
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

	// Create 5 active and 5 deleted books
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Active %02d", i),
			FilePath:         fmt.Sprintf("/books/active%02d.m4b", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBook(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert active book: %v", err)
		}
	}

	for i := 6; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Deleted %02d", i),
			FilePath:         fmt.Sprintf("/books/deleted%02d.m4b", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(true),
		}
		_, err := insertTestBook(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert deleted book: %v", err)
		}
	}

	// Default filter (exclude deleted)
	books, err := store.GetAllBooks_Chai(ctx, 100, 0, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("default filter: expected 5 books, got %d", len(books))
	}

	// Include deleted
	filters := map[string]interface{}{"marked_for_deletion": true}
	books, err = store.GetAllBooks_Chai(ctx, 100, 0, filters)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("with deleted filter: expected 5 books, got %d", len(books))
	}

	t.Logf("✓ Deleted book filtering tests passed")
}

// TestGetAllBooks_Chai_IsPrimaryVersion tests primary version filtering
func TestGetAllBooks_Chai_IsPrimaryVersion(t *testing.T) {
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

	// Create 5 primary and 5 non-primary books
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("PRI%08d", i),
			Title:            fmt.Sprintf("Primary %02d", i),
			FilePath:         fmt.Sprintf("/books/primary%02d.m4b", i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBook(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert primary book: %v", err)
		}
	}

	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("DUP%08d", i),
			Title:            fmt.Sprintf("Duplicate %02d", i),
			FilePath:         fmt.Sprintf("/books/dup%02d.m4b", i),
			IsPrimaryVersion: boolPtr(false),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBook(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert non-primary book: %v", err)
		}
	}

	// Default filter (primary only)
	books, err := store.GetAllBooks_Chai(ctx, 100, 0, nil)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("default filter: expected 5 primary books, got %d", len(books))
	}

	// Get non-primary only
	filters := map[string]interface{}{"is_primary_version": false}
	books, err = store.GetAllBooks_Chai(ctx, 100, 0, filters)
	if err != nil {
		t.Fatalf("GetAllBooks_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("non-primary filter: expected 5 books, got %d", len(books))
	}

	t.Logf("✓ Primary version filtering tests passed")
}

// insertTestBook is defined in chai_books_list_test_helper.go
