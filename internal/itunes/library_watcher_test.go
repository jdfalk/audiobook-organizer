// file: internal/itunes/library_watcher_test.go
// version: 1.0.0
// guid: f0a1b2c3-d4e5-6f7a-8b9c-0d1e2f3a4b5c

package itunes

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLibraryWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Library.xml")
	if err := os.WriteFile(path, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewLibraryWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if w.HasChanged() {
		t.Error("should not report changed before any modification")
	}

	// Modify the file
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for fsnotify to fire
	time.Sleep(500 * time.Millisecond)

	if !w.HasChanged() {
		t.Error("should report changed after modification")
	}
	if w.ChangedAt().IsZero() {
		t.Error("ChangedAt should be set")
	}

	// Reset
	w.ClearChanged()
	if w.HasChanged() {
		t.Error("should not report changed after clear")
	}
	if !w.ChangedAt().IsZero() {
		t.Error("ChangedAt should be zero after clear")
	}
}

func TestLibraryWatcher_NonexistentFile(t *testing.T) {
	_, err := NewLibraryWatcher("/nonexistent/path/Library.xml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
