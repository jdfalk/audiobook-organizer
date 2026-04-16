# Replace GlobalStore with DI — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task. Each task is one PR. Merge each before starting the next.

**Spec:** `docs/superpowers/specs/2026-04-15-replace-globalstore-with-di-design.md`
**Depends on:** Nothing. This is the foundation — do first.
**Unblocks:** 3.7 Multi-user (needs request-scoped state on Server)

---

### Task 1: Bootstrap — add Server.store field (1 PR)

**Goal:** Dual-live: GlobalStore still exists, Server also holds a store reference.

**Files:**
- Modify: `internal/server/server.go` — add `store database.Store` field, `Store()` accessor, update `NewServer()` constructor
- Modify: `cmd/root.go` — pass store into `NewServer()` after `initializeStore()`

- [ ] Add `store database.Store` field to `Server` struct
- [ ] Add `func (s *Server) Store() database.Store` accessor
- [ ] Update every `NewServer()` call site to pass the store
- [ ] Keep `database.GlobalStore` assigned at startup (dual-live)
- [ ] Verify: `go build ./...`, `go test ./internal/server/` — no behavior change

---

### Task 2: Migrate audiobooks + metadata handlers (1 PR)

**Files:**
- Modify: `internal/server/audiobooks_handlers.go` (~53 references)
- Modify: `internal/server/metadata_handlers.go`
- Modify: `internal/server/metadata_batch_candidates.go`
- Modify: `internal/server/metadata_fetch_service.go`
- Modify: `internal/server/versions_handlers.go`
- Modify: matching `*_test.go` files — replace `database.GlobalStore = mock` with `srv := &Server{store: mock}`

- [ ] Replace `database.GlobalStore` with `s.Store()` in each file
- [ ] Convert tests to construct Server with mock store
- [ ] Verify zero `database.GlobalStore` references remain in these files

---

### Task 3: Migrate organize + reconcile + maintenance handlers (1 PR)

**Files:**
- Modify: `internal/server/organize_handlers.go`
- Modify: `internal/server/organize_service.go`
- Modify: `internal/server/reconcile.go`
- Modify: `internal/server/maintenance_fixups.go`
- Modify: `internal/server/itunes_path_reconcile.go`
- Modify: matching `*_test.go` files

- [ ] Replace `database.GlobalStore` with `s.Store()` in each file
- [ ] Convert tests
- [ ] Verify zero references

---

### Task 4: Migrate auth + user tags + entities + system handlers (1 PR)

**Files:**
- Modify: `internal/server/auth_handlers.go` (~15 references)
- Modify: `internal/server/user_tags.go`
- Modify: `internal/server/entities_handlers.go`
- Modify: `internal/server/system_handlers.go`
- Modify: `internal/server/operations_handlers.go`
- Modify: matching `*_test.go` files

- [ ] Replace `database.GlobalStore` with `s.Store()` in each file
- [ ] Convert tests

---

### Task 5: Migrate AI + dedup + diagnostics + filesystem handlers (1 PR)

**Files:**
- Modify: `internal/server/ai_handlers.go`
- Modify: `internal/server/dedup_handlers.go`
- Modify: `internal/server/diagnostics_handlers.go`
- Modify: `internal/server/duplicates_handlers.go`
- Modify: `internal/server/filesystem_handlers.go`
- Modify: `internal/server/file_ops_handlers.go`
- Modify: matching `*_test.go` files

- [ ] Replace `database.GlobalStore` with `s.Store()` in each file
- [ ] Convert tests

---

### Task 6: Migrate remaining server-internal files (1 PR)

**Files:**
- Modify: `internal/server/batch_poller.go`
- Modify: `internal/server/embedding_backfill.go`
- Modify: `internal/server/external_id_backfill.go`
- Modify: `internal/server/itl_rebuild.go`
- Modify: `internal/server/itunes_writeback_batcher.go`
- Modify: `internal/server/itunes.go`
- Modify: `internal/server/openlibrary_service.go`
- Modify: `internal/server/scheduler.go`

- [ ] Replace `database.GlobalStore` with `s.Store()` in each file
- [ ] These files are mostly called from Server methods already, so the store is reachable via the receiver

---

### Task 7: Migrate non-handler packages (3 PRs)

**7a: scanner**
- Modify: `internal/scanner/scanner.go` — add `store` field to Scanner, accept via constructor
- Modify: `cmd/root.go` — pass store when creating scanner

**7b: organizer**
- Modify: `internal/organizer/organizer.go` — accept store as constructor arg if it references GlobalStore directly

**7c: backup**
- Modify: `internal/backup/backup.go` — accept store as constructor arg

- [ ] Each package gets its own PR
- [ ] Update callers to pass store explicitly

---

### Task 8: Delete GlobalStore (1 PR)

**Files:**
- Modify: `internal/database/store.go` — remove `var GlobalStore Store`
- Modify: `internal/database/web.go` — remove package-level helpers that delegate to GlobalStore
- Modify: `cmd/root.go` — `InitializeStore` returns a `Store` instead of assigning to GlobalStore

- [ ] Delete `var GlobalStore Store` declaration
- [ ] Compile — any surviving reference is a compile error, fix it
- [ ] Run full test suite
- [ ] No linter needed — deletion is the enforcement

---

### Estimated effort

| Task | Size | Risk |
|---|---|---|
| 1 (bootstrap) | S | Low — additive only |
| 2-6 (handler migration) | M each | Low — mechanical search-replace + test rewrite |
| 7a-c (packages) | S each | Low — constructor change |
| 8 (delete global) | S | Medium — surfaces any missed references |
| **Total** | ~10 PRs | Biggest risk is merge conflicts during migration |

### Testing strategy

- Each PR runs `go test ./...` before merge
- Task 8 enforces completeness via compile error
- After task 8: verify `t.Parallel()` works in server tests (was impossible with GlobalStore mutation)
