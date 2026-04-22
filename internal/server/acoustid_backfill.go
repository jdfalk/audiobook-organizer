// file: internal/server/acoustid_backfill.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package server

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
)

// backfillAcoustIDs walks all book_files without an acoustid_fingerprint and
// fingerprints them using fpcalc. Runs as a background goroutine after startup.
// Safe to run repeatedly — skips files that already have a fingerprint.
// No-ops silently if fpcalc is not installed.
func (s *Server) backfillAcoustIDs() {
	if !fingerprint.Available() {
		log.Println("[INFO] acoustid backfill: fpcalc not found, skipping")
		return
	}

	store := s.Store()
	if store == nil {
		return
	}

	books, err := store.GetAllBooks(100000, 0)
	if err != nil {
		log.Printf("[WARN] acoustid backfill: load books: %v", err)
		return
	}

	var fingerprinted, skipped, failed int
	for _, b := range books {
		files, err := store.GetBookFiles(b.ID)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.AcoustIDFingerprint != "" {
				skipped++
				continue
			}
			if f.FilePath == "" || f.Missing {
				continue
			}
			if _, ok := audioExtensions[strings.ToLower(filepath.Ext(f.FilePath))]; !ok {
				continue
			}
			if _, err := os.Stat(f.FilePath); err != nil {
				continue
			}

			result, err := fingerprint.File(f.FilePath)
			if err != nil {
				log.Printf("[WARN] acoustid backfill: fpcalc %s: %v", f.FilePath, err)
				failed++
				continue
			}

			updated := f
			updated.AcoustIDFingerprint = result.Fingerprint
			updated.AcoustIDDuration = int(result.Duration)
			if err := store.UpdateBookFile(f.ID, &updated); err != nil {
				log.Printf("[WARN] acoustid backfill: update %s: %v", f.ID, err)
				failed++
				continue
			}
			fingerprinted++

			// Throttle to avoid saturating disk I/O during active use.
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("[INFO] acoustid backfill complete: fingerprinted=%d skipped=%d failed=%d",
		fingerprinted, skipped, failed)
}

// AcoustIDLookupStore is the subset of the store needed for fingerprint lookups.
type AcoustIDLookupStore interface {
	database.BookFileStore
}
