// file: internal/server/itunes_writeback_batcher.go
// version: 3.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e90
//
// Combined write-back batcher: handles location updates, track additions,
// and track removals in a single ITL read-modify-write cycle.

package server

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// WriteBackBatcher collects ITL operations and flushes them in a single batch
// after a debounce delay. Supports location updates, track additions, and
// track removals — all applied in one read-modify-write cycle.
type WriteBackBatcher struct {
	mu             sync.Mutex
	pendingBooks   map[string]bool          // book IDs for location updates
	pendingAdds    []itunes.ITLNewTrack     // tracks to add
	pendingRemoves map[string]bool          // PIDs to remove (lowercase hex)
	timer          *time.Timer
	delay          time.Duration
	stopCh         chan struct{}
	stopped        bool
}

// GlobalWriteBackBatcher is the singleton batcher instance.
var GlobalWriteBackBatcher *WriteBackBatcher

// NewWriteBackBatcher creates a batcher with the given debounce delay.
func NewWriteBackBatcher(delay time.Duration) *WriteBackBatcher {
	return &WriteBackBatcher{
		pendingBooks:   make(map[string]bool),
		pendingRemoves: make(map[string]bool),
		delay:          delay,
		stopCh:         make(chan struct{}),
	}
}

// Enqueue adds a book ID to the pending location-update batch.
func (b *WriteBackBatcher) Enqueue(bookID string) {
	if !config.AppConfig.ITunesAutoWriteBack {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	b.pendingBooks[bookID] = true
	b.resetTimer()
}

// EnqueueAdd queues a new track for insertion into the ITL.
func (b *WriteBackBatcher) EnqueueAdd(track itunes.ITLNewTrack) {
	if !config.AppConfig.ITunesAutoWriteBack {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	b.pendingAdds = append(b.pendingAdds, track)
	b.resetTimer()
}

// EnqueueRemove queues a track PID for removal from the ITL.
// Also marks the PID as removed in the external_id_map.
func (b *WriteBackBatcher) EnqueueRemove(pid string) {
	if !config.AppConfig.ITunesAutoWriteBack {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	b.pendingRemoves[strings.ToLower(pid)] = true
	b.resetTimer()

	// Mark as removed in DB (best-effort, outside lock)
	go func() {
		if store := database.GlobalStore; store != nil {
			_ = store.MarkExternalIDRemoved("itunes", pid)
		}
	}()
}

func (b *WriteBackBatcher) resetTimer() {
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = time.AfterFunc(b.delay, b.flush)
}

func (b *WriteBackBatcher) hasPending() bool {
	return len(b.pendingBooks) > 0 || len(b.pendingAdds) > 0 || len(b.pendingRemoves) > 0
}

// flush writes all pending operations to the iTunes ITL binary in one pass.
func (b *WriteBackBatcher) flush() {
	b.mu.Lock()
	if !b.hasPending() {
		b.mu.Unlock()
		return
	}
	// Snapshot and clear pending state
	bookIDs := make([]string, 0, len(b.pendingBooks))
	for id := range b.pendingBooks {
		bookIDs = append(bookIDs, id)
	}
	adds := b.pendingAdds
	removes := make(map[string]bool, len(b.pendingRemoves))
	for pid := range b.pendingRemoves {
		removes[pid] = true
	}
	b.pendingBooks = make(map[string]bool)
	b.pendingAdds = nil
	b.pendingRemoves = make(map[string]bool)
	b.mu.Unlock()

	store := database.GlobalStore
	if store == nil {
		log.Printf("[WARN] iTunes write-back: no database store")
		return
	}

	if !config.AppConfig.ITLWriteBackEnabled || config.AppConfig.ITunesLibraryWritePath == "" {
		log.Printf("[WARN] iTunes write-back: ITL write-back not configured")
		return
	}

	// Build location updates from book IDs
	var locationUpdates []itunes.ITLLocationUpdate
	for _, id := range bookIDs {
		book, err := store.GetBookByID(id)
		if err != nil || book == nil {
			continue
		}
		files, _ := store.GetBookFiles(id)
		if len(files) > 0 {
			for _, f := range files {
				if f.ITunesPersistentID != "" && f.ITunesPath != "" {
					locationUpdates = append(locationUpdates, itunes.ITLLocationUpdate{
						PersistentID: f.ITunesPersistentID,
						NewLocation:  f.ITunesPath,
					})
				}
			}
		} else if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" &&
			book.ITunesPath != nil && *book.ITunesPath != "" {
			locationUpdates = append(locationUpdates, itunes.ITLLocationUpdate{
				PersistentID: *book.ITunesPersistentID,
				NewLocation:  *book.ITunesPath,
			})
		}
	}

	ops := itunes.ITLOperationSet{
		Removes:         removes,
		Adds:            adds,
		LocationUpdates: locationUpdates,
	}

	if ops.IsEmpty() {
		return
	}

	log.Printf("[INFO] iTunes write-back: flushing %d location updates, %d adds, %d removes",
		len(locationUpdates), len(adds), len(removes))

	itlPath := config.AppConfig.ITunesLibraryWritePath
	result, err := itunes.ApplyITLOperations(itlPath, itlPath+".tmp", ops)
	if err != nil {
		log.Printf("[WARN] iTunes write-back failed: %v", err)
		return
	}

	if renameErr := renameFile(itlPath+".tmp", itlPath); renameErr != nil {
		log.Printf("[WARN] iTunes write-back rename failed: %v", renameErr)
	} else {
		log.Printf("[INFO] iTunes write-back: %d operations applied", result.UpdatedCount)
	}
}

// renameFile is a helper for os.Rename.
var renameFile = os.Rename

// Stop flushes any pending writes and stops the batcher.
func (b *WriteBackBatcher) Stop() {
	b.mu.Lock()
	b.stopped = true
	if b.timer != nil {
		b.timer.Stop()
	}
	b.mu.Unlock()
	b.flush()
}

// InitWriteBackBatcher initializes the global write-back batcher.
func InitWriteBackBatcher() {
	GlobalWriteBackBatcher = NewWriteBackBatcher(5 * time.Second)
}
