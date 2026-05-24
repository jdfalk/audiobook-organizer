// file: internal/database/chai_author_counts_test.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-40d0-e1f2-a3b4c5d6e7f8
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllAuthorBookCounts_ChaiSQL validates SQL version returns correct counts
func TestGetAllAuthorBookCounts_ChaiSQL(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Initialize Chai database
	chaiPath := filepath.Join(tmpDir, "chai.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	// Insert test data: 3 authors with book counts
	// Author 1: 2 books
	// Author 2: 3 books
	// Author 3: 1 book (deleted)
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_authors (id, book_id, author_id, marked_for_deletion)
		VALUES
			('ba1', 'book1', 1, false),
			('ba2', 'book2', 1, false),
			('ba3', 'book3', 2, false),
			('ba4', 'book4', 2, false),
			('ba5', 'book5', 2, false),
			('ba6', 'book6', 3, true)
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Execute Chai SQL version
	counts, err := chaiStore.GetAllAuthorBookCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts_Chai failed: %v", err)
	}

	// Verify counts (should exclude deleted)
	expected := map[int]int{
		1: 2, // 2 non-deleted books
		2: 3, // 3 non-deleted books
		// 3 should not be in the map (all deleted)
	}

	if len(counts) != len(expected) {
		t.Errorf("expected %d authors, got %d", len(expected), len(counts))
	}

	for authorID, expectedCount := range expected {
		actualCount, exists := counts[authorID]
		if !exists {
			t.Errorf("author %d not in results", authorID)
			continue
		}
		if actualCount != expectedCount {
			t.Errorf("author %d: expected count %d, got %d", authorID, expectedCount, actualCount)
		}
	}

	// Verify author 3 (all deleted) is not in results
	if count, exists := counts[3]; exists {
		t.Errorf("expected author 3 (all deleted) to not be in results, but got count %d", count)
	}
}

// TestGetAllAuthorBookCounts_Pebble validates Pebble version works correctly
func TestGetAllAuthorBookCounts_Pebble(t *testing.T) {
	tmpDir := t.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	// Test with empty database
	counts, err := pebbleDB.GetAllAuthorBookCounts()
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts on empty DB failed: %v", err)
	}

	if len(counts) != 0 {
		t.Errorf("expected 0 authors in empty DB, got %d", len(counts))
	}
}

// TestGetAllAuthorBookCounts_ChaiVsPebble compares implementations
func TestGetAllAuthorBookCounts_ChaiVsPebble(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Setup both databases
	pebblePath := filepath.Join(tmpDir, "pebble.db")
	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	chaiPath := filepath.Join(tmpDir, "chai.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	// Insert identical test data to Chai
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_authors (id, book_id, author_id, marked_for_deletion)
		VALUES
			('ba1', 'book1', 1, false),
			('ba2', 'book2', 1, false),
			('ba3', 'book3', 2, false),
			('ba4', 'book4', 2, false),
			('ba5', 'book5', 3, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert test data to Chai: %v", err)
	}

	// Get results from Chai
	chaiCounts, err := chaiStore.GetAllAuthorBookCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts_Chai failed: %v", err)
	}

	// Pebble DB is empty, so we expect empty results
	pebbleCounts, err := pebbleDB.GetAllAuthorBookCounts()
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts on Pebble failed: %v", err)
	}

	// Verify Chai results structure
	expected := map[int]int{
		1: 2,
		2: 2,
		3: 1,
	}

	if len(chaiCounts) != len(expected) {
		t.Errorf("Chai: expected %d authors, got %d", len(expected), len(chaiCounts))
	}

	for authorID, expectedCount := range expected {
		actualCount, exists := chaiCounts[authorID]
		if !exists {
			t.Errorf("Chai: author %d not in results", authorID)
			continue
		}
		if actualCount != expectedCount {
			t.Errorf("Chai: author %d: expected count %d, got %d", authorID, expectedCount, actualCount)
		}
	}

	// Pebble is empty as expected
	if len(pebbleCounts) != 0 {
		t.Errorf("Pebble: expected 0 authors (empty DB), got %d", len(pebbleCounts))
	}
}

// TestGetAllAuthorBookCounts_Chai_SingleEntry tests single entry
func TestGetAllAuthorBookCounts_Chai_SingleEntry(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	chaiPath := filepath.Join(tmpDir, "chai.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	// Single author with single book
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_authors (id, book_id, author_id, marked_for_deletion)
		VALUES ('ba1', 'book1', 42, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert single entry: %v", err)
	}

	counts, err := chaiStore.GetAllAuthorBookCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts_Chai with single entry failed: %v", err)
	}

	if len(counts) != 1 {
		t.Errorf("single entry: expected 1 author, got %d", len(counts))
	}

	if count, exists := counts[42]; !exists || count != 1 {
		t.Errorf("single entry: author 42 should have count 1, got exists=%v count=%d", exists, count)
	}
}

// BenchmarkGetAllAuthorBookCounts_Chai benchmarks the SQL version
func BenchmarkGetAllAuthorBookCounts_Chai(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := context.Background()

	chaiPath := filepath.Join(tmpDir, "chai.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		b.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		b.Fatalf("NewChaiStore failed: %v", err)
	}

	// Insert benchmark data: 100 authors, 1000 book_authors entries
	for i := 1; i <= 100; i++ {
		for j := 1; j <= 10; j++ {
			id := fmt.Sprintf("ba_%d_%d", i, j)
			bookID := fmt.Sprintf("book_%d", i*1000+j)
			query := fmt.Sprintf(`INSERT INTO book_authors (id, book_id, author_id, marked_for_deletion) VALUES ('%s', '%s', %d, false)`, id, bookID, i)
			_, err = chaiDB.ExecContext(ctx, query)
			if err != nil {
				b.Fatalf("failed to insert benchmark data: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chaiStore.GetAllAuthorBookCounts_Chai(ctx)
		if err != nil {
			b.Fatalf("GetAllAuthorBookCounts_Chai failed: %v", err)
		}
	}
}

// BenchmarkGetAllAuthorBookCounts_Pebble benchmarks the Pebble version
func BenchmarkGetAllAuthorBookCounts_Pebble(b *testing.B) {
	tmpDir := b.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		b.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	// Empty database for baseline
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pebbleDB.GetAllAuthorBookCounts()
		if err != nil {
			b.Fatalf("GetAllAuthorBookCounts failed: %v", err)
		}
	}
}
