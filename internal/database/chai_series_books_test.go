// file: internal/database/chai_series_books_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetBooksBySeriesID_Chai_BasicPagination tests pagination with a specific series
func TestGetBooksBySeriesID_Chai_BasicPagination(t *testing.T) {
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

	// Create 10 books in series 1
	for i := 1; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ULID%010d", i),
			Title:            fmt.Sprintf("Series 1 Book %02d", i),
			FilePath:         fmt.Sprintf("/books/series1/book%02d.m4b", i),
			SeriesID:         intPtr(1),
			SeriesSequence:   intPtr(i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert test book %d: %v", i, err)
		}
	}

	// Test 1: Get first 3 books from series 1
	books, err := store.GetBooksBySeriesID_Chai(ctx, 1, 3, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books, got %d", len(books))
	}
	// Verify all books are from series 1
	for _, b := range books {
		if b.SeriesID == nil || *b.SeriesID != 1 {
			t.Errorf("expected series_id 1, got %v", b.SeriesID)
		}
	}

	// Test 2: Get next 3 books
	books, err = store.GetBooksBySeriesID_Chai(ctx, 1, 3, 3)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 3 {
		t.Errorf("expected 3 books, got %d", len(books))
	}

	// Test 3: Offset beyond results
	books, err = store.GetBooksBySeriesID_Chai(ctx, 1, 3, 20)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books for offset beyond end, got %d", len(books))
	}

	// Test 4: Limit larger than results
	books, err = store.GetBooksBySeriesID_Chai(ctx, 1, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 10 {
		t.Errorf("expected 10 books, got %d", len(books))
	}

	t.Logf("✓ Pagination tests passed")
}

// TestGetBooksBySeriesID_Chai_NullSeriesID tests that books without a series are excluded
func TestGetBooksBySeriesID_Chai_NullSeriesID(t *testing.T) {
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

	// Create 5 books in series 1
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("SER1%010d", i),
			Title:            fmt.Sprintf("Series 1 Book %02d", i),
			FilePath:         fmt.Sprintf("/books/series1/book%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert series book: %v", err)
		}
	}

	// Create 5 books with no series
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("NOSERIES%06d", i),
			Title:            fmt.Sprintf("Standalone Book %02d", i),
			FilePath:         fmt.Sprintf("/books/standalone/book%02d.m4b", i),
			SeriesID:         nil,
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert standalone book: %v", err)
		}
	}

	// Query series 1 - should get 5 books, not the standalone ones
	books, err := store.GetBooksBySeriesID_Chai(ctx, 1, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 books from series 1, got %d", len(books))
	}

	t.Logf("✓ Null series filtering tests passed")
}

// TestGetBooksBySeriesID_Chai_MarkedForDeletion tests deleted book filtering
func TestGetBooksBySeriesID_Chai_MarkedForDeletion(t *testing.T) {
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

	// Create 5 active books in series 1
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("ACT%010d", i),
			Title:            fmt.Sprintf("Active %02d", i),
			FilePath:         fmt.Sprintf("/books/active%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert active book: %v", err)
		}
	}

	// Create 5 deleted books in series 1
	for i := 6; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("DEL%010d", i),
			Title:            fmt.Sprintf("Deleted %02d", i),
			FilePath:         fmt.Sprintf("/books/deleted%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(true),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert deleted book: %v", err)
		}
	}

	// Query should exclude deleted books
	books, err := store.GetBooksBySeriesID_Chai(ctx, 1, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 active books, got %d (deleted books should be excluded)", len(books))
	}

	// Verify all returned books are not marked for deletion
	for _, b := range books {
		if b.MarkedForDeletion == nil || *b.MarkedForDeletion {
			t.Errorf("expected non-deleted book, got deleted=%v", b.MarkedForDeletion)
		}
	}

	t.Logf("✓ Deleted book filtering tests passed")
}

// TestGetBooksBySeriesID_Chai_PrimaryVersionFiltering tests non-primary version filtering
func TestGetBooksBySeriesID_Chai_PrimaryVersionFiltering(t *testing.T) {
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

	// Create 5 primary version books in series 1
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("PRI%010d", i),
			Title:            fmt.Sprintf("Primary %02d", i),
			FilePath:         fmt.Sprintf("/books/primary%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert primary book: %v", err)
		}
	}

	// Create 5 non-primary version books in series 1
	for i := 1; i <= 5; i++ {
		book := &Book{
			ID:               fmt.Sprintf("DUP%010d", i),
			Title:            fmt.Sprintf("Duplicate %02d", i),
			FilePath:         fmt.Sprintf("/books/dup%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(false),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			t.Fatalf("failed to insert non-primary book: %v", err)
		}
	}

	// Query should return only primary version books
	books, err := store.GetBooksBySeriesID_Chai(ctx, 1, 100, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 primary version books, got %d (non-primary should be excluded)", len(books))
	}

	// Verify all returned books are primary version
	for _, b := range books {
		if b.IsPrimaryVersion == nil || !*b.IsPrimaryVersion {
			t.Errorf("expected primary version book, got is_primary_version=%v", b.IsPrimaryVersion)
		}
	}

	t.Logf("✓ Primary version filtering tests passed")
}

// TestGetBooksBySeriesID_Chai_InvalidSeriesID tests error handling for invalid series ID
func TestGetBooksBySeriesID_Chai_InvalidSeriesID(t *testing.T) {
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

	// Test with zero series_id - should error
	_, err = store.GetBooksBySeriesID_Chai(ctx, 0, 10, 0)
	if err == nil {
		t.Errorf("expected error for series_id 0, got nil")
	}

	// Test with negative series_id - should error
	_, err = store.GetBooksBySeriesID_Chai(ctx, -1, 10, 0)
	if err == nil {
		t.Errorf("expected error for series_id -1, got nil")
	}

	// Test with non-existent series_id - should return empty slice (not error)
	books, err := store.GetBooksBySeriesID_Chai(ctx, 9999, 10, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID_Chai for non-existent series failed: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books for non-existent series, got %d", len(books))
	}

	t.Logf("✓ Invalid series ID tests passed")
}

// BenchmarkGetBooksBySeriesID_Chai_SmallSeries benchmarks performance with a small series (10 books)
func BenchmarkGetBooksBySeriesID_Chai_SmallSeries(b *testing.B) {
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

	// Create 10 books in series 1
	for i := 1; i <= 10; i++ {
		book := &Book{
			ID:               fmt.Sprintf("BENCH%010d", i),
			Title:            fmt.Sprintf("Book %02d", i),
			FilePath:         fmt.Sprintf("/books/book%02d.m4b", i),
			SeriesID:         intPtr(1),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			b.Fatalf("failed to insert test book: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.GetBooksBySeriesID_Chai(ctx, 1, 10, 0)
		if err != nil {
			b.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
		}
	}
}

// BenchmarkGetBooksBySeriesID_Chai_LargeSeries benchmarks performance with a large series (1000 books)
// Simulates production-like scenario with series that have many books
func BenchmarkGetBooksBySeriesID_Chai_LargeSeries(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench_large.db")

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

	// Create 1000 books in series 1 (large series like Harry Potter or similar)
	for i := 1; i <= 1000; i++ {
		book := &Book{
			ID:               fmt.Sprintf("LARGE%010d", i),
			Title:            fmt.Sprintf("Book %04d", i),
			FilePath:         fmt.Sprintf("/books/book%04d.m4b", i),
			SeriesID:         intPtr(1),
			SeriesSequence:   intPtr(i),
			IsPrimaryVersion: boolPtr(true),
			MarkedForDeletion: boolPtr(false),
		}
		_, err := insertTestBookFull(db.DB(), book)
		if err != nil {
			b.Fatalf("failed to insert test book: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Test with pagination (first 50 books)
		_, err := store.GetBooksBySeriesID_Chai(ctx, 1, 50, 0)
		if err != nil {
			b.Fatalf("GetBooksBySeriesID_Chai failed: %v", err)
		}
	}
}
