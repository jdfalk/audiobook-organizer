# Book Files Table Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `book_files` table that tracks every individual audio file, replacing `book_segments`. Every book gets rows — single-file or multi-file. Fixes multi-file ITL write-back, normalizes `book.file_path` to always be the directory, and enables per-file metadata display.

**Architecture:** New `book_files` table (migration 39) with 24 columns. `BookFile` struct and store interface methods follow existing patterns. Migration copies data from `book_segments`. Scanner, sync, organize, and rename pipelines populate/update `book_files`. Write-back reads per-file `itunes_path` from `book_files` instead of `book.ITunesPath`.

**Tech Stack:** Go, SQLite migrations, PebbleDB JSON, React/TypeScript

**Spec:** `docs/superpowers/specs/2026-03-28-book-files-table-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/database/store.go` | Add `BookFile` struct + store interface methods. Deprecation comment on `Book.ITunesPath`. |
| Modify | `internal/database/migrations.go` | Migration 39: create `book_files` table, migrate `book_segments` data |
| Modify | `internal/database/sqlite_store.go` | Implement BookFile CRUD + upsert for SQLite |
| Modify | `internal/database/pebble_store.go` | Implement BookFile CRUD + upsert for PebbleDB |
| Modify | `internal/database/mock_store.go` | Add mock implementations |
| Modify | `internal/scanner/scanner.go` | Replace `createSegmentsForBook` with `createBookFilesForBook` |
| Modify | `internal/server/server.go` | Add routes: `GET /audiobooks/:id/files`, backfill endpoint |
| Modify | `internal/server/maintenance_fixups.go` | Add `handleBackfillBookFiles` |
| Modify | `internal/server/itunes.go` | Populate `book_files` during sync, fix write-back-all |
| Modify | `internal/server/itunes_writeback_batcher.go` | Use `book_files` for ITL write-back |
| Modify | `internal/server/organize_service.go` | Update `book_files` during organize |
| Modify | `internal/server/rename_service.go` | Update `book_files.file_path` and `itunes_path` after rename |
| Modify | `internal/server/file_pipeline.go` | Use `BookFile` instead of `BookSegment` for path computation |
| Modify | `internal/database/mocks/mock_store.go` | Regenerate with new BookFile interface methods |
| Modify | `web/src/services/api.ts` | Add `BookFile` type + API functions |
| Modify | `web/src/pages/BookDetail.tsx` | Display `book_files` (replace segment display) |

---

## Task 1: BookFile Struct + Store Interface + Migration

**Files:**
- Modify: `internal/database/store.go`
- Modify: `internal/database/migrations.go`

- [ ] **Step 1: Add BookFile struct to store.go**

Add after the existing `BookSegment` struct:

```go
// BookFile represents an individual audio file within a book.
// Replaces BookSegment with additional fields for iTunes integration and audio metadata.
type BookFile struct {
	ID                 string    `json:"id"`
	BookID             string    `json:"book_id"`
	FilePath           string    `json:"file_path"`
	OriginalFilename   string    `json:"original_filename,omitempty"`
	ITunesPath         string    `json:"itunes_path,omitempty"`
	ITunesPersistentID string    `json:"itunes_persistent_id,omitempty"`
	TrackNumber        int       `json:"track_number,omitempty"`
	TrackCount         int       `json:"track_count,omitempty"`
	DiscNumber         int       `json:"disc_number,omitempty"`
	DiscCount          int       `json:"disc_count,omitempty"`
	Title              string    `json:"title,omitempty"`
	Format             string    `json:"format,omitempty"`
	Codec              string    `json:"codec,omitempty"`
	Duration           int       `json:"duration,omitempty"` // milliseconds
	FileSize           int64     `json:"file_size,omitempty"`
	BitrateKbps        int       `json:"bitrate_kbps,omitempty"`
	SampleRateHz       int       `json:"sample_rate_hz,omitempty"`
	Channels           int       `json:"channels,omitempty"`
	BitDepth           int       `json:"bit_depth,omitempty"`
	FileHash           string    `json:"file_hash,omitempty"`
	OriginalFileHash   string    `json:"original_file_hash,omitempty"`
	Missing            bool      `json:"missing"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Add store interface methods**

Add to the Store interface:

```go
// Book files (replaces book_segments for new code)
CreateBookFile(file *BookFile) error
UpdateBookFile(id string, file *BookFile) error
GetBookFiles(bookID string) ([]BookFile, error)
GetBookFileByPID(itunesPID string) (*BookFile, error)
GetBookFileByPath(filePath string) (*BookFile, error)
DeleteBookFile(id string) error
DeleteBookFilesForBook(bookID string) error
UpsertBookFile(file *BookFile) error
```

- [ ] **Step 3: Add deprecation comment on Book.ITunesPath**

Find `ITunesPath` field on the Book struct and add:

```go
// Deprecated: use book_files.itunes_path instead. Will be removed in a future migration.
ITunesPath         *string    `json:"itunes_path,omitempty"`
```

- [ ] **Step 4: Add migration 39**

In migrations.go, add migration that:
1. Creates `book_files` table with all 24 columns and indexes
2. Migrates existing `book_segments` data: maps numeric `book_id` to ULID string using the books table, copies file_path/format/size/duration/track_number/file_hash, converts duration from seconds to milliseconds
3. For books with no segments: creates a `book_files` row from `book.file_path` if it points to a file (not directory)

- [ ] **Step 5: Build**

Run: `go build ./internal/database/`
Expected: Build fails (interface methods not implemented) — that's OK for now

- [ ] **Step 6: Commit**

```bash
git add internal/database/store.go internal/database/migrations.go
git commit -m "feat: add BookFile struct, store interface, and migration 39"
```

---

## Task 2: SQLite Implementation

**Files:**
- Modify: `internal/database/sqlite_store.go`
- Modify: `internal/database/mock_store.go`

- [ ] **Step 1: Implement CreateBookFile**

Follow the `CreateBookSegment` pattern but with ULID string `book_id` and all 24 fields.

- [ ] **Step 2: Implement UpdateBookFile**

Update all mutable fields (file_path, itunes_path, track_number, file_hash, missing, etc.). Set `updated_at = NOW()`.

- [ ] **Step 3: Implement GetBookFiles**

Query `SELECT * FROM book_files WHERE book_id = ? ORDER BY track_number ASC, file_path ASC`.

- [ ] **Step 4: Implement GetBookFileByPID**

Query `SELECT * FROM book_files WHERE itunes_persistent_id = ? LIMIT 1`.

- [ ] **Step 5: Implement GetBookFileByPath**

Query `SELECT * FROM book_files WHERE file_path = ? LIMIT 1`.

- [ ] **Step 6: Implement DeleteBookFile and DeleteBookFilesForBook**

Standard DELETE queries.

- [ ] **Step 7: Implement UpsertBookFile**

Match priority:
1. If `itunes_persistent_id` is set: match by `(book_id, itunes_persistent_id)`
2. Else: match by `(book_id, file_path)`
3. If no match: INSERT
4. If match: UPDATE

- [ ] **Step 8: Add mock implementations**

In `mock_store.go`, add `*Func` fields and pass-through methods for all 8 new interface methods. Follow the existing pattern.

- [ ] **Step 9: Update generated mocks**

In `internal/database/mocks/mock_store.go`, add the 8 new BookFile methods with Call expectations matching the interface signatures.

- [ ] **Step 10: Build and test**

Run: `go build ./...` and `go test ./internal/database/ -v -count=1`

- [ ] **Step 11: Commit**

```bash
git add internal/database/sqlite_store.go internal/database/mock_store.go internal/database/mocks/mock_store.go
git commit -m "feat: implement BookFile CRUD for SQLite"
```

---

## Task 3: PebbleDB Implementation

**Files:**
- Modify: `internal/database/pebble_store.go`

- [ ] **Step 1: Implement all BookFile methods for PebbleDB**

Key schema:
- `book_file:<book_id>:<file_id>` → BookFile JSON
- `book_file_pid:<itunes_persistent_id>` → `<book_id>:<file_id>` (secondary index)
- `book_file_path:<crc32(file_path)>` → `<book_id>:<file_id>` (secondary index)

**Note:** Secondary index values store `<book_id>:<file_id>` (not just `file_id` as the spec says) because both are needed to reconstruct the primary key `book_file:<book_id>:<file_id>`. This is an intentional improvement over the spec.

Follow existing PebbleDB patterns (prefix scan for GetBookFiles, point lookups for GetByPID/GetByPath).

- [ ] **Step 2: Build and test**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```bash
git add internal/database/pebble_store.go
git commit -m "feat: implement BookFile CRUD for PebbleDB"
```

---

## Task 4: Scanner — Replace createSegmentsForBook

**Files:**
- Modify: `internal/scanner/scanner.go`

- [ ] **Step 1: Create `createBookFilesForBook` function**

Replace `createSegmentsForBook`. Same logic but:
- Use `BookFile` instead of `BookSegment`
- Use string `book.ID` instead of numeric CRC32 ID
- Set `Format` from file extension
- Set `OriginalFilename` from `filepath.Base()`
- Set `FileSize` from `os.Stat`
- Populate `Codec`, `BitrateKbps`, `SampleRateHz`, `Channels`, `BitDepth` by reading file tags via existing metadata extraction
- Use `UpsertBookFile` instead of `CreateBookSegment`

- [ ] **Step 2: Update callers**

Replace all calls to `createSegmentsForBook` with `createBookFilesForBook`.

- [ ] **Step 3: Normalize book.file_path to directory**

After creating book files, ensure `book.FilePath` points to the directory:
```go
if !isDir(book.FilePath) {
	book.FilePath = filepath.Dir(book.FilePath)
	store.UpdateBook(book.ID, book)
}
```

- [ ] **Step 4: Build and test**

Run: `go build ./...` and `go test ./internal/scanner/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat: scanner creates book_files instead of book_segments"
```

---

## Task 5: iTunes Sync — Populate book_files

**Files:**
- Modify: `internal/server/itunes.go`

- [ ] **Step 1: In executeITunesSync, create book_files for each matched track**

After matching a book with an iTunes track (the `if existing != nil` block), upsert a `BookFile`:

```go
store.UpsertBookFile(&database.BookFile{
	BookID:             existing.ID,
	FilePath:           remappedPath,
	ITunesPath:         firstTrack.Location, // raw file://localhost/... URL
	ITunesPersistentID: persistentID,
	TrackNumber:        firstTrack.TrackNumber,
	TrackCount:         firstTrack.TrackCount,
	DiscNumber:         firstTrack.DiscNumber,
	DiscCount:          firstTrack.DiscCount,
	Title:              firstTrack.Name,
	Format:             filepath.Ext(remappedPath),
	Duration:           int(firstTrack.TotalTime), // already ms
	FileSize:           firstTrack.Size,
})
```

For multi-track groups (40 MP3s), iterate all tracks in the group, not just `firstTrack`.

- [ ] **Step 2: Fix write-back-all to use book_files**

Replace the per-book approach with per-file:

```go
// For each book with PIDs:
files, _ := store.GetBookFiles(bookID)
for _, f := range files {
	if f.ITunesPersistentID != "" && f.ITunesPath != "" {
		itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
			PersistentID: f.ITunesPersistentID,
			NewLocation:  f.ITunesPath,
		})
	}
}
```

- [ ] **Step 3: Fix batcher flush() to use book_files**

Same change as write-back-all — iterate `book_files` instead of using `book.ITunesPath`.

- [ ] **Step 4: Build**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/server/itunes.go internal/server/itunes_writeback_batcher.go
git commit -m "feat: iTunes sync populates book_files, write-back uses per-file paths"
```

---

## Task 6: Organize + Rename — Update book_files

**Files:**
- Modify: `internal/server/organize_service.go`
- Modify: `internal/server/rename_service.go`
- Modify: `internal/server/file_pipeline.go`
- Modify: `internal/server/metadata_fetch_service.go`

- [ ] **Step 1: Update organize_service.go**

In `organizeDirectoryBook()` and `filterBooksNeedingOrganization()`:
- Replace `ListBookSegments(numericID)` with `GetBookFiles(book.ID)`
- Replace `CreateBookSegment` with `UpsertBookFile`
- In `copyBookWithNewLocation()`: create new `BookFile` rows with updated paths
- After organizing files, update each `BookFile.FilePath` and compute `ITunesPath`

- [ ] **Step 2: Update file_pipeline.go**

`ComputeTargetPaths()` currently takes `[]BookSegment`. Change to take `[]BookFile` or create a parallel function. Update path computation to use `BookFile` fields.

- [ ] **Step 3: Update rename_service.go**

After file renames, update the affected `BookFile` rows:
- `BookFile.FilePath` → new path
- `BookFile.ITunesPath` → recomputed from new path + mapping
- Call `UpdateBookFile()` for each affected file

- [ ] **Step 4: Update metadata_fetch_service.go**

This file has 20+ references to `BookSegment`/`ListBookSegments`. Key code paths to update:
- **Tag write hash update**: After `WriteTags` succeeds, update `BookFile.FileHash`
- **runApplyPipeline rename path**: Replace `ListBookSegments` with `GetBookFiles`, update `BookFile.FilePath` and `ITunesPath` after rename
- **RunApplyPipelineRenameOnly**: Same — replace segment usage with `BookFile`
- **computeITunesPath calls**: Apply to each `BookFile`, not just `book.ITunesPath`
- **ComputeTargetPaths call sites**: Pass `[]BookFile` instead of `[]BookSegment`

- [ ] **Step 4: Build and test**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/server/organize_service.go internal/server/file_pipeline.go internal/server/metadata_fetch_service.go
git commit -m "feat: organize and rename pipelines update book_files"
```

---

## Task 7: Backfill Endpoint

**Files:**
- Modify: `internal/server/maintenance_fixups.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add handleBackfillBookFiles handler**

```go
func (s *Server) handleBackfillBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	// 1. Query all books
	// 2. For each book:
	//    a. If book_files rows exist, skip
	//    b. If file_path is a file: normalize to directory, create one BookFile row
	//    c. If file_path is a directory: glob audio files, create BookFile per file
	//    d. Populate format, size, duration from os.Stat + tag reading
	// 3. Return summary
}
```

- [ ] **Step 2: Add route**

```go
protected.POST("/maintenance/backfill-book-files", s.handleBackfillBookFiles)
```

- [ ] **Step 3: Build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/server/maintenance_fixups.go internal/server/server.go
git commit -m "feat: add backfill-book-files maintenance endpoint"
```

---

## Task 8: API + Frontend

**Files:**
- Modify: `internal/server/server.go`
- Modify: `web/src/services/api.ts`
- Modify: `web/src/pages/BookDetail.tsx`

- [ ] **Step 1: Add GET /audiobooks/:id/files endpoint**

Returns `BookFile[]` for a book, sorted by track_number. Include `file_exists` check (same pattern as segment listing).

**Note:** Existing `book_segments`-based handlers in `server.go` (`GET /audiobooks/:id/segments`, segment tags, move-segments, split-version, etc.) are NOT migrated in this PR. They continue to work against the old `book_segments` table (read-only retention). The new `/files` endpoint coexists with the old `/segments` endpoint. Migration of those handlers happens in a future PR after `book_segments` is fully deprecated.

- [ ] **Step 2: Add BookFile type to api.ts**

```typescript
export interface BookFile {
	id: string;
	book_id: string;
	file_path: string;
	original_filename?: string;
	itunes_path?: string;
	itunes_persistent_id?: string;
	track_number?: number;
	track_count?: number;
	disc_number?: number;
	disc_count?: number;
	title?: string;
	format?: string;
	codec?: string;
	duration?: number;
	file_size?: number;
	bitrate_kbps?: number;
	sample_rate_hz?: number;
	channels?: number;
	bit_depth?: number;
	file_hash?: string;
	missing: boolean;
	file_exists?: boolean;
	created_at: string;
	updated_at: string;
}
```

Add `getBookFiles(bookId: string)` and `backfillBookFiles(dryRun: boolean)`.

- [ ] **Step 3: Update BookDetail.tsx**

Replace `segments` state with `bookFiles`. Replace `api.getBookSegments` with `api.getBookFiles`. Update the Files & History display to render `BookFile` fields. Keep backward compat — fall back to segments API if files endpoint returns empty.

- [ ] **Step 4: Build frontend**

Run: `cd web && npm run build`

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go web/src/services/api.ts web/src/pages/BookDetail.tsx
git commit -m "feat: add book_files API and frontend display"
```

---

## Task 9: Integration Tests + Full Build

- [ ] **Step 1: Write BookFile upsert test**

Test that verifies upsert matching priority:
- PID match takes precedence over path match
- Path match creates/updates when no PID
- No match creates new row

- [ ] **Step 2: Write multi-track iTunes sync test**

Update `TestITunesImport_MultiTrackBookSegments` (if it exists) or create a new test that:
- Syncs a multi-track book from iTunes XML
- Verifies `book_files` rows are created per-track (not one per book)
- Verifies each row has `itunes_persistent_id` and `itunes_path`

- [ ] **Step 3: Write backfill test**

Test that:
- Books with `file_path` pointing to a file get normalized to directory
- Books with `file_path` pointing to a directory get `book_files` rows for each audio file inside
- `missing=true` is set for non-existent paths

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/database/ -v && go test ./internal/server/ -v && go test ./internal/scanner/ -v`

- [ ] **Step 5: Full backend build**

Run: `go build ./...`

- [ ] **Step 6: Full frontend build**

Run: `cd web && npm run build`

- [ ] **Step 7: Commit tests and fixes**

---

## Summary

| Task | Description | Key Files |
|------|-------------|-----------|
| 1 | BookFile struct + interface + migration | store.go, migrations.go |
| 2 | SQLite implementation | sqlite_store.go, mock_store.go |
| 3 | PebbleDB implementation | pebble_store.go |
| 4 | Scanner creates book_files | scanner.go |
| 5 | iTunes sync populates book_files + write-back fix | itunes.go, batcher |
| 6 | Organize + rename update book_files | organize_service.go, file_pipeline.go |
| 7 | Backfill endpoint | maintenance_fixups.go |
| 8 | API + frontend | server.go, api.ts, BookDetail.tsx |
| 9 | Integration test + full build | — |
