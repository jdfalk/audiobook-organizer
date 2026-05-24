// file: internal/database/chai_user_preferences_test.go
// version: 1.0.0
// guid: f7a8b9c0-d1e2-43f4-a5b6-c7d8e9f0a1b2
// last-edited: 2026-05-24

package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestGetAllPreferencesForUser_Chai validates the Chai SQL implementation
// that replaces the Pebble prefix-scan for per-user preference retrieval.
func TestGetAllPreferencesForUser_Chai(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Second)

	// Insert preferences for two users; only user-1's rows should be returned.
	rows := []struct {
		userID  string
		key     string
		value   string
		version int
	}{
		{"user-1", "theme", "dark", 2},
		{"user-1", "pageSize", "50", 1},
		{"user-2", "theme", "light", 1},
	}
	for _, r := range rows {
		_, err := chaiDB.ExecContext(ctx,
			"INSERT INTO user_preferences (user_id, key, value, updated_at, version) VALUES (?, ?, ?, ?, ?)",
			r.userID, r.key, r.value, now, r.version,
		)
		if err != nil {
			t.Fatalf("insert failed (%s/%s): %v", r.userID, r.key, err)
		}
	}

	t.Run("returns all prefs for user-1", func(t *testing.T) {
		prefs, err := chaiStore.GetAllPreferencesForUser_Chai(ctx, "user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prefs) != 2 {
			t.Fatalf("expected 2 prefs, got %d", len(prefs))
		}
		// Build map for stable assertion order.
		byKey := make(map[string]UserPreferenceKV, len(prefs))
		for _, p := range prefs {
			byKey[p.Key] = p
		}
		if byKey["theme"].Value != "dark" {
			t.Errorf("theme: want 'dark', got %q", byKey["theme"].Value)
		}
		if byKey["pageSize"].Value != "50" {
			t.Errorf("pageSize: want '50', got %q", byKey["pageSize"].Value)
		}
		if byKey["theme"].UserID != "user-1" {
			t.Errorf("UserID: want 'user-1', got %q", byKey["theme"].UserID)
		}
		if byKey["theme"].Version != 2 {
			t.Errorf("theme version: want 2, got %d", byKey["theme"].Version)
		}
	})

	t.Run("does not return other user's prefs", func(t *testing.T) {
		prefs, err := chaiStore.GetAllPreferencesForUser_Chai(ctx, "user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, p := range prefs {
			if p.UserID != "user-1" {
				t.Errorf("got pref for wrong user: %q", p.UserID)
			}
		}
	})

	t.Run("returns empty slice for unknown user", func(t *testing.T) {
		prefs, err := chaiStore.GetAllPreferencesForUser_Chai(ctx, "no-such-user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prefs == nil {
			t.Error("expected non-nil empty slice, got nil")
		}
		if len(prefs) != 0 {
			t.Errorf("expected 0 prefs, got %d", len(prefs))
		}
	})

	t.Run("escapes SQL injection in userID", func(t *testing.T) {
		// A malicious userID containing a single quote must not cause an error
		// or leak rows from other users.
		prefs, err := chaiStore.GetAllPreferencesForUser_Chai(ctx, "bad' OR '1'='1")
		if err != nil {
			t.Fatalf("unexpected error on injection attempt: %v", err)
		}
		if len(prefs) != 0 {
			t.Errorf("injection returned %d rows, expected 0", len(prefs))
		}
	})
}

// TestGetAllPreferencesForUser_PebbleRouting validates that PebbleStore.GetAllPreferencesForUser
// falls through to the Pebble implementation when UseChaiDB is false (the default).
func TestGetAllPreferencesForUser_PebbleRouting(t *testing.T) {
	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()

	ps, ok := store.(*PebbleStore)
	if !ok {
		t.Fatal("store is not *PebbleStore")
	}

	// Seed a preference via the existing SetUserPreferenceForUser.
	if err := ps.SetUserPreferenceForUser("user-A", "myKey", "myValue"); err != nil {
		t.Fatalf("SetUserPreferenceForUser failed: %v", err)
	}

	// UseChaiDB defaults to false — should use Pebble path.
	if ps.UseChaiDB {
		t.Fatal("expected UseChaiDB=false for this test")
	}

	prefs, err := ps.GetAllPreferencesForUser("user-A")
	if err != nil {
		t.Fatalf("GetAllPreferencesForUser failed: %v", err)
	}
	if len(prefs) != 1 {
		t.Fatalf("expected 1 pref, got %d", len(prefs))
	}
	if prefs[0].Key != "myKey" {
		t.Errorf("key: want 'myKey', got %q", prefs[0].Key)
	}
	if prefs[0].Value != "myValue" {
		t.Errorf("value: want 'myValue', got %q", prefs[0].Value)
	}
}
