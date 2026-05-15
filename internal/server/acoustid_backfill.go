// file: internal/server/acoustid_backfill.go
// version: 2.7.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f
// last-edited: 2026-05-15

package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/diagnosis"
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
	// Skip files that already failed recently. Without this we re-run
	// ffmpeg on every unreadable MP3 in the library on every backfill
	// pass — the log gets flooded with the same "Failed to find two
	// consecutive MPEG audio frames" error per startup. Reattempt after
	// 7 days in case the file was replaced. Force=true overrides.
	if f.FingerprintFailedAt != nil && !force {
		if time.Since(*f.FingerprintFailedAt) < 7*24*time.Hour {
			return fingerprintOutcomeSkipped
		}
	}
	if f.FilePath == "" || f.Missing {
		return fingerprintOutcomeIneligible
	}
	if !fingerprint.IsAudioFile(f.FilePath) {
		return fingerprintOutcomeIneligible
	}
	if !fingerprint.FileExists(f.FilePath) {
		return fingerprintOutcomeIneligible
	}

	segs, err := fingerprint.FileSegments(f.FilePath, f.Duration)
	if err != nil {
		log.Printf("[WARN] fingerprint: %s: %v", f.FilePath, err)
		// Run the diagnosis cascade to record WHY this file fails.
		ctx := context.Background()
		diagResult := diagnosis.ProbeFile(ctx, f.FilePath)
		reason, detail := diagnosis.Classify(diagResult, err.Error())
		diagJSON := diagnosis.ToJSON(diagResult)

		failedAt := time.Now()
		marked := f
		marked.FingerprintFailedAt = &failedAt
		reasonStr := string(reason)
		marked.FingerprintFailureReason = &reasonStr
		marked.FingerprintFailureDetail = &detail
		marked.FingerprintDiagnosticJSON = &diagJSON
		_ = store.UpdateBookFile(f.ID, &marked)
		return fingerprintOutcomeFailed
	}

	updated := f
	// Normalize at write time: chromaprint/ffmpeg dialects produce
	// URL-safe alphabet and varying padding, which the database has been
	// accumulating since fingerprinting started. Canonicalize here so new
	// rows are uniform.
	updated.AcoustIDSeg0 = fingerprint.NormalizeFingerprint(segs[0])
	updated.AcoustIDSeg1 = fingerprint.NormalizeFingerprint(segs[1])
	updated.AcoustIDSeg2 = fingerprint.NormalizeFingerprint(segs[2])
	updated.AcoustIDSeg3 = fingerprint.NormalizeFingerprint(segs[3])
	updated.AcoustIDSeg4 = fingerprint.NormalizeFingerprint(segs[4])
	updated.AcoustIDSeg5 = fingerprint.NormalizeFingerprint(segs[5])
	updated.AcoustIDSeg6 = fingerprint.NormalizeFingerprint(segs[6])
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
func (s *Server) backfillAcoustIDs(ctx context.Context) {
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

	var fingerprinted, alreadyImported, failed int
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
		bookModified := false
		for _, f := range files {
			switch fingerprintBookFile(store, f, false) {
			case fingerprintOutcomeFingerprinted:
				fingerprinted++
				bookModified = true
				time.Sleep(fingerprintThrottle)
			case fingerprintOutcomeSkipped:
				alreadyImported++
			case fingerprintOutcomeFailed:
				failed++
			}
		}

		// After fingerprinting all files for this book, synthesize the book signature
		if bookModified || b.BookSigV1 == nil {
			if err := synthesizeBookSignatureForBook(store, b.ID); err != nil {
				log.Printf("[WARN] acoustid backfill: synthesize book signature for %s: %v", b.ID, err)
			}
		}
	}

	log.Printf("[INFO] acoustid backfill complete: fingerprinted=%d already_imported=%d failed=%d",
		fingerprinted, alreadyImported, failed)
}

// AcoustIDLookupStore is the subset of the store needed for fingerprint lookups.
type AcoustIDLookupStore interface {
	database.BookFileStore
}

// synthesizeBookSignatureForBook generates and persists the unified book signature
// for a single book from its files' 7-segment chromaprint fingerprints.
// Missing fingerprints are zero-padded (partial sig); the coverage mask is stored
// alongside so that dedup comparisons can skip zero-padded regions.
func synthesizeBookSignatureForBook(store database.Store, bookID string) error {
	files, err := store.GetBookFiles(bookID)
	if err != nil {
		return fmt.Errorf("get book files: %w", err)
	}

	// Sort files by sort_order or original_filename.
	orderedFiles := make([]fingerprint.FileWithSegments, 0, len(files))
	for _, f := range files {
		orderedFiles = append(orderedFiles, fingerprint.FileWithSegments{
			SortOrder: f.TrackNumber,
			Filename:  f.OriginalFilename,
			Segments: fingerprint.FileSegmentData{
				Seg0: f.AcoustIDSeg0,
				Seg1: f.AcoustIDSeg1,
				Seg2: f.AcoustIDSeg2,
				Seg3: f.AcoustIDSeg3,
				Seg4: f.AcoustIDSeg4,
				Seg5: f.AcoustIDSeg5,
				Seg6: f.AcoustIDSeg6,
			},
		})
	}
	fingerprint.SortFilesByOrder(orderedFiles)

	// Compute peer ratio (uint32s per byte) from files that have fingerprints,
	// so missing files can get a calibrated length estimate.
	peerRatio := peerSegmentRatio(files)

	// Build FileSegmentInputs, estimating lengths for missing files.
	inputs := make([]fingerprint.FileSegmentInput, 0, len(orderedFiles))
	for i, fw := range orderedFiles {
		missing := fw.Segments.Seg0 == ""
		var estLen int
		if missing {
			f := files[i] // same order after SortFilesByOrder (indices align because we built from the same slice)
			fileSize := 0
			if fi, statErr := os.Stat(f.FilePath); statErr == nil {
				fileSize = int(fi.Size())
			}
			estLen = fingerprint.EstimateSegmentCount(f.Duration, fileSize, f.BitrateKbps, peerRatio)
		}
		inputs = append(inputs, fingerprint.FileSegmentInput{
			Segments:     fw.Segments,
			Missing:      missing,
			EstimatedLen: estLen,
		})
	}

	const minCoverageForPartialSig = 50

	sig, mask, coveragePct, segCount, err := fingerprint.SynthesizePartialBookSignature(inputs)
	if err != nil {
		if err == fingerprint.ErrIncompleteFingerprint {
			return nil
		}
		return fmt.Errorf("synthesize signature: %w", err)
	}

	// Skip storing a partial sig with very low coverage — not reliable enough for dedup.
	if coveragePct < minCoverageForPartialSig {
		return nil
	}

	now := time.Now()
	book, err := store.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	book.BookSigV1 = &sig
	book.BookSigSegments = &segCount
	book.BookSigBuiltAt = &now
	if mask != "" {
		book.BookSigV1Mask = &mask
	}
	book.BookSigCoveragePct = &coveragePct

	_, err = store.UpdateBook(book.ID, book)
	if err != nil {
		return fmt.Errorf("update book: %w", err)
	}
	return nil
}

// peerSegmentRatio returns the average uint32-per-byte ratio across all files
// in the set that have both a fingerprint (non-empty seg0) and a known file size.
// Returns 0 if no peers are available.
func peerSegmentRatio(files []database.BookFile) float64 {
	var totalWords, totalBytes int64
	for _, f := range files {
		if f.AcoustIDSeg0 == "" {
			continue
		}
		fi, err := os.Stat(f.FilePath)
		if err != nil || fi.Size() == 0 {
			continue
		}
		// Each base64 segment encodes (len*3/4) bytes of uint32s.
		// We use seg0 length as a proxy for word count (decoding would be more
		// accurate but much more expensive in a hot path).
		approxWords := int64(len(f.AcoustIDSeg0)) * 3 / (4 * 4) // base64 → bytes → uint32
		totalWords += approxWords
		totalBytes += fi.Size()
	}
	if totalBytes == 0 {
		return 0
	}
	return float64(totalWords) / float64(totalBytes)
}
