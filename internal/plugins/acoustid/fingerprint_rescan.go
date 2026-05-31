// file: internal/plugins/acoustid/fingerprint_rescan.go
// version: 1.2.0
// guid: a7b8c9d0-e1f2-3456-def0-123456789abc
// last-edited: 2026-05-31

package acoustid

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
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
		ConcurrencyKey:  "acoustid.fingerprint",
		Isolate:         false, // DISABLED 2026-05-29: PR #1172 child-mode wire-up cannot work because Pebble is single-writer; child re-open fails. See MAYDEPLOY-A revisit.
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

	_ = reporter.UpdateProgress(0, 1, "Loading books for fingerprint rescan...")

	books, lerr := loadBooksForRescan(p.store, scope, req.BookIDs)
	if lerr != nil {
		reporter.Logger().Error("load books", "error", lerr)
		return fmt.Errorf("load books: %w", lerr)
	}

	total := len(books)
	if total == 0 {
		_ = reporter.UpdateProgress(1, 1, "No books matched the requested scope")
		return nil
	}

	workers := fpRescanWorkers()

	var (
		fingerprinted atomic.Int64
		skipped       atomic.Int64
		ineligible    atomic.Int64
		failed        atomic.Int64
	)
	startedAt := time.Now()

	// heartbeatTicker fires every 15 seconds so the registry watchdog never
	// sees the op as idle, regardless of how long individual fpcalc calls take.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	// progressMsg returns the current counter snapshot as a log string.
	progressMsg := func(bookIdx, bookTotal, filesDone, filesTotal int) string {
		return fmt.Sprintf("Books %d/%d files ~%d/%d (fp=%d skip=%d ineligible=%d fail=%d)",
			bookIdx, bookTotal, filesDone, filesTotal,
			fingerprinted.Load(), skipped.Load(), ineligible.Load(), failed.Load())
	}

	totalFiles := 0 // rough estimate, updated per book
	filesDone := 0

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
		totalFiles += len(files)

		// Fan out file fingerprinting across `workers` goroutines.
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for _, f := range files {
			select {
			case <-ctx.Done():
				wg.Wait()
				return ctx.Err()
			default:
			}

			sem <- struct{}{} // acquire slot
			wg.Add(1)
			go func(bf database.BookFile) {
				defer func() { <-sem; wg.Done() }()
				switch fingerprintBookFile(p.store, bf, req.Force) {
				case fingerprintOutcomeFingerprinted:
					fingerprinted.Add(1)
				case fingerprintOutcomeSkipped:
					skipped.Add(1)
				case fingerprintOutcomeIneligible:
					ineligible.Add(1)
				case fingerprintOutcomeFailed:
					failed.Add(1)
				}
			}(f)

			// Drain the heartbeat ticker non-blocking so we don't miss ticks
			// while goroutines are running.
			select {
			case <-heartbeat.C:
				_ = reporter.UpdateProgress(i+1, total, progressMsg(i+1, total, filesDone, totalFiles))
			default:
			}
		}
		wg.Wait()
		filesDone += len(files)

		if err := synthesizeBookSignatureForBook(p.store, b.ID); err != nil {
			reporter.Logger().Warn("synthesize book signature", "book_id", b.ID, "error", err)
		}

		// Progress after each book; also drain any pending ticker.
		select {
		case <-heartbeat.C:
		default:
		}
		_ = reporter.UpdateProgress(i+1, total, progressMsg(i+1, total, filesDone, totalFiles))
	}

	_ = reporter.UpdateProgress(total, total,
		fmt.Sprintf("Fingerprint rescan complete in %s — fp=%d skip=%d ineligible=%d fail=%d (of %d books, %d files)",
			time.Since(startedAt).Round(time.Second),
			fingerprinted.Load(), skipped.Load(), ineligible.Load(), failed.Load(), total, filesDone))
	return nil
}

// fpRescanWorkers returns the number of parallel fpcalc workers for rescan.
// Tunable via FP_PARALLEL_WORKERS env var; defaults to 4.
func fpRescanWorkers() int {
	if v := os.Getenv("FP_PARALLEL_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 32 {
			return n
		}
	}
	return 4
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

