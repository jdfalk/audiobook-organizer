<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1-migrate-itunes-path-field.md -->
<!-- version: 2.0.0 -->
<!-- guid: e4f5a6b7-c8d9-0e1f-2a3b-4c5d6e7f8a9b -->
<!-- last-edited: 2026-05-01 -->

# DEP-1 — Migrate deprecated `Book.ITunesPath` field: overview and sub-tasks

**Type:** Overview / Index
**Re-audit finding:** R-4 (2026-05-01)

---

## Background

Commit `af2ddae3` (2026-03-28) introduced the `book_files` table and the `BookFile`
struct as the per-file replacement for `book_segments`. Each `BookFile` has its own
`itunes_path STRING` column. At that point `Book.ITunesPath` was deprecated:

```go
// Deprecated: use book_files.itunes_path instead. Will be removed in a future migration.
ITunesPath *string `json:"itunes_path,omitempty"`
```

There are **~34 deprecated `Book.ITunesPath` usages** (staticcheck SA1019) across 13
production files. Three additional usages in `sqlite_store.go` are deferred — they
are the DB READ/WRITE path and can only be removed after a schema migration drops the
`books.itunes_path` column.

Note: many other usages of `.ITunesPath` in the codebase are on `BookFile` (not
`Book`) — those are the correct pattern and must NOT be changed.

---

## Sub-tasks (do in order — each is a separate PR)

| ID | Files | Deprecated usages |
|----|-------|------------------|
| [DEP-1a](2026-05-01-dep-1a-metafetch-itunes-path.md) | `internal/metafetch/batch.go`, `internal/metafetch/service.go` | ~9 |
| [DEP-1b](2026-05-01-dep-1b-organizer-itunes-path.md) | `internal/organizer/service.go` | 1 |
| [DEP-1c](2026-05-01-dep-1c-server-itunes-path.md) | `internal/server/itl_rebuild.go`, `internal/server/metadata_batch_candidates.go` | 6 |
| [DEP-1d](2026-05-01-dep-1d-itunes-service-path.md) | `internal/itunes/service/importer.go`, `path_reconcile.go`, `path_repair.go`, `writeback_batcher.go` | ~14 |
| DEP-1e (future) | `internal/database/sqlite_store.go` + schema migration | 3 + DROP COLUMN |

DEP-1e is blocked until DEP-1a through DEP-1d are all merged and
`staticcheck ./...` shows zero SA1019 ITunesPath warnings outside sqlite_store.go.

---

## Key rule for all sub-tasks

**Do NOT** remove `Book.ITunesPath` from the struct or the `books.itunes_path`
database column. Only change the call sites in application code. The struct field
and column stay until DEP-1e.
