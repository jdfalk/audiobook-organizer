// file: internal/database/chai_all_series_test.go
// version: 1.0.0
// guid: 4a1b2c3d-e4f5-6a7b-8c9d-0e1f2a3b4c5d
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllSeries_Chai_Basic validates SQL version returns all series ordered by name
func TestGetAllSeries_Chai_Basic(t *testing.T) {
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

	// Insert 3 series in non-alphabetical order to verify ORDER BY name
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO series (id, name, author_id, marked_for_deletion)
		VALUES
			(3, 'Zebra Chronicles', NULL, false),
			(1, 'Apex Legends', NULL, false),
			(2, 'Middle Earth', NULL, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	series, err := chaiStore.GetAllSeries_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeries_Chai failed: %v", err)
	}

	if len(series) != 3 {
		t.Fatalf("expected 3 series, got %d", len(series))
	}

	// Verify alphabetical order
	expectedOrder := []string{"Apex Legends", "Middle Earth", "Zebra Chronicles"}
	for i, name := range expectedOrder {
		if series[i].Name != name {
			t.Errorf("series[%d]: expected name %q, got %q", i, name, series[i].Name)
		}
	}
}

// TestGetAllSeries_Chai_EmptyDatabase validates behavior on empty DB
func TestGetAllSeries_Chai_EmptyDatabase(t *testing.T) {
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

	series, err := chaiStore.GetAllSeries_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeries_Chai failed on empty DB: %v", err)
	}

	if len(series) != 0 {
		t.Errorf("expected empty result, got %v", series)
	}
}

// TestGetAllSeries_Chai_WithAuthorID validates that author_id is populated correctly
func TestGetAllSeries_Chai_WithAuthorID(t *testing.T) {
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

	// Insert authors first (FK may not be enforced, but good practice)
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO authors (id, name, marked_for_deletion)
		VALUES (42, 'Brandon Sanderson', false)
	`)
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}

	// Insert series: one with author_id, one without
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO series (id, name, author_id, marked_for_deletion)
		VALUES
			(1, 'Mistborn', 42, false),
			(2, 'Standalone', NULL, false)
	`)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	series, err := chaiStore.GetAllSeries_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeries_Chai failed: %v", err)
	}

	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(series))
	}

	// Find Mistborn (should be first alphabetically)
	mistborn := series[0]
	if mistborn.Name != "Mistborn" {
		t.Fatalf("expected Mistborn first, got %q", mistborn.Name)
	}
	if mistborn.AuthorID == nil || *mistborn.AuthorID != 42 {
		t.Errorf("Mistborn: expected author_id=42, got %v", mistborn.AuthorID)
	}

	// Standalone should have nil author_id
	standalone := series[1]
	if standalone.Name != "Standalone" {
		t.Fatalf("expected Standalone second, got %q", standalone.Name)
	}
	if standalone.AuthorID != nil {
		t.Errorf("Standalone: expected nil author_id, got %v", standalone.AuthorID)
	}
}

// TestGetAllSeries_ChaiVsPebble compares Pebble and Chai implementations return equivalent results.
// Both stores are seeded with the same series names; we compare by name set (order may differ).
func TestGetAllSeries_ChaiVsPebble(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Build Pebble store with some series
	pstore, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	seriesNames := []string{"Fantasy Series", "Mystery Series", "Sci-Fi Series"}
	for _, name := range seriesNames {
		if _, err := pstore.CreateSeries(name, nil); err != nil {
			t.Fatalf("CreateSeries(%q) failed: %v", name, err)
		}
	}

	// Pebble result
	pebbleStore, ok := pstore.(*PebbleStore)
	if !ok {
		t.Fatal("expected *PebbleStore from setupPebbleTestDB")
	}
	pebbleSeries, err := pebbleStore.GetAllSeries_Pebble()
	if err != nil {
		t.Fatalf("GetAllSeries_Pebble failed: %v", err)
	}

	// Build Chai store with same series
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

	for i, name := range seriesNames {
		_, err = chaiDB.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO series (id, name, author_id, marked_for_deletion)
			VALUES (%d, '%s', NULL, false)
		`, i+1, name))
		if err != nil {
			t.Fatalf("failed to insert series %q into Chai: %v", name, err)
		}
	}

	// Chai result
	chaiSeries, err := chaiStore.GetAllSeries_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllSeries_Chai failed: %v", err)
	}

	// Compare counts
	if len(pebbleSeries) != len(chaiSeries) {
		t.Fatalf("result count mismatch: Pebble=%d, Chai=%d", len(pebbleSeries), len(chaiSeries))
	}

	// Build name sets for comparison (order may differ between implementations)
	pebbleNames := make(map[string]bool, len(pebbleSeries))
	for _, s := range pebbleSeries {
		pebbleNames[s.Name] = true
	}

	for _, s := range chaiSeries {
		if !pebbleNames[s.Name] {
			t.Errorf("Chai series %q not found in Pebble results", s.Name)
		}
	}
}

// BenchmarkGetAllSeries_Chai benchmarks the SQL implementation
func BenchmarkGetAllSeries_Chai(b *testing.B) {
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

	// Insert 100 series for a meaningful benchmark
	seriesInsert := "INSERT INTO series (id, name, author_id, marked_for_deletion) VALUES"
	values := []string{}
	for i := 1; i <= 100; i++ {
		values = append(values, fmt.Sprintf("(%d, 'Series %04d', NULL, false)", i, i))
	}
	query := seriesInsert + "\n" + values[0]
	for _, v := range values[1:] {
		query += ",\n" + v
	}
	if _, err := chaiDB.ExecContext(ctx, query); err != nil {
		b.Fatalf("failed to insert benchmark series: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := chaiStore.GetAllSeries_Chai(ctx); err != nil {
			b.Fatalf("GetAllSeries_Chai failed: %v", err)
		}
	}
}
