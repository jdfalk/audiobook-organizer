<!-- file: docs/superpowers/bot-tasks/2026-04-27-bleve-task7-cleanup.md -->
<!-- version: 1.0.0 -->
<!-- guid: 02a4c57b-f156-4064-e172-456724f1becd -->

# BOT TASK: Bleve Task 7 — Remove FTS5 + LIKE Path

**TODO ID:** DES-1-T7
**Companion human design:** [`docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md`](../specs/2026-04-27-bleve-task7-cleanup-design.md)

## Branch

```
chore/bleve-task7-cleanup
```

## Files

1. `internal/database/sqlite_store.go` — delete the old `SearchBooks` body (lines ~2810–2900, verify with `grep -n "func.*SearchBooks" internal/database/sqlite_store.go`)
2. `internal/database/pebble_store.go` — delete the old `SearchBooks` body
3. `internal/database/migrations.go` — append a new migration that drops `books_fts`
4. `internal/database/sqlite_store.go` — delete `sanitizeFTS5Query` (line ~2907) — only callers were inside the deleted SearchBooks
5. `internal/server/...` (search) — verify no callers of `SearchBooks` remain that bypass the bleve path

## Step 1 — Confirm callers

```
grep -rn "\.SearchBooks(" --include="*.go" internal/
```

Every caller must already route through the bleve indexed-store decorator. If a caller bypasses bleve (calls store.SearchBooks directly without going through the search service), STOP — that's a regression risk and needs human review.

## Step 2 — Delete the SQLite body

In `internal/database/sqlite_store.go`, the `SearchBooks` method around line 2810 contains:
- `ftsQuery := sanitizeFTS5Query(query)` block
- `books_fts MATCH ?` query
- `LIKE` UNION fallback

Replace the whole method body with:

```go
// SearchBooks: legacy search path removed in DES-1-T7. The bleve indexed-store
// decorator (internal/search/indexedstore) now serves all search queries.
// Direct calls to this method indicate a missed wiring — return a clear error.
func (s *SQLiteStore) SearchBooks(query string, limit int) ([]*Book, error) {
    return nil, fmt.Errorf("SearchBooks: legacy path removed; route through search.IndexedStore")
}
```

Bump the file version header.

## Step 3 — Delete `sanitizeFTS5Query`

Same file, around line 2907. Function only existed for the legacy path. Delete it.

## Step 4 — Delete the Pebble body

In `internal/database/pebble_store.go`, find `SearchBooks` and apply the same treatment as Step 2. Same error message.

## Step 5 — `fuzzyRankBooks` audit

```
grep -rn "fuzzyRankBooks\|FuzzyRankBooks" --include="*.go" internal/
```

If the only callers were the deleted SearchBooks bodies, delete `fuzzyRankBooks` too. If other callers exist (e.g. the path-repair tier-C resolver), leave it.

`matcher.ScoreMatch` should NOT be deleted — `path_repair_resolver.go` uses it (verified — see `internal/itunes/service/path_repair_resolver.go`).

## Step 6 — Drop the FTS5 virtual table (migration)

In `internal/database/migrations.go`, find the highest existing migration number (likely 53 or 54 after the cache_stats_history work). Append the next migration:

```go
{
    Version:     <NEXT>,
    Description: "Drop legacy books_fts FTS5 virtual table (Bleve replaces it)",
    Up: func(tx *sql.Tx) error {
        _, err := tx.Exec(`DROP TABLE IF EXISTS books_fts`)
        return err
    },
    Down: func(tx *sql.Tx) error {
        // Recreating the FTS5 table is migration 17's job — caller can revert
        // to that migration version if they need the index back.
        return nil
    },
},
```

Use the exact struct shape that other migrations in this file use. Read 2–3 nearby migrations first.

## Step 7 — Verify

```
go vet ./...
make test
make ci
```

After `make test`, run a manual search smoke (against a test database):

```
# Ensure the search service still serves queries (uses bleve path)
curl -s 'localhost:8484/api/v1/search?q=test' -H "Authorization: Bearer $TOKEN" | jq .
```

Skip the curl if the bot can't run a live server — the tests should cover it.

## Step 8 — Commit

```
chore(search): remove legacy FTS5 + LIKE path (Bleve task 7, DES-1-T7)

- Deletes ~200 LOC of dead SearchBooks bodies in sqlite_store.go and
  pebble_store.go. Both methods now return a clear error if anyone
  hits them directly.
- Removes sanitizeFTS5Query (only used by the deleted path).
- Migration <N> drops the books_fts virtual table on existing SQLite
  installs.
- fuzzyRankBooks left in place (matcher.ScoreMatch still used by
  iTunes path-repair tier C).

Spec: docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md
```

## Definition of done

- [ ] `grep -rn "books_fts\|FTS5\|fts5" internal/database/ | grep -v _test.go` shows only the migration that drops it
- [ ] `grep -rn "sanitizeFTS5Query" internal/` returns nothing
- [ ] Both `SearchBooks` methods are 3-line stubs that return an error
- [ ] Migration applied in test runs cleanly (`make test` includes migration roundtrip)
- [ ] `make ci` green
- [ ] CHANGELOG prepended under `## [Unreleased]`
- [ ] TODO.md `DES-1-T7` flipped to `[x]`; the `⏳` Bleve plan in TODO.md changes to `[x]`
- [ ] CHANGELOG entry calls out: "Existing SQLite installs gain ~10–20% disk space back after the migration runs."

## When to STOP

NEEDS_REVIEW if:

- A `SearchBooks` caller exists that doesn't go through the bleve indexed-store. Don't paper over it; surface for human review.
- The migration system has changed shape since other migrations were written and the new struct doesn't match. Surface; don't guess.
