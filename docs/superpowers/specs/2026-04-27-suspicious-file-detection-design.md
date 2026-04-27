# Suspicious File Detection Design

**Date:** 2026-04-27  
**Status:** Approved  

## Problem

The library occasionally contains single-file "books" that are only a few megabytes — far too small to be a real audiobook. These are typically:
- Partial downloads or failed moves/renames  
- A lone file left behind in a folder that was supposed to contain a full multi-file book  
- Cover art or sample files picked up by the audio-file extension filter  

Currently these files are fully processed: metadata is extracted, AI parsing may run, dedup hashes are computed, and they land in the main library as first-class books. The goal is to detect them early, skip all heavy processing, save them with a `library_state = 'suspicious'` marker for human review, and exclude them from normal library operations.

Multi-file books (entire folders of MP3s) are **not** affected — a folder of 67 split MP3s is fine regardless of per-file size.

## Detection Logic

**Location:** `internal/scanner/scanner.go` — `ProcessBooksParallel`, inside the per-worker goroutine, immediately after the incremental-skip check.

**Condition:**
```
filePath is NOT a directory
AND os.Stat(filePath).Size() < config.AppConfig.MinBookSizeBytes
```

Multi-file books always have `FilePath` pointing to a directory (set by `groupFilesIntoBooks`), so `!info.IsDir()` is a reliable single-file discriminator with no false positives.

**Action sequence when triggered:**
1. `os.Stat` the file (cheap — reuse result from this block)
2. Call `extractInfoFromPath(&books[idx])` to populate title/author from path string
3. Set `books[idx].LibraryState = ptr("suspicious")`
4. Set `books[idx].FileSize = ptr(fi.Size())`
5. Call `saveBookToDatabase(books[idx], store, scanLog)` — minimal DB record
6. Log: `scanLog.Warn("suspicious file (%d bytes, threshold %d): %s", size, threshold, filePath)`
7. Update scan cache with current mtime+size (so subsequent scans hit incremental-skip)
8. `return` — skip all metadata extraction, AI, hashing, dedup

## Configuration

**File:** `internal/config/config.go`

New field added to `AppConfig`:
```go
MinBookSizeBytes int64 `json:"min_book_size_bytes"`
```

- Default: `10485760` (10 MB)
- Applied in the config loader: if `MinBookSizeBytes` is absent/unset, default to 10 MB
- `MinBookSizeBytes = -1` disables the check entirely (no books skipped)
- 10 MB at 64 kbps ≈ 22 minutes — generous lower bound; a real audiobook is rarely under 30 MB

Users who have intentionally small audiobooks (short stories, samples) can lower the threshold or set `-1` to disable.

## Database

**No migration required.** The `library_state` column is already `TEXT` with no constraint. `'suspicious'` joins the existing values (`'imported'`, `'organized'`, `'needs_review'`) as a new string value.

The saved record will have:
- `file_path` — the file path
- `file_size` — actual size in bytes
- `library_state = 'suspicious'`
- `title`, `author` — path-extracted (may be approximate)
- `format` — file extension
- Everything else null/zero

## API

No new endpoints. `GET /api/v1/audiobooks` already accepts a `library_state` query parameter. Suspicious books are immediately queryable via `?library_state=suspicious`.

Suspicious books are excluded from:
- Metadata enrichment batch jobs
- AI title/author parsing
- Write-back operations
- Dedup hashing
- iTunes path repair

They are included in:
- Library stats (total file count / size)
- The main book list when filtered by `library_state=suspicious`

## Frontend

Add `'suspicious'` as a selectable option in the library-state filter (wherever `'needs_review'`, `'organized'`, etc. appear). No new component needed.

Optionally: surface a count badge on the Diagnostics page showing pending suspicious files.

## Testing

**Unit test** (`scanner_test.go`):
- Create a `Book{FilePath: "/some/file.mp3"}` pointing to a small temp file
- Call the suspicious-file guard logic directly
- Assert: `saveBookToDatabase` called with `library_state = 'suspicious'`; no further processing occurs

**Integration test** (`scanner_test.go`):
- Write a 1-byte `.mp3` file to a temp dir
- Run `ProcessBooksParallel` against it
- Assert: DB record has `library_state = 'suspicious'`, `duration` is null, no AI job created

**Config test** (`config_test.go`):
- Assert default of 10 MB when `min_book_size_bytes` is absent or 0

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `MinBookSizeBytes int64` with 10 MB default |
| `internal/scanner/scanner.go` | Add early-return suspicious guard in `ProcessBooksParallel` |
| `web/src/` (filter component) | Add `suspicious` option to library-state filter |

## Out of Scope

- Per-file size checks within multi-file books (different problem, different solution)
- Automatic deletion of suspicious files (human review first)
- Duration-based detection (size is sufficient; duration requires full tag read which defeats the purpose)
