# Series Name Normalization

**Date:** 2026-04-27
**Status:** Approved

## Problem

Series names in PebbleDB are contaminated from multiple sources (file tags, folder
structure, Audible, Google Books, Open Library). Two distinct failure modes:

1. **Embedded title/position** — the series field contains the full
   `"Series - N - Title"` string, e.g. series=`"My Long Series - 1 - Foo"` and
   title=`"My Long Series - 1 - Foo"`. The path formatter then builds
   `Author/My Long Series - 1 - Foo - My Long Series - 1 - Foo/file.m4b`,
   which exceeds the Windows MAX_PATH limit and breaks iTunes.

2. **Ordinal variants** — the same series appears under multiple names because the
   position was baked into the series name in different formats:
   `"The Long Earth One"`, `"The Long Earth Two"`, `"The Long Earth 1"`,
   `"The Long Earth 2"`. These create separate series rows in PebbleDB and
   fragment the library.

`NormalizeMetaSeries` / `ParseSeriesFromTitle` already handle the
`"(Long Earth 05) The Long Cosmos"` parenthesized-prefix format. The new patterns
fall through unhandled, and `CreateSeries` in the store does no normalization before
writing, so bad names enter PebbleDB regardless of source.

## Goals

- Block contaminated series names from entering PebbleDB from any code path.
- Remediate existing bad rows: rename, merge duplicates, then fix file tags and
  folder paths via the existing write-back + organize pipeline.
- Expose a dry-run preview before any writes, triggerable from the UI.

## Architecture

Three layers:

### 1. `normalizeSeriesName()` — pure function

**File:** `internal/metafetch/series_normalize.go`

Signature:
```go
func normalizeSeriesName(name, title string) (series, position string, flagForReview bool)
```

Rules applied in order, stopping at first match:

| Rule | Input | Output |
|------|-------|--------|
| Dash-embedded position+title | `"Long Earth - 1 - The Long Cosmos"` | series=`"Long Earth"`, pos=`"1"` |
| Trailing digit | `"The Long Earth 2"` | series=`"The Long Earth"`, pos=`"2"` |
| Trailing ordinal word (1–20) | `"The Long Earth One"` | series=`"The Long Earth"`, pos=`"1"` |
| Series equals title | normalized series == book title | flagForReview=true, no auto-change |

**Conservative bounds on rule 3:** Only ordinal words One through Twenty are matched
as standalone trailing tokens (space-separated). Words like `"Someone"` or
`"Everyone"` do not match because they are not preceded by a space boundary that
isolates them from the preceding word. Series with legitimate number-words in their
names (e.g. `"Fahrenheit 451"`) are not affected because the number is not a
recognized ordinal word.

The function is a pure, side-effect-free transformer — all callers are responsible
for deciding what to do with the result.

### 2. Normalization gate in the store

**Files:** `internal/database/pebble_store.go` — `CreateSeries`, `UpdateSeriesName`

Both functions call `normalizeSeriesName(name, "")` before writing. The `title`
argument is empty here because the store layer does not have book-title context; the
"series equals title" flag is therefore never set at this layer (it's only meaningful
at the metafetch/remediation layer where both values are available).

This ensures bad names cannot enter PebbleDB from any code path after this change.

### 3. `NormalizeMetaSeries` extension

**File:** `internal/metafetch/service.go`

The existing `NormalizeMetaSeries` function is extended to call
`normalizeSeriesName(meta.Series, meta.Title)` before the existing
`ParseSeriesFromTitle` logic. If `flagForReview` is set, the series field is left
unchanged and a warning is logged; it is not written to the DB.

### 4. Remediation endpoint

**Route:** `POST /api/v1/series/normalize?dry_run=true|false`

**Handler:** `seriesNormalizeHandler` in `internal/server/series_handlers.go` (or
equivalent server file following existing patterns)

**Pipeline (non-dry-run):**

1. **Scan** — load all series rows, apply `normalizeSeriesName(name, "")` to each.
2. **Rename** — for rows where name changes, call `UpdateSeriesName`. Log old→new.
3. **Merge** — after renaming, any two series rows with identical normalized name and
   same `author_id` are duplicates. Merge via existing `mergeSeriesGroup` logic,
   keeping the row with the most books as canonical.
4. **Collect** — gather all book IDs whose `series_id` was renamed or merged.
5. **Write-back** — call `WriteBackBatcher.Enqueue(bookID)` for each affected book.
   The batcher coalesces concurrent enqueues within its existing timer window.
6. **Organize** — enqueue an organize job for each affected book so files move to
   their corrected (shorter) paths on disk.

**Dry-run response:**
```json
{
  "actions": [
    {
      "series_id": 42,
      "old_name": "The Long Earth One",
      "new_name": "The Long Earth",
      "new_position": "1",
      "action": "rename",
      "book_count": 1
    },
    {
      "series_id": 43,
      "old_name": "The Long Earth Two",
      "new_name": "The Long Earth",
      "new_position": "2",
      "action": "merge_into",
      "merge_target_id": 42,
      "book_count": 1
    },
    {
      "series_id": 99,
      "old_name": "My Long Series - 1 - Foo",
      "new_name": "My Long Series",
      "new_position": "1",
      "action": "rename",
      "book_count": 3
    }
  ],
  "total_series_affected": 3,
  "total_books_affected": 5,
  "flagged_for_review": []
}
```

Modeled directly on the existing `series-prune-preview` + `series-prune` pattern.

### 5. Maintenance task integration

A new `series-normalize` task type is added to the maintenance scheduler, wiring the
remediation endpoint so it can be triggered on-demand from the Maintenance tab. No
automatic schedule — this runs manually after bulk imports or when the admin notices
fragmented series.

## Files to Create / Modify

| File | Change |
|------|--------|
| `internal/metafetch/series_normalize.go` | New — `normalizeSeriesName()` + unit tests |
| `internal/metafetch/service.go` | Extend `NormalizeMetaSeries` to call normalizer |
| `internal/database/pebble_store.go` | Add normalization gate in `CreateSeries` + `UpdateSeriesName` |
| `internal/server/duplicates_handlers.go` | Add `seriesNormalizeHandler` alongside `seriesPrune` |
| `internal/server/server.go` | Register new route |
| `internal/server/system_service.go` (or maintenance task registry) | Register `series-normalize` task type |

## Testing

- **Unit tests for `normalizeSeriesName()`**: cover all four rules, edge cases
  (ordinal at start, ordinal in middle, `"Someone"` false-positive guard, ordinals
  21+, series==title flag).
- **Integration test for the endpoint**: mock store returns a set of known-bad series
  names; assert dry-run response matches expected actions; assert non-dry-run
  triggers correct `UpdateSeriesName` + `mergeSeriesGroup` calls.
- **Gate test**: call `CreateSeries` with a known-bad name, assert the stored name is
  normalized.

## Rollback

- The normalization gate in `CreateSeries` / `UpdateSeriesName` can be reverted by
  removing the `normalizeSeriesName` call — the function is a single callsite in each.
- The remediation endpoint is idempotent — running it twice produces no additional
  changes after the first pass.
- Write-back and organize are the same pipeline used everywhere else; their existing
  rollback mechanisms (`.bak-*` copy-on-write, path history) apply.
