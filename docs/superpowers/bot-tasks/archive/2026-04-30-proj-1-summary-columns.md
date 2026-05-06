<!-- file: docs/superpowers/bot-tasks/2026-04-30-proj-1-summary-columns.md -->
<!-- version: 1.0.0 -->
<!-- guid: b4c5d6e7-f8a9-0123-bcde-456789012fa3 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: PROJ-1 — Define BookSummary DB Columns

**TODO ID:** PROJ-1
**Audience:** burndown bot
**Branch:** `perf/book-summary-columns`
**PR title:** `perf(database): define BookSummary struct for list query projection`

---

## What This Task Does

Defines a `BookSummary` struct in `internal/database/` that contains only the
columns needed for the library list view — as opposed to fetching the full `Book`
struct (which includes body text, cover image data, and other heavy fields). This
is the prerequisite for PROJ-2.

---

## What NOT to Do

- **Do NOT remove** the existing `Book` struct or any existing queries.
- **Do NOT add** the SQL query yet — that is PROJ-2.
- **Do NOT include** fields in `BookSummary` that are not needed for the list UI
  (e.g., full description body, raw cover bytes, embeddings).
- **Do NOT add** fields to the Store interface yet — that is PROJ-2.

---

## Read First

1. Find the `Book` struct definition:

```bash
grep -n 'type Book struct' internal/database/ -r | head -5
```

2. Read the full `Book` struct. Note which fields are heavy (large text/blob).
3. Read the list handler response:

```bash
grep -n 'GetBooks\|ListBooks\|GetAudiobooks\|AudiobookList' \
  internal/server/server.go internal/server/audiobook_service.go | head -20
```

4. Read the frontend `LibraryPage` or `BookCard` component to see exactly which
   fields it uses (title, author, cover thumbnail URL, duration, progress, etc.).

---

## Steps

### Step 1 — Identify required summary fields

From the frontend list view, the minimum fields needed are typically:
- `ID string`
- `Title string`
- `Author string` (or join via `BookSummaryAuthors []string`)
- `CoverURL string` (a URL, not raw bytes)
- `Duration float64` or `int`
- `ProgressPercent float64` (if stored in the DB)
- `AddedAt time.Time`
- `FileSize int64`

Add or remove based on what the actual frontend list view uses.

### Step 2 — Define the struct

In `internal/database/models.go` (or wherever `Book` is defined), add:

```go
// BookSummary contains only the fields needed for library list views.
// Use GetBookSummaries for paginated list queries instead of GetBooks.
type BookSummary struct {
    ID              string    `db:"id"`
    Title           string    `db:"title"`
    Author          string    `db:"author"`          // comma-joined or first author
    CoverURL        string    `db:"cover_url"`
    DurationSeconds int       `db:"duration_seconds"`
    ProgressPct     float64   `db:"progress_pct"`
    AddedAt         time.Time `db:"added_at"`
    FileSize        int64     `db:"file_size"`
}
```

Adjust field names to match the actual DB column names (use `db:` tags that match
the `audiobooks` table).

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
```

No tests should be needed for a struct definition.

### Step 4 — Commit and open PR

```bash
git checkout -b perf/book-summary-columns
git add internal/database/
git commit -m "perf(database): define BookSummary struct for list query projection

Adds BookSummary with only the columns needed for library list views.
This is the foundation for the projected list query in PROJ-2,
which avoids fetching large text/blob fields on every page load.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/book-summary-columns
gh pr create \
  --title "perf(database): define BookSummary struct for list query projection" \
  --body "Adds lightweight BookSummary struct. Prerequisite for PROJ-2 projected list query."
```

---

## Checklist

- [ ] `BookSummary` struct defined in `internal/database/`
- [ ] Struct contains only list-view fields (no heavy text/blob fields)
- [ ] DB column tags match the `audiobooks` table columns
- [ ] Existing `Book` struct is unchanged
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] PR opened with correct branch and title
