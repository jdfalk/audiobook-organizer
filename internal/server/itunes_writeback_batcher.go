// file: internal/server/itunes_writeback_batcher.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e90

package server

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// WriteBackBatcher collects book IDs that need iTunes write-back and flushes
// them in a single batch after a debounce delay. This avoids writing the XML/ITL
// files on every single edit when multiple edits happen in rapid succession.
type WriteBackBatcher struct {
	mu       sync.Mutex
	pending  map[string]bool // book IDs awaiting write-back
	timer    *time.Timer
	delay    time.Duration
	stopCh   chan struct{}
	stopped  bool
}

// GlobalWriteBackBatcher is the singleton batcher instance.
var GlobalWriteBackBatcher *WriteBackBatcher

// NewWriteBackBatcher creates a batcher with the given debounce delay.
func NewWriteBackBatcher(delay time.Duration) *WriteBackBatcher {
	return &WriteBackBatcher{
		pending: make(map[string]bool),
		delay:   delay,
		stopCh:  make(chan struct{}),
	}
}

// Enqueue adds a book ID to the pending write-back batch. If auto write-back
// is disabled in config, this is a no-op.
func (b *WriteBackBatcher) Enqueue(bookID string) {
	if !config.AppConfig.ITunesAutoWriteBack {
		return
	}
	if config.AppConfig.ITunesLibraryXMLPath == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stopped {
		return
	}

	b.pending[bookID] = true

	// Reset/start the debounce timer
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = time.AfterFunc(b.delay, b.flush)
}

// flush writes all pending book changes to the iTunes XML and optionally ITL.
func (b *WriteBackBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	ids := make([]string, 0, len(b.pending))
	for id := range b.pending {
		ids = append(ids, id)
	}
	b.pending = make(map[string]bool)
	b.mu.Unlock()

	log.Printf("[INFO] iTunes auto write-back: flushing %d books", len(ids))

	store := database.GlobalStore
	if store == nil {
		log.Printf("[WARN] iTunes auto write-back: no database store")
		return
	}

	xmlPath := config.AppConfig.ITunesLibraryXMLPath
	if xmlPath == "" {
		log.Printf("[WARN] iTunes auto write-back: no XML library path configured")
		return
	}

	// Build path mappings from config
	var pathMappings []itunes.PathMapping
	for _, m := range config.AppConfig.ITunesPathMappings {
		pathMappings = append(pathMappings, itunes.PathMapping{From: m.From, To: m.To})
	}

	var xmlUpdates []*itunes.WriteBackUpdate
	var itlUpdates []itunes.ITLLocationUpdate

	for _, id := range ids {
		book, err := store.GetBookByID(id)
		if err != nil || book == nil {
			continue
		}
		if book.ITunesPersistentID == nil || *book.ITunesPersistentID == "" {
			continue
		}

		itunesPath := itunes.ReverseRemapPath(book.FilePath, pathMappings)

		xmlUpdates = append(xmlUpdates, &itunes.WriteBackUpdate{
			ITunesPersistentID: *book.ITunesPersistentID,
			NewPath:            itunesPath,
		})

		if config.AppConfig.ITLWriteBackEnabled && config.AppConfig.ITunesLibraryITLPath != "" {
			itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
				PersistentID: *book.ITunesPersistentID,
				NewLocation:  itunesPath,
			})
		}
	}

	if len(xmlUpdates) == 0 {
		return
	}

	// Write XML
	result, err := itunes.WriteBack(itunes.WriteBackOptions{
		LibraryPath:  xmlPath,
		Updates:      xmlUpdates,
		CreateBackup: false, // Don't backup on every auto write-back
	})
	if err != nil {
		log.Printf("[WARN] iTunes auto write-back XML failed: %v", err)
	} else {
		log.Printf("[INFO] iTunes auto write-back XML: updated %d tracks", result.UpdatedCount)
		// Update fingerprint
		if fp, fpErr := itunes.ComputeFingerprint(xmlPath); fpErr == nil {
			_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
		}
	}

	// Write ITL
	if len(itlUpdates) > 0 {
		itlPath := config.AppConfig.ITunesLibraryITLPath
		itlResult, itlErr := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", itlUpdates)
		if itlErr != nil {
			log.Printf("[WARN] iTunes auto write-back ITL failed: %v", itlErr)
		} else {
			if renameErr := renameFile(itlPath+".tmp", itlPath); renameErr != nil {
				log.Printf("[WARN] iTunes auto write-back ITL rename failed: %v", renameErr)
			} else {
				log.Printf("[INFO] iTunes auto write-back ITL: updated %d tracks", itlResult.UpdatedCount)
			}
		}
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

	// Final flush
	b.flush()
}

// InitWriteBackBatcher initializes the global write-back batcher.
func InitWriteBackBatcher() {
	GlobalWriteBackBatcher = NewWriteBackBatcher(5 * time.Second)
}
