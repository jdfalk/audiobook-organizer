// file: internal/database/chai_get_all_authors_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6
// last-edited: 2026-05-24

package database

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// TestGetAllAuthors_Chai_Basic validates that all authors are returned ordered by name.
func TestGetAllAuthors_Chai_Basic(t *testing.T) {
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

	// Insert 3 authors intentionally out of alphabetical order
	names := []string{"Zelda Fitzgerald", "Alice Munro", "Brandon Sanderson"}
	for i, name := range names {
		_, err := chaiDB.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO authors (id, name) VALUES (%d, '%s')`, i+1, name,
		))
		if err != nil {
			t.Fatalf("failed to insert author %q: %v", name, err)
		}
	}

	authors, err := chaiStore.GetAllAuthors_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthors_Chai failed: %v", err)
	}
	if len(authors) != 3 {
		t.Fatalf("expected 3 authors, got %d", len(authors))
	}

	// Verify alphabetical ordering
	expected := []string{"Alice Munro", "Brandon Sanderson", "Zelda Fitzgerald"}
	for i, want := range expected {
		if authors[i].Name != want {
			t.Errorf("authors[%d].Name = %q, want %q", i, authors[i].Name, want)
		}
	}

	// Verify IDs are populated
	for _, a := range authors {
		if a.ID == 0 {
			t.Errorf("author %q has zero ID", a.Name)
		}
	}

	t.Log("GetAllAuthors_Chai basic ordering test passed")
}

// TestGetAllAuthors_Chai_EmptyDB validates that an empty authors table returns an empty (not nil) result.
func TestGetAllAuthors_Chai_EmptyDB(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	chaiPath := filepath.Join(tmpDir, "chai_empty.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	authors, err := chaiStore.GetAllAuthors_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthors_Chai failed on empty DB: %v", err)
	}
	// nil slice is acceptable; just must not error
	if len(authors) != 0 {
		t.Errorf("expected 0 authors from empty DB, got %d", len(authors))
	}

	t.Log("GetAllAuthors_Chai empty DB test passed")
}

// TestGetAllAuthors_Chai_NilDB validates that a nil DB returns an error.
func TestGetAllAuthors_Chai_NilDB(t *testing.T) {
	cs := &ChaiStore{db: nil}
	ctx := context.Background()

	_, err := cs.GetAllAuthors_Chai(ctx)
	if err == nil {
		t.Fatal("expected error from nil DB, got nil")
	}

	t.Log("GetAllAuthors_Chai nil DB error test passed")
}

// TestGetAllAuthors_Chai_LargeSet validates ordering holds for a larger data set.
func TestGetAllAuthors_Chai_LargeSet(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	chaiPath := filepath.Join(tmpDir, "chai_large.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	chaiStore, err := NewChaiStore(chaiDB.DB())
	if err != nil {
		t.Fatalf("NewChaiStore failed: %v", err)
	}

	const total = 100
	// Insert authors with names that sort predictably: "Author 001" ... "Author 100"
	for i := 1; i <= total; i++ {
		name := fmt.Sprintf("Author %03d", i)
		_, err := chaiDB.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO authors (id, name) VALUES (%d, '%s')`, i, name,
		))
		if err != nil {
			t.Fatalf("failed to insert author %d: %v", i, err)
		}
	}

	authors, err := chaiStore.GetAllAuthors_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthors_Chai failed: %v", err)
	}
	if len(authors) != total {
		t.Fatalf("expected %d authors, got %d", total, len(authors))
	}

	// Verify strictly ascending order
	for i := 1; i < len(authors); i++ {
		if authors[i].Name <= authors[i-1].Name {
			t.Errorf("ordering violated at index %d: %q <= %q", i, authors[i].Name, authors[i-1].Name)
		}
	}

	t.Logf("GetAllAuthors_Chai large set test passed (%d authors, correctly ordered)", total)
}
