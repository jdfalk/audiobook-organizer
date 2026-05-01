<!-- file: docs/superpowers/specs/2026-04-30-db-hygiene.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7f8e9d0c-1b2a-3948-5762-1234abcd5678 -->
<!-- last-edited: 2026-04-30 -->

# Database Hygiene

**Status:** Draft — awaiting implementation
**Scope:** `internal/database/`, `internal/server/ai_scan_pipeline.go`
**Related specs:** [`2026-04-30-context-propagation.md`](./2026-04-30-context-propagation.md)

---

## Problem

Six categories of database hygiene issues were identified in the 2026-04-30 audit:

**S-4 — Missing index on `books.file_hash`:**
`GetBookByFileHash` is called on every file scan to check for duplicates.
No index exists on `books.file_hash`, causing a full table scan for each call.
At 10,000 books this adds measurable scan latency.

**M-3 / N-8 — `db.Begin()` ignores context:**
~14 calls to `s.db.Begin()` in `sqlite_store.go` and 2 in `activity_store.go` use
the context-free form. If the caller's context is cancelled (client disconnect, server
shutdown) the transaction continues silently, wasting connections.

**M-4 — Silent error discards in AI scan pipeline:**
`internal/server/ai_scan_pipeline.go` uses `_ =` to discard errors from
`SavePhaseData`, `UpdateScanStatus`, `UpdateOperationStatus`, and `json.Marshal`.
These are swallowed entirely — no log, no metric.

**M-5 — `time.Parse` errors discarded in sqlite_store.go:**
Multiple `t, _ := time.Parse(...)` calls at lines ~4274, 4307, 4342, 4408.
Parse failures result in a silent zero-time, which can corrupt displayed timestamps.

**M-6 — Silent errors in pebble_store and sqlite_store:**
`_ = p.recomputeDurationMap(...)` at pebble line 4958, and two `_ = store.RecordPathChange(...)`
calls (pebble:1584, sqlite:2692) are silently discarded.

**N-12 — json.Marshal errors discarded in pipeline:**
Lines 334 and 359 of `ai_scan_pipeline.go` discard `json.Marshal` errors, writing
`nil` bytes to the store instead of a safe `[]byte("[]")` fallback.

---

## Core Rule / Goal

> **All database errors that are currently silently discarded must be logged.
> Index, context, and silent-error fixes must not change external API behaviour.**

---

## Approach

| Task | Change |
|------|--------|
| DB-1 | New migration: `CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash) WHERE file_hash IS NOT NULL` |
| DB-2 | Replace `s.db.Begin()` → `s.db.BeginTx(ctx, nil)` in `sqlite_store.go` |
| DB-3 | Replace `db.Begin()` → `db.BeginTx(ctx, nil)` in `activity_store.go` (2 call sites) |
| DB-4 | Replace `_ =` discards in `ai_scan_pipeline.go` with `if err != nil { log.Printf(...) }` |
| DB-5 | Replace `t, _ := time.Parse(...)` with `t, err := time.Parse(...)` + log on error |
| DB-6 | Replace `_ = p.recomputeDurationMap(...)` and `_ = store.RecordPathChange(...)` with logged error checks |

All changes are strictly additive — no return signatures change, no API contracts change.

---

## Acceptance Criteria

- [ ] `EXPLAIN QUERY PLAN SELECT * FROM books WHERE file_hash = ?` uses `idx_books_file_hash`.
- [ ] `go build ./...` is clean after all DB-* tasks.
- [ ] `go vet ./...` is clean.
- [ ] `grep -n 'db\.Begin()' internal/database/sqlite_store.go` returns 0 results.
- [ ] `grep -n 'db\.Begin()' internal/database/activity_store.go` returns 0 results.
- [ ] `grep -n '_ = pm\.' internal/server/ai_scan_pipeline.go` returns 0 results.
- [ ] `grep -n ', _) := time\.Parse' internal/database/sqlite_store.go` returns 0 results.

---

## Related Bot-Tasks

- [`2026-04-30-db-1-file-hash-index.md`](../bot-tasks/2026-04-30-db-1-file-hash-index.md) — DB-1
- [`2026-04-30-db-2-begin-tx-sqlite.md`](../bot-tasks/2026-04-30-db-2-begin-tx-sqlite.md) — DB-2
- [`2026-04-30-db-3-begin-tx-activity.md`](../bot-tasks/2026-04-30-db-3-begin-tx-activity.md) — DB-3
- [`2026-04-30-db-4-pipeline-errors.md`](../bot-tasks/2026-04-30-db-4-pipeline-errors.md) — DB-4
- [`2026-04-30-db-5-time-parse-errors.md`](../bot-tasks/2026-04-30-db-5-time-parse-errors.md) — DB-5
- [`2026-04-30-db-6-pebble-silent-errors.md`](../bot-tasks/2026-04-30-db-6-pebble-silent-errors.md) — DB-6
