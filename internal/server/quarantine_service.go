// file: internal/server/quarantine_service.go
// version: 1.0.0
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
	dest := filepath.Join(root, ".failed", author, title, filename)

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
	var origPath string
	for _, h := range history {
		if h.ChangeType == "quarantine" {
			origPath = h.OldPath
			break
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
	books, err := store.GetAllBooks(10000, 0)
	if err != nil {
		return
	}
	for _, b := range books {
		if b.QuarantinedAt != nil {
			continue
		}
		n, _ := store.GetScanFailCount(scanFailKey(b.FilePath))
		if n >= scanFailThreshold {
			log.Printf("[INFO] auto-quarantine: %s (fail count %d)", b.FilePath, n)
			_ = s.QuarantineBook(b.ID, fmt.Sprintf("taglib failed to read file after %d consecutive scan attempts", n))
		}
	}
}

// sanitizeDirName strips characters unsafe for directory names.
func sanitizeDirName(name string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	return strings.TrimSpace(replacer.Replace(name))
}
