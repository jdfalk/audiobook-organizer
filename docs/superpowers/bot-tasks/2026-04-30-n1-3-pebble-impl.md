<!-- file: docs/superpowers/bot-tasks/2026-04-30-n1-3-pebble-impl.md -->
<!-- version: 1.0.0 -->
<!-- guid: a5b6c7d8-e9f0-1234-abcd-567890123efa -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: N1-3 — PebbleStore: Implement Batch Author/Narrator Fetch

**TODO ID:** N1-3
**Audience:** burndown bot
**Branch:** `perf/n1-pebble-impl`
**PR title:** `perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in PebbleStore`

**Prerequisite:** N1-1 must be merged first (the interface methods must exist).

---

## What This Task Does

Implements `GetAuthorsByBookIDs` and `GetNarratorsByBookIDs` in `PebbleStore`.
PebbleDB is a key-value store, so this implementation iterates over keys with the
appropriate prefixes for each book ID in the input slice, returning the same
`map[string][]Author` shape as the SQLite implementation.

---

## What NOT to Do

- **Do NOT modify** the `Store` interface — that was done in N1-1.
- **Do NOT change** the existing single-book author/narrator lookup methods.
- **Do NOT use** a SQL query — PebbleDB has no SQL engine.
- **Do NOT modify** any server-layer files.

---

## Read First

1. `internal/database/pebble_store.go` — find the existing author lookup function
   (search for `GetBookAuthors` or similar). Study:
   - What key prefix is used for author entries? (e.g., `author:book:{bookID}:`, `book:{bookID}:author:`)
   - What value format is stored? (JSON? protobuf? a struct?)
   - How is the bookID embedded in the key?
   Find the same for narrator lookups.
2. `internal/database/store.go` or the N1-1 interface file — confirm the exact
   method signatures for `GetAuthorsByBookIDs` and `GetNarratorsByBookIDs`.

---

## Steps

### Step 1 — Study the existing single-book lookup

Read the existing `GetBookAuthors` (or equivalent) in `pebble_store.go` carefully.
It likely does something like:

```go
prefix := fmt.Sprintf("author:book:%s:", bookID)
iter := p.db.NewIter(&pebble.IterOptions{...})
for iter.SeekGE([]byte(prefix)); iter.Valid() && strings.HasPrefix(string(iter.Key()), prefix); iter.Next() {
    var author Author
    json.Unmarshal(iter.Value(), &author)
    authors = append(authors, author)
}
```

Understand this pattern before implementing the batch version.

### Step 2 — Implement GetAuthorsByBookIDs

The batch version loops over each bookID in the input and runs the same prefix
scan for each one, accumulating into the result map:

```go
func (p *PebbleStore) GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error) {
    if len(bookIDs) == 0 {
        return map[string][]Author{}, nil
    }
    result := make(map[string][]Author, len(bookIDs))
    for _, bookID := range bookIDs {
        authors, err := p.GetBookAuthors(ctx, bookID) // reuse existing single-book method
        if err != nil {
            return nil, fmt.Errorf("GetAuthorsByBookIDs: bookID %s: %w", bookID, err)
        }
        result[bookID] = authors
    }
    return result, nil
}
```

If `GetBookAuthors` doesn't accept a context parameter, pass `ctx` only if it does,
otherwise call it without ctx. Adapt to the actual signature.

### Step 3 — Implement GetNarratorsByBookIDs

Same pattern as Step 2 but for narrators.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
```

Must succeed.

### Step 5 — Commit and open PR

```bash
git checkout -b perf/n1-pebble-impl
git add internal/database/pebble_store.go
git commit -m "perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in PebbleStore

PebbleDB implementation reuses existing single-book prefix-scan per bookID,
returning the same map[bookID][]Author shape as the SQLite implementation.
Enables batch enrichment in N1-4.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/n1-pebble-impl
gh pr create \
  --title "perf(database): implement GetAuthorsByBookIDs and GetNarratorsByBookIDs in PebbleStore" \
  --body "PebbleDB implementation of batch author/narrator fetch from N1-1. Loops over bookIDs using existing prefix-scan. Depends on N1-1."
```

---

## Checklist

- [ ] `GetAuthorsByBookIDs` implemented in `pebble_store.go`
- [ ] `GetNarratorsByBookIDs` implemented in `pebble_store.go`
- [ ] Returns empty map (not nil) for empty input
- [ ] `go build ./...` passes
- [ ] Existing single-book lookup methods unchanged
- [ ] PR opened with correct branch and title
