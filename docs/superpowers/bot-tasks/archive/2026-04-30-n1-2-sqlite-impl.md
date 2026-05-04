<!-- file: docs/superpowers/bot-tasks/2026-04-30-n1-2-sqlite-impl.md -->
<!-- version: 1.0.0 -->
<!-- guid: f4a5b6c7-d8e9-0123-fabc-456789012def -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: N1-2 — SQLiteStore: Implement Batch Author/Narrator Fetch

**TODO ID:** N1-2
**Audience:** burndown bot
**Branch:** `perf/n1-sqlite-impl`
**PR title:** `perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in SQLiteStore`

**Prerequisite:** N1-1 must be merged first (the interface methods must exist).

---

## What This Task Does

Implements `GetAuthorsByBookIDs` and `GetNarratorsByBookIDs` in `SQLiteStore`
using a single SQL `IN (?)` query per method. Each returns a
`map[string][]Author` (or `[]Narrator`) grouped by book ID.

---

## What NOT to Do

- **Do NOT modify** the `Store` interface — that was done in N1-1.
- **Do NOT change** the existing `GetBookAuthors` or `GetBookNarrators` methods —
  they remain for single-book use.
- **Do NOT use** a loop of per-book queries — the whole point is one query for all IDs.
- **Do NOT modify** any server-layer files.

---

## Read First

1. `internal/database/sqlite_store.go` — find the existing `GetBookAuthors` function.
   Read its SQL query carefully to understand:
   - The join table name (likely `book_authors`)
   - The `authors` table schema (what columns exist)
   - How `book_id` relates to author rows
   Do the same for `GetBookNarrators` (join table likely `book_narrators`).
2. `internal/database/store.go` (or the interface file from N1-1) — confirm the exact
   method signatures.
3. `go.mod` — check if `github.com/jmoiron/sqlx` is a dependency. If yes, you can use
   `sqlx.In`. If not, build the IN placeholder manually.

---

## Steps

### Step 1 — Build the IN placeholder

If `sqlx` is available, use `sqlx.In`:
```go
query, args, err := sqlx.In(`SELECT ba.book_id, a.* FROM book_authors ba JOIN authors a ON ba.author_id = a.id WHERE ba.book_id IN (?)`, bookIDs)
if err != nil { return nil, err }
query = s.db.Rebind(query)
rows, err := s.db.QueryContext(ctx, query, args...)
```

If `sqlx` is NOT available, build placeholders manually:
```go
placeholders := strings.Repeat("?,", len(bookIDs))
placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
query := fmt.Sprintf(
    "SELECT ba.book_id, a.id, a.name FROM book_authors ba JOIN authors a ON ba.author_id = a.id WHERE ba.book_id IN (%s)",
    placeholders,
)
args := make([]interface{}, len(bookIDs))
for i, id := range bookIDs { args[i] = id }
rows, err := s.db.QueryContext(ctx, query, args...)
```

### Step 2 — Implement GetAuthorsByBookIDs

Add the following method to `sqlite_store.go` (adapt schema details to match what
you found in Step 1 of Read First):

```go
func (s *SQLiteStore) GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error) {
    if len(bookIDs) == 0 {
        return map[string][]Author{}, nil
    }
    // build query with IN placeholder (see Step 1 above)
    // ...
    result := make(map[string][]Author, len(bookIDs))
    for rows.Next() {
        var bookID string
        var author Author
        if err := rows.Scan(&bookID, &author.ID, &author.Name /*, other fields */); err != nil {
            return nil, err
        }
        result[bookID] = append(result[bookID], author)
    }
    return result, rows.Err()
}
```

Scan exactly the same columns that `GetBookAuthors` scans for individual books.

### Step 3 — Implement GetNarratorsByBookIDs

Same pattern as Step 2 but for narrators. Use the `book_narrators` join table and
`narrators` table (confirm exact names from the existing `GetBookNarrators` query).

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go test ./internal/database/... -v 2>&1 | tail -20
go build ./...
```

Both must succeed.

### Step 5 — Commit and open PR

```bash
git checkout -b perf/n1-sqlite-impl
git add internal/database/sqlite_store.go
git commit -m "perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in SQLiteStore

Uses a single IN(?) query per method to batch-fetch author/narrator data
for multiple books. Returns map[bookID][]Author to enable O(1) lookup in
the enrichment layer. Eliminates N per-book queries for list endpoints.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/n1-sqlite-impl
gh pr create \
  --title "perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in SQLiteStore" \
  --body "SQLite implementation of the batch author/narrator fetch interface from N1-1. Single IN() query per method. Depends on N1-1."
```

---

## Checklist

- [ ] `GetAuthorsByBookIDs` implemented in `sqlite_store.go` with IN query
- [ ] `GetNarratorsByBookIDs` implemented in `sqlite_store.go` with IN query
- [ ] Returns empty map (not nil) for empty input
- [ ] `go test ./internal/database/...` passes
- [ ] `go build ./...` passes
- [ ] Existing `GetBookAuthors` / `GetBookNarrators` unchanged
- [ ] PR opened with correct branch and title
