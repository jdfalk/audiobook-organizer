// file: internal/database/close_store_test.go
// version: 1.1.0
// guid: b2d533a6-cd50-4b7e-9b50-b8c0c6b4c9b6

package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCloseStoreWithDBOnly(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(tempDir, "close.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	origStore := GlobalStore
	origDB := DB
	GlobalStore = nil
	DB = db
	defer func() {
		GlobalStore = origStore
		DB = origDB
	}()

	if err := CloseStore(); err != nil {
		t.Fatalf("CloseStore failed: %v", err)
	}
}

// TestDBInterfaceClose tests the Close method of sqlDBWrapper.
func TestDBInterfaceClose(t *testing.T) {
	tempDir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(tempDir, "interface_close.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create DBInterface wrapper
	dbInterface := NewDBInterface(db)
	if dbInterface == nil {
		t.Fatal("NewDBInterface returned nil")
	}

	// Test Close
	if err := dbInterface.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify db is closed by attempting to ping (should fail)
	if err := db.Ping(); err == nil {
		t.Error("expected ping to fail after Close, but it succeeded")
	}
}

// TestGetDBInterface tests the GetDBInterface function.
func TestGetDBInterface(t *testing.T) {
	// Save original DB
	origDB := DB
	defer func() {
		DB = origDB
	}()

	t.Run("returns nil when DB is nil", func(t *testing.T) {
		DB = nil
		dbInterface := GetDBInterface()
		if dbInterface != nil {
			t.Errorf("expected nil interface when DB is nil, got %T", dbInterface)
		}
	})

	t.Run("returns wrapped interface when DB is set", func(t *testing.T) {
		tempDir := t.TempDir()
		db, err := sql.Open("sqlite3", filepath.Join(tempDir, "get_interface.db"))
		if err != nil {
			t.Fatalf("failed to open db: %v", err)
		}
		defer db.Close()

		DB = db
		dbInterface := GetDBInterface()
		if dbInterface == nil {
			t.Fatal("expected non-nil interface when DB is set")
		}

		// Verify it's a functioning DBInterface by calling a method
		row := dbInterface.QueryRow("SELECT 1")
		if row == nil {
			t.Error("QueryRow returned nil")
		}
	})
}
