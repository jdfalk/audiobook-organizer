<!-- file: docs/superpowers/specs/2026-04-28-async-operations-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7f3e2d1c-9a8b-4c5d-6e7f-8a9b0c1d2e3f -->

# Design: Async Operations — Maintenance Handler Conversion

**Status:** Spec written, not yet planned  
**Audience:** Human review → burndown bot (after plan written)  
**TODO IDs:** ASYNC-1, ASYNC-2, ASYNC-3  
**Related:** `internal/operations/`, `internal/server/maintenance_fixups.go`

---

## Problem

13+ maintenance endpoints in `maintenance_fixups.go` execute synchronously (inline handler, HTTP response blocked until done). They:

- Show no progress in the UI spinner / notification badge
- Cannot be cancelled by the user
- Are not resumable on server restart
- Risk HTTP client timeouts on large libraries
- Mix validation, scanning, and mutation into one uninterruptible pass

Already-async operations (`scan`, `organize`, `bulk_write_back`, `iTunes import`, etc.) handle all of the above correctly via `operations.OperationQueue`.

---

## Goal

Every maintenance endpoint that touches > ~100 rows OR can take > ~1 second should:

1. Validate inputs synchronously → return 4xx immediately on bad params
2. Enqueue the work via `s.queue.Enqueue()` → return 202 immediately
3. Report progress via `ProgressReporter.UpdateProgress()` and `Log()`
4. Respect `ProgressReporter.IsCanceled()` at least once per batch loop
5. Persist checkpoints via `operations.SaveCheckpoint()` so restart resumes from last committed position
6. Follow check-then-apply: scan/count first, then mutate — so the dry-run preview is always consistent with the actual run

Short ops (<1s, <100 rows) may stay synchronous — these are noted below.

---

## Handlers to Convert

### Must Convert (async, progress, cancel, resumable)

| Handler | Route | Why |
|---------|-------|-----|
| `fix-read-by-narrator` | `POST /api/v1/maintenance/fix-read-by-narrator` | Touches all books; already has dry-run mode |
| `cleanup-series` | `POST /api/v1/maintenance/cleanup-series` | Merges series — can touch thousands of records |
| `backfill-book-files` | `POST /api/v1/maintenance/backfill-book-files` | Scans disk; long on large libraries |
| `cleanup-empty-folders` | `POST /api/v1/maintenance/cleanup-empty-folders` | File I/O, unbounded |
| `cleanup-organize-mess` | `POST /api/v1/maintenance/cleanup-organize-mess` | Batch file moves |
| `fix-author-narrator-swap` | `POST /api/v1/maintenance/fix-author-narrator-swap` | Touches all books |
| `fix-version-groups` | `POST /api/v1/maintenance/fix-version-groups` | Touches all version groups |
| `enrich-book-files` | `POST /api/v1/maintenance/enrich-book-files` | File I/O per book |
| `dedup-books` | `POST /api/v1/maintenance/dedup-books` | Full library scan |
| `fix-book-file-paths` | `POST /api/v1/maintenance/fix-book-file-paths` | Scans book_files table |
| `refetch-missing-authors` | `POST /api/v1/maintenance/refetch-missing-authors` | External API calls |
| `recompute-itunes-paths` | `POST /api/v1/maintenance/recompute-itunes-paths` | Touches all iTunes mappings |
| `fix-library-states` | `POST /api/v1/maintenance/fix-library-states` | Full library pass |
| `duplicates/scan` | `POST /api/v1/audiobooks/duplicates/scan` | Full library scan |

### May Stay Synchronous (fast, bounded)

| Handler | Route | Reason |
|---------|-------|--------|
| `cleanup-backups` | `POST /api/v1/maintenance/cleanup-backups` | Glob + delete, bounded by backup count |
| `generate-itl-tests` | `POST /api/v1/maintenance/generate-itl-tests` | Dev-only, test data generation |

---

## Implementation Pattern

Each converted handler follows this structure:

```go
// 1. Validate inputs — return 4xx immediately
params, err := parseAndValidateRequest(c)
if err != nil {
    RespondWithBadRequest(c, err.Error())
    return
}

// 2. Enqueue — return 202
opID := ulid.Make().String()
if err := s.queue.Enqueue(opID, "fix_read_by_narrator", operations.PriorityNormal, func(ctx context.Context, reporter operations.ProgressReporter) error {
    return s.runFixReadByNarrator(ctx, reporter, params)
}); err != nil {
    RespondWithInternalError(c, "failed to enqueue operation")
    return
}
c.JSON(http.StatusAccepted, gin.H{"operation_id": opID})

// 3. The actual work function:
func (s *Server) runFixReadByNarrator(ctx context.Context, reporter operations.ProgressReporter, params FixReadByNarratorParams) error {
    // Phase 1: scan (check-then-apply)
    books, err := s.Store().GetAllBooks()
    if err != nil { return err }
    
    affected := filterAffected(books, params)
    reporter.UpdateProgress(0, len(affected), "Scanning...")
    
    for i, book := range affected {
        if reporter.IsCanceled() { return nil }  // graceful cancel
        
        // Checkpoint every N items for resumability
        if i % 100 == 0 {
            operations.SaveCheckpoint(s.Store(), reporter.OperationID(), &operations.OperationState{
                PhaseIndex: i,
                PhaseTotal: len(affected),
            })
        }
        
        if params.DryRun {
            reporter.Log("info", fmt.Sprintf("Would fix: %s", book.Title), nil)
            continue
        }
        
        // Apply
        if err := s.Store().UpdateBook(book); err != nil {
            reporter.Log("error", fmt.Sprintf("Failed: %s: %v", book.Title, err), nil)
            continue
        }
        reporter.UpdateProgress(i+1, len(affected), fmt.Sprintf("Fixed %d/%d", i+1, len(affected)))
    }
    return nil
}
```

---

## New OperationState Param Types

Add to `internal/operations/state.go` for each resumable handler:

```go
type FixReadByNarratorParams struct {
    DryRun bool `json:"dry_run"`
}

type CleanupSeriesParams struct {
    DryRun bool `json:"dry_run"`
}

type EnrichBookFilesParams struct {
    BookIDs []string `json:"book_ids,omitempty"` // nil = all
    Force   bool     `json:"force"`
}

// ... etc for each converted handler
```

---

## Resume on Restart

Add each new operation type to `resumeInterruptedOperations()` in `server.go`:

```go
case "fix_read_by_narrator":
    params, err := operations.LoadParams[operations.FixReadByNarratorParams](store, opID)
    if err != nil { break }
    resumeFn := func(ctx context.Context, reporter operations.ProgressReporter) error {
        // Checkpoint.PhaseIndex tells us where to resume from
        checkpoint := operations.LoadCheckpoint(store, opID)
        return s.runFixReadByNarrator(ctx, reporter, params, checkpoint.PhaseIndex)
    }
    oq.EnqueueResume(opID, opType, operations.PriorityLow, resumeFn)
```

The work functions need to accept a `startFrom int` parameter for resume. This requires the check-then-apply scan to be deterministic (same order each run), which is satisfied by sorting by book ID.

---

## Frontend Impact

No frontend changes needed beyond ASYNC-0 (already shipped). The toast notification system and OperationsIndicator badge pick up all operations via SSE and the 15-second active-operations poll.

---

## Test Strategy

For each converted handler:
1. Unit test the work function with a mock store and mock `ProgressReporter`
2. Verify `IsCanceled()` is checked — inject a reporter that returns true after N calls
3. Verify checkpoint is saved — assert `SaveCheckpoint` called with correct `PhaseIndex`
4. Verify resume starts from checkpoint — call with `startFrom=50`, assert first 50 are skipped
5. Integration test: call the endpoint, assert 202, poll operation status to completion

---

## Ordering Recommendation

Execute as one batch via `/parallel-sweep` — each handler is independent, no shared state between them. Suggested wave:

- **Wave 1 (4 handlers):** `fix-read-by-narrator`, `cleanup-series`, `fix-author-narrator-swap`, `fix-version-groups` — simplest patterns
- **Wave 2 (4 handlers):** `backfill-book-files`, `cleanup-empty-folders`, `cleanup-organize-mess`, `fix-library-states` — file I/O variants
- **Wave 3 (5 handlers):** `enrich-book-files`, `dedup-books`, `fix-book-file-paths`, `refetch-missing-authors`, `recompute-itunes-paths` — external API or complex DB joins

Each wave: one PR per handler, admin-merge via `make ci` gate, rebase loop between siblings.
