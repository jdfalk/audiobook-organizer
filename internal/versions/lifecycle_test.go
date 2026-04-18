// file: internal/versions/lifecycle_test.go
// version: 1.0.0

package versions

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestAutoPromoteAlt(t *testing.T) {
	pebblePath := filepath.Join(t.TempDir(), "pebble")
	store, err := database.NewPebbleStore(pebblePath)
	if err != nil {
		t.Fatalf("open pebble: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	_, _ = store.CreateBook(&database.Book{ID: "b1", Title: "Test Book", FilePath: "/tmp/b1"})

	// Create two alt versions with different ingest dates.
	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)

	v1, _ := store.CreateBookVersion(&database.BookVersion{
		BookID: "b1", Status: database.BookVersionStatusAlt, Format: "mp3",
		Source: "imported", IngestDate: older,
	})
	v2, _ := store.CreateBookVersion(&database.BookVersion{
		BookID: "b1", Status: database.BookVersionStatusAlt, Format: "m4b",
		Source: "imported", IngestDate: newer,
	})

	if err := AutoPromoteAlt(store, "b1"); err != nil {
		t.Fatalf("auto-promote: %v", err)
	}

	got1, _ := store.GetBookVersion(v1.ID)
	got2, _ := store.GetBookVersion(v2.ID)

	// The newer alt should become active.
	if got2.Status != database.BookVersionStatusActive {
		t.Errorf("newer version status = %s, want active", got2.Status)
	}
	if got1.Status != database.BookVersionStatusAlt {
		t.Errorf("older version status = %s, want alt", got1.Status)
	}
}
