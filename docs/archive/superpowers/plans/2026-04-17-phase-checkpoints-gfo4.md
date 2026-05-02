<!-- file: docs/superpowers/plans/2026-04-17-phase-checkpoints-gfo4.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2beb8572-2226-44be-8a9b-bfac9945be23 -->
<!-- last-edited: 2026-04-16 -->

# GFO-4: Phase Checkpoints in Apply Pipeline — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** GFO-4 — Phase checkpoints in apply pipeline
**Spec:** None — this plan is self-contained.
**Depends on:** Nothing

## Overview

The metadata apply pipeline (`runApplyPipeline` in `internal/server/metadata_fetch_service.go`) runs several phases sequentially: file rename, cover art embedding, tag write-back, iTunes writeback enqueue, and DB update. If the process crashes or is interrupted mid-pipeline, recovery currently re-runs all phases from scratch — including phases that already succeeded. This wastes time (ffmpeg cover embed is expensive) and risks double-renames.

This plan adds per-phase checkpoint keys in PebbleDB. On resume, the pipeline checks which phases completed and skips them. On successful completion, all checkpoint keys are cleared.

## Prerequisites

- Familiarity with `runApplyPipeline` (metadata_fetch_service.go:3138) and the `WriteBackMetadataForBook` method
- PebbleDB key prefix convention: `apply_checkpoint:{bookID}:{phase}` → timestamp

---

### Task 1: Define checkpoint constants + PebbleDB helpers (1 PR)

**Goal:** Create the checkpoint key schema and Store interface methods for reading/writing/clearing phase checkpoints.

**Files:**
- Modify: `internal/database/store.go` — add checkpoint interface methods
- Modify: `internal/database/pebble_store.go` — implement checkpoint methods
- Modify: `internal/database/mock_store.go` — add mock implementations
- Create: `internal/database/pebble_store_checkpoint_test.go`

Phase constants to define:
- [ ] `PhaseRename` — file rename completed
- [ ] `PhaseCoverEmbed` — cover art embedded into audio files
- [ ] `PhaseTagWrite` — taglib metadata tags written
- [ ] `PhaseDBUpdate` — book/book_file DB records updated with new paths
- [ ] `PhaseITunesEnqueue` — iTunes writeback enqueued

Store methods:
- [ ] `SetApplyCheckpoint(bookID string, phase string) error` — writes `apply_checkpoint:{bookID}:{phase}` → current timestamp
- [ ] `GetApplyCheckpoints(bookID string) (map[string]time.Time, error)` — returns all completed phases for a book
- [ ] `ClearApplyCheckpoints(bookID string) error` — deletes all checkpoint keys for a book (prefix scan + delete)
- [ ] `ListInFlightApplyBooks() ([]string, error)` — scans all `apply_checkpoint:` keys and returns distinct book IDs (for startup recovery)

**Acceptance criteria:**
- [ ] Round-trip test: set 3 checkpoints, get returns all 3, clear removes all
- [ ] ListInFlightApplyBooks returns only books with at least one checkpoint
- [ ] Keys use the `apply_checkpoint:` prefix consistently

---

### Task 2: Instrument `runApplyPipeline` with checkpoint checks and writes (1 PR)

**Goal:** Wrap each phase of `runApplyPipeline` with checkpoint logic: skip if already done, record on success.

**Files:**
- Modify: `internal/server/metadata_fetch_service.go` — update `runApplyPipeline` and `embedCoverInBookFiles`

Changes to `runApplyPipeline` (line 3138):
- [ ] At entry: call `GetApplyCheckpoints(id)` to load completed phases
- [ ] Before rename block (line 3204 `AutoRenameOnApply`): skip if `PhaseRename` in completed set; after success, call `SetApplyCheckpoint(id, PhaseRename)`
- [ ] Before cover embed: skip if `PhaseCoverEmbed` in completed; after `embedCoverInBookFiles` returns, set checkpoint
- [ ] Before tag write (line 3278 `AutoWriteTagsOnApply`): skip if `PhaseTagWrite` in completed; after success, set checkpoint
- [ ] Before iTunes enqueue (line 3291): skip if `PhaseITunesEnqueue` in completed; after enqueue, set checkpoint
- [ ] At successful return: call `ClearApplyCheckpoints(id)` to remove all checkpoint keys
- [ ] On error return: do NOT clear checkpoints (they are the recovery breadcrumb)

**Acceptance criteria:**
- [ ] If pipeline succeeds end-to-end, no checkpoint keys remain
- [ ] If pipeline fails at tag write, rename and cover checkpoints are set, and on retry those phases are skipped
- [ ] Log messages indicate when a phase is skipped due to checkpoint

---

### Task 3: Startup recovery for interrupted pipelines (1 PR)

**Goal:** On server startup, detect books with leftover checkpoint keys and resume their apply pipeline.

**Files:**
- Modify: `internal/server/server.go` — add recovery call in startup sequence
- Create: `internal/server/apply_recovery.go` — recovery logic
- Create: `internal/server/apply_recovery_test.go`

Recovery logic:
- [ ] `RecoverInterruptedApplyPipelines(db Store, mfs *MetadataFetchService)` — called once during server init
- [ ] Call `ListInFlightApplyBooks()` to find books with incomplete pipelines
- [ ] For each book: log a warning, load the book from DB, re-run `runApplyPipeline` in a goroutine
- [ ] Limit concurrency to 2 simultaneous recovery pipelines (semaphore)
- [ ] If the book no longer exists in DB, clear its checkpoints and skip
- [ ] Add a config flag `DisableApplyRecovery` (default false) for safety in case recovery causes issues

**Acceptance criteria:**
- [ ] Test: set checkpoints for 2 books, call recovery, verify pipelines re-run and checkpoints are cleared on success
- [ ] Test: set checkpoints for a deleted book, verify checkpoints are cleared without error
- [ ] Recovery runs in background goroutines so it does not block server startup

---

### Task 4: Checkpoint TTL cleanup (1 PR)

**Goal:** Stale checkpoints (from books that were deleted or pipelines that silently failed) should be cleaned up by the maintenance scheduler.

**Files:**
- Modify: `internal/server/scheduler.go` — add `apply_checkpoint_cleanup` maintenance task
- Create: `internal/server/apply_checkpoint_cleanup_test.go`

- [ ] Scan all `apply_checkpoint:` keys older than 24 hours
- [ ] For each stale book ID: if book still exists, attempt one more recovery run; if book is gone, clear checkpoints
- [ ] Log each cleanup action
- [ ] Run as part of the existing daily maintenance window

**Acceptance criteria:**
- [ ] Checkpoints older than 24h for deleted books are cleaned up
- [ ] Checkpoints older than 24h for existing books trigger one recovery attempt
- [ ] Fresh checkpoints (under 24h) are left alone

---

### Task 5: `RunApplyPipelineRenameOnly` checkpoint support (1 PR)

**Goal:** The rename-only path (`RunApplyPipelineRenameOnly`, line 3297) should also use checkpoints for consistency.

**Files:**
- Modify: `internal/server/metadata_fetch_service.go` — update `RunApplyPipelineRenameOnly`

- [ ] Use a separate checkpoint key prefix: `apply_rename_checkpoint:{bookID}:{phase}` to avoid collision with the full pipeline
- [ ] Phases: `PhaseRename`, `PhaseDBUpdate` (only two phases in this path)
- [ ] Same pattern: check on entry, set on success, clear on completion
- [ ] Add corresponding Store methods or reuse existing ones with a different prefix parameter

**Acceptance criteria:**
- [ ] Rename-only pipeline skips rename if checkpoint exists
- [ ] Checkpoints are cleared on successful completion
- [ ] Rename-only checkpoints do not interfere with full-pipeline checkpoints

---

### Estimated effort

| Task | Size | Depends on |
|------|------|------------|
| 1 (checkpoint helpers) | S | -- |
| 2 (instrument pipeline) | M | 1 |
| 3 (startup recovery) | M | 1, 2 |
| 4 (TTL cleanup) | S | 1 |
| 5 (rename-only) | S | 1 |
| **Total** | ~5 PRs, M overall | |

### Critical path

Task 1 must be done first. Tasks 2-5 can proceed in parallel after task 1, though task 3 depends on the instrumentation from task 2 to be meaningful.
