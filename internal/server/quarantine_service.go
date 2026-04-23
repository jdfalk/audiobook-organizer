// file: internal/server/quarantine_service.go
// version: 1.2.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
)

// scanFailKey returns the PebbleDB key suffix for a file's scan-fail counter.
func scanFailKey(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h[:8])
}

// QuarantineBook moves a book's file to .failed/{author}/{title}/{filename},
// updates the DB, records path history, sets iTunes purge_pending if linked,
// and publishes a book.quarantined event.
func (s *Server) QuarantineBook(bookID, reason string) error {
	store := s.Store()
	if store == nil {
		return fmt.Errorf("store not initialized")
	}

	book, err := store.GetBookByID(bookID)
	if err != nil || book == nil {
		return fmt.Errorf("book not found: %s", bookID)
	}
	if book.QuarantinedAt != nil {
		return nil // already quarantined
	}

	root := config.AppConfig.RootDir
	if root == "" {
		return fmt.Errorf("RootDir not configured")
	}

	author := "Unknown Author"
	if book.Author != nil && book.Author.Name != "" {
		author = sanitizeDirName(book.Author.Name)
	}
	title := sanitizeDirName(book.Title)
	if title == "" {
		title = "Unknown"
	}
	filename := filepath.Base(book.FilePath)

	failedRoot := filepath.Clean(filepath.Join(root, ".failed"))
	dest := filepath.Clean(filepath.Join(failedRoot, author, title, filename))
	// Boundary check: dest must stay inside .failed/
	if !strings.HasPrefix(dest, failedRoot+string(filepath.Separator)) {
		return fmt.Errorf("quarantine path %q escapes .failed directory", dest)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("mkdir .failed: %w", err)
	}
	if err := os.Rename(book.FilePath, dest); err != nil {
		return fmt.Errorf("move to .failed: %w", err)
	}

	oldPath := book.FilePath
	now := time.Now()
	book.FilePath = dest
	book.QuarantineReason = &reason
	book.QuarantinedAt = &now

	if book.ITunesPersistentID != nil {
		purge := "purge_pending"
		book.ITunesSyncStatus = &purge
	}

	if _, err := store.UpdateBook(bookID, book); err != nil {
		// Rollback: move file back before returning the error.
		if rollbackErr := os.Rename(dest, oldPath); rollbackErr != nil {
			log.Printf("[ERROR] QuarantineBook: DB update failed and file rollback failed: %v (original: %v)", rollbackErr, err)
		}
		return fmt.Errorf("update book: %w", err)
	}

	_ = store.RecordPathChange(&database.BookPathChange{
		BookID:     bookID,
		OldPath:    oldPath,
		NewPath:    dest,
		ChangeType: "quarantine",
	})

	log.Printf("[INFO] QuarantineBook: %s → %s (%s)", oldPath, dest, reason)

	s.publishEvent(context.Background(), plugin.NewEvent(plugin.EventBookQuarantined, bookID, map[string]any{
		"title":          book.Title,
		"author":         author,
		"file_path":      dest,
		"original_path":  oldPath,
		"reason":         reason,
		"quarantined_at": now.Format(time.RFC3339),
	}))

	return nil
}

// UnquarantineBook moves a quarantined book back to its original path
// (retrieved from path history) and clears the quarantine fields.
func (s *Server) UnquarantineBook(bookID string) error {
	store := s.Store()
	if store == nil {
		return fmt.Errorf("store not initialized")
	}

	book, err := store.GetBookByID(bookID)
	if err != nil || book == nil {
		return fmt.Errorf("book not found: %s", bookID)
	}
	if book.QuarantinedAt == nil {
		return nil // not quarantined
	}

	history, err := store.GetBookPathHistory(bookID)
	if err != nil {
		return fmt.Errorf("get path history: %w", err)
	}
	// Find the most-recent quarantine entry (history is ordered oldest-first).
	var origPath string
	for _, h := range history {
		if h.ChangeType == "quarantine" {
			origPath = h.OldPath
		}
	}
	if origPath == "" {
		return fmt.Errorf("no quarantine history entry found for book %s", bookID)
	}

	if err := os.MkdirAll(filepath.Dir(origPath), 0755); err != nil {
		return fmt.Errorf("mkdir original path: %w", err)
	}
	if err := os.Rename(book.FilePath, origPath); err != nil {
		return fmt.Errorf("restore from .failed: %w", err)
	}

	quarPath := book.FilePath
	book.FilePath = origPath
	book.QuarantineReason = nil
	book.QuarantinedAt = nil
	if book.ITunesSyncStatus != nil && *book.ITunesSyncStatus == "purge_pending" {
		dirty := "dirty"
		book.ITunesSyncStatus = &dirty
	}

	if _, err := store.UpdateBook(bookID, book); err != nil {
		// Rollback: move file back to quarantine location.
		if rollbackErr := os.Rename(origPath, quarPath); rollbackErr != nil {
			log.Printf("[ERROR] UnquarantineBook: DB update failed and file rollback failed: %v (original: %v)", rollbackErr, err)
		}
		return fmt.Errorf("update book: %w", err)
	}

	_ = store.RecordPathChange(&database.BookPathChange{
		BookID:     bookID,
		OldPath:    quarPath,
		NewPath:    origPath,
		ChangeType: "unquarantine",
	})

	log.Printf("[INFO] UnquarantineBook: %s → %s", quarPath, origPath)

	s.publishEvent(context.Background(), plugin.NewEvent(plugin.EventBookUnquarantined, bookID, map[string]any{
		"file_path":      origPath,
		"quarantine_path": quarPath,
	}))

	return nil
}

const scanFailThreshold = 3

// autoQuarantineFailedScans checks for books whose scan-fail counter has reached
// the threshold and quarantines them automatically.
func (s *Server) autoQuarantineFailedScans() {
	store := s.Store()
	if store == nil {
		return
	}
	// Paginate to avoid missing books when library exceeds page size.
	const pageSize = 1000
	var offset int
	for {
		page, err := store.GetAllBooks(pageSize, offset)
		if err != nil || len(page) == 0 {
			break
		}
		for _, b := range page {
			if b.QuarantinedAt != nil {
				continue
			}
			n, _ := store.GetScanFailCount(scanFailKey(b.FilePath))
			if n >= scanFailThreshold {
				log.Printf("[INFO] auto-quarantine: %s (fail count %d)", b.FilePath, n)
				_ = s.QuarantineBook(b.ID, fmt.Sprintf("taglib failed to read file after %d consecutive scan attempts", n))
			}
		}
		if len(page) < pageSize {
			break
		}
		offset += pageSize
	}
}

// processITunesPurgePending finds books with itunes_sync_status = "purge_pending",
// enqueues their PIDs for ITL removal, and clears their iTunes linkage.
// Called at the start of each iTunes sync cycle.
func (s *Server) processITunesPurgePending() {
	store := s.Store()
	if store == nil || s.writeBackBatcher == nil {
		return
	}
	books, err := store.GetITunesPurgePendingBooks()
	if err != nil || len(books) == 0 {
		return
	}
	for _, b := range books {
		if b.ITunesPersistentID == nil {
			continue
		}
		s.writeBackBatcher.EnqueueRemove(*b.ITunesPersistentID)
		log.Printf("[INFO] processITunesPurgePending: queued ITL removal for %s (book %s)", *b.ITunesPersistentID, b.ID)

		// Clear iTunes linkage so the book is no longer tied to iTunes.
		cleared := "unlinked"
		b.ITunesSyncStatus = &cleared
		b.ITunesPersistentID = nil
		if _, err := store.UpdateBook(b.ID, &b); err != nil {
			log.Printf("[WARN] processITunesPurgePending: UpdateBook %s: %v", b.ID, err)
		}
	}
}

// sanitizeDirName strips characters unsafe for directory names, including path
// traversal sequences and control characters.
func sanitizeDirName(name string) string {
	// Replace path-separator and shell-special characters.
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-", "|", "-",
	)
	name = replacer.Replace(name)

	// Strip control characters (including null bytes).
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	// Replace ".." traversal component with "-".
	name = strings.ReplaceAll(name, "..", "-")

	return strings.TrimSpace(name)
}
