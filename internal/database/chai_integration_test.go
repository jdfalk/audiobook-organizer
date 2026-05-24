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
