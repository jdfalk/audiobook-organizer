// file: internal/server/acoustid_backfill.go
// version: 2.2.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package server

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
)

// fingerprintFileOutcome is the result of attempting to fingerprint a single
// book_file. Returned by fingerprintBookFile so callers can aggregate stats.
type fingerprintFileOutcome int

const (
	fingerprintOutcomeFingerprinted fingerprintFileOutcome = iota
	fingerprintOutcomeSkipped                              // already had seg0 (and force=false)
	fingerprintOutcomeIneligible                           // missing/non-audio/file gone
	fingerprintOutcomeFailed                               // ffmpeg/fpcalc error or store update error
)

// fingerprintBookFile generates and persists 7-segment chromaprint segments
// for a single book_file row. Honours `force` (clears seg0..seg6 first if
// true) and respects the same eligibility rules used by the startup backfill
// (file must exist, be a known audio extension, not be marked missing).
//
// Shared between the startup backfill and the on-demand rescan endpoint so
// the two paths can never drift.
func fingerprintBookFile(store database.Store, f database.BookFile, force bool) fingerprintFileOutcome {
	if f.AcoustIDSeg0 != "" && !force {
		return fingerprintOutcomeSkipped
	}
	if f.FilePath == "" || f.Missing {
		return fingerprintOutcomeIneligible
	}
	if _, ok := audioExtensions[strings.ToLower(filepath.Ext(f.FilePath))]; !ok {
		return fingerprintOutcomeIneligible
	}
	if _, err := os.Stat(f.FilePath); err != nil {
		return fingerprintOutcomeIneligible
	}

	segs, err := fingerprint.FileSegments(f.FilePath, f.Duration)
	if err != nil {
		log.Printf("[WARN] fingerprint: %s: %v", f.FilePath, err)
		return fingerprintOutcomeFailed
	}

	updated := f
	updated.AcoustIDSeg0 = segs[0]
	updated.AcoustIDSeg1 = segs[1]
	updated.AcoustIDSeg2 = segs[2]
	updated.AcoustIDSeg3 = segs[3]
	updated.AcoustIDSeg4 = segs[4]
	updated.AcoustIDSeg5 = segs[5]
	updated.AcoustIDSeg6 = segs[6]
	if err := store.UpdateBookFile(f.ID, &updated); err != nil {
		log.Printf("[WARN] fingerprint: update %s: %v", f.ID, err)
		return fingerprintOutcomeFailed
	}
	return fingerprintOutcomeFingerprinted
}

// fingerprintThrottle is the sleep between successful fingerprint operations
// (both backfill and on-demand rescan). Keeps disk/CPU available for active
// scans / organize runs.
const fingerprintThrottle = 10 * time.Millisecond

// backfillAcoustIDs walks all book_files without acoustid_seg0 and generates
// 7-segment fingerprints. Runs as a background goroutine after startup.
// Safe to run repeatedly — skips files that already have seg0 set.
// No-ops silently if neither fpcalc nor ffmpeg is installed.
func (s *Server) backfillAcoustIDs() {
	if !fingerprint.Available() {
		log.Println("[INFO] acoustid backfill: no fingerprint backend found, skipping")
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
	ctx := context.Background()
	for _, b := range books {
		select {
		case <-ctx.Done():
			return
		default:
		}
		files, err := store.GetBookFiles(b.ID)
		if err != nil {
			continue
		}
		for _, f := range files {
			switch fingerprintBookFile(store, f, false) {
			case fingerprintOutcomeFingerprinted:
				fingerprinted++
				time.Sleep(fingerprintThrottle)
			case fingerprintOutcomeSkipped:
				skipped++
			case fingerprintOutcomeFailed:
				failed++
			}
		}
	}

	log.Printf("[INFO] acoustid backfill complete: fingerprinted=%d skipped=%d failed=%d",
		fingerprinted, skipped, failed)
}

// AcoustIDLookupStore is the subset of the store needed for fingerprint lookups.
type AcoustIDLookupStore interface {
	database.BookFileStore
}
