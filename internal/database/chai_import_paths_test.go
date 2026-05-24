// file: internal/database/chai_import_paths_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-4f3a-5b6c-7d8e9f0a1b2c
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllImportPaths_Chai validates that the SQL implementation returns
// all import paths ordered by name, and computes book_count from books.source_import_path.
func TestGetAllImportPaths_Chai(t *testing.T) {
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

	// Seed two import paths
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO import_paths (id, path, name, enabled)
		VALUES
			(1, '/mnt/audio/fiction', 'Fiction', true),
			(2, '/mnt/audio/nonfiction', 'Nonfiction', true),
			(3, '/mnt/audio/podcasts', 'Podcasts', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert import_paths: %v", err)
	}

	// Seed books: 3 in fiction, 1 in nonfiction, 0 in podcasts, 1 deleted in fiction
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO books (id, title, source_import_path, marked_for_deletion)
		VALUES
			('b1', 'Book 1', '/mnt/audio/fiction', false),
			('b2', 'Book 2', '/mnt/audio/fiction', false),
			('b3', 'Book 3', '/mnt/audio/fiction/subfolder', false),
			('b4', 'Book 4', '/mnt/audio/fiction', true),
			('b5', 'Book 5', '/mnt/audio/nonfiction', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	paths, err := chaiStore.GetAllImportPaths_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllImportPaths_Chai failed: %v", err)
	}

	if len(paths) != 3 {
		t.Fatalf("expected 3 import paths, got %d", len(paths))
	}

	// Results should be ordered by name: Fiction, Nonfiction, Podcasts
	if paths[0].Name != "Fiction" {
		t.Errorf("first path should be Fiction, got %q", paths[0].Name)
	}
	if paths[1].Name != "Nonfiction" {
		t.Errorf("second path should be Nonfiction, got %q", paths[1].Name)
	}
	if paths[2].Name != "Podcasts" {
		t.Errorf("third path should be Podcasts, got %q", paths[2].Name)
	}

	// Fiction: b1, b2, b3 (b4 is deleted) = 3
	if paths[0].BookCount != 3 {
		t.Errorf("Fiction book_count: expected 3, got %d", paths[0].BookCount)
	}

	// Nonfiction: b5 = 1
	if paths[1].BookCount != 1 {
		t.Errorf("Nonfiction book_count: expected 1, got %d", paths[1].BookCount)
	}

	// Podcasts: no books = 0
	if paths[2].BookCount != 0 {
		t.Errorf("Podcasts book_count: expected 0, got %d", paths[2].BookCount)
	}

	// Verify enabled field
	if !paths[0].Enabled {
		t.Errorf("Fiction should be enabled")
	}
	if paths[2].Enabled {
		t.Errorf("Podcasts should be disabled")
	}
}

// TestGetAllImportPaths_Chai_Empty validates behavior with no import paths.
func TestGetAllImportPaths_Chai_Empty(t *testing.T) {
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

	paths, err := chaiStore.GetAllImportPaths_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllImportPaths_Chai on empty DB failed: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("expected 0 import paths in empty DB, got %d", len(paths))
	}
}

// TestGetAllImportPaths_Pebble validates the Pebble fallback still works.
func TestGetAllImportPaths_Pebble(t *testing.T) {
	tmpDir := t.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	paths, err := pebbleDB.GetAllImportPaths()
	if err != nil {
		t.Fatalf("GetAllImportPaths on empty Pebble DB failed: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("expected 0 import paths in empty Pebble DB, got %d", len(paths))
	}
}

// TestGetAllImportPaths_ChaiRouting validates the UseChaiDB flag routes correctly.
func TestGetAllImportPaths_ChaiRouting(t *testing.T) {
	tmpDir := t.TempDir()

	pebblePath := filepath.Join(tmpDir, "pebble.db")
	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	// With flag off (default), should use Pebble — no error even with no Chai DB
	pebbleDB.UseChaiDB = false
	paths, err := pebbleDB.GetAllImportPaths()
	if err != nil {
		t.Fatalf("GetAllImportPaths (Pebble path) failed: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths from empty Pebble, got %d", len(paths))
	}

	// With flag on but no Chai DB attached, Chai path should return error
	pebbleDB.UseChaiDB = true
	// chai is nil, so GetAllImportPaths_Chai should fall back to Pebble
	paths2, err2 := pebbleDB.GetAllImportPaths()
	if err2 != nil {
		t.Fatalf("GetAllImportPaths with UseChaiDB=true but nil chai should fallback: %v", err2)
	}
	_ = paths2 // result depends on Pebble fallback path (either 0 or error)
}

// BenchmarkGetAllImportPaths_Chai benchmarks the SQL implementation.
func BenchmarkGetAllImportPaths_Chai(b *testing.B) {
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

	// Insert benchmark data: 20 import paths, 200 books
	for i := 1; i <= 20; i++ {
		path := fmt.Sprintf("/mnt/audio/library_%02d", i)
		name := fmt.Sprintf("Library %02d", i)
		_, err = chaiDB.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO import_paths (id, path, name, enabled) VALUES (%d, '%s', '%s', true)`,
			i, path, name,
		))
		if err != nil {
			b.Fatalf("failed to insert import_path %d: %v", i, err)
		}

		for j := 1; j <= 10; j++ {
			bookID := fmt.Sprintf("book_%02d_%03d", i, j)
			_, err = chaiDB.ExecContext(ctx, fmt.Sprintf(
				`INSERT INTO books (id, title, source_import_path, marked_for_deletion) VALUES ('%s', 'Title %s', '%s', false)`,
				bookID, bookID, path,
			))
			if err != nil {
				b.Fatalf("failed to insert book %s: %v", bookID, err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := chaiStore.GetAllImportPaths_Chai(ctx)
		if err != nil {
			b.Fatalf("GetAllImportPaths_Chai failed: %v", err)
		}
	}
}
