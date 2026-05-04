// file: internal/server/fingerprint_rescan.go
// version: 1.2.0
// guid: e8cf338d-2d99-47ae-a4b8-d31d8772d955

package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// FingerprintRescanRequest controls the scope of a manual fingerprint rescan.
type FingerprintRescanRequest struct {
	// Scope selects which book_files to (re)fingerprint.
	//   "missing" (default) — only files where acoustid_seg0 is empty
	//   "all"               — every audio file across every book
	//   "books"             — every audio file across the books listed in BookIDs
	Scope string `json:"scope,omitempty"`

	// BookIDs is required when Scope == "books"; ignored otherwise.
	BookIDs []string `json:"book_ids,omitempty"`

	// Force, when true, ignores any existing acoustid_seg0..seg6 values and
	// recomputes them. Default false (existing fingerprints are kept).
	Force bool `json:"force,omitempty"`
}

const (
	scopeMissing = "missing"
	scopeAll     = "all"
	scopeBooks   = "books"
)

// triggerFingerprintRescan handles POST /api/v1/dedup/fingerprint-rescan.
//
// Generates per-file 7-segment chromaprint fingerprints on demand, tracked
// as an Operation so the standard scan-status / cancel endpoints work.
//
// Unlike the startup backfill (which only runs once and silently skips files
// that already have acoustid_seg0), this endpoint accepts a `force` flag and
// scope selectors so an operator can repair a stale or incomplete library
// without restarting the service.
func (s *Server) triggerFingerprintRescan(c *gin.Context) {
	// Validate request body first so bad input always wins 400 over
	// environment-state errors (503/500). Keeps the contract stable across
	// CI environments where ffmpeg/fpcalc may or may not be installed.
	var req FingerprintRescanRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		httputil.RespondWithBadRequest(c, err.Error())
		return
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
			httputil.RespondWithBadRequest(c, "book_ids is required when scope is \"books\"")
			return
		}
	default:
		httputil.RespondWithBadRequest(c, "scope must be one of: missing, all, books")
		return
	}

	if !fingerprint.Available() {
		httputil.RespondWithServiceUnavailable(c, "no fingerprint backend (fpcalc / ffmpeg) found")
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		httputil.RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "fingerprint-rescan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create fingerprint-rescan operation", err)
		return
	}

	store := s.Store()
	bookIDs := append([]string(nil), req.BookIDs...)
	force := req.Force

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Loading books for fingerprint rescan...")

		books, lerr := loadBooksForRescan(store, scope, bookIDs)
		if lerr != nil {
			return fmt.Errorf("load books: %w", lerr)
		}
		total := len(books)
		if total == 0 {
			_ = progress.UpdateProgress(100, 100, "No books matched the requested scope")
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

			files, ferr := store.GetBookFiles(b.ID)
			if ferr != nil {
				continue
			}
			for _, f := range files {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				switch fingerprintBookFile(store, f, force) {
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

			if i%25 == 0 || i == total-1 {
				pct := 1 + (98 * (i + 1) / total)
				_ = progress.UpdateProgress(pct, 100,
					fmt.Sprintf("Books %d/%d  (fp=%d skip=%d ineligible=%d fail=%d)",
						i+1, total, fingerprinted, skipped, ineligible, failed))
			}
		}

		_ = progress.UpdateProgress(100, 100,
			fmt.Sprintf("Fingerprint rescan complete in %s — fp=%d skip=%d ineligible=%d fail=%d (of %d books)",
				time.Since(startedAt).Round(time.Second),
				fingerprinted, skipped, ineligible, failed, total))
		return nil
	}

	if err := s.queue.Enqueue(opID, "fingerprint-rescan", operations.PriorityLow, opFunc); err != nil {
		httputil.InternalError(c, "failed to enqueue fingerprint-rescan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, op)
}

// loadBooksForRescan fetches the set of books targeted by the requested
// scope. For "books" scope, missing IDs are silently skipped (the response
// will report a smaller total, which the caller can detect).
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
