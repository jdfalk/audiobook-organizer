// file: internal/database/mock_store_coverage_test.go
// version: 1.0.0
// guid: 19d1c833-6756-4b2d-8c9c-6657b3f0f0d8

package database

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestMockStoreAdvancedStubs(t *testing.T) {
	store := NewMockStore()

	if _, err := store.CreateUser("user", "user@example.com", "algo", "hash", []string{"user"}, "active"); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if _, err := store.GetUserByID("id"); err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if _, err := store.GetUserByUsername("user"); err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if _, err := store.GetUserByEmail("user@example.com"); err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if err := store.UpdateUser(&User{ID: "id"}); err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	if _, err := store.CreateSession("user", "127.0.0.1", "agent", time.Minute); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if _, err := store.GetSession("sess"); err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if err := store.RevokeSession("sess"); err != nil {
		t.Fatalf("RevokeSession failed: %v", err)
	}
	if _, err := store.ListUserSessions("user"); err != nil {
		t.Fatalf("ListUserSessions failed: %v", err)
	}

	if err := store.SetUserPreferenceForUser("user", "theme", "dark"); err != nil {
		t.Fatalf("SetUserPreferenceForUser failed: %v", err)
	}
	if _, err := store.GetUserPreferenceForUser("user", "theme"); err != nil {
		t.Fatalf("GetUserPreferenceForUser failed: %v", err)
	}
	if _, err := store.GetAllPreferencesForUser("user"); err != nil {
		t.Fatalf("GetAllPreferencesForUser failed: %v", err)
	}

	if _, err := store.CreateBookSegment(1, &BookSegment{FilePath: "/tmp/seg.mp3"}); err != nil {
		t.Fatalf("CreateBookSegment failed: %v", err)
	}
	if _, err := store.ListBookSegments(1); err != nil {
		t.Fatalf("ListBookSegments failed: %v", err)
	}
	if err := store.MergeBookSegments(1, &BookSegment{FilePath: "/tmp/seg.mp3"}, []string{"seg"}); err != nil {
		t.Fatalf("MergeBookSegments failed: %v", err)
	}

	if err := store.AddPlaybackEvent(&PlaybackEvent{UserID: "user"}); err != nil {
		t.Fatalf("AddPlaybackEvent failed: %v", err)
	}
	if _, err := store.ListPlaybackEvents("user", 1, 10); err != nil {
		t.Fatalf("ListPlaybackEvents failed: %v", err)
	}
	if err := store.UpdatePlaybackProgress(&PlaybackProgress{UserID: "user"}); err != nil {
		t.Fatalf("UpdatePlaybackProgress failed: %v", err)
	}
	if _, err := store.GetPlaybackProgress("user", 1); err != nil {
		t.Fatalf("GetPlaybackProgress failed: %v", err)
	}

	if err := store.IncrementBookPlayStats(1, 10); err != nil {
		t.Fatalf("IncrementBookPlayStats failed: %v", err)
	}
	if _, err := store.GetBookStats(1); err != nil {
		t.Fatalf("GetBookStats failed: %v", err)
	}
	if err := store.IncrementUserListenStats("user", 10); err != nil {
		t.Fatalf("IncrementUserListenStats failed: %v", err)
	}
	if _, err := store.GetUserStats("user"); err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}
}

func TestGetDBInterfaceAndClose(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(tempDir, "db.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	origDB := DB
	DB = db
	t.Cleanup(func() {
		DB = origDB
	})

	wrapper := GetDBInterface()
	if wrapper == nil {
		t.Fatal("expected DB interface wrapper")
	}
	if err := wrapper.Close(); err != nil {
		t.Fatalf("wrapper close failed: %v", err)
	}
}

func TestGetMigrationHistory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	record := MigrationRecord{
		Version:     99,
		Description: "test migration",
		AppliedAt:   time.Now(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal record: %v", err)
	}
	if err := store.SetUserPreference("migration_99", string(data)); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}

	history, err := GetMigrationHistory(store)
	if err != nil {
		t.Fatalf("GetMigrationHistory failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected migration history entries")
	}
}
