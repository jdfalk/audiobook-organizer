// file: internal/plugins/acoustid/fingerprint_rescan.go
// version: 1.1.0
// guid: a7b8c9d0-e1f2-3456-def0-123456789abc
// last-edited: 2026-05-19

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

// FingerprintRescanParams controls the scope and options of a fingerprint rescan operation.
type FingerprintRescanParams struct {
	Scope   string   `json:"scope,omitempty"`    // "missing" (default), "all", or "books"
	BookIDs []string `json:"book_ids,omitempty"` // required when scope=="books"
	Force   bool     `json:"force,omitempty"`    // ignore existing fingerprints and recompute
}

const (
	scopeMissing = "missing"
	scopeAll     = "all"
	scopeBooks   = "books"
)

func (p *Plugin) fingerprintRescanDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "acoustid.fingerprint-rescan",
		Plugin:          "acoustid",
		DisplayName:     "Fingerprint rescan",
		Description:     "Generates per-file fingerprints on demand with scope and force options.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityLow,
		Isolate:         true,
		Timeout:         12 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
			sdk.CapFilesExecute,
			sdk.CapSubprocessSpawn,
		},
		Run: p.runFingerprintRescan,
	}
}

func (p *Plugin) runFingerprintRescan(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	if p.store == nil {
		return fmt.Errorf("database store not available")
	}

	if !fingerprint.Available() {
		return fmt.Errorf("no fingerprint backend (fpcalc / ffmpeg) found")
	}

	var req FingerprintRescanParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			reporter.Logger().Error("failed to unmarshal params", "error", err)
			req = FingerprintRescanParams{}
		}
	}

	scope := req.Scope
	if scope == "" {
		scope = scopeMissing
	}
	switch scope {
	case scopeMissing, scopeAll:
		// ok
	case scopeBooks:
		if len(req.BookIDs) == 0 {
			return fmt.Errorf("book_ids is required when scope is \"books\"")
		}
	default:
		return fmt.Errorf("scope must be one of: missing, all, books")
	}

	_ = reporter.UpdateProgress(0, 100, "Loading books for fingerprint rescan...")

	books, lerr := loadBooksForRescan(p.store, scope, req.BookIDs)
	if lerr != nil {
		reporter.Logger().Error("load books", "error", lerr)
		return fmt.Errorf("load books: %w", lerr)
	}

	total := len(books)
	if total == 0 {
		_ = reporter.UpdateProgress(100, 100, "No books matched the requested scope")
		return nil
	}

	var fingerprinted, skipped, ineligible, failed int
	startedAt := time.Now()

	for i, b := range books {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		files, ferr := p.store.GetBookFiles(b.ID)
		if ferr != nil {
			continue
		}

		for _, f := range files {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			switch fingerprintBookFileWithForce(p.store, f, req.Force) {
			case fingerprintOutcomeFingerprinted:
				fingerprinted++
				time.Sleep(fingerprintThrottle)
			case fingerprintOutcomeSkipped:
				skipped++
			case fingerprintOutcomeIneligible:
				ineligible++
			case fingerprintOutcomeFailed:
				failed++
			}
		}

		// After fingerprinting all files for this book, synthesize the book signature
		if err := synthesizeBookSignatureForBook(p.store, b.ID); err != nil {
			reporter.Logger().Warn("synthesize book signature", "book_id", b.ID, "error", err)
		}

		if i%25 == 0 || i == total-1 {
			pct := 1 + (98 * (i + 1) / total)
			_ = reporter.UpdateProgress(pct, 100,
				fmt.Sprintf("Books %d/%d (fp=%d skip=%d ineligible=%d fail=%d)",
					i+1, total, fingerprinted, skipped, ineligible, failed))
		}
	}

	_ = reporter.UpdateProgress(100, 100,
		fmt.Sprintf("Fingerprint rescan complete in %s — fp=%d skip=%d ineligible=%d fail=%d (of %d books)",
			time.Since(startedAt).Round(time.Second),
			fingerprinted, skipped, ineligible, failed, total))
	return nil
}

// loadBooksForRescan fetches the set of books targeted by the requested scope.
func loadBooksForRescan(store database.Store, scope string, bookIDs []string) ([]database.Book, error) {
	switch scope {
	case scopeAll, scopeMissing:
		return store.GetAllBooks(0, 0)
	case scopeBooks:
		out := make([]database.Book, 0, len(bookIDs))
		for _, id := range bookIDs {
			b, err := store.GetBookByID(id)
			if err != nil || b == nil {
				continue
			}
			out = append(out, *b)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown scope %q", scope)
	}
}

// fingerprintBookFileWithForce is the same as fingerprintBookFile but accepts a force flag.
// Used by the fingerprint-rescan operation to optionally recompute existing fingerprints.
func fingerprintBookFileWithForce(store database.Store, f database.BookFile, force bool) fingerprintFileOutcome {
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
	if f.SkipScan {
		slog.Debug("skipping file due to SkipScan flag", "file_id", f.ID, "file_path", f.FilePath)
		return fingerprintOutcomeSkipped
	}

	segs, err := fingerprint.FileSegments(f.FilePath, f.Duration)
	if err != nil {
		return fingerprintOutcomeFailed
	}

	updated := f
	updated.AcoustIDSeg0 = fingerprint.NormalizeFingerprint(segs[0])
	updated.AcoustIDSeg1 = fingerprint.NormalizeFingerprint(segs[1])
	updated.AcoustIDSeg2 = fingerprint.NormalizeFingerprint(segs[2])
	updated.AcoustIDSeg3 = fingerprint.NormalizeFingerprint(segs[3])
	updated.AcoustIDSeg4 = fingerprint.NormalizeFingerprint(segs[4])
	updated.AcoustIDSeg5 = fingerprint.NormalizeFingerprint(segs[5])
	updated.AcoustIDSeg6 = fingerprint.NormalizeFingerprint(segs[6])
	if err := store.UpdateBookFile(f.ID, &updated); err != nil {
		return fingerprintOutcomeFailed
	}
	return fingerprintOutcomeFingerprinted
}
