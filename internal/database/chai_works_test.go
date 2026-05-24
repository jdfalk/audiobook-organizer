// file: internal/database/chai_works_test.go
// version: 1.0.0
// guid: f1a2b3c4-d5e6-47f8-9a0b-c1d2e3f4a5b6
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
)

// TestGetAllWorks_Chai_StubReturnsEmpty verifies that GetAllWorks_Chai returns an
// empty (non-nil) slice while the works table is not yet in chai_schema.go.
func TestGetAllWorks_Chai_StubReturnsEmpty(t *testing.T) {
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

	works, err := chaiStore.GetAllWorks_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllWorks_Chai returned unexpected error: %v", err)
	}
	if works == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(works) != 0 {
		t.Errorf("expected 0 works (schema not yet extended), got %d", len(works))
	}
}

// TestGetAllWorks_PebbleRoutesViaChai confirms that when UseChaiDB=true the routing
// delegates to the Chai path and returns successfully (stub path, empty result).
func TestGetAllWorks_PebbleRoutesViaChai(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	pebbleStore := store.(*PebbleStore)

	chaiPath := filepath.Join(tmpDir, "chai.db")
	chaiDB, err := NewChaiDB(ctx, chaiPath)
	if err != nil {
		t.Fatalf("NewChaiDB failed: %v", err)
	}
	defer chaiDB.Close()

	pebbleStore.chai = chaiDB
	pebbleStore.UseChaiDB = true

	works, err := pebbleStore.GetAllWorks()
	if err != nil {
		t.Fatalf("GetAllWorks (Chai routing) returned unexpected error: %v", err)
	}
	if works == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
}

// TestGetAllWorks_PebbleRoutesViaPebble confirms that when UseChaiDB=false the Pebble
// implementation is used and returns successfully with an empty store.
func TestGetAllWorks_PebbleRoutesViaPebble(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	// Default: UseChaiDB = false
	works, err := store.GetAllWorks()
	if err != nil {
		t.Fatalf("GetAllWorks (Pebble routing) returned unexpected error: %v", err)
	}
	// Empty store — nil or empty slice, both fine
	_ = works
}
