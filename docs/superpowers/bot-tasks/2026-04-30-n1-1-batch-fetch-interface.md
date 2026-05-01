<!-- file: docs/superpowers/bot-tasks/2026-04-30-n1-1-batch-fetch-interface.md -->
<!-- version: 1.0.0 -->
<!-- guid: e3f4a5b6-c7d8-9012-efab-345678901cde -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: N1-1 — Add Batch Author/Narrator Fetch to Store Interface

**TODO ID:** N1-1
**Audience:** burndown bot
**Branch:** `perf/n1-batch-fetch-interface`
**PR title:** `perf(database): add GetAuthorsByBookIDs and GetNarratorsByBookIDs to Store interface`

---

## What This Task Does

Adds two new methods to the `Store` interface and adds stub implementations to
`MockStore` so the mock satisfies the interface immediately. This is the foundation
for the N+1 query elimination.

**STOP after the interface + mock stubs.** Do NOT implement in SQLiteStore, PebbleStore,
or any server code. Those are N1-2, N1-3, and N1-4 respectively.

---

## What NOT to Do

- **Do NOT implement the methods** in `sqlite_store.go` or `pebble_store.go`.
- **Do NOT modify** any server-layer files (`server.go`, `audiobook_service.go`).
- **Do NOT add real SQL** in the mock — stub returns `nil, nil` only.
- **Do NOT delete** any existing author/narrator fetch methods.

---

## Read First

1. `internal/database/store.go` — find the `Store` interface definition. Note where
   existing author-related methods are defined (e.g., `GetBookAuthors`, `GetBookNarrators`).
2. Find the file where `Author` and `Narrator` types are defined — likely
   `internal/database/models.go` or similar. Confirm the struct field names.
3. `internal/database/mocks/mock_store.go` — understand how existing mock methods
   are structured (they use `m.Called(...)` and return typed results).

---

## Steps

### Step 1 — Add methods to the Store interface

Open the file containing the `Store` interface (likely `internal/database/store.go`
or the relevant `iface_*.go` file). Add the two new methods near the existing
author/narrator methods:

```go
// GetAuthorsByBookIDs returns a map from bookID → []Author for all given book IDs.
// Returns an empty map (not nil) if bookIDs is empty.
GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error)

// GetNarratorsByBookIDs returns a map from bookID → []Narrator for all given book IDs.
// Returns an empty map (not nil) if bookIDs is empty.
GetNarratorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Narrator, error)
```

Ensure `context.Context` is imported in that file (it likely already is).

### Step 2 — Add stub implementations to MockStore

Open `internal/database/mocks/mock_store.go`. Add two mock methods following the
exact pattern of existing methods in that file. The stubs return `nil, nil`:

```go
func (m *MockStore) GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]database.Author, error) {
    args := m.Called(ctx, bookIDs)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(map[string][]database.Author), args.Error(1)
}

func (m *MockStore) GetNarratorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]database.Narrator, error) {
    args := m.Called(ctx, bookIDs)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(map[string][]database.Narrator), args.Error(1)
}
```

Adjust the package/type qualifications to match what already exists in the mock file
(the mock may use `Author` directly if it's in the same package, or `database.Author`
if imported).

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
```

Must succeed with zero errors. If there are type errors about the `Author` or
`Narrator` types, check the exact type names in the database package and adjust.

### Step 4 — Commit and open PR

```bash
git checkout -b perf/n1-batch-fetch-interface
git add internal/database/store.go internal/database/mocks/mock_store.go
# (add the iface file if the methods were added there instead)
git commit -m "perf(database): add GetAuthorsByBookIDs and GetNarratorsByBookIDs to Store interface

Adds two new batch-fetch methods to the Store interface with stub MockStore
implementations. Real implementations follow in N1-2 (SQLite) and N1-3 (Pebble).
These methods will eliminate N+1 author/narrator queries in list endpoints.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/n1-batch-fetch-interface
gh pr create \
  --title "perf(database): add GetAuthorsByBookIDs and GetNarratorsByBookIDs to Store interface" \
  --body "Interface + mock stubs only. Implementations in follow-up PRs N1-2, N1-3, N1-4."
```

---

## Checklist

- [ ] `GetAuthorsByBookIDs` added to Store interface with correct signature
- [ ] `GetNarratorsByBookIDs` added to Store interface with correct signature
- [ ] `MockStore.GetAuthorsByBookIDs` stub added
- [ ] `MockStore.GetNarratorsByBookIDs` stub added
- [ ] `go build ./...` passes
- [ ] No SQLite or Pebble implementations added (those are N1-2 / N1-3)
- [ ] PR opened with correct branch and title
