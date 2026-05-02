# Bulk Organize Undo — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Spec:** `docs/superpowers/specs/2026-04-15-bulk-organize-undo-design.md`
**Depends on:** Nothing. Can run in parallel with 4.4 DI.
**Pairs with:** existing `operation_changes` table + per-book revert buttons.

---

### Task 1: Verify existing operation_changes schema (research, no code)

- [ ] Read `internal/database/store.go` — `OperationChange` struct, `CreateOperationChange`, `GetOperationChanges`, `RevertOperationChanges`
- [ ] Confirm fields match the spec: `operation_id`, `book_id`, `change_type`, `before_json`, `after_json`, `created_at`, `reverted_at`
- [ ] If `before_json` / `after_json` are missing (or named differently), note what migration is needed
- [ ] Document findings as a comment in the PR for task 2

---

### Task 2: Ensure organize records operation_changes rows (1 PR)

**Goal:** Every organize rename produces a `change_type = "organize_rename"` row with `before_json = {file_path, library_state, last_organized_at}` and `after_json` with the new values. Some of this may already be recorded — fill gaps.

**Files:**
- Modify: `internal/server/organize_service.go` — in `organizeBooks` worker, expand `CreateOperationChange` calls to include `before_json` / `after_json` with the fields organize touches
- Modify: `internal/server/organize_service.go` — in `reOrganizeInPlace`, same
- Maybe modify: `internal/database/store.go` — add `reverted_at` column if absent (migration)

- [ ] Audit existing `CreateOperationChange` calls in organize_service.go
- [ ] Add `before_json` / `after_json` to each call with the specific fields organize mutated
- [ ] Add migration if `reverted_at` column is missing
- [ ] Test: run organize, verify operation_changes rows have correct before/after snapshots

---

### Task 3: Undo engine — reversal logic (1 PR)

**Files:**
- Create: `internal/server/undo_engine.go` — `runUndoOperation(ctx, targetOpID, scope, progress) error`
- Create: `internal/server/undo_engine_test.go`

- [ ] Load non-reverted `operation_changes` rows for the target op, reverse order
- [ ] For each `change_type`:
  - `organize_rename` / `file_move`: `os.Rename(after_path, before_path)` with conflict detection
  - `db_update`: field-scoped restore from `before_json`
  - `dir_create`: remove if empty
- [ ] Set `reverted_at = now()` on each successfully reversed row
- [ ] Aggregate result: `{reverted, skipped_conflict, failed}`
- [ ] Test with mock store + temp filesystem

---

### Task 4: Undo as a tracked operation (1 PR)

**Files:**
- Modify: `internal/server/operations_handlers.go` or create `internal/server/undo_handlers.go` — `startUndoOperation` handler
- Modify: `internal/server/server.go` — route `POST /api/v1/operations/:id/undo`
- Modify: `internal/server/server.go` — resume case for `undo_operation` in `resumeInterruptedOperations`
- Modify: `internal/operations/state.go` — add `UndoOperationParams` struct

- [ ] Handler creates operation, saves params `{target_operation_id, scope}`, enqueues
- [ ] Operation body calls `runUndoOperation`
- [ ] Resume case loads params + re-invokes (idempotent — already-reverted rows are skipped)
- [ ] Test via httptest: trigger undo, verify operation completes

---

### Task 5: Pre-flight conflict detection (1 PR)

**Files:**
- Modify: `internal/server/undo_engine.go` — add `preflightUndoConflicts(opID) ConflictReport`
- Create or modify: handler to return conflict report before executing

- [ ] Scan `operation_changes` rows, check each:
  - File mtime > created_at → "content changed"
  - Book soft-deleted since → "book deleted"
  - Book re-organized in a later op → "re-organized"
- [ ] Return counts per category
- [ ] Handler: `GET /api/v1/operations/:id/undo/preflight` → conflict report JSON

---

### Task 6: Frontend — Undo button + pre-flight dialog (1 PR)

**Files:**
- Modify: `web/src/components/layout/OperationsIndicator.tsx` — add "Undo" button on completed organize ops
- Create: `web/src/components/UndoPreflightDialog.tsx` — shows conflict counts, Proceed/Cancel
- Modify: `web/src/services/api.ts` — add `undoPreflight(opID)` and `startUndo(opID)` API calls

- [ ] Undo button visible when op type is in allowlist + has non-reverted changes
- [ ] Click → fetch preflight → show dialog → confirm → POST undo
- [ ] Progress shown via existing operation progress infra
- [ ] Post-undo: result summary + link to undo op detail

---

### Task 7: Torrent move_storage on undo (1 PR, deferred if no deluge integration yet)

**Files:**
- Modify: `internal/server/undo_engine.go` — after filesystem reversal, check `book_versions.torrent_hash` and call deluge `move_storage`

- [ ] Skip if deluge integration not yet wired (3.1 dependency)
- [ ] Retry 3x with backoff per the 3.1 convention

---

### Estimated effort

| Task | Size | Depends on |
|---|---|---|
| 1 (research) | XS | — |
| 2 (record changes) | S | 1 |
| 3 (undo engine) | M | 2 |
| 4 (tracked op) | S | 3 |
| 5 (pre-flight) | S | 3 |
| 6 (frontend) | M | 4+5 |
| 7 (deluge) | S | 3.1 centralization |
| **Total** | ~7 PRs, M overall | |
