<!-- file: docs/superpowers/specs/2026-04-30-n1-query-elimination.md -->
<!-- version: 1.0.0 -->
<!-- guid: a9b8c7d6-e5f4-3210-fedc-ba9876543210 -->
<!-- last-edited: 2026-04-30 -->

# N+1 Query Elimination — Batch Author/Narrator Fetch

**Status:** Draft — awaiting implementation
**Scope:** `internal/database/`, `internal/server/server.go`, `internal/server/audiobook_service.go`
**Related specs:** [`2026-04-30-db-hygiene.md`](./2026-04-30-db-hygiene.md)

---

## Problem

Two hot-path functions make per-book DB round-trips inside list/search response handlers:

1. **`enrichBookForResponse`** (`internal/server/server.go:334–406`) — called in a loop
   over every book on a page. Makes 4–5 DB calls per book (authors, narrators, series,
   tags, cover).
2. **`EnrichAudiobooksWithNames`** (`internal/server/audiobook_service.go:782`) — loops
   with per-book author/narrator DB calls.

At 1,000 books per page with 2 authors each: ≈ 4,000 DB round-trips per request.
Even with connection pooling this is a 10–100× latency hit vs. batch queries.

---

## Core Rule / Goal

> **List and search endpoints must fetch authors/narrators in one query per page,
> not one query per book.**

The JSON response shape must not change — only the number of DB round-trips.

---

## Approach

### Step 1 — N1-1: Extend the Store interface

Add two new methods to the `Store` interface in `internal/database/store.go` (or
the relevant interface file):

```go
// GetAuthorsByBookIDs returns a map from bookID → []Author for all given book IDs.
GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error)
// GetNarratorsByBookIDs returns a map from bookID → []Narrator for all given book IDs.
GetNarratorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Narrator, error)
```

Add stub implementations to `MockStore` (`return nil, nil`) so the mock satisfies the
interface immediately. Do NOT implement in the real stores yet.

### Step 2 — N1-2: SQLiteStore implementation

Implement both methods in `internal/database/sqlite_store.go` using a single SQL
`IN (?)` query expanded with `sqlx.In` or manual `strings.Repeat` placeholder
building. Group results by book ID and return a `map[string][]Author`.

### Step 3 — N1-3: PebbleStore implementation

Implement both methods in `internal/database/pebble_store.go` using the existing
per-book iteration pattern, but loop over all book IDs together and populate the
same `map[string][]Author` return type.

### Step 4 — N1-4: Wire into enrichBookForResponse and EnrichAudiobooksWithNames

At the list-response call site, before the per-book enrichment loop:

1. Collect all book IDs for the current page into `[]string`.
2. Call `GetAuthorsByBookIDs(ctx, ids)` and `GetNarratorsByBookIDs(ctx, ids)` once.
3. Pass the resulting maps into `enrichBookForResponse` (change its signature or use
   a closure) so it reads from the pre-fetched map instead of hitting the DB.
4. Apply the same fix to `EnrichAudiobooksWithNames`.

---

## Acceptance Criteria

- [ ] `GetAuthorsByBookIDs` and `GetNarratorsByBookIDs` exist on the `Store` interface.
- [ ] SQLiteStore implementation passes `go test ./internal/database/...`.
- [ ] PebbleStore implementation compiles cleanly.
- [ ] `enrichBookForResponse` no longer calls per-book author/narrator DB lookups.
- [ ] `EnrichAudiobooksWithNames` no longer loops with per-book DB calls.
- [ ] JSON response from `GET /api/v1/audiobooks` is unchanged.
- [ ] `go test ./internal/server/...` passes.
- [ ] `go vet ./...` is clean.

---

## Related Bot-Tasks

- [`2026-04-30-n1-1-batch-fetch-interface.md`](../bot-tasks/2026-04-30-n1-1-batch-fetch-interface.md) — N1-1
- [`2026-04-30-n1-2-sqlite-impl.md`](../bot-tasks/2026-04-30-n1-2-sqlite-impl.md) — N1-2
- [`2026-04-30-n1-3-pebble-impl.md`](../bot-tasks/2026-04-30-n1-3-pebble-impl.md) — N1-3
- [`2026-04-30-n1-4-enrich-response.md`](../bot-tasks/2026-04-30-n1-4-enrich-response.md) — N1-4
