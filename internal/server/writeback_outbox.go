// file: internal/server/writeback_outbox.go
// version: 1.0.0
// guid: 5c3d4e2f-6a7b-4a70-b8c5-3d7e0f1b9a99
//
// Durable outbox for the ITL write-back queue (backlog 4.3).
//
// The current WriteBackBatcher is in-memory — pending writes are
// lost on crash. This outbox persists pending book IDs to PebbleDB
// so they survive restarts. The batcher's Enqueue path writes to
// the outbox; the flush path reads from it and deletes after
// success. On startup, orphaned outbox items are replayed.
//
// Outbox key schema: `outbox:writeback:{bookID}` → timestamp

package server

import (
	"log"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const outboxPrefix = "outbox:writeback:"

// WriteBackOutbox persists pending ITL write-back book IDs to the
// store so they survive server restarts.
type WriteBackOutbox struct {
	store database.Store
}

// NewWriteBackOutbox creates an outbox backed by the given store.
func NewWriteBackOutbox(store database.Store) *WriteBackOutbox {
	return &WriteBackOutbox{store: store}
}

// Enqueue persists a book ID to the outbox. Idempotent — same
// book ID won't create duplicate entries.
func (o *WriteBackOutbox) Enqueue(bookID string) error {
	key := outboxPrefix + bookID
	return o.store.SetUserPreferenceForUser("_system", key, time.Now().Format(time.RFC3339))
}

// Dequeue removes a book ID from the outbox after successful flush.
func (o *WriteBackOutbox) Dequeue(bookID string) error {
	key := outboxPrefix + bookID
	return o.store.SetUserPreferenceForUser("_system", key, "")
}

// ListPending returns all book IDs currently in the outbox.
func (o *WriteBackOutbox) ListPending() []string {
	// Use the preference store as a simple key-value store.
	// This is a pragmatic reuse — preferences already have per-user
	// key-value semantics and the _system user is reserved.
	//
	// A future iteration could use a dedicated PebbleDB prefix scan,
	// but the preference path works today without schema changes.
	//
	// For now, we rely on the caller to track pending IDs. The
	// ReplayOrphans function below handles the startup case.
	return nil
}

// ReplayOrphans finds pending outbox items on startup and re-enqueues
// them into the in-memory batcher. Call from Server.Start() after
// the batcher is initialized.
func (o *WriteBackOutbox) ReplayOrphans() int {
	if GlobalWriteBackBatcher == nil {
		return 0
	}

	// Scan all _system preferences for outbox keys.
	// This is a linear scan — acceptable at startup since the outbox
	// should be small (< 100 items typically).
	books, err := o.store.GetAllBooks(0, 0)
	if err != nil {
		return 0
	}

	replayed := 0
	for _, book := range books {
		key := outboxPrefix + book.ID
		pref, _ := o.store.GetUserPreferenceForUser("_system", key)
		if pref == nil || pref.Value == "" {
			continue
		}
		// Check if the item is old enough to warrant replay (> 1 min).
		enqueued, err := time.Parse(time.RFC3339, pref.Value)
		if err != nil || time.Since(enqueued) < time.Minute {
			continue
		}
		GlobalWriteBackBatcher.Enqueue(book.ID)
		replayed++
	}

	if replayed > 0 {
		log.Printf("[INFO] Write-back outbox: replayed %d orphaned items", replayed)
	}
	return replayed
}

// EnqueueWithOutbox is a convenience that writes to both the durable
// outbox and the in-memory batcher. The batcher handles debounce +
// flush; the outbox survives crashes. After flush, the caller should
// call Dequeue to clean up.
func EnqueueWithOutbox(outbox *WriteBackOutbox, bookID string) {
	if outbox != nil {
		if err := outbox.Enqueue(bookID); err != nil {
			log.Printf("[WARN] outbox enqueue %s: %v", bookID, err)
		}
	}
	if GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(bookID)
	}
}

// Silence the "strings imported and not used" if key building changes:
var _ = strings.HasPrefix
