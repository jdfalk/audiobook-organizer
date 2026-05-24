package database

// version: 1.0.0
// guid: c3d4e5f6-a7b8-49c0-d1e2-f3g4h5i6j7k8
// last-edited: 2026-05-24

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestNewChaiDB_InitializesDatabase validates that NewChaiDB opens and initializes
func TestNewChaiDB_InitializesDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("expected non-nil ChaiDB")
	}

	if db.Path() != dbPath {
		t.Errorf("expected path %s, got %s", dbPath, db.Path())
	}

	// Verify the database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file not created at %s", dbPath)
	}
}

// TestNewChaiDB_SchemaCreatedOnFirstRun validates that schema is created
func TestNewChaiDB_SchemaCreatedOnFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist by querying system information
	tables := []string{"authors", "series", "books", "book_files", "book_authors"}
	for _, tableName := range tables {
		var count int
		// Try to count rows in the table - if table doesn't exist, this will fail
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		err := db.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			t.Errorf("failed to query table %s: %v", tableName, err)
		}
	}
}

// TestNewChaiDB_SchemaIsIdempotent validates that schema can be initialized multiple times
func TestNewChaiDB_SchemaIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()

	// First initialization
	db1, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("first NewChaiDB failed: %v", err)
	}
	db1.Close()

	// Second initialization (should not fail)
	db2, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("second NewChaiDB failed: %v", err)
	}
	defer db2.Close()

	// Verify we can still query
	var count int
	err = db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&count)
	if err != nil {
		t.Errorf("failed to query after reinitialization: %v", err)
	}
}

// TestChaiDB_SelectQuery validates that SELECT queries work
func TestChaiDB_SelectQuery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Query with no parameters
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM authors").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 authors initially, got %d", count)
	}
}

// TestChaiDB_LimitQuery validates LIMIT queries
func TestChaiDB_LimitQuery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Query with LIMIT on empty table
	rows, err := db.QueryContext(ctx, "SELECT id, title FROM books LIMIT 5")
	if err != nil {
		t.Fatalf("failed to query books: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 rows in empty table, got %d", count)
	}
}

// TestChaiDB_Close validates cleanup
func TestChaiDB_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}

	// Close should succeed
	if err := db.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail gracefully
	_, err = db.QueryContext(ctx, "SELECT 1")
	if err == nil {
		t.Errorf("expected error after close, got nil")
	}
}

// TestChaiDB_Health validates health check
func TestChaiDB_Health(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Health check should succeed
	if err := db.Health(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

// TestChaiDB_ResetSchema validates schema reset
func TestChaiDB_ResetSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Reset schema
	if err := db.ResetSchema(ctx); err != nil {
		t.Fatalf("ResetSchema failed: %v", err)
	}

	// Verify tables are empty and accessible
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM authors").Scan(&count)
	if err != nil || count != 0 {
		t.Errorf("expected 0 authors after reset, got %d (err: %v)", count, err)
	}
}

// TestChaiDB_Stats validates database stats
func TestChaiDB_Stats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Get stats - should not panic
	stats := db.Stats()
	if stats.OpenConnections < 0 {
		t.Errorf("unexpected open connections: %d", stats.OpenConnections)
	}
}

// TestNewChaiDBFromPebble validates integration with Pebble input validation
func TestNewChaiDBFromPebble_ValidatesInput(t *testing.T) {
	tmpDir := t.TempDir()
	chaiPath := filepath.Join(tmpDir, "chai.db")
	ctx := context.Background()

	// nil pebbleDB should fail
	_, err := NewChaiDBFromPebble(ctx, nil, chaiPath)
	if err == nil {
		t.Error("expected error with nil pebbleDB, got nil")
	}
}

// BenchmarkChaiDB_CountQuery benchmarks COUNT queries
func BenchmarkChaiDB_CountQuery(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	ctx := context.Background()
	db, err := NewChaiDB(ctx, dbPath)
	if err != nil {
		b.Fatalf("NewChaiDB failed: %v", err)
	}
	defer db.Close()

	// Benchmark simple COUNT query
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM books").Scan(&count)
		if err != nil {
			b.Fatalf("query failed: %v", err)
		}
	}
}

// TestChaiStore_GetAllAuthorFileCounts_Chai validates the SQL-based author file count aggregation
func TestChaiStore_GetAllAuthorFileCounts_Chai(t *testing.T) {
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

	// Test 1: Empty database should return empty map
	counts, err := store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty map for empty DB, got %d authors", len(counts))
	}

	// Test 2: Single author with no books
	_, err = db.ExecContext(ctx, "INSERT INTO authors (id, name, normalized_name) VALUES (1, 'Author One', 'author one')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}

	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	// Author with no books in book_authors should not appear
	if len(counts) != 0 {
		t.Errorf("expected no authors with books, got %d", len(counts))
	}

	// Test 3: Insert a book and create book_authors relationship
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book1', 'Test Book', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_authors (id, book_id, author_id, role, position, marked_for_deletion)
		VALUES ('ba1', 'book1', 1, 'author', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book_authors: %v", err)
	}

	// Book with no files should count as 0 files
	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if count, exists := counts[1]; !exists {
		t.Errorf("expected author 1 to appear in counts")
	} else if count != 0 {
		t.Errorf("expected 0 files for author with book but no files, got %d", count)
	}

	// Test 4: Add files to the book
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, duration_ms, file_size_bytes, missing)
		VALUES ('file1', 'book1', '/tmp/test1.m4b', 3600000, 1000000, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if count, exists := counts[1]; !exists {
		t.Errorf("expected author 1 to appear in counts")
	} else if count != 1 {
		t.Errorf("expected 1 file for author 1, got %d", count)
	}

	// Test 5: Add multiple files to the same book
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, duration_ms, file_size_bytes, missing)
		VALUES ('file2', 'book1', '/tmp/test2.m4b', 3600000, 1000000, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file2: %v", err)
	}

	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if count, exists := counts[1]; !exists {
		t.Errorf("expected author 1 to appear in counts")
	} else if count != 2 {
		t.Errorf("expected 2 files for author 1, got %d", count)
	}

	// Test 6: Multiple authors with different file counts
	_, err = db.ExecContext(ctx, "INSERT INTO authors (id, name, normalized_name) VALUES (2, 'Author Two', 'author two')")
	if err != nil {
		t.Fatalf("failed to insert author 2: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book2', 'Another Book', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book2: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_authors (id, book_id, author_id, role, position, marked_for_deletion)
		VALUES ('ba2', 'book2', 2, 'author', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book_authors for author 2: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, duration_ms, file_size_bytes, missing)
		VALUES ('file3', 'book2', '/tmp/test3.m4b', 3600000, 1000000, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file for book2: %v", err)
	}

	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if count1, exists := counts[1]; !exists || count1 != 2 {
		t.Errorf("expected author 1 to have 2 files, got %d", counts[1])
	}
	if count2, exists := counts[2]; !exists || count2 != 1 {
		t.Errorf("expected author 2 to have 1 file, got %d", counts[2])
	}

	// Test 7: Marked for deletion should be excluded
	_, err = db.ExecContext(ctx, `
		UPDATE book_authors SET marked_for_deletion = 1 WHERE author_id = 2
	`)
	if err != nil {
		t.Fatalf("failed to update book_authors: %v", err)
	}

	counts, err = store.GetAllAuthorFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
	}
	if _, exists := counts[2]; exists {
		t.Errorf("expected author 2 to be excluded (marked_for_deletion), but found in results")
	}
	if count1, exists := counts[1]; !exists || count1 != 2 {
		t.Errorf("expected author 1 to still have 2 files, got %d", counts[1])
	}
}

// BenchmarkChaiStore_GetAllAuthorFileCounts_Chai benchmarks the SQL JOIN query
func BenchmarkChaiStore_GetAllAuthorFileCounts_Chai(b *testing.B) {
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

	// Set up test data: 10 authors, 20 books, 50 files
	for i := 1; i <= 10; i++ {
		_, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO authors (id, name, normalized_name) VALUES (%d, 'Author %d', 'author %d')", i, i, i))
		if err != nil {
			b.Fatalf("failed to insert author: %v", err)
		}
	}

	for i := 1; i <= 20; i++ {
		authorID := (i-1)%10 + 1
		_, err := db.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO books (id, title, is_primary_version, marked_for_deletion) VALUES ('book%d', 'Book %d', 1, 0)",
			i, i))
		if err != nil {
			b.Fatalf("failed to insert book: %v", err)
		}

		_, err = db.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO book_authors (id, book_id, author_id, role, position, marked_for_deletion) VALUES ('ba%d', 'book%d', %d, 'author', 0, 0)",
			i, i, authorID))
		if err != nil {
			b.Fatalf("failed to insert book_authors: %v", err)
		}
	}

	for i := 1; i <= 50; i++ {
		bookNum := (i-1)%20 + 1
		_, err := db.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO book_files (id, book_id, file_path, duration_ms, file_size_bytes, missing) VALUES ('file%d', 'book%d', '/tmp/file%d.m4b', 3600000, 1000000, 0)",
			i, bookNum, i))
		if err != nil {
			b.Fatalf("failed to insert file: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetAllAuthorFileCounts_Chai(ctx)
		if err != nil {
			b.Fatalf("GetAllAuthorFileCounts_Chai failed: %v", err)
		}
	}
}

// TestChaiStore_CountFiles_Chai validates the SQL-based file counting
// This is Task 2.5 - 90% code reduction test
func TestChaiStore_CountFiles_Chai(t *testing.T) {
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

	// Test 1: Empty database should return 0
	count, err := store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files for empty DB, got %d", count)
	}

	// Test 2: Single book with no files counts as 1
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book1', 'Test Book', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file for book with no file records, got %d", count)
	}

	// Test 3: Add one file to the book
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (1, 'book1', '/tmp/test1.m4b', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file, got %d", count)
	}

	// Test 4: Add another file to the same book
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (2, 'book1', '/tmp/test2.m4b', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file 2: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 files, got %d", count)
	}

	// Test 5: Add another book with files
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book2', 'Another Book', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book2: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (3, 'book2', '/tmp/test3.m4b', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file 3: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 files (book1:2 + book2:1), got %d", count)
	}

	// Test 6: Missing files should not be counted
	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (4, 'book2', '/tmp/test4_missing.m4b', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert missing file: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 files (missing file should not count), got %d", count)
	}

	// Test 7: Non-primary versions should not be counted
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book3', 'Duplicate', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert non-primary book: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (5, 'book3', '/tmp/test5.m4b', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file for non-primary book: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 files (non-primary should not count), got %d", count)
	}

	// Test 8: Marked for deletion should not be counted
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book4', 'Deleted', 1, 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert deleted book: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
		VALUES (6, 'book4', '/tmp/test6.m4b', 0, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert file for deleted book: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 files (deleted book should not count), got %d", count)
	}

	// Test 9: Add a third primary book with no files (should count as 1)
	_, err = db.ExecContext(ctx, `
		INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
		VALUES ('book5', 'Book Five', 1, 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert book5: %v", err)
	}

	count, err = store.CountFiles_Chai(ctx)
	if err != nil {
		t.Fatalf("CountFiles_Chai failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 files (3 actual + 1 for book with no files), got %d", count)
	}
}

// BenchmarkChaiStore_CountFiles_Chai benchmarks the SQL file counting query
// Task 2.5: Compare with Pebble version (should be 10-100x faster)
func BenchmarkChaiStore_CountFiles_Chai(b *testing.B) {
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

	// Set up test data: 100 books, 300 files
	for i := 1; i <= 100; i++ {
		query := fmt.Sprintf(`
			INSERT INTO books (id, title, is_primary_version, marked_for_deletion)
			VALUES ('book%d', 'Book %d', 1, 0)
		`, i, i)
		_, err := db.ExecContext(ctx, query)
		if err != nil {
			b.Fatalf("failed to insert book: %v", err)
		}
	}

	for i := 1; i <= 300; i++ {
		bookID := fmt.Sprintf("book%d", (i-1)%100+1)
		query := fmt.Sprintf(`
			INSERT INTO book_files (id, book_id, file_path, missing, marked_for_deletion)
			VALUES (%d, '%s', '/tmp/file%d.m4b', 0, 0)
		`, i, bookID, i)
		_, err := db.ExecContext(ctx, query)
		if err != nil {
			b.Fatalf("failed to insert file: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.CountFiles_Chai(ctx)
		if err != nil {
			b.Fatalf("CountFiles_Chai failed: %v", err)
		}
	}
}
