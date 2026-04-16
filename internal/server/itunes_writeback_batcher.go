// file: internal/server/itunes_writeback_batcher.go
// version: 3.2.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e90
//
// Combined write-back batcher: handles location updates, track additions,
// and track removals in a single ITL read-modify-write cycle.

package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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
//
// Adaptive debounce: the initial delay is 5s, but if new enqueues arrive
// within the window, the timer extends up to maxDelay (30s). This batches
// rapid-fire applies into a single ITL write instead of multiple.
type WriteBackBatcher struct {
	mu             sync.Mutex
	pendingBooks   map[string]bool      // book IDs for location updates
	pendingAdds    []itunes.ITLNewTrack // tracks to add
	pendingRemoves map[string]bool      // PIDs to remove (lowercase hex)
	timer          *time.Timer
	delay          time.Duration
	maxDelay       time.Duration
	firstEnqueue   time.Time // when the first enqueue in this batch happened
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
		maxDelay:       30 * time.Second,
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
	// Track when the first enqueue in this batch happened
	if b.firstEnqueue.IsZero() {
		b.firstEnqueue = time.Now()
	}
	// If we've been accumulating for longer than maxDelay, flush now
	elapsed := time.Since(b.firstEnqueue)
	if elapsed >= b.maxDelay {
		go b.flush()
		return
	}
	// Otherwise, extend the timer (but don't exceed maxDelay from first enqueue)
	remaining := b.maxDelay - elapsed
	delay := b.delay
	if delay > remaining {
		delay = remaining
	}
	b.timer = time.AfterFunc(delay, b.flush)
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
	b.firstEnqueue = time.Time{} // reset for next batch
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

	// Build location and metadata updates from book IDs
	var locationUpdates []itunes.ITLLocationUpdate
	var metadataUpdates []itunes.ITLMetadataUpdate
	for _, id := range bookIDs {
		book, err := store.GetBookByID(id)
		if err != nil || book == nil {
			continue
		}

		// Get author name for metadata
		authorName := ""
		if book.AuthorID != nil {
			if author, err := store.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				authorName = author.Name
			}
		}
		// Narrator ends up in the Composer field for audiobooks —
		// Apple Music shows it there and most audiobook workflows
		// (scanners, converters, players) key on that mapping.
		narrator := ""
		if book.Narrator != nil {
			narrator = *book.Narrator
		}
		// Genre: prefer the book's own genre when set, fall back to
		// "Audiobook" so iTunes classifies correctly. Previously
		// every write hardcoded "Audiobook" even when the user had
		// set a more specific value.
		genre := "Audiobook"
		if book.Genre != nil && *book.Genre != "" {
			genre = *book.Genre
		}

		files, _ := store.GetBookFiles(id)
		if len(files) > 0 {
			for _, f := range files {
				if f.ITunesPersistentID == "" {
					continue
				}
				if f.ITunesPath != "" {
					locationUpdates = append(locationUpdates, itunes.ITLLocationUpdate{
						PersistentID: f.ITunesPersistentID,
						NewLocation:  f.ITunesPath,
					})
				}
				// Always push metadata so iTunes has current values
				metadataUpdates = append(metadataUpdates, itunes.ITLMetadataUpdate{
					PersistentID: f.ITunesPersistentID,
					Name:         f.Title,
					Album:        book.Title,
					Artist:       authorName,
					Composer:     narrator,
					Genre:        genre,
				})
			}
		} else if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
			if book.ITunesPath != nil && *book.ITunesPath != "" {
				locationUpdates = append(locationUpdates, itunes.ITLLocationUpdate{
					PersistentID: *book.ITunesPersistentID,
					NewLocation:  *book.ITunesPath,
				})
			}
			metadataUpdates = append(metadataUpdates, itunes.ITLMetadataUpdate{
				PersistentID: *book.ITunesPersistentID,
				Name:         book.Title,
				Album:        book.Title,
				Artist:       authorName,
				Composer:     narrator,
				Genre:        genre,
			})
		}
	}

	ops := itunes.ITLOperationSet{
		Removes:         removes,
		Adds:            adds,
		LocationUpdates: locationUpdates,
		MetadataUpdates: metadataUpdates,
	}

	if ops.IsEmpty() {
		return
	}

	log.Printf("[INFO] iTunes write-back: flushing %d location updates, %d metadata updates, %d adds, %d removes",
		len(locationUpdates), len(metadataUpdates), len(adds), len(removes))

	itlPath := config.AppConfig.ITunesLibraryWritePath
	if err := safeWriteITL(itlPath, ops); err != nil {
		log.Printf("[WARN] iTunes write-back failed: %v", err)
		return
	}

	// Mark the books we wrote as iTunes-synced so downstream UI /
	// filters reflect current state. Only books with at least one
	// update (location or metadata) count — pendingBooks may include
	// IDs whose PIDs weren't found in the DB, those stay unmarked.
	if len(bookIDs) > 0 {
		if n, markErr := store.MarkITunesSynced(bookIDs); markErr != nil {
			log.Printf("[WARN] iTunes write-back: MarkITunesSynced failed: %v", markErr)
		} else if n > 0 {
			log.Printf("[INFO] iTunes write-back: marked %d books as iTunes-synced", n)
		}
	}
}

// Test hooks. Production code wires these to the real itunes
// package functions at package init. Tests override them so the
// safe-write cycle can be unit-tested without needing a valid
// ITL fixture on disk — the fixture itself is fragile, format
// changes have broken it before, and mocking the two external
// calls lets us test the logic in isolation.
var (
	itlValidateFn           = itunes.ValidateITL
	itlApplyOperationsFn    = itunes.ApplyITLOperations
)

// safeWriteITL performs a backup → write-temp → validate-temp →
// rename → validate-final → cleanup cycle for ITL write-back. At
// every failure point the original ITL is either untouched or
// restored from the backup, so a corrupted write can never leave
// the user with an unreadable library file.
//
// Sequence:
//  1. Ensure the source ITL currently parses (pre-condition check).
//     If it doesn't, abort — we won't compound an existing problem.
//  2. Copy itlPath → itlPath+".bak-YYYYMMDD-HHMMSS" as the rollback
//     anchor. Prune older backups to keep the last `itlBackupRetention`.
//  3. Run ApplyITLOperations to produce itlPath+".tmp".
//  4. Validate the .tmp file. If invalid, remove .tmp and abort — the
//     original itlPath is still intact.
//  5. Rename .tmp over itlPath.
//  6. Validate the renamed itlPath. If invalid (the rename itself
//     produced corruption, or the write was partial), copy the
//     backup back over itlPath.
//  7. Log the result.
func safeWriteITL(itlPath string, ops itunes.ITLOperationSet) error {
	// Step 1: sanity-check the source. If the ITL we're about to
	// write over is ALREADY corrupted, the write has nothing to
	// validate against and a rollback wouldn't help anyway.
	if err := itlValidateFn(itlPath); err != nil {
		return fmt.Errorf("source ITL validation failed (refusing to write to a broken file): %w", err)
	}

	// Step 2: backup-before-write.
	backupPath, backupErr := writeITLBackup(itlPath)
	if backupErr != nil {
		// A failing backup isn't fatal for the write — the user can
		// still recover from the next successful run — but we log
		// prominently so they know the safety net was missing.
		log.Printf("[WARN] iTunes write-back: backup failed (%v); proceeding without a rollback anchor", backupErr)
		backupPath = ""
	} else {
		if pruneErr := pruneITLBackups(itlPath, itlBackupRetention); pruneErr != nil {
			log.Printf("[WARN] iTunes write-back: backup prune failed: %v", pruneErr)
		}
	}

	// Step 3: write the updated ITL to a temp file.
	tmpPath := itlPath + ".tmp"
	result, err := itlApplyOperationsFn(itlPath, tmpPath, ops)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ApplyITLOperations: %w", err)
	}

	// Step 4: validate the temp BEFORE renaming. If the write
	// produced a file iTunes can't read, the original is still
	// intact at itlPath and we abort cleanly.
	if err := itlValidateFn(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("validation of temp ITL failed (original preserved): %w", err)
	}

	// Step 5: rename .tmp over the original.
	if err := renameFile(tmpPath, itlPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename .tmp → itl: %w", err)
	}

	// Step 6: validate the final file. On paranoid-filesystem
	// failures or weird permission issues the rename can land but
	// the result is still corrupt. Catch that and roll back.
	if err := itlValidateFn(itlPath); err != nil {
		log.Printf("[ERROR] iTunes write-back: post-rename validation failed (%v)", err)
		if backupPath != "" {
			if rbErr := copyFileContents(backupPath, itlPath); rbErr != nil {
				return fmt.Errorf("post-rename validation failed AND backup restore failed: validation=%v restore=%v", err, rbErr)
			}
			log.Printf("[INFO] iTunes write-back: restored from backup %s after corrupted write", backupPath)
			return fmt.Errorf("post-rename validation failed (restored from backup): %w", err)
		}
		return fmt.Errorf("post-rename validation failed (no backup available): %w", err)
	}

	log.Printf("[INFO] iTunes write-back: %d operations applied and validated", result.UpdatedCount)
	return nil
}

// itlBackupRetention is how many rotating .bak-YYYYMMDD-HHMMSS
// files to keep per ITL file. Balances "enough history to
// investigate a regression" vs "don't fill the disk". Five is
// typical for config-file backup schemes.
const itlBackupRetention = 5

// writeITLBackup copies itlPath to a timestamped sibling and
// returns the new path. Uses time.Now with seconds precision so
// rapid-fire backups in the same millisecond collide by design —
// the per-run batcher's debounce makes that effectively
// impossible, but documenting the assumption.
func writeITLBackup(itlPath string) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.bak-%s", itlPath, stamp)
	if err := copyFileContents(itlPath, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

// copyFileContents duplicates src to dst by reading the whole file
// into memory and writing it out. Small enough for ITL files
// (typically < 100 MB) and avoids needing io.Copy's dance.
func copyFileContents(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// pruneITLBackups deletes rotating backups beyond the keep limit.
// Sorts siblings by name (lexicographic on the timestamp suffix,
// which is monotonic) and removes the oldest excess.
func pruneITLBackups(itlPath string, keep int) error {
	if keep <= 0 {
		return nil
	}
	dir := filepath.Dir(itlPath)
	base := filepath.Base(itlPath) + ".bak-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var backups []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		backups = append(backups, filepath.Join(dir, name))
	}
	if len(backups) <= keep {
		return nil
	}
	sort.Strings(backups) // oldest first (lex sort on timestamp)
	toRemove := backups[:len(backups)-keep]
	for _, p := range toRemove {
		if err := os.Remove(p); err != nil {
			log.Printf("[WARN] iTunes write-back: prune %s: %v", p, err)
		}
	}
	return nil
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
