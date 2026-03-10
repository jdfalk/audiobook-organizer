<!-- file: docs/plans/2026-03-09-book-versions-dedup-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90 -->

# Book Versions, Dedup & File Organization Design

**Date:** 2026-03-09
**Status:** Draft
**Priority order:** Series pruning > Book dedup > Version management > Preview rename > iTunes single-book sync > File recovery

---

## 1. Series Auto-Pruning

**Invariant:** Series count must never exceed book count.

### Trigger Points

- After any scan completes (post-hook in `ScanService.PerformScan`)
- After any import completes (post-hook in iTunes import flow)
- As a scheduled task (`series_prune` in `TaskScheduler`)
- On demand via API

### Algorithm

1. Find exact duplicate series: group by `LOWER(TRIM(name))` + `author_id`
2. For each group with >1 entry, pick the canonical series (prefer the one with more books attached, then lowest ID)
3. Reassign all books from duplicate series to canonical: `UPDATE books SET series_id = ? WHERE series_id = ?`
4. Delete the now-empty duplicate series records
5. Find orphan series with 0 books: `SELECT s.id FROM series s LEFT JOIN books b ON b.series_id = s.id WHERE b.id IS NULL`
6. Delete orphan series

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/series/prune` | Trigger manual prune, returns operation ID |
| `GET` | `/api/v1/series/prune/preview` | Dry-run: returns what would be merged/deleted |

### Database Changes

None. Uses existing `Series`, `Book` tables. The `GetAllSeries`, `GetBooksBySeriesID`, `UpdateBook`, `DeleteSeries` store methods are sufficient.

### Scheduler Integration

Register `series_prune` task in `scheduler.go`:
- Category: `maintenance`
- Interval: 0 (manual only, but auto-triggered post-scan/import)
- Config fields: `ScheduledSeriesPruneEnabled`, `ScheduledSeriesPruneOnImport`

### Frontend

- Add a "Prune Series" button to the Series tab on the BookDedup page
- Show preview count ("X duplicates, Y orphans will be cleaned up")
- No new page needed

### Key Considerations

- Must handle case-insensitive matching: "Harry Potter" vs "harry potter"
- Must handle series with different `author_id` values as separate (same name, different author = different series)
- Log all merges to operation logs for auditability
- Invalidate `dedupCache` key `series-duplicates` after pruning

---

## 2. Book Deduplication

Detect when the same logical book exists multiple times (e.g., imported from iTunes folder and separately from audiobook library).

### Detection Strategy (ranked by confidence)

1. **File hash match** (`file_hash` field) -- identical files, 100% confidence
2. **Original hash match** (`original_file_hash`) -- same source file even if transcoded
3. **Title + Author + Duration** -- fuzzy match using normalized title comparison + same author ID + duration within 5% tolerance
4. **Title + Author similarity** -- Levenshtein/Jaro-Winkler on normalized titles, same or similar author

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/audiobooks/duplicates` | List detected duplicate groups |
| `POST` | `/api/v1/audiobooks/duplicates/scan` | Trigger async duplicate scan operation |
| `POST` | `/api/v1/audiobooks/duplicates/merge` | Merge a duplicate group into versions |
| `POST` | `/api/v1/audiobooks/duplicates/dismiss` | Mark a group as "not duplicates" |

### Database Changes

New store method needed:
```go
GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error)
```

The existing `GetDuplicateBooks()` (by hash) and `GetFolderDuplicates()` already handle exact matches. The new method handles fuzzy title+author matching.

New field on `Book` (optional):
```go
DedupDismissedGroup *string `json:"dedup_dismissed_group,omitempty"`
```
This tracks groups the user has explicitly dismissed as not-duplicates.

### Merge Behavior

When merging duplicates:
1. Pick the "best" version as primary (highest bitrate, M4B preferred, largest file)
2. Assign all books in the group to the same `VersionGroupID`
3. Set `IsPrimaryVersion` accordingly
4. Preserve all metadata from the richest source

### Frontend

- New sub-tab "Book Duplicates" on the BookDedup page (alongside existing Author and Series tabs)
- Each group shows: title, author, and a row per duplicate with file path, format, bitrate, size
- Actions: "Merge as Versions", "Dismiss", "View Details"

### Key Considerations

- Must not auto-merge without user confirmation (false positives are destructive)
- The scan should be an async operation (could be slow for large libraries)
- Existing `GetDuplicateBooks()` returns hash-based groups; reuse for high-confidence matches
- Depends on: Version Management (feature 3) for the merge target

---

## 3. Version Management Overhaul

Replace the current popup-based flow with a dedicated flat page per book.

### Current State

- `Book` has `VersionGroupID`, `IsPrimaryVersion`, `VersionNotes` fields
- API: `GET /audiobooks/:id/versions`, `POST /audiobooks/:id/versions`, `PUT /audiobooks/:id/set-primary`, `GET /version-groups/:id`
- Transcode already creates version groups (see `startTranscode` handler lines 4220-4282)
- `linkAudiobookVersion` merges two books into a version group

### New API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/audiobooks/:id/versions/add-file` | Add a file as a new version |
| `DELETE` | `/api/v1/audiobooks/:id/versions/:versionId` | Unlink a version from group |
| `PUT` | `/api/v1/audiobooks/:id/versions/:versionId/notes` | Update version notes |
| `POST` | `/api/v1/audiobooks/:id/versions/replace-primary` | Upload/pick new M4B, make it primary |

### "Replace with M4B" Workflow

1. User selects an M4B file via the file browser (`/api/v1/filesystem/browse` already exists)
2. Server creates a new `Book` record for the M4B with the same metadata as the original
3. New M4B becomes primary, old file(s) become non-primary versions
4. Old files optionally moved to a `versions/` subfolder under the book's organized directory
5. Database updated atomically

### Frontend: Book Versions Page

Route: `/books/:id/versions`

Layout:
- Header: Book title, author, series
- Table of versions (one row per version):
  - File path (truncated, with tooltip)
  - Format badge (M4B, MP3, M4A)
  - Bitrate, sample rate, duration
  - File size
  - Quality score (computed: format weight + bitrate)
  - Primary indicator (radio button)
  - Version notes (inline editable)
  - Actions: Set Primary, Remove from Group, Delete File
- "Add Version" section at bottom:
  - File browser component (reuse existing `filesystem/browse`)
  - "Link Existing Book" search field (for manual duplicate linking)
- "Replace with M4B" prominent button

### Key Considerations

- Setting primary must update all other versions in the group to `is_primary_version = false`
- The existing `setAudiobookPrimary` handler already does this correctly
- "Add Version" from file must: probe the file for metadata, create a Book record, assign to version group
- Must handle the case where a version's file no longer exists on disk (show warning)
- Depends on: nothing (existing infrastructure is sufficient)

---

## 4. Preview Rename & Metadata Writeback

Atomic operation: write tags to file, rename/move file, update database.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/audiobooks/:id/rename/preview` | Returns current path, proposed path, and tag diff |
| `POST` | `/api/v1/audiobooks/:id/rename/apply` | Execute rename + tag write + DB update |
| `POST` | `/api/v1/audiobooks/batch-rename/preview` | Preview for multiple books |
| `POST` | `/api/v1/audiobooks/batch-rename/apply` | Execute batch (queued as operation) |

### Preview Response Shape

```json
{
  "book_id": "01HXYZ...",
  "current_path": "/import/iTunes/Author - Title.mp3",
  "proposed_path": "/library/Author/Series/01 - Title.m4b",
  "tag_changes": [
    { "field": "title", "current": "Author - Title", "proposed": "Title" },
    { "field": "artist", "current": "", "proposed": "Author Name" },
    { "field": "album", "current": "", "proposed": "Series Name" },
    { "field": "track", "current": "0", "proposed": "1" }
  ],
  "versions": [
    { "version_id": "01HXYZ...", "current_path": "...", "proposed_path": "..." }
  ]
}
```

### Apply Operation (3 steps, atomic)

1. **Write tags** via ffmpeg: `ffmpeg -i input -metadata title="..." -metadata artist="..." -codec copy output.tmp`
2. **Move/rename** file: `os.Rename(tmpPath, proposedPath)` (or copy + delete if cross-device)
3. **Update DB**: `UpdateBook` with new `FilePath`, set `OrganizedFileHash`, record `OperationChange` for undo

If any step fails, roll back previous steps. Use `OperationChange` records for undo support.

### Tag Writing Implementation

Use ffmpeg (already a dependency for transcoding):
```bash
ffmpeg -i "input.m4b" \
  -metadata title="Book Title" \
  -metadata artist="Author Name" \
  -metadata album_artist="Author Name" \
  -metadata album="Series Name" \
  -metadata track="1" \
  -metadata genre="Audiobook" \
  -codec copy \
  "output.tmp.m4b"
```

Must handle: MP3 (ID3v2), M4A/M4B (MP4 atoms), FLAC (Vorbis comments).

### Database Changes

No schema changes needed. Uses existing:
- `OperationChange` for undo tracking (`change_type = "file_move"`, `"tag_write"`)
- `LastWrittenAt` timestamp on `Book`

### Frontend

- "Preview Rename" button on book detail page
- Modal/drawer showing: current vs proposed path, tag change diff
- "Apply" button with confirmation
- Progress indicator for batch operations
- Must show all versions of the book, not just primary

### Key Considerations

- ffmpeg must run on Linux (this is a Linux server, not macOS tools)
- Must preserve chapter markers when rewriting M4B tags (`-codec copy` does this)
- Cross-filesystem moves require copy+delete, not `os.Rename`
- The organize service (`organize_service.go`) already builds proposed paths; reuse `organizer.BuildOrganizedPath`
- Must handle all versions in a version group, not just the primary
- Record all changes in `OperationChange` for revert support (existing `revertOperation` handler)

---

## 5. iTunes Single-Book Sync

"Sync to iTunes" button on book detail page that writes one book to the remote Windows .itl file.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/audiobooks/:id/sync-itunes` | Sync one book to iTunes |
| `POST` | `/api/v1/audiobooks/:id/sync-itunes/preview` | Preview what would be written |

### Implementation

Two modes depending on whether the book already exists in the ITL:

**Update existing track** (book has `ITunesPersistentID`):
- Use `itunes.UpdateITLLocations()` with a single `ITLLocationUpdate`
- Updates the track's file location to the new organized path

**Insert new track** (no `ITunesPersistentID`):
- Use `itunes.InsertITLTracks()` with a single `ITLNewTrack`
- Populates: Name, Album (series), Artist (author), Location, Kind, Size, TotalTime, BitRate, SampleRate
- After insert, store the generated persistent ID back on the Book record

### Prerequisites

- Config must have `itl_path` pointing to the .itl file (remote Windows share mounted locally)
- The `itunes.ValidateITL()` check should run before any write
- Backup is mandatory (use existing `WriteBackOptions.CreateBackup`)

### Frontend

- "Sync to iTunes" button on book detail page (only shown when `itl_path` is configured)
- Confirmation dialog showing what will be written
- Success/error toast notification

### Key Considerations

- The ITL file is on a remote Windows machine (mounted via SMB/NFS)
- File locking: must ensure iTunes is not running when writing (or accept the risk)
- The `writeback.go` XML writer works for Library.xml; the `itl.go` binary writer works for .itl files. Use the appropriate one based on config.
- Per MEMORY.md: "iTunes Library.xml is EXPORT-ONLY. Modifying it does nothing." -- must use the .itl binary writer.
- Path mapping: organized paths are Linux paths, iTunes expects Windows paths. Need configurable path prefix replacement (e.g., `/library/` -> `E:\Audiobooks\`)

### Database Changes

None. Uses existing `ITunesPersistentID` field on `Book`.

---

## 6. File Organization Recovery

Handle the case where files were organized/moved but the DB still points to old paths.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/operations/reconcile` | Start async reconciliation operation |
| `GET` | `/api/v1/operations/reconcile/preview` | Preview: list orphaned DB records and untracked files |

### Reconciliation Algorithm

1. **Find broken DB records:** For each book, check if `file_path` exists on disk. Collect those that don't.
2. **Scan directories:** Walk both source (import paths) and destination (library root) directories. Collect all audio files not tracked in DB.
3. **Match by hash:** For each broken record with `file_hash` or `original_file_hash`, search untracked files for matching hash.
4. **Match by metadata fingerprint:** For remaining unmatched, probe file metadata (title + author from tags) and compare against broken DB records.
5. **Update or prompt:** High-confidence matches (hash) auto-fix. Low-confidence matches (metadata) presented to user for confirmation.

### Database Changes

New store method:
```go
GetBooksWithMissingFiles() ([]Book, error)
```
This queries all books and checks `file_path` existence (or uses a cached file-existence check).

### Frontend

- "Reconcile Library" button on Settings/Maintenance page
- Results view: table of broken records with match status
  - Green: auto-matched by hash, will fix
  - Yellow: metadata match, needs confirmation
  - Red: no match found, manual action needed
- "Apply Fixes" button for confirmed matches

### Scheduler Integration

Optional: register `file_reconcile` task that runs after `library_scan` completes, checking for broken paths.

### Key Considerations

- File hashing is expensive; use `file_hash` from DB when available, only compute for untracked files
- The existing `auditFileConsistency` handler (`/operations/audit-files`) does a simpler version of this -- extend rather than replace
- Must not modify files, only update DB paths
- Record changes in `OperationChange` for undo
- Could be triggered automatically when a scan finds 0 books in a previously populated directory

---

## Dependency Graph

```
Series Pruning (1) -----> standalone, no deps
Book Dedup (2) ----------> depends on Version Management (3) for merge target
Version Management (3) --> standalone (existing infra sufficient)
Preview Rename (4) ------> depends on Version Management (3) to handle all versions
iTunes Sync (5) ---------> depends on Preview Rename (4) for correct paths
File Recovery (6) -------> standalone, but benefits from Book Dedup (2)
```

**Recommended implementation order:** 1 -> 3 -> 2 -> 4 -> 5 -> 6

Series Pruning is simplest and most impactful. Version Management provides the foundation for Book Dedup. Preview Rename needs version awareness. iTunes Sync needs correct file paths. File Recovery is independent but least urgent.
