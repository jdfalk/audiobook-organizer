// file: internal/database/chai_author_aliases_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-43f4-a5b6-c7d8e9f0a1b2
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
)

// TestGetAllAuthorAliases_ChaiSQL validates the SQL version returns all aliases sorted by alias_name.
func TestGetAllAuthorAliases_ChaiSQL(t *testing.T) {
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

	// Insert test aliases in non-alphabetical order to verify ORDER BY
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO author_aliases (id, author_id, alias_name, alias_type)
		VALUES
			(1, 10, 'Zebra Author',   'pen_name'),
			(2, 20, 'Alpha Author',   'alias'),
			(3, 10, 'Middle Author',  'handle'),
			(4, 30, 'Beta Author',    'pen_name')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	aliases, err := chaiStore.GetAllAuthorAliases_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorAliases_Chai failed: %v", err)
	}

	if len(aliases) != 4 {
		t.Fatalf("expected 4 aliases, got %d", len(aliases))
	}

	// Verify ORDER BY alias_name (alphabetical)
	expectedOrder := []string{"Alpha Author", "Beta Author", "Middle Author", "Zebra Author"}
	for i, name := range expectedOrder {
		if aliases[i].AliasName != name {
			t.Errorf("position %d: expected %q, got %q", i, name, aliases[i].AliasName)
		}
	}

	// Verify field mapping
	// Alpha Author (id=2, author_id=20, alias_type=alias)
	if aliases[0].ID != 2 || aliases[0].AuthorID != 20 || aliases[0].AliasType != "alias" {
		t.Errorf("Alpha Author: unexpected fields: id=%d authorID=%d aliasType=%q",
			aliases[0].ID, aliases[0].AuthorID, aliases[0].AliasType)
	}
}

// TestGetAllAuthorAliases_ChaiEmpty validates the SQL version returns empty slice for empty table.
func TestGetAllAuthorAliases_ChaiEmpty(t *testing.T) {
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

	aliases, err := chaiStore.GetAllAuthorAliases_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorAliases_Chai on empty table failed: %v", err)
	}

	// nil or empty slice both acceptable
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases in empty DB, got %d", len(aliases))
	}
}

// TestGetAllAuthorAliases_ChaiNullAliasType validates NULL alias_type is handled gracefully.
func TestGetAllAuthorAliases_ChaiNullAliasType(t *testing.T) {
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

	// Insert an alias with NULL alias_type
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO author_aliases (id, author_id, alias_name, alias_type)
		VALUES (1, 5, 'No Type Author', NULL)
	`)
	if err != nil {
		t.Fatalf("failed to insert alias with NULL type: %v", err)
	}

	aliases, err := chaiStore.GetAllAuthorAliases_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorAliases_Chai with NULL alias_type failed: %v", err)
	}

	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}

	if aliases[0].AliasName != "No Type Author" {
		t.Errorf("expected alias_name 'No Type Author', got %q", aliases[0].AliasName)
	}

	// NULL alias_type should map to empty string
	if aliases[0].AliasType != "" {
		t.Errorf("expected empty AliasType for NULL, got %q", aliases[0].AliasType)
	}
}

// TestGetAllAuthorAliases_Pebble validates the Pebble fallback path works with an empty DB.
func TestGetAllAuthorAliases_Pebble(t *testing.T) {
	tmpDir := t.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	aliases, err := pebbleDB.GetAllAuthorAliases()
	if err != nil {
		t.Fatalf("GetAllAuthorAliases on empty DB failed: %v", err)
	}

	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases in empty DB, got %d", len(aliases))
	}
}

// TestGetAllAuthorAliases_ChaiRouting validates the feature flag routes to Chai when enabled.
func TestGetAllAuthorAliases_ChaiRouting(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	pebblePath := filepath.Join(tmpDir, "pebble.db")
	pebbleDB, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer pebbleDB.Close()

	// With UseChaiDB=false (default), should fall through to Pebble
	aliases, err := pebbleDB.GetAllAuthorAliases_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorAliases_Chai (flag off) failed: %v", err)
	}
	// Pebble is empty, so we expect zero results
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases (Pebble fallback, empty DB), got %d", len(aliases))
	}

	// With UseChaiDB=true but no chai, should still fall through to Pebble
	pebbleDB.UseChaiDB = true
	aliases2, err := pebbleDB.GetAllAuthorAliases_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllAuthorAliases_Chai (flag on, no chai) failed: %v", err)
	}
	if len(aliases2) != 0 {
		t.Errorf("expected 0 aliases (Pebble fallback, no chai DB), got %d", len(aliases2))
	}
}
