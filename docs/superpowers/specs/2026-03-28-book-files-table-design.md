# Book Files Table Design

**Date:** 2026-03-28
**Status:** Approved

## Goal

Add a `book_files` table that tracks every individual audio file in the library. Every book — single-file or multi-file — gets rows in this table. This replaces the current model where `book.file_path` sometimes points to a file, sometimes to a directory, with no per-file tracking for multi-file books.

## Background

### Existing `book_segments` Table

The codebase already has a `book_segments` table (migration 16, file_hash added in migration 30) with:
- `id` (ULID), `book_id` (int — legacy numeric ID), `file_path`, `format`, `size_bytes`, `duration_seconds`
- `track_number`, `total_tracks`, `segment_title`, `file_hash`
- `active`, `superseded_by`, `version` — versioning/superseding support
- Used by scanner (`createSegmentsForBook`), rename pipeline, Files & History display

**`book_files` replaces `book_segments`.** The new table uses ULID string `book_id` (not legacy numeric), adds iTunes fields (itunes_path, itunes_persistent_id), audio metadata (codec, bitrate, sample_rate, channels, bit_depth), and drops the versioning fields (active, superseded_by, version) which are better handled by `book_path_history`.

Migration 39 creates `book_files` and migrates existing `book_segments` data. The old `book_segments` table is retained but no longer written to. A future migration will drop it.

### Current Problems

1. **Multi-file write-back is broken**: A 40-track MP3 audiobook stores one directory path on the book record. ITL write-back would point all 40 iTunes tracks to one directory instead of individual files.
2. **Inconsistent `file_path`**: Single-file books store the file path directly. Multi-file books store the directory. Some old single-file books store the file path instead of the directory. Two code paths, inconsistent behavior.
3. **No per-file metadata**: Multi-file books have no per-track data (track number, duration, size per file). The Files & History display can't show individual files.
4. **Tag writing on directories fails**: The write-back code tries to write tags to a directory path and crashes.
5. **`book_segments` uses legacy numeric book IDs**: The rest of the codebase uses ULID string IDs.

### What This Fixes

- Write-back gets per-file `itunes_path` for correct ITL updates
- `book.file_path` becomes consistently "the directory"
- Files & History can render individual files sorted by track number
- Tag writing iterates `book_files` rows instead of guessing
- Single-file and multi-file books use the same code path everywhere

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Replaces `book_segments` | Yes — `book_files` is the successor | Adds iTunes fields, uses ULID book IDs, drops versioning fields. Migration copies existing data. |
| Scope | Universal — every book gets `book_files` rows | One code path for everything |
| `book_id` type | ULID string (not legacy numeric) | Matches rest of codebase. Migration maps numeric IDs to ULIDs. |
| `book.file_path` | Always the directory | Consistency; files live in `book_files` |
| `external_id_map` | Keep alongside `book_files` | Handles non-iTunes IDs (ASIN, ISBN). Coexists. |
| `book.ITunesPath` | Deprecated | Was added in migration 38 as a stopgap for write-back. Superseded by per-file `itunes_path` in `book_files`. Keep for backward compat, remove in a future migration. |
| `book.ITunesPersistentID` | Stays on book record | Still useful as "this book has an iTunes link." Per-file PIDs live in `book_files`. |
| `book_segments` | Retained read-only | Migration copies data to `book_files`. Old table kept but no longer written to. Dropped in future migration. |
| Backfill | Maintenance endpoint + ongoing hooks | Immediate fix for existing data + keeps current going forward |
| PebbleDB | JSON at `book_file:<book_id>:<file_id>` | Follows existing key patterns |
| Duration unit | Milliseconds (`duration`) | Matches existing `Book.Duration` and iTunes track data. (Note: `book_segments` used seconds.) |
| Missing files with no rows | Create row with `missing=true` | Ensures `GetBookFiles` always returns something for books that had files. |

## Schema

### `book_files` Table

```sql
CREATE TABLE IF NOT EXISTS book_files (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    original_filename TEXT,
    itunes_path TEXT,
    itunes_persistent_id TEXT,
    track_number INTEGER,
    track_count INTEGER,
    disc_number INTEGER,
    disc_count INTEGER,
    title TEXT,
    format TEXT,
    codec TEXT,
    duration INTEGER,
    file_size INTEGER,
    bitrate_kbps INTEGER,
    sample_rate_hz INTEGER,
    channels INTEGER,
    bit_depth INTEGER,
    file_hash TEXT,
    original_file_hash TEXT,
    missing INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_book_files_book_id ON book_files(book_id);
CREATE INDEX IF NOT EXISTS idx_book_files_itunes_pid ON book_files(itunes_persistent_id) WHERE itunes_persistent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_book_files_file_hash ON book_files(file_hash) WHERE file_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_book_files_file_path ON book_files(file_path);
```

### Migration Number

Migration 39 (current latest is 38).

### PebbleDB Key Schema

```
book_file:<book_id>:<file_id>        → BookFile JSON
book_file_pid:<itunes_persistent_id> → file_id (secondary index for PID lookup)
book_file_path:<file_path_hash>      → file_id (secondary index for path lookup)
```

## BookFile Struct

```go
type BookFile struct {
    ID                 string     `json:"id"`
    BookID             string     `json:"book_id"`
    FilePath           string     `json:"file_path"`
    OriginalFilename   string     `json:"original_filename,omitempty"`
    ITunesPath         string     `json:"itunes_path,omitempty"`
    ITunesPersistentID string     `json:"itunes_persistent_id,omitempty"`
    TrackNumber        int        `json:"track_number,omitempty"`
    TrackCount         int        `json:"track_count,omitempty"`
    DiscNumber         int        `json:"disc_number,omitempty"`
    DiscCount          int        `json:"disc_count,omitempty"`
    Title              string     `json:"title,omitempty"`
    Format             string     `json:"format,omitempty"`
    Codec              string     `json:"codec,omitempty"`
    Duration           int        `json:"duration,omitempty"`
    FileSize           int64      `json:"file_size,omitempty"`
    BitrateKbps        int        `json:"bitrate_kbps,omitempty"`
    SampleRateHz       int        `json:"sample_rate_hz,omitempty"`
    Channels           int        `json:"channels,omitempty"`
    BitDepth           int        `json:"bit_depth,omitempty"`
    FileHash           string     `json:"file_hash,omitempty"`
    OriginalFileHash   string     `json:"original_file_hash,omitempty"`
    Missing            bool       `json:"missing"`
    CreatedAt          time.Time  `json:"created_at"`
    UpdatedAt          time.Time  `json:"updated_at"`
}
```

## Store Interface

```go
// Book files
CreateBookFile(file *BookFile) error
UpdateBookFile(id string, file *BookFile) error
GetBookFiles(bookID string) ([]BookFile, error)
GetBookFileByPID(itunesPID string) (*BookFile, error)
GetBookFileByPath(filePath string) (*BookFile, error)
DeleteBookFile(id string) error
DeleteBookFilesForBook(bookID string) error
UpsertBookFile(file *BookFile) error  // match by (book_id, file_path) or itunes_persistent_id
```

## `book.file_path` Normalization

**Rule:** `book.file_path` always points to the directory containing the audio files. Never a file.

**Backfill logic:**
1. If `file_path` points to a file (not a directory): move it to `filepath.Dir(file_path)`, create one `book_files` row pointing to the original file path.
2. If `file_path` points to a directory: glob audio files inside, create `book_files` rows for each.
3. If `file_path` is empty or doesn't exist on disk: mark all `book_files` rows as `missing=true`.

## Population Hooks

| Event | Action |
|-------|--------|
| **iTunes sync** | For each matched track: upsert `book_files` row with file_path (remapped), itunes_path (raw XML URL), itunes_persistent_id, track_number, duration, file_size from XML track data |
| **Scanner** | For each book found: glob audio files in directory (or single file), create `book_files` row per file with file_path, format, track_number (from filename/tag), duration, size, codec |
| **Organize/rename** | Update `file_path` and `itunes_path` on affected `book_files` rows. Also update `book.file_path` to the new directory. |
| **Tag write** | Update `file_hash` on the affected `book_files` row after writing tags |
| **Book delete** | `ON DELETE CASCADE` removes `book_files` rows automatically |
| **Backfill endpoint** | `POST /api/v1/maintenance/backfill-book-files` — scans all books, normalizes `file_path` to directory, creates missing `book_files` rows. Dry-run by default. |

**Upsert matching priority:**
1. Match by `(book_id, itunes_persistent_id)` if PID is set
2. Match by `(book_id, file_path)` if path matches
3. Otherwise create new row

## Write-Back Flow (Updated)

**Old flow:**
```
For each book with iTunes PID:
  → read book.ITunesPath (one path, might be a directory)
  → add to ITL update map
```

**New flow:**
```
For each book with iTunes PIDs:
  → query book_files WHERE book_id = ? AND itunes_persistent_id IS NOT NULL
  → for each file:
    → add {PID: file.itunes_path} to ITL update map
  → call UpdateITLLocations() with all updates
```

This correctly maps each of the 40 iTunes tracks to its individual file path.

## Deprecation: `book.ITunesPath`

The `ITunesPath` field on the Book struct (added in migration 38) is **deprecated**. It will remain in the schema for backward compatibility but:
- New code should read/write `itunes_path` from `book_files` instead
- The write-back uses `book_files`, not `book.ITunesPath`
- A future migration will drop the column

Add a code comment: `// Deprecated: use book_files.itunes_path instead. Will be removed in a future migration.`

## Files Affected

### Modify
| File | Change |
|------|--------|
| `internal/database/migrations.go` | Migration 39: create `book_files` table, migrate `book_segments` data, add indexes |
| `internal/database/store.go` | Add `BookFile` struct, add store interface methods, deprecation comment on `ITunesPath` |
| `internal/database/sqlite_store.go` | Implement `book_files` CRUD + upsert |
| `internal/database/pebble_store.go` | Implement `book_files` with PebbleDB key schema |
| `internal/database/mock_store.go` | Add mock implementations for new interface methods |
| `internal/database/mocks/mock_store.go` | Regenerate or manually add new methods |
| `internal/server/server.go` | Add routes: `GET /audiobooks/:id/files`, `POST /maintenance/backfill-book-files` |
| `internal/server/maintenance_fixups.go` | Add `handleBackfillBookFiles` handler |
| `internal/server/itunes.go` | Populate `book_files` during sync; update write-back-all to use `book_files` |
| `internal/server/itunes_writeback_batcher.go` | Use `book_files` for ITL write-back |
| `internal/server/metadata_fetch_service.go` | Update `book_files` rows after tag write (file_hash update) |
| `internal/server/rename_service.go` | Update `book_files.file_path` and `itunes_path` after rename |
| `internal/server/organize_service.go` | Update `book_files` after organize (file copy + rename) |
| `internal/scanner/scanner.go` | Create `book_files` rows during scan (replaces `createSegmentsForBook`) |
| `web/src/services/api.ts` | Add `BookFile` type, `getBookFiles()`, `backfillBookFiles()` |
| `web/src/pages/BookDetail.tsx` | Display `book_files` in Files & History tab (replace segment-based display) |

## Out of Scope

- Importing directly from ITL instead of XML (future — separate spec)
- Removing `external_id_map` (stays as-is)
- Removing `book.ITunesPath` column (future migration)
- Files & History layout redesign (separate UI issue)
- Sorting Track PIDs by track number (separate UI fix)
