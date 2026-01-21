// file: internal/database/migrations_extra_test.go
// version: 1.0.0
// guid: 67d3f1c5-8c24-4a3c-9a79-35fb6d68fdd9

package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGetCurrentVersionWithPreference(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	version := DatabaseVersion{Version: 3, UpdatedAt: time.Now()}
	data, err := json.Marshal(version)
	if err != nil {
		t.Fatalf("marshal version failed: %v", err)
	}
	if err := store.SetUserPreference("db_version", string(data)); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	got, err := getCurrentVersion(store)
	if err != nil {
		t.Fatalf("getCurrentVersion failed: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected version 3, got %d", got)
	}
}

func TestGetCurrentVersionInvalidPreference(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if err := store.SetUserPreference("db_version", "not-json"); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	if _, err := getCurrentVersion(store); err == nil {
		t.Fatal("expected getCurrentVersion to fail on invalid JSON")
	}
}

func TestMigration007UpWithLegacyTable(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()
	sqliteStore := store.(*SQLiteStore)

	if _, err := sqliteStore.db.Exec("DROP TABLE IF EXISTS import_paths"); err != nil {
		t.Fatalf("failed to drop import_paths: %v", err)
	}
	if _, err := sqliteStore.db.Exec("DROP INDEX IF EXISTS idx_import_paths_path"); err != nil {
		t.Fatalf("failed to drop idx_import_paths_path: %v", err)
	}
	if _, err := sqliteStore.db.Exec(`
		CREATE TABLE library_folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_scan DATETIME,
			book_count INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		t.Fatalf("failed to create library_folders: %v", err)
	}
	if _, err := sqliteStore.db.Exec("CREATE INDEX idx_library_folders_path ON library_folders(path)"); err != nil {
		t.Fatalf("failed to create legacy index: %v", err)
	}

	if err := migration007Up(store); err != nil {
		t.Fatalf("migration007Up failed: %v", err)
	}
}
