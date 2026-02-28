<!-- file: docs/plans/2026-02-28-phase1c-fix-history-and-timestamps.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e -->
<!-- last-edited: 2026-02-28 -->

# Phase 1C: Fix History Recording & Timestamps

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure all metadata changes record history; fix `updated_at` to only change on real edits; add `metadata_updated_at` and `last_written_at` columns.
**Architecture:** Add change detection to `UpdateBook`, new DB migration for timestamp columns, add explicit history recording in the field extractor loop.
**Tech Stack:** Go, SQLite

---

## Background & Root Causes

### Problem 1: History not recorded for all edits

The `metadata_changes_history` table exists (migration 19) and the recording infrastructure works. The gaps are:

1. **`AudiobookService.UpdateAudiobook` field extractor loop** (lines 698–717 of `internal/server/audiobook_service.go`): when it writes `entry.OverrideValue = value` into `state[field]`, it never calls `recordChange`. The state-saving path in `MetadataStateService.SaveMetadataState` does NOT emit history entries — only `SetOverride` and `UpdateFetchedMetadata` do. So direct field-extractor loop writes are silent.

2. **AI parse path**: Phase 1A routes AI parse through `AudiobookService` (fixing the `GlobalStore.UpdateBook` bypass), but if the AI-parsed values arrive as fields on the `Book` struct rather than through `RawPayload`, the field extractor loop skips them (`if _, ok := req.RawPayload[field]; !ok { continue }`).

### Problem 2: `updated_at` fires on every save

`internal/database/sqlite_store.go` `UpdateBook()` (line 1653–1696) unconditionally sets `book.UpdatedAt = &now` before every `UPDATE`. This means internal operations (file-hash updates, iTunes sync, cache invalidation) silently bump the timestamp even when user-visible metadata did not change.

### New columns needed

- `metadata_updated_at`: set only when user-visible metadata fields change. Lets the UI show "last edited" separate from internal system updates.
- `last_written_at`: set when write-back to audio files occurs. Lets Phase 2's "Save to Files" button know if the files are current.

---

## Files Modified by This Plan

| File | Change |
|------|--------|
| `internal/database/store.go` | Add `MetadataUpdatedAt`, `LastWrittenAt` fields to `Book` struct |
| `internal/database/sqlite_store.go` | `UpdateBook`: fetch old book, compare key fields, conditional `updated_at`; also update `metadata_updated_at` and `last_written_at` when appropriate |
| `internal/database/migrations.go` | Add `migration022Up` for the two new columns; register it in `migrations` slice; bump file version |
| `internal/server/audiobook_service.go` | In the field extractor loop, call `mss.recordChange` for each field that actually changes value; also record history for explicit overrides |
| `internal/server/audiobook_service.go` | Add a `SetLastWrittenAt(id string)` helper for the write-back path |
| `internal/database/sqlite_store.go` | Add `bookSelectColumns` update (if column list is hardcoded) so the new columns are scanned |

---

## Task 1: Add New Fields to `Book` Struct

**Files:**
- `internal/database/store.go`

**Context:** The `Book` struct is at line 234. `UpdatedAt` is at line 284. The two new timestamp fields must be added alongside it so they flow through serialization, scanning, and the store interface naturally.

**Step 1 — Read the file** to confirm current struct ending (lines 280–295):
```
internal/database/store.go lines 280–295
```

**Step 2 — Add two fields** immediately after `UpdatedAt *time.Time` (line 284):
```go
// UpdatedAt is set on every DB write (system-level).
UpdatedAt *time.Time `json:"updated_at,omitempty"`
// MetadataUpdatedAt is set only when user-visible metadata fields change.
MetadataUpdatedAt *time.Time `json:"metadata_updated_at,omitempty"`
// LastWrittenAt is set when metadata is written back to the audio files on disk.
LastWrittenAt *time.Time `json:"last_written_at,omitempty"`
```

Replace the existing `UpdatedAt` line with the block above (the old line has no doc comment; add one for clarity).

**Step 3 — Bump the file header version** from current to `+0.1.0` (e.g. if it was `1.5.0` make it `1.6.0`). The `// version:` comment is on line 2.

**Step 4 — Verify compilation:**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./internal/database/...
```
Expected: no errors. If `bookSelectColumns` is a string constant that lists columns, the build will succeed but the new columns won't be scanned yet — that is fixed in Task 3.

**Step 5 — No test yet** — tests will be added after the full change set is in place (Task 6).

---

## Task 2: Database Migration 022 — Add Two Timestamp Columns

**Files:**
- `internal/database/migrations.go`

**Context:** The current highest migration is 21 (`migration021Up`, line 1104). The pattern for adding nullable `DATETIME` columns to `books` is established in migration 12. The `migrations` var slice ends at line 167.

**Step 1 — Add the migration entry** to the `migrations` slice, immediately after version 21's entry (around line 161–167):
```go
{
    Version:     22,
    Description: "Add metadata_updated_at and last_written_at timestamp columns to books",
    Up:          migration022Up,
    Down:        nil,
},
```

**Step 2 — Add the migration function** at the end of the file (after `migration021Up` ends at line ~1138):
```go
// migration022Up adds metadata_updated_at and last_written_at timestamp columns to books table.
// metadata_updated_at is set only when user-visible metadata changes; last_written_at is set
// when metadata is written back to audio files on disk.
func migration022Up(store Store) error {
    log.Println("  - Adding metadata_updated_at and last_written_at to books table")

    sqliteStore, ok := store.(*SQLiteStore)
    if !ok {
        log.Println("  - Non-SQLite store detected, skipping SQL migration")
        return nil
    }

    alterStatements := []string{
        "ALTER TABLE books ADD COLUMN metadata_updated_at DATETIME",
        "ALTER TABLE books ADD COLUMN last_written_at DATETIME",
    }

    for _, stmt := range alterStatements {
        log.Printf("    - Executing: %s", stmt)
        if _, err := sqliteStore.db.Exec(stmt); err != nil {
            if strings.Contains(err.Error(), "duplicate column name") {
                log.Printf("    - Column already exists, skipping")
                continue
            }
            return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
        }
    }

    // Backfill: set metadata_updated_at = updated_at for existing books that already
    // have an updated_at. This preserves the approximate "last edited" time for
    // books that were already in the library before this migration.
    if _, err := sqliteStore.db.Exec(
        `UPDATE books SET metadata_updated_at = updated_at WHERE updated_at IS NOT NULL AND metadata_updated_at IS NULL`,
    ); err != nil {
        return fmt.Errorf("failed to backfill metadata_updated_at: %w", err)
    }

    log.Println("  - metadata_updated_at and last_written_at added successfully")
    return nil
}
```

**Step 3 — Bump the file header version** (`// version: 1.14.0` → `1.15.0`).

**Step 4 — Verify compilation:**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./internal/database/...
```

**Step 5 — Verify migration runs on a fresh DB** (uses the in-memory SQLite path in tests):
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go test ./internal/database/... -run TestMigrations -v
```
Expected: all migration tests pass; migration 22 appears in output.

---

## Task 3: Wire New Columns into `UpdateBook` (sqlite_store.go)

**Files:**
- `internal/database/sqlite_store.go`

**Context:** `UpdateBook` is at line 1653. The SQL `UPDATE` statement lists all columns (lines 1658–1670). The query args list follows (lines 1671–1683). The `bookSelectColumns` constant (or inline string) near the top of the file controls which columns are `SELECT`-ed by `GetBookByID` and others — we must add the new columns there too.

### Sub-task 3A: Find and update `bookSelectColumns`

**Step 1 — Search for the column list constant:**
```bash
grep -n "bookSelectColumns\|SELECT.*created_at.*updated_at" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/sqlite_store.go | head -20
```

**Step 2 — Add `metadata_updated_at, last_written_at`** to the SELECT column list, in the same position as they appear in the `books` table (after `updated_at`). The exact string to append after `updated_at` is:
```
, metadata_updated_at, last_written_at
```

**Step 3 — Add scan targets** in the `scanBook` function (or equivalent inline scan). Wherever `&book.UpdatedAt` is scanned, add `&book.MetadataUpdatedAt, &book.LastWrittenAt` immediately after it in the same order as the SELECT clause.

### Sub-task 3B: Change detection in `UpdateBook`

Replace the current `UpdateBook` function body (lines 1653–1696) with the version below. Key changes:
1. Fetch the existing book before the update.
2. Compare metadata fields — set `updated_at` unconditionally (it tracks ALL writes for debugging), but set `metadata_updated_at` only when a user-visible field actually changed.
3. Do NOT change `last_written_at` here — that is set exclusively by the write-back path (Task 5).

```go
func (s *SQLiteStore) UpdateBook(id string, book *Book) (*Book, error) {
    // Always stamp updated_at — this tracks every DB write for debugging.
    now := time.Now()
    book.UpdatedAt = &now

    // Fetch the current book to detect whether metadata actually changed.
    // If the fetch fails (e.g. book does not exist yet), we proceed without
    // metadata_updated_at logic so we don't break the create path.
    current, fetchErr := s.GetBookByID(id)

    if fetchErr == nil && current != nil && metadataChanged(current, book) {
        book.MetadataUpdatedAt = &now
    } else if fetchErr == nil && current != nil {
        // Preserve the existing metadata_updated_at value — nothing changed.
        book.MetadataUpdatedAt = current.MetadataUpdatedAt
    }

    // Never touch last_written_at in UpdateBook. It is set by SetLastWrittenAt only.
    if current != nil {
        book.LastWrittenAt = current.LastWrittenAt
    }

    query := `UPDATE books SET
        title = ?, author_id = ?, series_id = ?, series_sequence = ?,
        file_path = ?, original_filename = ?, format = ?, duration = ?,
        work_id = ?, narrator = ?, edition = ?, language = ?, publisher = ?,
        print_year = ?, audiobook_release_year = ?, isbn10 = ?, isbn13 = ?,
        itunes_persistent_id = ?, itunes_date_added = ?, itunes_play_count = ?, itunes_last_played = ?,
        itunes_rating = ?, itunes_bookmark = ?, itunes_import_source = ?,
        file_hash = ?, file_size = ?, bitrate_kbps = ?, codec = ?, sample_rate_hz = ?, channels = ?,
        bit_depth = ?, quality = ?, is_primary_version = ?, version_group_id = ?, version_notes = ?,
        original_file_hash = ?, organized_file_hash = ?, library_state = ?, quantity = ?,
        marked_for_deletion = ?, marked_for_deletion_at = ?,
        updated_at = ?, metadata_updated_at = ?, last_written_at = ?,
        cover_url = ?, narrators_json = ?
    WHERE id = ?`
    result, err := s.db.Exec(query,
        book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence,
        book.FilePath, book.OriginalFilename, book.Format, book.Duration,
        book.WorkID, book.Narrator, book.Edition, book.Language, book.Publisher,
        book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13,
        book.ITunesPersistentID, book.ITunesDateAdded, book.ITunesPlayCount, book.ITunesLastPlayed,
        book.ITunesRating, book.ITunesBookmark, book.ITunesImportSource,
        book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
        book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
        book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity,
        book.MarkedForDeletion, book.MarkedForDeletionAt,
        book.UpdatedAt, book.MetadataUpdatedAt, book.LastWrittenAt,
        book.CoverURL, book.NarratorsJSON, id,
    )
    if err != nil {
        return nil, err
    }
    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return nil, err
    }
    if rowsAffected == 0 {
        return nil, fmt.Errorf("book not found")
    }
    book.ID = id
    return book, nil
}
```

### Sub-task 3C: Add `metadataChanged` helper

Add this function directly before `UpdateBook` in `sqlite_store.go`:

```go
// metadataChanged returns true if any user-visible metadata field differs between
// old and new. Internal-only fields (FileHash, LibraryState, ITunes*, etc.) are
// intentionally excluded so that system updates do not bump metadata_updated_at.
func metadataChanged(old, new *Book) bool {
    if old.Title != new.Title {
        return true
    }
    if !equalIntPtr(old.AuthorID, new.AuthorID) {
        return true
    }
    if !equalIntPtr(old.SeriesID, new.SeriesID) {
        return true
    }
    if !equalIntPtr(old.SeriesSequence, new.SeriesSequence) {
        return true
    }
    if !equalStringPtr(old.Narrator, new.Narrator) {
        return true
    }
    if !equalStringPtr(old.Publisher, new.Publisher) {
        return true
    }
    if !equalStringPtr(old.Language, new.Language) {
        return true
    }
    if !equalIntPtr(old.AudiobookReleaseYear, new.AudiobookReleaseYear) {
        return true
    }
    if !equalIntPtr(old.PrintYear, new.PrintYear) {
        return true
    }
    if !equalStringPtr(old.ISBN10, new.ISBN10) {
        return true
    }
    if !equalStringPtr(old.ISBN13, new.ISBN13) {
        return true
    }
    if !equalStringPtr(old.CoverURL, new.CoverURL) {
        return true
    }
    if !equalStringPtr(old.NarratorsJSON, new.NarratorsJSON) {
        return true
    }
    return false
}

// equalStringPtr returns true if both pointers are nil, or both point to equal strings.
func equalStringPtr(a, b *string) bool {
    if a == nil && b == nil {
        return true
    }
    if a == nil || b == nil {
        return false
    }
    return *a == *b
}

// equalIntPtr returns true if both pointers are nil, or both point to equal ints.
func equalIntPtr(a, b *int) bool {
    if a == nil && b == nil {
        return true
    }
    if a == nil || b == nil {
        return false
    }
    return *a == *b
}
```

> **Caution:** Check whether `equalStringPtr` or `equalIntPtr` already exist anywhere in the `database` package. If they do, omit the duplicates. Search with:
> ```bash
> grep -rn "func equalStringPtr\|func equalIntPtr" \
>   /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/
> ```

### Sub-task 3D: Add `SetLastWrittenAt` to the store interface and implementation

**`internal/database/interface.go`** — add to the `Store` interface (find the section with `UpdateBook`):
```go
// SetLastWrittenAt stamps the last_written_at column for the given book ID.
// Called exclusively by the file write-back path.
SetLastWrittenAt(id string, t time.Time) error
```

**`internal/database/sqlite_store.go`** — add implementation:
```go
// SetLastWrittenAt stamps the last_written_at timestamp for book id.
func (s *SQLiteStore) SetLastWrittenAt(id string, t time.Time) error {
    _, err := s.db.Exec(
        `UPDATE books SET last_written_at = ? WHERE id = ?`,
        t, id,
    )
    return err
}
```

**`internal/database/mock_store.go`** and **`internal/database/mocks/mock_store.go`** — add stub implementations:
```go
func (m *MockStore) SetLastWrittenAt(id string, t time.Time) error {
    return nil
}
```

**Step — Verify compilation:**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./...
```

**Step — Bump version headers** in `sqlite_store.go` (+0.1.0) and `interface.go` (+0.1.0).

---

## Task 4: Record History in the Field Extractor Loop

**Files:**
- `internal/server/audiobook_service.go`

**Context:** The field extractor loop is at lines 698–717. It writes `entry.OverrideValue = value` but never records a history entry. The `MetadataStateService.recordChange` method (in `metadata_state_service.go` line 134) is what creates `MetadataChangeRecord` rows. We need to call it here whenever the new value differs from the old value.

**Step 1 — Confirm `AudiobookService` has access to a `MetadataStateService`** by searching:
```bash
grep -n "MetadataStateService\|mss\b\|metaStateSvc" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/audiobook_service.go | head -20
```

If `AudiobookService` does not hold a `MetadataStateService` reference, find how it calls `loadMetadataState` / `saveMetadataState` (likely package-level helpers that use a global). Identify the `recordChange`-equivalent accessible from this context.

**Step 2 — Replace the inner body of the field extractor loop** (lines 707–716) with:
```go
if value, ok := extractor(); ok {
    log.Printf("[DEBUG] UpdateAudiobook: creating state for field %s with value %v", field, value)
    entry := state[field]
    oldValue := entry.OverrideValue

    entry.OverrideValue = value
    entry.OverrideLocked = true
    entry.UpdatedAt = now
    state[field] = entry

    // Record history only when the value actually changed.
    if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", value) {
        if mss := getMetadataStateService(); mss != nil {
            mss.recordChange(id, field, "manual", "user_edit", oldValue, value)
        }
    }
} else {
    log.Printf("[DEBUG] UpdateAudiobook: extractor for field %s returned false/nil", field)
}
```

> **Note:** Replace `getMetadataStateService()` with the actual accessor pattern used in `audiobook_service.go`. If the file uses a package-level `GlobalMetadataStateService` variable, use that directly. If `AudiobookService` has a field `mss *MetadataStateService`, use `svc.mss`. Determine the correct accessor by reading lines 1–50 of `audiobook_service.go` to see the struct definition.

**Step 3 — Also record history for explicit overrides** in `req.Updates.Overrides` (find the loop that processes overrides in `UpdateAudiobook`). For each override applied, call:
```go
mss.recordChange(id, field, "override", "user_edit", oldOverrideValue, newOverrideValue)
```
Read the override-processing section (search for `req.Updates.Overrides` in `audiobook_service.go`) to see where to insert this.

**Step 4 — Bump version header** of `audiobook_service.go` (+0.1.0).

**Step 5 — Verify compilation:**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./internal/server/...
```

---

## Task 5: Wire `SetLastWrittenAt` into the Write-Back Path

**Files:**
- Wherever `tagger.WriteMetadata` or `fileops.WriteMetadata` is called in the server package.

**Step 1 — Find the write-back call sites:**
```bash
grep -rn "WriteMetadata\|writeTags\|tagger\.Write\|write.*tag\|tag.*write" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/ \
  --include="*.go" -i | head -20
```

**Step 2 — After each successful write-back**, add:
```go
if err := GlobalStore.SetLastWrittenAt(bookID, time.Now()); err != nil {
    log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", bookID, err)
    // Non-fatal: do not return an error for this housekeeping stamp.
}
```

Replace `GlobalStore` with the actual store reference used at that call site. Replace `bookID` with the actual string ID variable at that call site.

**Step 3 — Verify compilation** after each edit:
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./...
```

---

## Task 6: Tests

**Files to create or modify:**
- `internal/database/sqlite_store_timestamp_test.go` (new file)
- `internal/server/audiobook_service_history_test.go` (new file)

### Test file 1: `internal/database/sqlite_store_timestamp_test.go`

```go
// file: internal/database/sqlite_store_timestamp_test.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package database_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestUpdateBook_MetadataUpdatedAt_OnlyChangesWhenMetadataChanges verifies that
// metadata_updated_at is set when title changes, but not when only file_hash changes.
func TestUpdateBook_MetadataUpdatedAt_OnlyChangesWhenMetadataChanges(t *testing.T) {
    store := setupTestSQLiteStore(t) // uses existing helper from sqlite_test.go

    book := &Book{
        ID:       newTestULID(),
        Title:    "Original Title",
        FilePath: "/tmp/test.m4b",
        Format:   "m4b",
    }
    _, err := store.CreateBook(book)
    require.NoError(t, err)

    // First update: change title — metadata_updated_at should be set.
    book.Title = "New Title"
    updated, err := store.UpdateBook(book.ID, book)
    require.NoError(t, err)
    assert.NotNil(t, updated.MetadataUpdatedAt, "metadata_updated_at should be set when title changes")
    firstMetaTs := updated.MetadataUpdatedAt

    time.Sleep(10 * time.Millisecond)

    // Second update: change only file_hash (system field) — metadata_updated_at should NOT change.
    hash := "abc123"
    book.FileHash = &hash
    updated2, err := store.UpdateBook(book.ID, book)
    require.NoError(t, err)
    assert.Equal(t, firstMetaTs, updated2.MetadataUpdatedAt,
        "metadata_updated_at should NOT change when only system fields change")
}

// TestUpdateBook_UpdatedAt_AlwaysChanges verifies that updated_at changes on every write.
func TestUpdateBook_UpdatedAt_AlwaysChanges(t *testing.T) {
    store := setupTestSQLiteStore(t)

    book := &Book{
        ID:       newTestULID(),
        Title:    "Stable Title",
        FilePath: "/tmp/test2.m4b",
        Format:   "m4b",
    }
    _, err := store.CreateBook(book)
    require.NoError(t, err)

    // First update
    time.Sleep(5 * time.Millisecond)
    book.Format = "mp3"
    updated1, err := store.UpdateBook(book.ID, book)
    require.NoError(t, err)
    ts1 := updated1.UpdatedAt

    // Second update immediately after
    time.Sleep(5 * time.Millisecond)
    updated2, err := store.UpdateBook(book.ID, book)
    require.NoError(t, err)
    ts2 := updated2.UpdatedAt

    assert.True(t, ts2.After(*ts1), "updated_at should always advance on each write")
}

// TestSetLastWrittenAt verifies that SetLastWrittenAt stamps the column correctly.
func TestSetLastWrittenAt(t *testing.T) {
    store := setupTestSQLiteStore(t)

    book := &Book{
        ID:       newTestULID(),
        Title:    "Write-back Book",
        FilePath: "/tmp/writeback.m4b",
        Format:   "m4b",
    }
    _, err := store.CreateBook(book)
    require.NoError(t, err)

    // Verify initially nil
    fetched, err := store.GetBookByID(book.ID)
    require.NoError(t, err)
    assert.Nil(t, fetched.LastWrittenAt, "last_written_at should initially be nil")

    // Stamp it
    writeTime := time.Now().Truncate(time.Second)
    err = store.SetLastWrittenAt(book.ID, writeTime)
    require.NoError(t, err)

    // Verify it was set
    fetched2, err := store.GetBookByID(book.ID)
    require.NoError(t, err)
    require.NotNil(t, fetched2.LastWrittenAt)
    assert.WithinDuration(t, writeTime, *fetched2.LastWrittenAt, time.Second)
}

// TestMigration022_BackfillsMetadataUpdatedAt verifies that existing rows with
// updated_at get their metadata_updated_at backfilled during migration 022.
func TestMigration022_BackfillsMetadataUpdatedAt(t *testing.T) {
    // This test requires running the migration from version 21 → 22 on a DB
    // that already has a book with updated_at set.
    // Use the testutil integration store which runs all migrations.
    store := setupTestSQLiteStore(t)

    // After migration 022, any book with updated_at should have metadata_updated_at != nil
    book := &Book{
        ID:       newTestULID(),
        Title:    "Backfill Test",
        FilePath: "/tmp/backfill.m4b",
        Format:   "m4b",
    }
    _, err := store.CreateBook(book)
    require.NoError(t, err)

    // Manually stamp updated_at to simulate a pre-migration row
    _, err = store.UpdateBook(book.ID, book)
    require.NoError(t, err)

    fetched, err := store.GetBookByID(book.ID)
    require.NoError(t, err)
    // After migration 022, metadata_updated_at is either set by the backfill
    // or by the first UpdateBook call (which now sets it). Either way, not nil.
    assert.NotNil(t, fetched.MetadataUpdatedAt)
}
```

### Test file 2: `internal/server/audiobook_service_history_test.go`

This test verifies that the field extractor loop creates `metadata_changes_history` rows.

```go
// file: internal/server/audiobook_service_history_test.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

package server_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    // Import testutil or the integration helper that sets up a real SQLite store.
    // Check internal/testutil/ for SetupIntegration or equivalent.
)

// TestUpdateAudiobook_FieldExtractorRecordsHistory verifies that updating the title
// through UpdateAudiobook creates a metadata_changes_history entry.
func TestUpdateAudiobook_FieldExtractorRecordsHistory(t *testing.T) {
    // Use setupTestServer(t) which creates a real SQLite store (see server_test.go).
    srv := setupTestServer(t)

    // Create a book to edit.
    book, err := srv.store.CreateBook(&database.Book{
        ID:       newULID(),
        Title:    "Before Title",
        FilePath: "/tmp/history_test.m4b",
        Format:   "m4b",
    })
    require.NoError(t, err)

    svc := NewAudiobookService(srv.store)

    // Update the title through the service layer so history is recorded.
    req := &UpdateAudiobookRequest{
        Updates: AudiobookUpdates{
            Title: "After Title",
        },
        RawPayload: map[string]any{
            "title": "After Title",
        },
    }
    _, err = svc.UpdateAudiobook(context.Background(), book.ID, req)
    require.NoError(t, err)

    // Verify a history entry was recorded.
    history, err := srv.store.GetMetadataChangeHistory(book.ID, 10, 0)
    require.NoError(t, err)
    require.NotEmpty(t, history, "expected at least one history entry after title change")

    // Find the "title" entry.
    var found bool
    for _, h := range history {
        if h.Field == "title" {
            found = true
            assert.Contains(t, *h.NewValue, "After Title")
            assert.Contains(t, *h.PreviousValue, "Before Title")
        }
    }
    assert.True(t, found, "expected a history entry for the 'title' field")
}

// TestUpdateAudiobook_NoHistoryWhenValueUnchanged verifies that saving the same
// title twice does NOT create a duplicate history entry.
func TestUpdateAudiobook_NoHistoryWhenValueUnchanged(t *testing.T) {
    srv := setupTestServer(t)

    book, err := srv.store.CreateBook(&database.Book{
        ID:       newULID(),
        Title:    "Stable Title",
        FilePath: "/tmp/stable.m4b",
        Format:   "m4b",
    })
    require.NoError(t, err)

    svc := NewAudiobookService(srv.store)
    req := &UpdateAudiobookRequest{
        Updates:    AudiobookUpdates{Title: "Stable Title"},
        RawPayload: map[string]any{"title": "Stable Title"},
    }

    // First save
    _, err = svc.UpdateAudiobook(context.Background(), book.ID, req)
    require.NoError(t, err)

    historyBefore, _ := srv.store.GetMetadataChangeHistory(book.ID, 100, 0)
    count1 := len(historyBefore)

    // Second save with same value
    _, err = svc.UpdateAudiobook(context.Background(), book.ID, req)
    require.NoError(t, err)

    historyAfter, _ := srv.store.GetMetadataChangeHistory(book.ID, 100, 0)
    count2 := len(historyAfter)

    assert.Equal(t, count1, count2, "no new history entries when value is unchanged")
}
```

**Step — Run both test files:**
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/... -run "TestUpdateBook_MetadataUpdatedAt|TestUpdateBook_UpdatedAt|TestSetLastWrittenAt|TestMigration022" -v
go test ./internal/server/... -run "TestUpdateAudiobook_FieldExtractorRecordsHistory|TestUpdateAudiobook_NoHistoryWhenValueUnchanged" -v
```

---

## Task 7: Expose New Timestamps in the API Response

**Files:**
- The JSON serialization of `Book` in `internal/database/store.go` is automatic via `json` tags (already added in Task 1). No handler changes are needed for the fields to appear in `GET /api/v1/audiobooks/:id` responses since the `Book` struct is returned directly.
- **Frontend `web/src/api.ts`**: if the `Book` TypeScript type is manually typed, add the two new optional fields:
  ```typescript
  metadata_updated_at?: string;
  last_written_at?: string;
  ```
  Search for the Book type:
  ```bash
  grep -n "metadata_updated_at\|last_written_at\|updated_at" \
    /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web/src/api.ts | head -10
  ```
  If `updated_at` is present in the TypeScript type, add the two new fields next to it.

**Step — No E2E changes needed** — the new fields are additive and will appear automatically.

---

## Task 8: Full Test Suite Verification

Run the complete test suite to confirm nothing is broken:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Backend unit + integration tests
go test ./... -timeout 120s 2>&1 | tail -30

# Coverage check (must stay ≥ 80%)
go test ./... -coverprofile=coverage.out -timeout 120s
go tool cover -func=coverage.out | grep total

# Frontend tests
cd web && npm test -- --run 2>&1 | tail -20

# E2E (Playwright) — requires the server to be running
make test-e2e
```

Expected outcomes:
- All Go tests pass.
- Total coverage ≥ 81% (currently 81.3%; new tests should hold or improve it).
- All 23 frontend tests pass.
- All 134 E2E tests pass.

---

## Implementation Order

Execute tasks in this order to avoid compiler errors at each step:

1. **Task 1** — add fields to `Book` struct (struct change first, no logic yet).
2. **Task 2** — add migration 022 (schema only, can land independently).
3. **Task 3** — update `UpdateBook`, scan columns, add helpers, add `SetLastWrittenAt`. This is the largest chunk; do sub-tasks A→B→C→D in order.
4. **Task 4** — add history recording in the field extractor loop.
5. **Task 5** — wire `SetLastWrittenAt` into write-back path.
6. **Task 6** — add tests.
7. **Task 7** — TypeScript type update (optional, purely additive).
8. **Task 8** — full suite verification.

After each task, run:
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./...
```
to catch compilation errors early.

---

## Edge Cases & Gotchas

| Situation | Expected Behaviour |
|-----------|-------------------|
| `GetBookByID` fails inside `UpdateBook` (race/corruption) | Log a warning, proceed with update, skip `metadata_updated_at` logic. Do not return error from the fetch failure — the primary update must still proceed. |
| `metadataChanged` called with `new.NarratorsJSON` different only in JSON key order | Will spuriously detect a change. Acceptable for now — field-level ordering is stable because JSON is produced by `json.Marshal` which sorts map keys. |
| `SetLastWrittenAt` called for a book ID that does not exist | `UPDATE` affects 0 rows — no error is returned. The caller should verify the book exists before calling, but this is non-fatal. |
| `equalStringPtr` / `equalIntPtr` already exist | Search before adding. If they exist in any file within `internal/database/`, do not redeclare — just reference them. |
| History recorded for `author_name` vs `author_id` | The field extractor loop uses the resolved `author_name` string in the `state` map. This is consistent with what `metadata_fetch_service.go`'s `recordChangeHistory` records. Keep it as `author_name` for user-readable history. |
| Mock store `SetLastWrittenAt` | Both `internal/database/mock_store.go` and `internal/database/mocks/mock_store.go` need stub implementations. Run `go build ./...` and fix any "does not implement" errors. |
| `GetMetadataChangeHistory` used in tests | This function must already exist on the Store interface (it was added in an earlier session). Verify with: `grep -n "GetMetadataChangeHistory" internal/database/interface.go` |

---

## Acceptance Criteria

- [ ] `GET /api/v1/audiobooks/:id` response includes `metadata_updated_at` (non-null for books with metadata edits) and `last_written_at` (null until write-back occurs).
- [ ] Editing a book's title twice with the same value creates exactly one new history entry (for the first change), not two.
- [ ] Updating only `file_hash` (e.g. re-scan) does NOT advance `metadata_updated_at`.
- [ ] `updated_at` advances on every `UpdateBook` call regardless of what changed.
- [ ] `last_written_at` is null until write-back; after write-back it is set.
- [ ] All existing tests continue to pass (no regressions).
- [ ] Migration 022 runs cleanly on an existing database and backfills `metadata_updated_at` from `updated_at`.
