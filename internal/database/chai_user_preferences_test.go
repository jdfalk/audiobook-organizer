// file: internal/database/chai_user_preferences_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-43f4-a5b6-c7d8e9f0a1b2
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
)

// TestGetAllUserPreferences_Chai validates that the SQL implementation reads back
// key-value rows from the user_preferences table.
func TestGetAllUserPreferences_Chai(t *testing.T) {
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

	// Insert test rows
	_, err = chaiDB.ExecContext(ctx, `
		INSERT INTO user_preferences (key, value) VALUES
			('theme', 'dark'),
			('language', 'en'),
			('volume', '80')
	`)
	if err != nil {
		t.Fatalf("failed to insert test preferences: %v", err)
	}

	prefs, err := chaiStore.GetAllUserPreferences_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllUserPreferences_Chai failed: %v", err)
	}

	expected := map[string]string{
		"theme":    "dark",
		"language": "en",
		"volume":   "80",
	}

	if len(prefs) != len(expected) {
		t.Errorf("expected %d preferences, got %d", len(expected), len(prefs))
	}

	for k, wantVal := range expected {
		gotVal, ok := prefs[k]
		if !ok {
			t.Errorf("preference %q missing from result", k)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("preference %q: want %q, got %q", k, wantVal, gotVal)
		}
	}
}

// TestGetAllUserPreferences_Chai_Empty validates that an empty table returns an empty map.
func TestGetAllUserPreferences_Chai_Empty(t *testing.T) {
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

	prefs, err := chaiStore.GetAllUserPreferences_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllUserPreferences_Chai on empty table failed: %v", err)
	}

	if len(prefs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(prefs))
	}
}

// TestGetAllUserPreferences_Chai_NullValue validates that NULL values are returned as empty strings.
func TestGetAllUserPreferences_Chai_NullValue(t *testing.T) {
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

	_, err = chaiDB.ExecContext(ctx, `INSERT INTO user_preferences (key, value) VALUES ('nullpref', NULL)`)
	if err != nil {
		t.Fatalf("failed to insert NULL preference: %v", err)
	}

	prefs, err := chaiStore.GetAllUserPreferences_Chai(ctx)
	if err != nil {
		t.Fatalf("GetAllUserPreferences_Chai with NULL value failed: %v", err)
	}

	val, ok := prefs["nullpref"]
	if !ok {
		t.Fatal("expected 'nullpref' key in result")
	}
	if val != "" {
		t.Errorf("expected empty string for NULL value, got %q", val)
	}
}

// TestGetAllUserPreferences_PebbleRouting validates that PebbleStore.GetAllUserPreferences
// falls through to Pebble iteration when UseChaiDB is false.
func TestGetAllUserPreferences_PebbleRouting(t *testing.T) {
	tmpDir := t.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	store, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer store.Close()

	// UseChaiDB defaults to false; should use Pebble path
	if store.UseChaiDB {
		t.Fatal("UseChaiDB should default to false")
	}

	prefs, err := store.GetAllUserPreferences()
	if err != nil {
		t.Fatalf("GetAllUserPreferences on empty Pebble DB failed: %v", err)
	}

	if len(prefs) != 0 {
		t.Errorf("expected empty result from empty Pebble DB, got %d entries", len(prefs))
	}
}

// TestGetAllUserPreferences_Pebble validates the _Pebble variant directly.
func TestGetAllUserPreferences_Pebble(t *testing.T) {
	tmpDir := t.TempDir()
	pebblePath := filepath.Join(tmpDir, "pebble.db")

	store, err := NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("NewPebbleStore failed: %v", err)
	}
	defer store.Close()

	// Write a preference the Pebble way
	if err := store.SetUserPreference("color_scheme", "light"); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}

	prefs, err := store.GetAllUserPreferences_Pebble()
	if err != nil {
		t.Fatalf("GetAllUserPreferences_Pebble failed: %v", err)
	}

	if len(prefs) != 1 {
		t.Fatalf("expected 1 preference, got %d", len(prefs))
	}

	if prefs[0].Key != "color_scheme" {
		t.Errorf("expected key 'color_scheme', got %q", prefs[0].Key)
	}

	if prefs[0].Value == nil || *prefs[0].Value != "light" {
		val := "<nil>"
		if prefs[0].Value != nil {
			val = *prefs[0].Value
		}
		t.Errorf("expected value 'light', got %q", val)
	}
}
