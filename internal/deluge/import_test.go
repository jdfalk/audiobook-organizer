// file: internal/deluge/import_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678902
// last-edited: 2026-05-11
//
// Tests for ImportToLibrary in internal/deluge/import.go.

package deluge

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/config"
	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// fakeDelugStore is a minimal database.Store for import tests.
type fakeDelugStore struct {
	database.Store
	updated *database.BookFile
}

func (f *fakeDelugStore) UpdateBookFile(id string, file *database.BookFile) error {
	f.updated = file
	return nil
}

func TestImportToLibrary_BasicCopy(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootDir := t.TempDir()
	cfg := &config.Config{RootDir: rootDir, DelugeMoveEnabled: false}
	store := &fakeDelugStore{}
	bf := &database.BookFile{ID: "id-001", FilePath: srcFile}

	newPath, err := ImportToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("ImportToLibrary: %v", err)
	}
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Errorf("destination file does not exist: %v", statErr)
	}
	if store.updated == nil {
		t.Fatal("UpdateBookFile was not called")
	}
	if store.updated.FilePath != newPath {
		t.Errorf("FilePath = %q, want %q", store.updated.FilePath, newPath)
	}
	if store.updated.DelugeOriginalPath != srcFile {
		t.Errorf("DelugeOriginalPath = %q, want %q", store.updated.DelugeOriginalPath, srcFile)
	}
	if store.updated.ImportedFromDelugeAt == nil {
		t.Error("ImportedFromDelugeAt is nil, want non-nil")
	}
}

func TestImportToLibrary_SamePath_NoOp(t *testing.T) {
	rootDir := t.TempDir()
	srcFile := filepath.Join(rootDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{RootDir: rootDir}
	store := &fakeDelugStore{}
	bf := &database.BookFile{ID: "id-002", FilePath: srcFile}

	newPath, err := ImportToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("ImportToLibrary: %v", err)
	}
	if newPath != srcFile {
		t.Errorf("newPath = %q, want %q (same as src)", newPath, srcFile)
	}
	if store.updated != nil {
		t.Error("UpdateBookFile should not be called when src == dest")
	}
}

func TestImportToLibrary_Idempotent(t *testing.T) {
	rootDir := t.TempDir()
	srcFile := filepath.Join(rootDir, "imported.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{RootDir: rootDir}
	store := &fakeDelugStore{}
	importedAt := time.Now().Add(-time.Hour)
	bf := &database.BookFile{ID: "id-003", FilePath: srcFile, ImportedFromDelugeAt: &importedAt}

	newPath, err := ImportToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("ImportToLibrary: %v", err)
	}
	if newPath != srcFile {
		t.Errorf("newPath = %q, want %q (unchanged)", newPath, srcFile)
	}
	if store.updated != nil {
		t.Error("UpdateBookFile should not be called on already-imported file")
	}
}

func TestImportToLibrary_NilBookFile(t *testing.T) {
	cfg := &config.Config{RootDir: t.TempDir()}
	store := &fakeDelugStore{}

	_, err := ImportToLibrary(cfg, nil, store, nil)
	if err == nil {
		t.Error("expected error for nil bookFile")
	}
}

func TestImportToLibrary_MoveStorageNilClient_OK(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootDir := t.TempDir()
	cfg := &config.Config{RootDir: rootDir, DelugeMoveEnabled: true}
	store := &fakeDelugStore{}
	bf := &database.BookFile{ID: "id-004", FilePath: srcFile, DelugeHash: "abc123"}

	// nil client → MoveStorage skipped, but copy still succeeds.
	newPath, err := ImportToLibrary(cfg, nil, store, bf)
	if err != nil {
		t.Fatalf("ImportToLibrary: %v", err)
	}
	if _, statErr := os.Stat(newPath); statErr != nil {
		t.Errorf("destination file does not exist: %v", statErr)
	}
}
