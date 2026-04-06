# Bulk Metadata Review

## Summary

A workflow between fully-automatic metadata fetch and manual one-by-one search. Users select books, trigger a background operation that fetches the best metadata candidate for each book in parallel, then review all results in a dialog where they can apply/skip individually or in bulk.

## Problem

- Auto-fetch applies blindly — no human review
- Manual search is one-book-at-a-time — 100 books takes forever
- No middle ground for "fetch everything, let me review before applying"

## Design Decisions

- **Dialog-based** (not a new page) — fits existing UI patterns
- **Background operation** — fetch may take minutes for hundreds of books; tracked in activity page
- **Structured operation results** — candidates stored as JSON on operation results table
- **Parallel workers** with shared rate limiter — concurrent fetches without hitting API limits
- **Compact + two-column view** — user preference in settings, toggleable per-row

## Backend

### New Operation Type: `metadata_candidate_fetch`

Triggered by `POST /api/v1/metadata/batch-fetch-candidates` with body `{ book_ids: string[] }`.

Creates a standard operation record, then spawns parallel workers to fetch candidates.

### Parallel Fetch Workers

- Worker pool: configurable concurrency (default 8 goroutines)
- Workers pull book IDs from a buffered channel
- Each worker calls `SearchMetadataForBook` (existing function — queries Audible, Google Books, Open Library, Audnexus in priority order)
- Picks the top-scoring candidate per book
- Writes result to `operation_results` table
- Updates operation progress counter
- **Shared rate limiter**: all workers share a `golang.org/x/time/rate.Limiter` to avoid 429s from metadata sources. Global limiter: 10 requests/second across all workers. Per-source limiters layered on top: Audible 3 req/s, Google Books 5 req/s, Open Library 5 req/s. Workers acquire both the global and source-specific token before making a request. On 429/rate-limit response, the source limiter backs off (doubles wait time, capped at 30s).

If a source fails for one book, other sources are still tried. Partial results are fine — the worker records whatever it found (or `status: "no_match"` / `status: "error"`).

### Operation Results Storage

New table `operation_results`:

```sql
CREATE TABLE operation_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  operation_id TEXT NOT NULL,
  book_id TEXT NOT NULL,
  result_json TEXT NOT NULL,  -- structured JSON (see below)
  status TEXT NOT NULL,       -- "matched", "no_match", "error"
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_op_results_op ON operation_results(operation_id);
CREATE INDEX idx_op_results_book ON operation_results(operation_id, book_id);
```

`result_json` structure per row:

```json
{
  "book": {
    "id": "01KND...",
    "title": "Valor's Trial",
    "author": "Tanya Huff",
    "file_path": "/mnt/bigdata/books/...",
    "itunes_path": "W:\\...",
    "cover_url": "/api/v1/covers/local/...",
    "format": "m4b",
    "duration_seconds": 43200,
    "file_size_bytes": 1073741824
  },
  "candidate": {
    "title": "Valor's Trial",
    "author": "Tanya Huff",
    "narrator": "Marguerite Gavin",
    "series": "Confederation",
    "series_position": "4",
    "year": 2009,
    "publisher": "Podium Audio",
    "isbn": "",
    "asin": "B004...",
    "cover_url": "https://m.media-amazon.com/...",
    "description": "...",
    "source": "audible",
    "score": 1.98,
    "language": "en"
  },
  "error_message": ""
}
```

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/metadata/batch-fetch-candidates` | Start fetch operation. Body: `{ book_ids: [] }`. Returns `{ operation_id }`. |
| GET | `/api/v1/operations/:id/results` | Get structured results for review. Returns `{ results: [...], summary: { matched, no_match, errors, total } }`. |
| POST | `/api/v1/metadata/batch-apply-candidates` | Apply candidates for selected books. Body: `{ operation_id, book_ids: [] }`. DB updates inline, file I/O in background. Returns `{ applied: N }`. |

### Batch Apply Flow

`batch-apply-candidates` for each book_id:
1. Reads the stored candidate from `operation_results`
2. Calls `ApplyMetadataCandidate` (existing function — updates DB, downloads cover inline)
3. Kicks off `ApplyMetadataFileIO` + `WriteBackMetadataForBook` + `GlobalWriteBackBatcher.Enqueue` in a background goroutine
4. Returns immediately after all DB updates complete

### Resumability

Already-fetched results persist in `operation_results`. If the operation is cancelled or server restarts, the user can re-trigger for just the remaining books (the endpoint skips book_ids that already have results for the same operation).

## Frontend

### Trigger

New button in library batch actions toolbar: **"Fetch & Review Metadata"** (enabled when 2+ books selected).

Clicking it calls `POST /api/v1/metadata/batch-fetch-candidates`, shows a toast "Fetching metadata for N books...", and the operation appears in the nav operations dropdown with progress.

### Operations Dropdown Enhancement

The nav bar operations area shows:
- Active operations with progress bars (existing)
- **Last 5-10 completed operations**: type icon, timestamp (relative: "2m ago"), result summary, status chip
- Each row clickable → navigates to `/activity?op={id}`
- `metadata_candidate_fetch` operations show a **"Review Results"** button that opens the review dialog

### Review Dialog

Full-screen-ish dialog (maxWidth `lg` or `xl`).

**Top bar:**
- Title: "Review Metadata Matches — N books"
- Stats chips: "87 matched", "5 no match", "8 errors"
- Confidence slider: adjustable threshold (default 85%), filters the visible list
- Source filter chips: All | Audible (N) | Google Books (N) | Open Library (N)

**Smart action buttons:**
- "Apply High Confidence" — applies all matches above the confidence threshold that have a narrator match (audiobook indicator). Uses the slider value.
- "Apply All Visible" — applies everything currently shown (respects source filter + confidence slider)
- "Skip All Unmatched" — marks no_match and error rows as skipped

**List area:**
Scrollable, virtualized if >100 rows.

**Compact mode (default):**
One row per book:
- Small cover thumbnail (40x50)
- Current title → proposed title (with arrow or diff highlighting)
- Author, score badge, source chip
- Apply / Skip buttons
- Click row to expand to two-column

**Two-column mode:**
Card per book:
- Left: current book info (cover, title, author, format chip, file path, iTunes path, duration, has_cover chip)
- Right: proposed match (cover, title, author, narrator, series · Book N, year, publisher, score, source chip)
- Apply / Skip / "Select fields..." buttons

**Row states:**
- Pending: default appearance
- Applied: faded green tint, checkmark icon, Apply button shows "Applied" (disabled)
- Skipped: faded gray, dimmed text
- Error: red border, error message shown

**Bulk select + apply:**
Checkbox per row. "Apply Selected" button appears when any are checked. Applies all checked rows in one API call. Toast with "Undo All" action (calls `undoLastApply` for each applied book).

### Settings

New user preference:
- `metadata_review_default_view`: `"compact"` | `"two-column"` (default: `"compact"`)

Added to config update service bool/string fields.

## Migration

Migration 45:
```sql
CREATE TABLE operation_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  operation_id TEXT NOT NULL,
  book_id TEXT NOT NULL,
  result_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'matched',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_op_results_op ON operation_results(operation_id);
CREATE INDEX idx_op_results_book ON operation_results(operation_id, book_id);
```

## Files to Create/Modify

### New Files
- `internal/server/metadata_batch_candidates.go` — operation handler, parallel workers, rate limiter, batch-apply endpoint
- `web/src/components/audiobooks/MetadataReviewDialog.tsx` — the review dialog
- `web/src/components/common/OperationsDropdown.tsx` — nav dropdown showing recent + active operations

### Modified Files
- `internal/database/migrations.go` — migration 45
- `internal/database/store.go` — Store interface: `CreateOperationResult`, `GetOperationResults`, `GetRecentOperations`
- `internal/database/sqlite_store.go` — implementations
- `internal/database/pebble_store.go` — stubs
- `internal/database/mock_store.go` + `mocks/mock_store.go` — stubs
- `internal/server/server.go` — register new endpoints, add operations dropdown API
- `web/src/pages/Library.tsx` — add "Fetch & Review Metadata" button, wire dialog
- `web/src/services/api.ts` — new API functions
- `web/src/components/layout/` (or wherever nav lives) — operations dropdown

## Out of Scope

- Editing candidates before apply (user can use the existing single-book search for that)
- Re-fetching with different search terms from the review dialog
- Comparing multiple candidates per book (only top match shown)
- Cover art preview zoom in review dialog
