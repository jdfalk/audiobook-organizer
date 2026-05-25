# Fix: Library 524 timeout — push `sort_by=title` down to memdb

## Goal

Library request `/api/v1/audiobooks?limit=20&offset=0&sort_by=title&sort_order=asc&is_primary_version=true` times out at 125s (524). Cause: service treats `sort_by=title` as a heavy post-filter → fetches ALL 68K books, sorts in-memory, then paginates. Memdb's `title` index is a sorted radix tree; iterate it directly, apply IsPrimary, stop at offset+limit.

## Affected files

- `internal/database/memdb_summaries.go` — extend `BookSummaryFilter` with `SortBy` + `SortAscending`; new title-index iteration path.
- `internal/audiobooks/service.go` — when SortBy=="title" with no other heavy filters, treat as pushdownable; forward sort params.

## Steps

1. Add `SortBy string` and `SortAscending bool` to `BookSummaryFilter`.
2. In `GetBookSummaries`, when SortBy=="title", use `txn.Get`/`ReverseGet` on `memIdxTitle`; reuse IsPrimary + pagination loop.
3. Plumb sort params through `GetAllBookSummariesFiltered` and `summariesPushdown`.
4. Update `LoadBooks` heavy-filter classification + cache key.
5. Unit test new path (asc/desc × IsPrimary).
6. `/ship` → verify library load < 500 ms.

## Test strategy

- `go test ./internal/database/... -run TestMemStore_GetBookSummaries`
- Manual prod hit: same URL, expect < 500 ms.

## Rollback

Revert PR. Previous behavior is slow but eventually 200; current 524 is the regression we're fixing.
