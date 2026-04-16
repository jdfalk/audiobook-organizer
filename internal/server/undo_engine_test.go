// file: internal/server/undo_engine_test.go
// version: 1.0.0
// guid: 1c9d0e7f-2a8b-4a70-b8c5-3d7e0f1b9a99

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestUndo_FileMove(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "original", "Book.m4b")
	newPath := filepath.Join(dir, "organized", "Book.m4b")

	writeTestFile(t, newPath, "audio-content")

	if cerr := store.CreateOperationChange(&database.OperationChange{
		ID: "c1", OperationID: "op1", BookID: "b1",
		ChangeType: "file_move",
		OldValue:   oldPath,
		NewValue:   newPath,
	}); cerr != nil {
		t.Fatalf("create change: %v", cerr)
	}

	result, err := RunUndoOperation(store, "op1", nil)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if result.Reverted != 1 {
		t.Errorf("reverted = %d, want 1", result.Reverted)
	}

	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("old path should exist after undo: %v", err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("new path should not exist after undo")
	}
	if got := readTestFile(t, oldPath); got != "audio-content" {
		t.Errorf("content = %q", got)
	}
}

func TestUndo_ConflictDetected(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "original", "Book.m4b")
	newPath := filepath.Join(dir, "organized", "Book.m4b")

	writeTestFile(t, oldPath, "something-new-at-old-location")
	writeTestFile(t, newPath, "audio-content")

	_ = store.CreateOperationChange(&database.OperationChange{
		ID: "c1", OperationID: "op1", BookID: "b1",
		ChangeType: "file_move",
		OldValue:   oldPath,
		NewValue:   newPath,
	})

	result, err := RunUndoOperation(store, "op1", nil)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("failed = %d, want 1 (conflict)", result.Failed)
	}
	if len(result.Errors) == 0 {
		t.Error("expected error message about conflict")
	}
}

func TestUndo_SkipsAlreadyReverted(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	dir := t.TempDir()
	oldPath := filepath.Join(dir, "original", "Book.m4b")
	newPath := filepath.Join(dir, "organized", "Book.m4b")
	writeTestFile(t, newPath, "content")

	_ = store.CreateOperationChange(&database.OperationChange{
		ID: "c1", OperationID: "op1", BookID: "b1",
		ChangeType: "file_move",
		OldValue:   oldPath,
		NewValue:   newPath,
	})

	// First undo reverts the change.
	result, _ := RunUndoOperation(store, "op1", nil)
	if result.Reverted != 1 {
		t.Fatalf("first undo: reverted = %d", result.Reverted)
	}

	// Second undo should skip because RevertedAt is set.
	result, _ = RunUndoOperation(store, "op1", nil)
	if result.SkippedReverted != 1 {
		t.Errorf("second undo: skipped_reverted = %d, want 1", result.SkippedReverted)
	}
}

func TestUndo_EmptyChanges(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	result, err := RunUndoOperation(store, "nonexistent-op", nil)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if result.Reverted != 0 {
		t.Errorf("reverted = %d, want 0", result.Reverted)
	}
}

func TestUndo_MetadataUpdate(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	book, _ := store.CreateBook(&database.Book{
		Title: "New Title", FilePath: "/tmp/book", Format: "m4b",
	})

	_ = store.CreateOperationChange(&database.OperationChange{
		ID: "c1", OperationID: "op1", BookID: book.ID,
		ChangeType: "metadata_update",
		FieldName:  "title",
		OldValue:   "Original Title",
		NewValue:   "New Title",
	})

	result, err := RunUndoOperation(store, "op1", nil)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if result.Reverted != 1 {
		t.Errorf("reverted = %d, want 1", result.Reverted)
	}

	restored, _ := store.GetBookByID(book.ID)
	if restored.Title != "Original Title" {
		t.Errorf("title = %q, want Original Title", restored.Title)
	}
}

func TestUndo_DirCreateRemovesEmpty(t *testing.T) {
	store, err := database.NewPebbleStore(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	dir := filepath.Join(t.TempDir(), "new-dir")
	if err := os.MkdirAll(dir, 0o775); err != nil {
		t.Fatal(err)
	}

	_ = store.CreateOperationChange(&database.OperationChange{
		ID: "c1", OperationID: "op1", BookID: "b1",
		ChangeType: "dir_create",
		NewValue:   dir,
	})

	result, _ := RunUndoOperation(store, "op1", nil)
	if result.Reverted != 1 {
		t.Errorf("reverted = %d, want 1", result.Reverted)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("empty dir should be removed")
	}
}
