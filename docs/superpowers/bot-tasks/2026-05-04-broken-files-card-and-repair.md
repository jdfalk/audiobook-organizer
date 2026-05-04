<!-- file: docs/superpowers/bot-tasks/2026-05-04-broken-files-card-and-repair.md -->
<!-- version: 1.0.0 -->
<!-- guid: b7e2f54a-9c83-4d6b-be1a-2c3d4e5f6789 -->
<!-- last-edited: 2026-05-04 -->

# BOT TASK: Broken-files dashboard card + repair pipeline

## Branch

```
feat/broken-files-card
```

## Problem

We used to surface a "you have N authors that look like duplicates" card on
the dashboard. That one wasn't super useful in the end, but the *pattern*
is — a glanceable card that exposes a piece of background-discovered
trouble the user can act on.

Right now, AcoustID / fingerprint / metadata flows generate ffmpeg-style
errors like:

```
acoustid_backfill.go:55: [WARN] fingerprint: /mnt/bigdata/.../06 Phantom Menace.mp3:
  ffmpeg chromaprint .../06 Phantom Menace.mp3@0.00:
  [mp3 @ 0x...] Failed to find two consecutive MPEG audio frames.
[in#0 @ 0x...] Error opening input: Invalid data found when processing input
Error opening input file /mnt/bigdata/.../06 Phantom Menace.mp3.
Error opening input files: Invalid data found when processing input
```

These warnings vanish into journalctl. Nothing in the UI tells the user
"you have N files ffmpeg can't read"; nothing in the DB says "this book's
file 06.mp3 is broken". Two consequences:

1. The user can't filter the library to "books with broken files" and act
   on them in bulk.
2. There's no repair pipeline: re-encode, replace from a known-good copy,
   delete-and-rescan, etc. The user has to find each broken file by hand.

## Goal

1. **Persist** every per-file processing error to the DB, associated with
   the file (and its parent book). At minimum: file path, error class
   (e.g. `ffmpeg_invalid_data`, `corrupt_mp3_frames`, `missing_fingerprint`),
   raw error message, when it was last observed, how many times.
2. **Surface** a dashboard card: "N books have files with errors → review".
3. **Filter** the library page by a "has broken files" facet.
4. **Repair pipeline** with at least these options per book/file:
   - Re-extract from source if a versioned copy exists (`.versions/`).
   - Trigger `make_safe` / `WriteTagsSafe`-style remux to fix container.
   - Mark file as `ignored` (suppress future scans against it).
   - Delete + rescan if the user has decided to drop the bad copy.

## Files (likely)

- `internal/database/migrations/` — new migration:
  - `book_file_errors` table: `(file_path TEXT PK, book_id TEXT, error_class TEXT,
    last_message TEXT, occurrences INT, first_seen, last_seen)`.
- `internal/fingerprint/*.go` and `internal/server/acoustid_backfill.go`
  — when ffmpeg returns non-zero / chromaprint fails, classify the error
  and call a new store method `RecordFileError`.
- `internal/database/store.go` (or a sidecar) — `RecordFileError`,
  `ListBooksWithFileErrors`, `ClearFileError(filePath)`.
- `internal/server/dashboard_handlers.go` — extend the dashboard summary
  to include `broken_file_count`.
- Frontend:
  - `web/src/pages/Dashboard.tsx` — new card "N books with broken files",
    routes to library filter.
  - `web/src/pages/Library.tsx` — facet filter `has_file_errors=true`,
    column for error class.
  - `web/src/pages/BookDetail.tsx` — Errors tab (or section in Files tab)
    listing per-file errors with repair actions.
- New `internal/repair/` package — repair pipeline orchestrator:
  - `Repair.RemuxFile(ctx, path)`
  - `Repair.RestoreFromVersion(ctx, bookID, fileID)`
  - `Repair.MarkIgnored(filePath)`
  - `Repair.DeleteAndRescan(filePath)`

## Constraints

- **No re-classification on every read.** The `error_class` is computed
  once at the producing site (the fingerprint/ffmpeg wrapper), not at
  query time. Frontend just renders the stored class.
- **De-dupe on file path.** A book with the same broken file scanned 50
  times shouldn't make 50 rows; bump `occurrences` and `last_seen`.
- **Source-of-truth for "broken" is `last_seen`.** A successful re-scan
  clears the row (or sets a `resolved_at`).
- **Repair is opt-in per book.** No automatic remux without user click.

## Out of scope

- Auto-repair without user consent.
- Tracking transient/retryable errors (network blips). Only persistent
  file-level failures (ffmpeg invalid-data, corrupt frames, etc.).
- Backfilling errors from past journalctl. Forward-only.

## Test strategy

- Inject a corrupt MP3 fixture; run the fingerprint pass; assert one
  `book_file_errors` row with the right class.
- Re-run on the same path; assert `occurrences=2`, no new row.
- Run the dashboard summary endpoint; assert `broken_file_count` is 1.
- Run the repair Remux path against the fixture; assert success → row
  deleted (or `resolved_at` set).

## Rollback

The migration adds a new table — drop it on revert. Repair package and
frontend additions are pure adds; no existing flow changes.

## Notes

- Pairs naturally with the operation-log tagging spec
  (`2026-05-04-tag-operation-logs.md`): once op-scoped logs land, the
  Errors tab on a book can deep-link into the operation that surfaced
  the failure.
- Dashboard card pattern: copy the styling of the old "duplicate authors"
  card so it feels native.
