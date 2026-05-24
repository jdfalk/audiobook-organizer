// file: internal/database/chai_series_test.go
// version: 1.0.0
// guid: f8a9b0c1-d2e3-4f5a-6b7c-8d9e0f1a2b3c
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllSeriesFileCounts_ChaiSQL validates SQL version returns correct counts
func TestGetAllSeriesFileCounts_ChaiSQL(t *testing.T) {
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

	// Insert test data: 2 series with different file counts
	// Series 1: 2 books with 3 files each = 6 files
	// Series 2: 1 book with 2 files = 2 files
	// Series 3: 1 book (marked for deletion) with 5 files = 0 files (excluded)
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO series (id, name, normalized_name, marked_for_deletion)
		VALUES
			(1, 'Fantasy Series', 'fantasy series', false),
			(2, 'Mystery Series', 'mystery series', false),
			(3, 'Deleted Series', 'deleted series', true)
	`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Insert books
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO books (id, title, normalized_title, series_id, is_primary_version, marked_for_deletion)
		VALUES
			('book-1-1', 'Fantasy Book 1', 'fantasy book 1', 1, true, false),
			('book-1-2', 'Fantasy Book 2', 'fantasy book 2', 1, true, false),
			('book-2-1', 'Mystery Book 1', 'mystery book 1', 2, true, false),
			('book-3-1', 'Deleted Book', 'deleted book', 3, true, true)
	`)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Insert files
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, marked_for_deletion)
		VALUES
			('file-1-1', 'book-1-1', '/path/to/file1.m4b', false),
			('file-1-2', 'book-1-1', '/path/to/file2.m4b', false),
			('file-1-3', 'book-1-1', '/path/to/file3.m4b', false),
			('file-1-4', 'book-1-2', '/path/to/file4.m4b', false),
			('file-1-5', 'book-1-2', '/path/to/file5.m4b', false),
			('file-1-6', 'book-1-2', '/path/to/file6.m4b', false),
			('file-2-1', 'book-2-1', '/path/to/file7.m4b', false),
			('file-2-2', 'book-2-1', '/path/to/file8.m4b', false),
			('file-3-1', 'book-3-1', '/path/to/file9.m4b', false),
			('file-3-2', 'book-3-1', '/path/to/file10.m4b', false),
			('file-3-3', 'book-3-1', '/path/to/file11.m4b', false),
			('file-3-4', 'book-3-1', '/path/to/file12.m4b', false),
			('file-3-5', 'book-3-1', '/path/to/file13.m4b', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert book files: %v", err)
	}

	// Execute Chai SQL version
	counts, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
	}

	// Verify counts (series 3 should be excluded because marked_for_deletion is true)
	expected := map[int]int{
		1: 6, // 2 books * 3 files
		2: 2, // 1 book * 2 files
		// 3 should not be in the map (marked for deletion)
	}

	if len(counts) != len(expected) {
		t.Errorf("expected %d series, got %d (results: %v)", len(expected), len(counts), counts)
	}

	for seriesID, expectedCount := range expected {
		actualCount, exists := counts[seriesID]
		if !exists {
			t.Errorf("series %d not in results", seriesID)
			continue
		}
		if actualCount != expectedCount {
			t.Errorf("series %d: expected count %d, got %d", seriesID, expectedCount, actualCount)
		}
	}

	// Verify series 3 (deleted) is not in results
	if count, exists := counts[3]; exists {
		t.Errorf("series 3 (marked for deletion) should not be in results, got count %d", count)
	}
}

// TestGetAllSeriesFileCounts_EmptyDatabase validates behavior on empty DB
func TestGetAllSeriesFileCounts_EmptyDatabase(t *testing.T) {
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

	counts, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
	}

	if len(counts) != 0 {
		t.Errorf("expected empty counts, got %v", counts)
	}
}

// TestGetAllSeriesFileCounts_NoFilesForSeries validates series with no files
func TestGetAllSeriesFileCounts_NoFilesForSeries(t *testing.T) {
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

	// Insert series and book without files
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO series (id, name, normalized_name, marked_for_deletion)
		VALUES (1, 'Empty Series', 'empty series', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO books (id, title, normalized_title, series_id, is_primary_version, marked_for_deletion)
		VALUES ('book-empty', 'Book 1', 'book 1', 1, true, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	counts, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
	}

	// Series with book but no files should have count 0
	if count, ok := counts[1]; !ok || count != 0 {
		t.Errorf("expected series 1 to have count 0, got %v (exists: %v)", count, ok)
	}
}

// TestGetAllSeriesFileCounts_NullSeriesID validates NULL series handling
func TestGetAllSeriesFileCounts_NullSeriesID(t *testing.T) {
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

	// Insert book with NULL series_id and files
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO books (id, title, normalized_title, series_id, is_primary_version, marked_for_deletion)
		VALUES ('book-standalone', 'Standalone', 'standalone', NULL, true, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert standalone book: %v", err)
	}

	// Insert files for standalone book
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, marked_for_deletion)
		VALUES
			('file-s-1', 'book-standalone', '/path/to/file1.m4b', false),
			('file-s-2', 'book-standalone', '/path/to/file2.m4b', false),
			('file-s-3', 'book-standalone', '/path/to/file3.m4b', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert book files: %v", err)
	}

	counts, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
	}

	// Books with NULL series_id should not appear in results
	if len(counts) != 0 {
		t.Errorf("expected no series in results (NULL series_id should be excluded), got %v", counts)
	}
}

// TestGetAllSeriesFileCounts_OnlyPrimaryVersions validates non-primary versions are excluded
func TestGetAllSeriesFileCounts_OnlyPrimaryVersions(t *testing.T) {
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

	// Insert series
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO series (id, name, normalized_name, marked_for_deletion)
		VALUES (1, 'Test Series', 'test series', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Insert primary and alternate books
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO books (id, title, normalized_title, series_id, is_primary_version, marked_for_deletion)
		VALUES
			('book-primary', 'Book Edition 1', 'book edition 1', 1, true, false),
			('book-alt', 'Book Edition 2', 'book edition 2', 1, false, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Insert files: 3 for primary, 5 for alternate
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO book_files (id, book_id, file_path, marked_for_deletion)
		VALUES
			('file-p-1', 'book-primary', '/path/to/primary/file1.m4b', false),
			('file-p-2', 'book-primary', '/path/to/primary/file2.m4b', false),
			('file-p-3', 'book-primary', '/path/to/primary/file3.m4b', false),
			('file-a-1', 'book-alt', '/path/to/alt/file1.m4b', false),
			('file-a-2', 'book-alt', '/path/to/alt/file2.m4b', false),
			('file-a-3', 'book-alt', '/path/to/alt/file3.m4b', false),
			('file-a-4', 'book-alt', '/path/to/alt/file4.m4b', false),
			('file-a-5', 'book-alt', '/path/to/alt/file5.m4b', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert book files: %v", err)
	}

	counts, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
	}

	// Should only count primary version's 3 files, not alternate's 5
	if count, ok := counts[1]; !ok || count != 3 {
		t.Errorf("series 1: expected 3 (primary only), got %v (exists: %v)", count, ok)
	}
}

// BenchmarkGetAllSeriesFileCounts_ChaiSQL benchmarks the SQL query performance
func BenchmarkGetAllSeriesFileCounts_ChaiSQL(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := context.Background()

	chaiPath := filepath.Join(tmpDir, "bench.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		b.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		b.Fatalf("NewChaiStore failed: %v", err)
	}

	// Create test data: 10 series, 3 books each, 5 files each
	// Total: 10 series, 30 books, 150 files
	seriesInserts := "INSERT INTO series (id, name, normalized_name, marked_for_deletion) VALUES"
	bookInserts := "INSERT INTO books (id, title, normalized_title, series_id, is_primary_version, marked_for_deletion) VALUES"
	fileInserts := "INSERT INTO book_files (id, book_id, file_path, marked_for_deletion) VALUES"

	seriesValues := []string{}
	bookValues := []string{}
	fileValues := []string{}

	fileID := 1
	for seriesID := 1; seriesID <= 10; seriesID++ {
		seriesValues = append(seriesValues, fmt.Sprintf("(%d, 'Series %d', 'series %d', false)", seriesID, seriesID, seriesID))

		for bookNum := 1; bookNum <= 3; bookNum++ {
			bookID := fmt.Sprintf("book-%d-%d", seriesID, bookNum)
			bookValues = append(bookValues, fmt.Sprintf("('%s', 'Book %d Series %d', 'book %d series %d', %d, true, false)", bookID, bookNum, seriesID, bookNum, seriesID, seriesID))

			for fileNum := 1; fileNum <= 5; fileNum++ {
				filePath := fmt.Sprintf("/path/to/file-%d-%d-%d.m4b", seriesID, bookNum, fileNum)
				fileIDStr := fmt.Sprintf("file-%d-%d-%d", seriesID, bookNum, fileNum)
				fileValues = append(fileValues, fmt.Sprintf("('%s', '%s', '%s', false)", fileIDStr, bookID, filePath))
				fileID++
			}
		}
	}

	// Batch insert series
	if len(seriesValues) > 0 {
		query := seriesInserts + "\n" + fmt.Sprintf("%s", seriesValues[0])
		for _, val := range seriesValues[1:] {
			query += ",\n" + val
		}
		_, err = chaiDB.ExecContext(ctx, query)
		if err != nil {
			b.Fatalf("failed to insert series: %v", err)
		}
	}

	// Batch insert books
	if len(bookValues) > 0 {
		query := bookInserts + "\n" + fmt.Sprintf("%s", bookValues[0])
		for _, val := range bookValues[1:] {
			query += ",\n" + val
		}
		_, err = chaiDB.ExecContext(ctx, query)
		if err != nil {
			b.Fatalf("failed to insert books: %v", err)
		}
	}

	// Batch insert files
	if len(fileValues) > 0 {
		query := fileInserts + "\n" + fmt.Sprintf("%s", fileValues[0])
		for _, val := range fileValues[1:] {
			query += ",\n" + val
		}
		_, err = chaiDB.ExecContext(ctx, query)
		if err != nil {
			b.Fatalf("failed to insert book files: %v", err)
		}
	}

	// Benchmark the query
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chaiStore.GetAllSeriesFileCounts_Chai(ctx)
		if err != nil {
			b.Fatalf("GetAllSeriesFileCounts_Chai failed: %v", err)
		}
	}
}
