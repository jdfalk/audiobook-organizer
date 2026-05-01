<!-- file: docs/superpowers/specs/2026-04-30-book-query-projection.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3a4b5c6d-7e8f-9012-3456-789012345678 -->
<!-- last-edited: 2026-04-30 -->

# Book Query Projection — Summary Column Set for List Endpoints

**Status:** Draft — awaiting implementation
**Scope:** `internal/database/sqlite_store.go`
**Related specs:** [`2026-04-30-n1-query-elimination.md`](./2026-04-30-n1-query-elimination.md)

---

## Problem

**M-14 / N-14 — Over-selecting columns in list queries:**
`bookSelectColumns` selects all 50+ columns from the `books` table for every query,
including the list and search endpoints. This includes large text fields:

- `description` — often 500–2,000 characters per book
- `raw_metadata` — can be 10–50 KB of JSON per book

For a page of 50 books, this is up to 2.5 MB of unnecessary text transferred from
SQLite into Go memory, then discarded before serialization to the client (the list
endpoint does not return `description` or `raw_metadata`).

---

## Core Rule / Goal

> **List and search endpoints must select only the columns they actually return.
> Detail endpoints (getByID) continue to use the full column set.**

---

## Approach

### PROJ-1 — Add bookSummaryColumns constant and scanner

Add a new constant `bookSummaryColumns` to `sqlite_store.go` that contains only the
~15 columns needed for list responses:

```
id, title, author_name, narrator_name, series_name, series_position,
duration, cover_url, file_hash, metadata_source,
user_rating_overall, is_organized, created_at, updated_at
```

Add a corresponding `scanBookSummaryRow(rows *sql.Rows, book *Book) error` function
that scans exactly those columns in the same order.

Do NOT modify `bookSelectColumns` or `scanBookRow` — they remain for detail views.

### PROJ-2 — Wire summary columns into the list query

Change `GetBooks` (the primary list/search query) to use `bookSummaryColumns` and
`scanBookSummaryRow`. Leave `GetBookByID` and all other detail queries unchanged.

---

## What Does NOT Change

- The JSON response from list endpoints — the fields that were already returned
  continue to be returned.
- `GetBookByID`, `GetBookByFileHash`, and any single-book fetch — these continue
  to use the full column set.
- The `Book` struct — no fields are removed.

---

## Acceptance Criteria

- [ ] `bookSummaryColumns` constant exists and contains ~15 columns.
- [ ] `scanBookSummaryRow` function exists and scans exactly the summary columns.
- [ ] `GetBooks` (list query) uses `bookSummaryColumns`.
- [ ] `GetBookByID` still uses `bookSelectColumns`.
- [ ] `go test ./internal/database/...` passes.
- [ ] List endpoint response JSON is unchanged for the fields it previously returned.
- [ ] `go build ./...` is clean.

---

## Related Bot-Tasks

- [`2026-04-30-proj-1-summary-columns.md`](../bot-tasks/2026-04-30-proj-1-summary-columns.md) — PROJ-1
- [`2026-04-30-proj-2-list-query.md`](../bot-tasks/2026-04-30-proj-2-list-query.md) — PROJ-2
