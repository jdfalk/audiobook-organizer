# External ID Mapping (PID Map)

**Date:** 2026-03-15
**Status:** Approved

## Problem

When books are merged, deleted, or reimported, external IDs (iTunes PIDs, Audible ASINs, etc.) are lost because they're stored as a single field on the Book record. This causes:
- Reimport of deleted books on next iTunes sync
- Lost linkage after merge operations
- No way to block reimport of intentionally removed books
- Lookup requires scanning all books instead of direct index hit

## Solution

A dedicated external ID mapping table that maps any external identifier to a book ID, with tombstone support for blocking reimport. Generalizes beyond iTunes to any external ID system.

## Database Schema

### SQLite (Migration 34)

```sql
CREATE TABLE IF NOT EXISTS external_id_map (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,          -- 'itunes', 'audible', 'google_books', etc.
    external_id TEXT NOT NULL,     -- the actual PID/ASIN/etc.
    book_id TEXT NOT NULL,         -- our book ID
    track_number INTEGER,          -- position within the book (NULL for single-file)
    file_path TEXT,                -- original file path at import time
    tombstoned INTEGER DEFAULT 0,  -- 1 = don't reimport this ID
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ext_id_source_eid ON external_id_map(source, external_id);
CREATE INDEX IF NOT EXISTS idx_ext_id_book ON external_id_map(book_id);
CREATE INDEX IF NOT EXISTS idx_ext_id_tombstone ON external_id_map(source, tombstoned) WHERE tombstoned = 0;
```

### PebbleDB Key Schema

```
ext_id:<source>:<external_id>  ->  ExternalIDMapping JSON
ext_id:book:<book_id>:<source>:<external_id>  ->  external_id (reverse index)
```

## Store Interface

```go
type ExternalIDMapping struct {
    ID          int       `json:"id"`
    Source      string    `json:"source"`       // "itunes", "audible", etc.
    ExternalID  string    `json:"external_id"`  // the PID/ASIN
    BookID      string    `json:"book_id"`
    TrackNumber *int      `json:"track_number,omitempty"`
    FilePath    string    `json:"file_path,omitempty"`
    Tombstoned  bool      `json:"tombstoned"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

Add to `database.Store`:

```go
// External ID mapping
CreateExternalIDMapping(mapping *ExternalIDMapping) error
GetBookByExternalID(source, externalID string) (string, error)  // returns book_id
GetExternalIDsForBook(bookID string) ([]ExternalIDMapping, error)
IsExternalIDTombstoned(source, externalID string) (bool, error)
TombstoneExternalID(source, externalID string) error
ReassignExternalIDs(oldBookID, newBookID string) error  // for merges
BulkCreateExternalIDMappings(mappings []ExternalIDMapping) error  // for import
```

## Data Flow

### iTunes Sync (Import)

```
Parse XML → group tracks by album
For each album:
  1. Check GetBookByExternalID("itunes", album_tracks[0].pid)
     → Book exists? Update metadata, done.
     → Tombstoned? Skip entirely.
     → Not found? Create new Book.
  2. For each track in album:
     BulkCreateExternalIDMappings([]{
       Source: "itunes", ExternalID: track.pid,
       BookID: book.id, TrackNumber: track.number
     })
```

### Merge Books

```
MergeBooks(bookIDs, primaryID):
  1. Normal merge (set version group, primary)
  2. ReassignExternalIDs(each non-primary book ID → primary book ID)
  // All PIDs now point to the surviving book
```

### Delete Book

```
DeleteBook(bookID):
  1. GetExternalIDsForBook(bookID)
  2. For each: TombstoneExternalID(source, externalID)
  3. Delete the book record
  // PIDs are tombstoned, reimport is blocked
```

### Lookup (Is this file already imported?)

```
// Old way: scan all books by PID field — O(n)
// New way:
bookID, err := store.GetBookByExternalID("itunes", pid)
// Single indexed lookup — O(1)
```

## Migration Path

### Backfill existing data

After migration 34, run a one-time backfill:

```go
// For every book with itunes_persistent_id set:
books := store.GetAllBooks(...)
for _, book := range books {
    if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
        store.CreateExternalIDMapping(&ExternalIDMapping{
            Source:     "itunes",
            ExternalID: *book.ITunesPersistentID,
            BookID:     book.ID,
        })
    }
}
```

The `books.itunes_persistent_id` field stays for backward compatibility but is no longer the authority. The `external_id_map` table is.

### Gradual cutover

1. **Phase 1:** Add table and Store methods. Backfill from existing PID fields. iTunes sync writes to BOTH the old field and the new table.
2. **Phase 2:** Merge and delete operations update the new table. Lookup uses the new table.
3. **Phase 3:** iTunes sync reads from the new table only. Old field becomes read-only/deprecated.

## Scale

- 88,427 iTunes track PIDs → 88K rows in `external_id_map`
- Future: Audible ASINs, Google Books IDs, Open Library IDs add more rows
- Unique index on `(source, external_id)` keeps lookups O(1)
- Reverse index on `book_id` makes "find all IDs for a book" fast

## Edge Cases

- **Same file, different PIDs:** iTunes can reassign PIDs. The old PID stays mapped (won't cause reimport) and the new PID gets a new mapping.
- **PID collision across sources:** Impossible — `source` + `external_id` is the unique key, not just `external_id`.
- **Tombstoned book gets reimported manually:** User explicitly imports → tombstone is cleared and new mapping created.
- **Bulk import from fresh iTunes XML:** `BulkCreateExternalIDMappings` with `INSERT OR IGNORE` semantics — existing mappings untouched.

## Testing

- Unit: CRUD operations on external_id_map
- Unit: Tombstone blocks reimport
- Unit: ReassignExternalIDs on merge
- Integration: iTunes sync creates mappings, second sync finds them
- Integration: Delete + tombstone → reimport blocked
