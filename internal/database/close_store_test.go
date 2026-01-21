// file: internal/database/close_store_test.go
// version: 1.0.0
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
