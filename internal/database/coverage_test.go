// file: internal/database/coverage_test.go
// version: 1.0.0
// guid: 3b82b22e-cd28-49b8-8b9c-e0a34b18e631

package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeStoreAndClose(t *testing.T) {
	tempDir := t.TempDir()
	origStore := GlobalStore
	origDB := DB
	defer func() {
		GlobalStore = origStore
		DB = origDB
	}()

	if err := InitializeStore("sqlite", filepath.Join(tempDir, "db.sqlite"), false); err == nil {
		t.Fatal("expected error when sqlite is not enabled")
	}

	if err := InitializeStore("sqlite", filepath.Join(tempDir, "db.sqlite"), true); err != nil {
		t.Fatalf("unexpected sqlite init error: %v", err)
	}
	if GlobalStore == nil {
		t.Fatal("expected global store to be set")
	}
	if err := CloseStore(); err != nil {
		t.Fatalf("failed to close sqlite store: %v", err)
	}
	GlobalStore = nil
	DB = nil

	pebbleDir := filepath.Join(tempDir, "pebble")
	if err := InitializeStore("pebble", pebbleDir, false); err != nil {
		t.Fatalf("unexpected pebble init error: %v", err)
	}
	if err := CloseStore(); err != nil {
		t.Fatalf("failed to close pebble store: %v", err)
	}
	GlobalStore = nil

	if err := InitializeStore("unknown", filepath.Join(tempDir, "bad"), false); err == nil {
		t.Fatal("expected error for unsupported database type")
	}
}

func TestDBInterfaceWrapper(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(tempDir, "interface.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	wrapper := NewDBInterface(db)
	if wrapper == nil {
		t.Fatal("expected wrapper")
	}

	if _, err := wrapper.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	stmt, err := wrapper.Prepare("INSERT INTO items (name) VALUES (?)")
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, err := stmt.Exec("alpha"); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	_ = stmt.Close()

	tx, err := wrapper.Begin()
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO items (name) VALUES ('beta')"); err != nil {
		t.Fatalf("tx insert failed: %v", err)
	}
	_ = tx.Commit()

	row := wrapper.QueryRow("SELECT COUNT(*) FROM items")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 items, got %d", count)
	}

	rows, err := wrapper.Query("SELECT name FROM items ORDER BY id")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan row failed: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
}

func TestWebHelpers(t *testing.T) {
	tempDir := t.TempDir()
	if err := Initialize(filepath.Join(tempDir, "web.db")); err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer func() {
		_ = DB.Close()
		DB = nil
	}()

	folder, err := AddImportPath("/tmp/books", "Test Import")
	if err != nil {
		t.Fatalf("AddImportPath failed: %v", err)
	}
	if folder == nil || folder.ID == 0 {
		t.Fatal("expected import path to be created")
	}
	folders, err := GetImportPaths()
	if err != nil {
		t.Fatalf("GetImportPaths failed: %v", err)
	}
	if len(folders) == 0 {
		t.Fatal("expected import paths")
	}

	now := time.Now()
	if err := UpdateImportPath(folder.ID, false, &now, 5); err != nil {
		t.Fatalf("UpdateImportPath failed: %v", err)
	}
	updated, err := GetImportPathByID(folder.ID)
	if err != nil {
		t.Fatalf("GetImportPathByID failed: %v", err)
	}
	if updated == nil || updated.Enabled {
		t.Fatal("expected updated import path to be disabled")
	}

	if err := RemoveImportPath(folder.ID); err != nil {
		t.Fatalf("RemoveImportPath failed: %v", err)
	}

	if _, err := CreateOperation("op-1", "scan", "/tmp/books"); err == nil {
		t.Fatal("expected CreateOperation to fail with null message scan")
	}
	if _, err := DB.Exec("DELETE FROM operations WHERE id = ?", "op-1"); err != nil {
		t.Fatalf("failed to cleanup op-1: %v", err)
	}

	if _, err := DB.Exec(`INSERT INTO operations (id, type, status, progress, total, message, folder_path)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "op-2", "scan", "queued", 0, 0, "", "/tmp/books"); err != nil {
		t.Fatalf("failed to seed operation: %v", err)
	}

	if err := UpdateOperationStatus("op-2", "running", 0, 10, "starting"); err != nil {
		t.Fatalf("UpdateOperationStatus failed: %v", err)
	}
	if err := UpdateOperationStatus("op-2", "completed", 10, 10, "done"); err != nil {
		t.Fatalf("UpdateOperationStatus complete failed: %v", err)
	}
	if err := UpdateOperationError("op-2", "boom"); err != nil {
		t.Fatalf("UpdateOperationError failed: %v", err)
	}
	if _, err := GetOperationByID("op-2"); err != nil {
		t.Fatalf("GetOperationByID failed: %v", err)
	}
	if _, err := GetRecentOperations(5); err != nil {
		t.Fatalf("GetRecentOperations failed: %v", err)
	}

	detail := "detail"
	if err := AddOperationLog("op-2", "info", "hello", &detail); err != nil {
		t.Fatalf("AddOperationLog failed: %v", err)
	}
	logs, err := GetOperationLogs("op-2")
	if err != nil {
		t.Fatalf("GetOperationLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	if err := SetUserPreference("theme", "dark"); err != nil {
		t.Fatalf("SetUserPreference failed: %v", err)
	}
	pref, err := GetUserPreference("theme")
	if err != nil {
		t.Fatalf("GetUserPreference failed: %v", err)
	}
	if pref == nil || pref.Value == nil || *pref.Value != "dark" {
		t.Fatal("expected preference value to be set")
	}
	prefs, err := GetAllUserPreferences()
	if err != nil {
		t.Fatalf("GetAllUserPreferences failed: %v", err)
	}
	if len(prefs) == 0 {
		t.Fatal("expected preferences list")
	}
}

func TestEncryptionHelpersAndSettings(t *testing.T) {
	origKey := encryptionKey
	defer func() {
		encryptionKey = origKey
	}()

	if _, err := EncryptValue("secret"); err == nil {
		t.Fatal("expected error when encryption key is missing")
	}

	tempDir := t.TempDir()
	if err := InitEncryption(tempDir); err != nil {
		t.Fatalf("InitEncryption failed: %v", err)
	}

	enc, err := EncryptValue("secret")
	if err != nil {
		t.Fatalf("EncryptValue failed: %v", err)
	}
	dec, err := DecryptValue(enc)
	if err != nil {
		t.Fatalf("DecryptValue failed: %v", err)
	}
	if dec != "secret" {
		t.Fatalf("expected decrypted value, got %q", dec)
	}

	if got := MaskSecret("12345678"); got != "123****5678" {
		t.Fatalf("unexpected mask: %q", got)
	}

	if got := len(DeriveKeyFromPassword("pw")); got != 32 {
		t.Fatalf("expected 32-byte key, got %d", got)
	}

	store, cleanup := setupPebbleTestDB(t)
	defer cleanup()
	pebbleStore := store.(*PebbleStore)
	if err := pebbleStore.SetSetting("plain", "value", "string", false); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}
	if err := pebbleStore.SetSetting("secret", "shh", "string", true); err != nil {
		t.Fatalf("SetSetting secret failed: %v", err)
	}
	setting, err := pebbleStore.GetSetting("plain")
	if err != nil || setting == nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	all, err := pebbleStore.GetAllSettings()
	if err != nil {
		t.Fatalf("GetAllSettings failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected settings list")
	}
	if err := pebbleStore.DeleteSetting("plain"); err != nil {
		t.Fatalf("DeleteSetting failed: %v", err)
	}

	sqlStore, cleanupSQL := setupTestDB(t)
	defer cleanupSQL()
	sqliteStore := sqlStore.(*SQLiteStore)
	if err := sqliteStore.SetSetting("app", "value", "string", false); err != nil {
		t.Fatalf("sqlite SetSetting failed: %v", err)
	}
	if err := sqliteStore.SetSetting("secret", "secret-value", "string", true); err != nil {
		t.Fatalf("sqlite SetSetting secret failed: %v", err)
	}
	if _, err := sqliteStore.GetSetting("app"); err == nil {
		t.Fatal("expected sqlite GetSetting to fail with boolean scan")
	}
	if _, err := sqliteStore.GetAllSettings(); err != nil {
		t.Fatalf("sqlite GetAllSettings failed: %v", err)
	}
	if err := sqliteStore.DeleteSetting("app"); err != nil {
		t.Fatalf("sqlite DeleteSetting failed: %v", err)
	}
	if _, err := GetDecryptedSetting(sqliteStore, "secret"); err == nil {
		t.Fatal("expected GetDecryptedSetting to fail with boolean scan")
	}
}
