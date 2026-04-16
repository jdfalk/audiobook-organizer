// file: internal/server/itunes_path_reconcile.go
// version: 1.1.0
// guid: 9e3b7a1d-4c2f-4a60-b8d5-2f1e8c0d9a47
//
// One-time (repeatable) backfill that walks every book with an
// iTunes persistent ID, recomputes book.ITunesPath and
// book_files.ITunesPath from the current FilePath, and enqueues the
// writeback batcher. Fixes libraries where organize ran before the
// path-update bug was patched and iTunes now shows "missing files"
// for books that were moved under the hood.

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

type iTunesPathReconcileResult struct {
	Scanned            int `json:"scanned"`
	ITunesTracked      int `json:"itunes_tracked"`
	PathsUpdated       int `json:"paths_updated"`
	FilePathsUpdated   int `json:"file_paths_updated"`
	EnqueuedForWrite   int `json:"enqueued_for_write"`
	Errors             int `json:"errors"`
}

// startITunesPathReconcile kicks off a tracked operation that walks the
// library and re-enqueues every iTunes-tracked book so the batcher
// pushes updated locations + metadata back to the ITL.
func (s *Server) startITunesPathReconcile(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(500, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(500, gin.H{"error": "operation queue not initialized"})
		return
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, "itunes_path_reconcile", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runITunesPathReconcile(ctx, id, progress)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_path_reconcile", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(202, op)
}

// runITunesPathReconcile is the operation body. Read-only over iTunes
// PIDs (PIDs are not changed), read-write over ITunesPath fields.
// Idempotent — safe to re-run. Skips books without an iTunes PID.
func (s *Server) runITunesPathReconcile(ctx context.Context, opID string, progress operations.ProgressReporter) error {
	store := s.Store()
	if store == nil {
		return fmt.Errorf("database not initialized")
	}

	_ = progress.Log("info", "Starting iTunes path reconcile", nil)

	// Load all books — 100k is the same cap other maintenance ops use.
	books, err := store.GetAllBooks(100000, 0)
	if err != nil {
		return fmt.Errorf("load books: %w", err)
	}

	result := iTunesPathReconcileResult{Scanned: len(books)}
	_ = progress.Log("info", fmt.Sprintf("Reconciling iTunes paths for %d books", len(books)), nil)
	_ = progress.UpdateProgress(0, len(books), "Scanning books for iTunes PID coverage")

	for i := range books {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		b := &books[i]

		// Skip books with no iTunes connection at any level.
		hasITunesBook := b.ITunesPersistentID != nil && *b.ITunesPersistentID != ""

		bookFiles, _ := store.GetBookFiles(b.ID)
		hasITunesFile := false
		for _, bf := range bookFiles {
			if bf.ITunesPersistentID != "" {
				hasITunesFile = true
				break
			}
		}

		if !hasITunesBook && !hasITunesFile {
			continue
		}
		result.ITunesTracked++

		// Recompute book.ITunesPath from the current FilePath if needed.
		if b.FilePath != "" {
			wantBookITunesPath := computeITunesPath(b.FilePath)
			if wantBookITunesPath != "" {
				current := ""
				if b.ITunesPath != nil {
					current = *b.ITunesPath
				}
				if current != wantBookITunesPath {
					b.ITunesPath = &wantBookITunesPath
					if _, err := store.UpdateBook(b.ID, b); err != nil {
						result.Errors++
						_ = progress.Log("warn", fmt.Sprintf("update book %s: %v", b.ID, err), nil)
					} else {
						result.PathsUpdated++
					}
				}
			}
		}

		// Recompute book_files.ITunesPath per file.
		for _, bf := range bookFiles {
			if bf.ITunesPersistentID == "" || bf.FilePath == "" {
				continue
			}
			want := computeITunesPath(bf.FilePath)
			if want == "" || want == bf.ITunesPath {
				continue
			}
			bf.ITunesPath = want
			if err := store.UpdateBookFile(bf.ID, &bf); err != nil {
				result.Errors++
				_ = progress.Log("warn", fmt.Sprintf("update book_file %s: %v", bf.ID, err), nil)
				continue
			}
			result.FilePathsUpdated++
		}

		// Enqueue the batcher so the next flush pushes both location
		// and metadata updates to iTunes for this book.
		if GlobalWriteBackBatcher != nil {
			GlobalWriteBackBatcher.Enqueue(b.ID)
			result.EnqueuedForWrite++
		}

		if (i+1)%500 == 0 {
			_ = progress.UpdateProgress(i+1, len(books),
				fmt.Sprintf("reconciled %d/%d (%d iTunes-tracked, %d paths fixed, %d files fixed)",
					i+1, len(books), result.ITunesTracked, result.PathsUpdated, result.FilePathsUpdated))
			_ = operations.SaveCheckpoint(store, opID, "itunes_path_reconcile", "scanning", i+1, len(books))
		}
	}

	// Nudge the batcher to flush sooner rather than waiting out its
	// maxDelay. Give it a brief grace window so log output stays
	// ordered with the flush INFO line.
	if GlobalWriteBackBatcher != nil && result.EnqueuedForWrite > 0 {
		time.Sleep(200 * time.Millisecond)
	}

	_ = operations.ClearState(store, opID)
	summary := fmt.Sprintf(
		"iTunes path reconcile complete: scanned=%d iTunes-tracked=%d book-paths-updated=%d file-paths-updated=%d enqueued=%d errors=%d",
		result.Scanned, result.ITunesTracked, result.PathsUpdated, result.FilePathsUpdated, result.EnqueuedForWrite, result.Errors,
	)
	_ = progress.Log("info", summary, nil)
	_ = progress.UpdateProgress(len(books), len(books), summary)
	return nil
}
