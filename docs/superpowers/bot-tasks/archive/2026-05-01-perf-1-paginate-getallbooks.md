<!-- file: docs/superpowers/bot-tasks/2026-05-01-perf-1-paginate-getallbooks.md -->
<!-- version: 1.0.0 -->
<!-- guid: b7c8d9e0-f1a2-3b4c-5d6e-7f8a9b0c1d2e -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: PERF-1 — Paginate unbounded `GetAllBooks(0, 0)` calls in background jobs

**TODO ID:** PERF-1  
**Audience:** burndown bot  
**Branch:** `fix/perf-paginate-getallbooks`  
**PR title:** `fix(server): paginate unbounded GetAllBooks(0,0) calls in background jobs`

---

## What This Task Does

Replaces unbounded `GetAllBooks(0, 0)` calls (no limit) in background jobs with
cursor-based pagination (batch of 1,000 books, loop until empty). Loading all
books in one allocation is an OOM risk for libraries with 50,000+ books.

---

## What NOT to Do

- **Do NOT** change handler-level list endpoints — those have their own pagination.
- **Do NOT** change `GetAllBooks` signature in the `Store` interface.
- **Do NOT** apply pagination to callers that explicitly need all books atomically
  (e.g., migration scripts that need a consistent snapshot).
- **Do NOT** change the `PebbleStore` internal uses — focus on `internal/server/`
  and `internal/itunes/` background jobs.

---

## Target Call Sites

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -rn 'GetAllBooks(0, 0)\|GetAllBooks(100000\|GetAllBooks(0,0)' \
  --include='*.go' internal/server/ internal/itunes/ | grep -v test | head -20
```

Primary targets:
| File | Line | Context |
|------|------|---------|
| `internal/server/archive_sweep.go` | 27 | Background sweep job |
| `internal/server/audiobook_service.go` | 783 | `EnrichAudiobooksWithNames` |
| `internal/server/writeback_outbox.go` | 76 | Write-back outbox processor |
| `internal/server/acoustid_backfill.go` | 33 | AcoustID backfill goroutine |
| `internal/server/metadata_handlers.go` | 947, 1170, 1412 | Admin metadata handlers |
| `internal/itunes/service/importer.go` | 481, 813, 945 | iTunes import jobs |
| `internal/itunes/service/path_reconcile.go` | 111 | Path reconciliation |
| `internal/itunes/service/position_sync.go` | 72 | Position sync |

---

## Steps

### Step 1 — Create a helper function for paginated iteration

In a shared utility file (e.g., `internal/server/book_iter.go` or inline as a
closure), create a helper:

```go
// forEachBook calls fn for every book in the store, using cursor-based pagination
// to avoid loading all books into memory at once.
func forEachBook(ctx context.Context, store database.BookReader, batchSize int,
    fn func(b database.Book) error) error {
    if batchSize <= 0 {
        batchSize = 1000
    }
    for offset := 0; ; offset += batchSize {
        if ctx.Err() != nil {
            return ctx.Err()
        }
        books, err := store.GetAllBooks(batchSize, offset)
        if err != nil {
            return fmt.Errorf("GetAllBooks offset=%d: %w", offset, err)
        }
        if len(books) == 0 {
            return nil
        }
        for _, b := range books {
            if err := fn(b); err != nil {
                return err
            }
        }
    }
}
```

### Step 2 — Refactor each target call site

Replace patterns like:
```go
books, err := store.GetAllBooks(0, 0)
if err != nil { … }
for _, book := range books { … }
```

With:
```go
if err := forEachBook(ctx, store, 1000, func(book database.Book) error {
    // … existing loop body …
    return nil
}); err != nil { … }
```

Work through the target files one at a time. Read each function fully before
changing it.

### Step 3 — Handle progress reporting

Where the original code reported progress based on `len(books)` (total), update
to report based on processed count with an unknown total, or do a COUNT query
first:

```go
// If a progress count is needed:
total, _ := store.CountBooks() // if this method exists
```

### Step 4 — Build and test

```bash
go build ./...
go vet ./...
go test ./internal/server/... ./internal/itunes/... -timeout 120s 2>&1 | grep -E 'FAIL|ok'
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/perf-paginate-getallbooks
git add internal/server/ internal/itunes/
git commit -m "fix(server): paginate unbounded GetAllBooks(0,0) calls in background jobs

Replaces GetAllBooks(0, 0) calls in archive_sweep, writeback_outbox,
EnrichAudiobooksWithNames, acoustid_backfill, and iTunes service jobs
with cursor-based pagination (batch=1000). Prevents OOM for libraries
with 50k+ books. Re-audit finding R-7 / PERF-1.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/perf-paginate-getallbooks
gh pr create \
  --title "fix(server): paginate unbounded GetAllBooks(0,0) calls in background jobs" \
  --body "Prevents OOM by paginating background jobs that load all books at once. Re-audit finding R-7."
```

---

## Checklist

- [ ] `archive_sweep.go` uses paginated loop
- [ ] `writeback_outbox.go` uses paginated loop
- [ ] `EnrichAudiobooksWithNames` uses paginated loop
- [ ] `acoustid_backfill.go` uses paginated loop
- [ ] iTunes service jobs use paginated loop (importer, path_reconcile, position_sync)
- [ ] No remaining `GetAllBooks(0, 0)` in production server/itunes code
- [ ] `go build ./...` clean
- [ ] Tests pass
- [ ] PR opened with correct branch and title
