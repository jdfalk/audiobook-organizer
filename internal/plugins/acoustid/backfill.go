// file: internal/plugins/acoustid/backfill.go
// version: 1.0.1
// guid: f6a7b8c9-d0e1-2345-def0-123456789abc
// last-edited: 2026-05-06

package acoustid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// BackfillParams encodes the checkpoint state for resumable backfill.
type BackfillParams struct {
	LastProcessedBookID string `json:"last_processed_book_id,omitempty"`
	Stats               struct {
		Fingerprinted int `json:"fingerprinted"`
		Skipped       int `json:"skipped"`
		Failed        int `json:"failed"`
	} `json:"stats"`
}

func (p *Plugin) backfillDef() sdk.OperationDef {
	sched := "0 3 * * *"
	return sdk.OperationDef{
		ID:              "acoustid.backfill",
		Plugin:          "acoustid",
		DisplayName:     "AcoustID backfill",
		Description:     "Generates AcoustID fingerprints for files missing acoustid_seg0.",
		ResumePolicy:    sdk.ResumeRestart,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "acoustid.fingerprint",
		Schedule:        &sched,
		Isolate:         false, // DISABLED 2026-05-29: PR #1172 child-mode wire-up cannot work because Pebble is single-writer; child re-open fails. See MAYDEPLOY-A revisit.
		Timeout:         24 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
			sdk.CapFilesExecute,
			sdk.CapSubprocessSpawn,
		},
		Run: p.runBackfill,
	}
}

func (p *Plugin) runBackfill(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return fmt.Errorf("dedup engine not available")
	}

	if !fingerprint.Available() {
		reporter.Logger().Info("acoustid backfill: no fingerprint backend found, skipping")
		return nil
	}

	if p.store == nil {
		return fmt.Errorf("database store not available")
	}

	var state BackfillParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &state); err != nil {
			reporter.Logger().Error("failed to unmarshal checkpoint", "error", err)
			state = BackfillParams{}
		}
	}

	_ = reporter.UpdateProgress(0, 100, "Loading books for fingerprint backfill...")

	books, err := p.store.GetAllBooks(100000, 0)
	if err != nil {
		reporter.Logger().Error("load books", "error", err)
		return fmt.Errorf("load books: %w", err)
	}

	var startIdx int
	if state.LastProcessedBookID != "" {
		for i, b := range books {
			if b.ID == state.LastProcessedBookID {
				startIdx = i + 1
				break
			}
		}
	}

	var fingerprinted, skipped, failed int
	total := len(books)

	for i := startIdx; i < total; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		b := books[i]
		files, err := p.store.GetBookFiles(b.ID)
		if err != nil {
			continue
		}

		bookModified := false
		for _, f := range files {
			outcome := fingerprintBookFile(p.store, f, false)
			switch outcome {
			case fingerprintOutcomeFingerprinted:
				fingerprinted++
				bookModified = true
				time.Sleep(fingerprintThrottle)
			case fingerprintOutcomeSkipped:
				skipped++
			case fingerprintOutcomeFailed:
				failed++
			}
		}

		// After fingerprinting all files for this book, synthesize the book signature
		if bookModified || b.BookSigV1 == nil {
			if err := synthesizeBookSignatureForBook(p.store, b.ID); err != nil {
				reporter.Logger().Warn("synthesize book signature", "book_id", b.ID, "error", err)
			}
		}

		if i%25 == 0 || i == total-1 {
			pct := 1 + (99 * (i + 1) / total)
			_ = reporter.UpdateProgress(pct, 100,
				fmt.Sprintf("Books %d/%d (fp=%d skip=%d fail=%d)", i+1, total, fingerprinted, skipped, failed))

			// Checkpoint progress every 25 books for resumability
			state.LastProcessedBookID = b.ID
			state.Stats.Fingerprinted = fingerprinted
			state.Stats.Skipped = skipped
			state.Stats.Failed = failed
			stateJSON, _ := json.Marshal(state)
			_ = reporter.Checkpoint(stateJSON)
		}
	}

	_ = reporter.UpdateProgress(100, 100,
		fmt.Sprintf("Acoustid backfill complete: fingerprinted=%d skipped=%d failed=%d",
			fingerprinted, skipped, failed))
	return nil
}

// fingerprintFileOutcome is the result of attempting to fingerprint a single book_file.
type fingerprintFileOutcome int

const (
	fingerprintOutcomeFingerprinted fingerprintFileOutcome = iota
	fingerprintOutcomeSkipped
	fingerprintOutcomeIneligible
	fingerprintOutcomeFailed
)

// fingerprintThrottle is the sleep between successful fingerprint operations.
const fingerprintThrottle = 10 * time.Millisecond

// audioExtensions maps audio file extensions to true (from internal/server/acoustid_backfill.go pattern).
var audioExtensions = map[string]bool{
	".aac":  true,
	".aiff": true,
	".alac": true,
	".ape":  true,
	".flac": true,
	".m4a":  true,
	".m4b":  true,
	".mp3":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".wma":  true,
	".wv":   true,
}

// fingerprintBookFile generates and persists 7-segment chromaprint segments
// for a single book_file row. Honours `force` and respects eligibility rules.
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
		slog.Warn("fingerprint", "path", f.FilePath, "err", err)
		return fingerprintOutcomeFailed
	}

	updated := f
	// Normalize at write time: canonicalize fingerprints for uniformity
	updated.AcoustIDSeg0 = fingerprint.NormalizeFingerprint(segs[0])
	updated.AcoustIDSeg1 = fingerprint.NormalizeFingerprint(segs[1])
	updated.AcoustIDSeg2 = fingerprint.NormalizeFingerprint(segs[2])
	updated.AcoustIDSeg3 = fingerprint.NormalizeFingerprint(segs[3])
	updated.AcoustIDSeg4 = fingerprint.NormalizeFingerprint(segs[4])
	updated.AcoustIDSeg5 = fingerprint.NormalizeFingerprint(segs[5])
	updated.AcoustIDSeg6 = fingerprint.NormalizeFingerprint(segs[6])
	if err := store.UpdateBookFile(f.ID, &updated); err != nil {
		slog.Warn("fingerprint update", "id", f.ID, "err", err)
		return fingerprintOutcomeFailed
	}
	return fingerprintOutcomeFingerprinted
}

// synthesizeBookSignatureForBook generates and persists the unified book signature
// for a single book from its files' 7-segment chromaprint fingerprints.
func synthesizeBookSignatureForBook(store database.Store, bookID string) error {
	files, err := store.GetBookFiles(bookID)
	if err != nil {
		return fmt.Errorf("get book files: %w", err)
	}

	// Sort files by sort_order or original_filename
	var orderedFiles []fingerprint.FileWithSegments
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

	// Extract just the segments in order
	var segData []fingerprint.FileSegmentData
	for _, f := range orderedFiles {
		segData = append(segData, f.Segments)
	}

	sig, segCount, err := fingerprint.SynthesizeBookSignature(segData)
	if err != nil {
		if err == fingerprint.ErrIncompleteFingerprint {
			return nil
		}
		return fmt.Errorf("synthesize signature: %w", err)
	}

	now := time.Now()
	book, err := store.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	book.BookSigV1 = &sig
	book.BookSigSegments = &segCount
	book.BookSigBuiltAt = &now

	_, err = store.UpdateBook(book.ID, book)
	if err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	return nil
}
