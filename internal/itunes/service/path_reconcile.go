// file: internal/itunes/service/path_reconcile.go
// version: 2.0.0
// guid: 9e3b7a1d-4c2f-4a60-b8d5-2f1e8c0d9a47
//
// One-time (repeatable) backfill that walks every book with an
// iTunes persistent ID, recomputes book.ITunesPath and
// book_files.ITunesPath from the current FilePath, and enqueues the
// writeback batcher. Fixes libraries where organize ran before the
// path-update bug was patched and iTunes now shows "missing files"
// for books that were moved under the hood.

package itunesservice

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// pathReconcilerStore is the narrow slice of the service's Store that
// PathReconciler needs.
type pathReconcilerStore interface {
	database.BookStore
	database.BookFileStore
	database.OperationStore
}

// PathReconciler walks iTunes-tracked books, recomputes their
// ITunesPath fields from the current FilePath, and enqueues the
// write-back batcher so the ITL learns the new locations.
type PathReconciler struct {
	store    pathReconcilerStore
	enqueuer Enqueuer
	queue    operations.Queue
}

// newPathReconciler wires a PathReconciler with the given store,
// enqueuer and operation queue. All three are required in production;
// nil enqueuer just skips the write-back enqueue step (useful for
// tests).
func newPathReconciler(store pathReconcilerStore, enqueuer Enqueuer, queue operations.Queue) *PathReconciler {
	return &PathReconciler{store: store, enqueuer: enqueuer, queue: queue}
}

// iTunesPathReconcileResult is the per-run tally returned in
// progress logs. Exported so the future handler-level test can
// assert on it.
type iTunesPathReconcileResult struct {
	Scanned          int `json:"scanned"`
	ITunesTracked    int `json:"itunes_tracked"`
	PathsUpdated     int `json:"paths_updated"`
	FilePathsUpdated int `json:"file_paths_updated"`
	EnqueuedForWrite int `json:"enqueued_for_write"`
	Errors           int `json:"errors"`
}

// Start kicks off a tracked operation that walks the library and
// re-enqueues every iTunes-tracked book so the batcher pushes
// updated locations + metadata back to the ITL. Returns the operation
// via the gin context.
func (r *PathReconciler) Start(c *gin.Context) {
	if r.store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if r.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	id := ulid.Make().String()
	op, err := r.store.CreateOperation(id, "itunes_path_reconcile", nil)
	if err != nil {
		log.Printf("[ERROR] failed to create operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create operation"})
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return r.Reconcile(ctx, id, progress)
	}

	if err := r.queue.Enqueue(op.ID, "itunes_path_reconcile", operations.PriorityNormal, operationFunc); err != nil {
		log.Printf("[ERROR] failed to enqueue operation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue operation"})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// Reconcile is the operation body. Read-only over iTunes PIDs (PIDs
// are not changed), read-write over ITunesPath fields. Idempotent —
// safe to re-run. Skips books without an iTunes PID.
func (r *PathReconciler) Reconcile(ctx context.Context, opID string, progress operations.ProgressReporter) error {
	if r.store == nil {
		return fmt.Errorf("database not initialized")
	}

	_ = progress.Log("info", "Starting iTunes path reconcile", nil)

	// Load all books — 100k is the same cap other maintenance ops use.
	books, err := r.store.GetAllBooks(100000, 0)
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

		hasITunesBook := b.ITunesPersistentID != nil && *b.ITunesPersistentID != ""

		bookFiles, _ := r.store.GetBookFiles(b.ID)
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

		if b.FilePath != "" {
			wantBookITunesPath := metafetch.ComputeITunesPath(b.FilePath)
			if wantBookITunesPath != "" {
				current := ""
				if b.ITunesPath != nil {
					current = *b.ITunesPath
				}
				if current != wantBookITunesPath {
					b.ITunesPath = &wantBookITunesPath
					if _, err := r.store.UpdateBook(b.ID, b); err != nil {
						result.Errors++
						_ = progress.Log("warn", fmt.Sprintf("update book %s: %v", b.ID, err), nil)
					} else {
						result.PathsUpdated++
					}
				}
			}
		}

		for _, bf := range bookFiles {
			if bf.ITunesPersistentID == "" || bf.FilePath == "" {
				continue
			}
			want := metafetch.ComputeITunesPath(bf.FilePath)
			if want == "" || want == bf.ITunesPath {
				continue
			}
			bf.ITunesPath = want
			if err := r.store.UpdateBookFile(bf.ID, &bf); err != nil {
				result.Errors++
				_ = progress.Log("warn", fmt.Sprintf("update book_file %s: %v", bf.ID, err), nil)
				continue
			}
			result.FilePathsUpdated++
		}

		if r.enqueuer != nil {
			r.enqueuer.Enqueue(b.ID)
			result.EnqueuedForWrite++
		}

		if (i+1)%500 == 0 {
			_ = progress.UpdateProgress(i+1, len(books),
				fmt.Sprintf("reconciled %d/%d (%d iTunes-tracked, %d paths fixed, %d files fixed)",
					i+1, len(books), result.ITunesTracked, result.PathsUpdated, result.FilePathsUpdated))
			_ = operations.SaveCheckpoint(r.store, opID, "itunes_path_reconcile", "scanning", i+1, len(books))
		}
	}

	if r.enqueuer != nil && result.EnqueuedForWrite > 0 {
		time.Sleep(200 * time.Millisecond)
	}

	_ = operations.ClearState(r.store, opID)
	summary := fmt.Sprintf(
		"iTunes path reconcile complete: scanned=%d iTunes-tracked=%d book-paths-updated=%d file-paths-updated=%d enqueued=%d errors=%d",
		result.Scanned, result.ITunesTracked, result.PathsUpdated, result.FilePathsUpdated, result.EnqueuedForWrite, result.Errors,
	)
	_ = progress.Log("info", summary, nil)
	_ = progress.UpdateProgress(len(books), len(books), summary)
	return nil
}
