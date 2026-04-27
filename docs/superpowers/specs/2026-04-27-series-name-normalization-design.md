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

### 1. `StripSeriesContamination()` — pure function

**File:** `internal/metadata/series_normalize.go`

Signature:
```go
func StripSeriesContamination(name, title string) (series, position string, flagForReview bool)
```

Rules applied in order, stopping at first match:

| Rule | Input | Output |
|------|-------|--------|
| Dash-embedded position+title | `"Long Earth - 1 - The Long Cosmos"` | series=`"Long Earth"`, pos=`"1"` |
| Trailing 1-2 digit number | `"The Long Earth 2"` | series=`"The Long Earth"`, pos=`"2"` |
| Trailing ordinal word (1–20) | `"The Long Earth One"` | series=`"The Long Earth"`, pos=`"1"` |
| Series equals title | normalized series == book title | flagForReview=true, no auto-change |

**Conservative bounds on rule 3:** Only ordinal words One through Twenty are matched
as standalone trailing tokens (space-separated). Words like `"Someone"` or
`"Everyone"` do not match. Series with legitimate number-words in their names
(e.g. `"Fahrenheit 451"`) are not affected because the number is not a recognized
ordinal word. Trailing digits are capped at 1-2 digits (1–99) to avoid
`"Fahrenheit 451"` false positives.

### 2. Call sites — ingest gates

**Files:**
- `internal/metafetch/service.go` — extend `NormalizeMetaSeries` to call
  `StripSeriesContamination` on `meta.Series` before the existing
  `ParseSeriesFromTitle` logic.
- `internal/scanner/scanner.go` — call in `resolveSeriesID` before
  `GetSeriesByName` / `CreateSeries`.
- `internal/itunes/service/importer.go` — call in `ensureSeriesID` before
  `GetSeriesByName` / `CreateSeries`.

Note: `internal/database` deliberately does not import `internal/metadata` (see
`metadata_fetch_cache.go` comment). The gate therefore lives at the call sites, not
inside `CreateSeries` / `UpdateSeriesName`.

### 3. Remediation endpoint

**Route:** `POST /api/v1/series/normalize`
**Preview:** `GET /api/v1/series/normalize/preview`

**Handler:** `seriesNormalize` + `seriesNormalizePreview` in
`internal/server/duplicates_handlers.go`

**Pipeline (non-dry-run):**

1. **Scan** — load all series rows, apply `StripSeriesContamination(name, "")`.
2. **Collect affected book IDs** — before any writes, gather all book IDs for
   series that will change (so we don't lose track after the rename/merge).
3. **Rename** — for rows where name changes, call `UpdateSeriesName`. Log old→new.
4. **Merge** — after renaming, any two series with identical normalized name and
   same `author_id` are duplicates. Merge via existing `mergeSeriesGroup` logic.
5. **Write-back** — call `WriteBackBatcher.Enqueue(bookID)` for each affected book.
6. **Organize** — call `organizeService.ReOrganizeInPlace(book, log)` for each
   affected book inline in the async operation.

**Dry-run response:**
```json
{
  "actions": [
    {"series_id": 42, "old_name": "The Long Earth One", "new_name": "The Long Earth",
     "new_position": "1", "action": "rename", "book_count": 1},
    {"series_id": 43, "old_name": "The Long Earth Two", "new_name": "The Long Earth",
     "new_position": "2", "action": "merge_into", "merge_target_id": 42, "book_count": 1}
  ],
  "total_series_affected": 2,
  "total_books_affected": 2,
  "flagged_for_review": []
}
```

### 4. Maintenance task

A new `series_normalize` task type registered in `internal/server/scheduler.go`.
Manual-only (`GetInterval` returns 0, `RunOnStart` false).

## Files to Create / Modify

| File | Change |
|------|--------|
| `internal/metadata/series_normalize.go` | New — `StripSeriesContamination()` |
| `internal/metadata/series_normalize_test.go` | New — unit tests |
| `internal/metafetch/service.go` | Extend `NormalizeMetaSeries` |
| `internal/scanner/scanner.go` | Normalize in `resolveSeriesID` |
| `internal/itunes/service/importer.go` | Normalize in `ensureSeriesID` |
| `internal/server/duplicates_handlers.go` | Add preview + normalize handlers |
| `internal/server/server.go` | Register routes + interrupt-recovery entry |
| `internal/server/scheduler.go` | Register `series_normalize` task |

## Testing

- Unit tests for `StripSeriesContamination`: all four rules, edge cases
  (ordinal at start/middle, `"Someone"` guard, ordinals 21+, series==title).
- Integration test for `computeSeriesNormalizeActions` via MockStore.
- Integration test for `executeSeriesNormalizeCore` via MockStore.
- Gate test: call `resolveSeriesID` with a known-bad name, assert normalized name
  is passed to store.

## Rollback

- The normalization call sites are single-line additions — revert by removing them.
- The endpoint is idempotent — running it twice produces no additional changes.
- Write-back and organize use the existing copy-on-write (`.bak-*`) and path
  history mechanisms for rollback.
