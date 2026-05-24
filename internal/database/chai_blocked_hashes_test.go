// file: internal/database/chai_blocked_hashes_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-4f3a-b4c5-d6e7f8a9b0c1
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestGetAllBlockedHashes_Chai_Empty validates empty result on fresh database
func TestGetAllBlockedHashes_Chai_Empty(t *testing.T) {
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

	items, err := chaiStore.GetAllBlockedHashes_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllBlockedHashes_Chai failed on empty DB: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on empty DB, got %d", len(items))
	}
}

// TestGetAllBlockedHashes_Chai_SingleHash validates retrieval of a single hash
func TestGetAllBlockedHashes_Chai_SingleHash(t *testing.T) {
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

	// Insert directly via SQL (no Add_Chai method yet)
	now := time.Now().UTC().Truncate(time.Second)
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO blocked_hashes (hash, reason, created_at)
		VALUES ('abc123', 'duplicate file', ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to insert test hash: %v", err)
	}

	items, err := chaiStore.GetAllBlockedHashes_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllBlockedHashes_Chai failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Hash != "abc123" {
		t.Errorf("expected hash 'abc123', got %q", items[0].Hash)
	}
	if items[0].Reason != "duplicate file" {
		t.Errorf("expected reason 'duplicate file', got %q", items[0].Reason)
	}
}

// TestGetAllBlockedHashes_Chai_MultipleHashes validates retrieval of multiple hashes
func TestGetAllBlockedHashes_Chai_MultipleHashes(t *testing.T) {
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

	now := time.Now().UTC()
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO blocked_hashes (hash, reason, created_at) VALUES
			('hash1', 'reason one', ?),
			('hash2', 'reason two', ?),
			('hash3', 'corrupted audio', ?)
	`, now, now, now)
	if err != nil {
		t.Fatalf("failed to insert test hashes: %v", err)
	}

	items, err := chaiStore.GetAllBlockedHashes_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllBlockedHashes_Chai failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Build a map for order-independent comparison
	byHash := make(map[string]DoNotImport)
	for _, item := range items {
		byHash[item.Hash] = item
	}

	expected := map[string]string{
		"hash1": "reason one",
		"hash2": "reason two",
		"hash3": "corrupted audio",
	}

	for hash, reason := range expected {
		item, ok := byHash[hash]
		if !ok {
			t.Errorf("expected hash %q in results", hash)
			continue
		}
		if item.Reason != reason {
			t.Errorf("hash %q: expected reason %q, got %q", hash, reason, item.Reason)
		}
	}
}

// TestGetAllBlockedHashes_Chai_NullFields validates NULL reason/created_at handled gracefully
func TestGetAllBlockedHashes_Chai_NullFields(t *testing.T) {
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

	// Insert with NULL reason and NULL created_at
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO blocked_hashes (hash, reason, created_at) VALUES ('nullhash', NULL, NULL)
	`)
	if err != nil {
		t.Fatalf("failed to insert hash with NULL fields: %v", err)
	}

	items, err := chaiStore.GetAllBlockedHashes_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllBlockedHashes_Chai failed with NULL fields: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Hash != "nullhash" {
		t.Errorf("expected hash 'nullhash', got %q", items[0].Hash)
	}
	// Reason should be empty string when NULL
	if items[0].Reason != "" {
		t.Errorf("expected empty reason for NULL, got %q", items[0].Reason)
	}
	// CreatedAt should be zero time when NULL
	if !items[0].CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt for NULL, got %v", items[0].CreatedAt)
	}
}

// TestPebbleStore_GetAllBlockedHashes_FeatureFlag validates feature flag routing
func TestPebbleStore_GetAllBlockedHashes_FeatureFlag(t *testing.T) {
	tmpDir := t.TempDir()

	pebblePath := filepath.Join(tmpDir, "pebble.db")
	store, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer store.Close()

	// Feature flag off (default) — should use Pebble path and return empty
	store.UseChaiDB = false
	items, err := store.GetAllBlockedHashes()
	if err != nil {
		t.Fatalf("GetAllBlockedHashes (Pebble path) failed: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on empty Pebble DB, got %d", len(items))
	}

	// Add a hash via Pebble directly
	if err := store.AddBlockedHash("pebblehash", "test reason"); err != nil {
		t.Fatalf("AddBlockedHash failed: %v", err)
	}

	// Verify it's returned via Pebble path
	items, err = store.GetAllBlockedHashes()
	if err != nil {
		t.Fatalf("GetAllBlockedHashes (Pebble) after add failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after add, got %d", len(items))
	}
	if items[0].Hash != "pebblehash" {
		t.Errorf("expected hash 'pebblehash', got %q", items[0].Hash)
	}
}
