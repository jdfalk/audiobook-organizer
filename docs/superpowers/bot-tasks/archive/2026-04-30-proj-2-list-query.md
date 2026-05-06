<!-- file: docs/superpowers/bot-tasks/2026-04-30-proj-2-list-query.md -->
<!-- version: 1.0.0 -->
<!-- guid: c5d6e7f8-a9b0-1234-cdef-567890123ab4 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: PROJ-2 — Implement GetBookSummaries Projected List Query

**TODO ID:** PROJ-2
**Audience:** burndown bot
**Branch:** `perf/book-list-summary-query`
**PR title:** `perf(database): implement GetBookSummaries projected list query`

**Prerequisite:** PROJ-1 must be merged first.

---

## What This Task Does

Adds `GetBookSummaries(ctx, filter, page, pageSize)` to the `Store` interface and
SQLite implementation. This query SELECTs only the `BookSummary` columns (not
`SELECT *`). Updates the list handler to call `GetBookSummaries` instead of
`GetBooks`.

---

## What NOT to Do

- **Do NOT remove** `GetBooks` — it is still needed for single-book detail views.
- **Do NOT change** the JSON response shape of the list endpoint.
- **Do NOT use** `SELECT *` in the new query.
- **Do NOT add** cover image data to the summary query.

---

## Read First

1. `internal/database/store.go` — read the `Store` interface definition. Find
   `GetBooks` (or `GetAudiobooks`). Read its signature and how pagination/filtering
   is done.
2. `internal/database/sqlite_store.go` — read the `GetBooks` implementation. Note
   the SQL query.
3. `internal/server/server.go` — find the list handler that calls `GetBooks`.
   Understand how the response is assembled.

---

## Steps

### Step 1 — Add GetBookSummaries to the Store interface

In `internal/database/store.go`, add:
```go
GetBookSummaries(ctx context.Context, filter BookFilter, page, pageSize int) ([]BookSummary, int, error)
```

Where `BookFilter` is the existing filter struct (or equivalent).

### Step 2 — Implement in sqlite_store.go

```go
func (s *SQLiteStore) GetBookSummaries(
    ctx context.Context, filter BookFilter, page, pageSize int,
) ([]BookSummary, int, error) {
    const q = `
        SELECT
            id, title,
            COALESCE((SELECT GROUP_CONCAT(name) FROM authors a
                      JOIN book_authors ba ON ba.author_id = a.id
                      WHERE ba.book_id = b.id LIMIT 1), '') AS author,
            cover_url, duration_seconds, progress_pct, added_at, file_size
        FROM audiobooks b
        WHERE /* apply filter conditions here */
        ORDER BY added_at DESC
        LIMIT ? OFFSET ?`
    
    // ... build WHERE clause from filter (copy the pattern from GetBooks)
    // ... scan rows into []BookSummary
    // ... return total count from a separate COUNT(*) query
}
```

Copy the filter-building logic from `GetBooks` — only the SELECT list changes.

### Step 3 — Update the list handler

In `internal/server/server.go` (or `audiobook_service.go`), find the paginated list
handler. Replace the call:
```go
// Before:
books, total, err := s.store.GetBooks(ctx, filter, page, pageSize)

// After:
books, total, err := s.store.GetBookSummaries(ctx, filter, page, pageSize)
```

Update the response assembly to use `BookSummary` fields. Verify the JSON field
names match what the frontend expects.

### Step 4 — Update mock (if using mockery)

If `internal/database/mock_store.go` exists, add the mock method:
```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go generate ./internal/database/...
# or manually add the mock method following the existing pattern
```

### Step 5 — Verify

```bash
go build ./...
go vet ./...
go test ./internal/database/... ./internal/server/... -v 2>&1 | tail -30
```

### Step 6 — Commit and open PR

```bash
git checkout -b perf/book-list-summary-query
git add internal/database/ internal/server/
git commit -m "perf(database): implement GetBookSummaries projected list query

Adds GetBookSummaries that SELECTs only the 8 summary columns instead
of all columns + joins. Updates the paginated list handler to use it.
Detail endpoint still uses GetBooks. Reduces per-page data transfer
from ~40 columns to 8.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/book-list-summary-query
gh pr create \
  --title "perf(database): implement GetBookSummaries projected list query" \
  --body "Implements projected SELECT for list view. Uses BookSummary (PROJ-1). Reduces data per page. Depends on PROJ-1."
```

---

## Checklist

- [ ] `GetBookSummaries` added to `Store` interface
- [ ] SQLite implementation uses projected SELECT (no `SELECT *`)
- [ ] List handler uses `GetBookSummaries`
- [ ] Detail/single-book handler still uses `GetBooks`
- [ ] Mock updated (if using mockery)
- [ ] JSON response shape of list endpoint unchanged
- [ ] `go build ./...` passes
- [ ] `go test ./internal/database/... ./internal/server/...` passes
- [ ] PR opened with correct branch and title
