<!-- file: docs/superpowers/specs/2026-03-10-incremental-scan-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e -->

# Incremental Scan & Single-Pass I/O Design

## Goal

Make library scans fast by default. Incremental scans skip unchanged files (targeting ~30s for a 47k-file library). Full scans get ~2-3x faster via single-pass file I/O. Worker pool auto-scales to available CPU cores.

## Architecture

Three independent improvements that compound:

1. **Scan cache** — track file mtime/size in DB, skip unchanged files on rescan
2. **Single-pass I/O** — combine tag reading, mediainfo extraction, and file hashing into one file open
3. **Worker pool auto-scaling** — default to `runtime.NumCPU()` instead of hardcoded 4

## Scan Cache

### Database Changes (Migration 32)

New columns on `books` table:

| Column | Type | Default | Purpose |
|--------|------|---------|---------|
| `last_scan_mtime` | INTEGER | NULL | Unix timestamp of file mtime at last scan |
| `last_scan_size` | INTEGER | NULL | File size in bytes at last scan |
| `needs_rescan` | BOOLEAN | false | Dirty flag — other services set this to trigger rescan |

New composite index: `idx_books_scan_cache ON books(file_path, last_scan_mtime, last_scan_size)`.

### New Store Methods

```go
// GetScanCacheMap returns a map of file_path → (mtime, size, needs_rescan)
// for all books. Used at scan start for fast in-memory lookups.
GetScanCacheMap() (map[string]ScanCacheEntry, error)

// UpdateScanCache sets mtime/size and clears needs_rescan for a book.
UpdateScanCache(bookID string, mtime int64, size int64) error

// MarkNeedsRescan sets needs_rescan=true for a book.
MarkNeedsRescan(bookID string) error

// GetDirtyBookFolders returns distinct parent folders for all books
// where needs_rescan=true.
GetDirtyBookFolders() ([]string, error)
```

`ScanCacheEntry` struct:

```go
type ScanCacheEntry struct {
    Mtime       int64
    Size        int64
    NeedsRescan bool
}
```

### Incremental Scan Flow

1. **Pre-load cache:** Single query loads path→ScanCacheEntry map (~5MB for 47k books)
2. **Walk directory tree:** `stat()` each file
3. **Skip check:** For each file, look up path in cache map. If mtime and size match and `needs_rescan` is false → skip
4. **Dirty folder collection:** Query `GetDirtyBookFolders()`, add those folders to a "must scan" set. All files in dirty folders get full processing regardless of mtime/size.
5. **Process changed/new files:** Run through single-pass `ProcessFile` with worker pool
6. **Update cache:** After processing, call `UpdateScanCache` with new mtime/size (clears `needs_rescan`)

### Dirty Flag Usage

Services that modify book files set `needs_rescan=true`:

- `metadata_fetch_service` — after writing metadata tags back to audio files
- `organize_service` — after moving/renaming files
- Any future service that modifies a book's audio files

When `needs_rescan` is true, the scan processes the book's **parent folder** (not just the single file). This catches new segments, renamed files, and structural changes in the folder.

### Full Scan Mode

`force_update=true` (existing parameter) bypasses all cache checks. Every file gets full processing. Available as:
- Manual trigger via API (`POST /api/v1/operations/scan` with `force_update=true`)
- Scheduler task with `RunOnStart` if the user enables it

## Single-Pass File I/O

### Problem

Currently each file is opened and read three times during scan:
1. `metadata.ExtractMetadata()` — opens file, calls `tag.ReadFrom()`
2. `mediainfo.Extract()` — opens file again, calls `tag.ReadFrom()` again
3. `scanner.ComputeFileHash()` — opens file, reads content for SHA256

### Solution

New function in the scanner package:

```go
// ProcessFile opens a file once and extracts all scan data in a single pass.
// Returns metadata, media info, and file hash.
func ProcessFile(path string, log logger.Logger) (*metadata.Metadata, *mediainfo.MediaInfo, string, error)
```

Implementation:
1. Open file once
2. Call `tag.ReadFrom()` once — extract both metadata fields (title, artist, etc.) and media info (bitrate, codec, duration, channels)
3. Seek to start, stream through file computing SHA256 (or partial hash for files >100MB: first 10MB + last 10MB + file size)
4. Close file, return all three results

### What Stays the Same

- `metadata.ExtractMetadata()` and `mediainfo.Extract()` remain as standalone functions for callers that only need one piece (e.g., metadata fetch service)
- `ProcessFile` is scan-specific orchestration that avoids redundant I/O
- Workers in `ProcessBooksParallel` call `ProcessFile` instead of the three separate functions

### Expected Impact

~2-3x speedup for the file processing phase of full scans. For 47k files, eliminates ~94k redundant file opens.

## Worker Pool Auto-Scaling

### Change

In `ProcessBooksParallel` and `ScanDirectoryParallel`: if `config.AppConfig.ConcurrentScans` is 0 or unset, default to `runtime.NumCPU()` instead of hardcoded 4.

### Rationale

The current default of 4 workers is arbitrarily low. Scan work is I/O-bound, so more workers keep the disk busy. The user's production box has 48 cores and they manually configured 48 workers — the default should be smart enough to do this automatically.

## Files Modified

| File | Change |
|------|--------|
| `internal/database/migrations.go` | Migration 32: add scan cache columns + index |
| `internal/database/store.go` | ScanCacheEntry struct, 4 new interface methods |
| `internal/database/sqlite_store.go` | Implement new methods |
| `internal/database/pebble_store.go` | Implement new methods |
| `internal/database/mock_store.go` | Stub implementations |
| `internal/database/mocks/mock_store.go` | Testify mock implementations |
| `internal/scanner/scanner.go` | `ProcessFile()`, incremental skip logic in `ProcessBooksParallel` |
| `internal/server/scan_service.go` | Incremental flow: pre-load cache, dirty folder collection, skip logic |
| `internal/config/config.go` | Default ConcurrentScans to 0 (auto-detect) |

## Performance Expectations

| Scenario | Current | After |
|----------|---------|-------|
| Full scan, 47k files | ~50 min | ~15-20 min |
| Incremental scan, nothing changed | ~50 min | ~30 sec |
| Incremental scan, 100 files changed | ~50 min | ~1 min |
| Incremental scan, dirty folder with 50 files | ~50 min | ~30 sec |

## Non-Goals

- **Batch database inserts:** The upsert logic is too complex (hash lookups, version groups, author/series resolution). With incremental scans processing few files, DB overhead is negligible.
- **Filesystem watcher (fsnotify):** Unreliable for large directory trees on Linux due to inotify watch limits. Doesn't survive restarts. Adds complexity for marginal benefit over incremental scans.
- **Separate discovery vs processing worker pools:** stat() calls are cheap enough that one pool size works for both phases.
