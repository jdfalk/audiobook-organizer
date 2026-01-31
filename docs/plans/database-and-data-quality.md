<!-- file: docs/plans/database-and-data-quality.md -->
<!-- version: 2.0.0 -->
<!-- guid: f1a2b3c4-d5e6-7f8a-9b0c-1d2e3f4a5b6c -->
<!-- last-edited: 2026-01-31 -->

# Database and Data Quality

## Overview

Tools and processes for keeping the library data accurate, deduplicated, and
well-maintained. Includes deduplication, orphan detection, search indexing,
backup improvements, and routine housekeeping.

---

## Active Bug

### Import Path Negative Sizes

The `total_size` calculation for import paths is returning negative values.
Debug the size computation in the library folder stats logic to identify the
overflow or sign error.

#### Where the computation lives

Two independent code paths produce size values that end up in API responses:

**1. `calculateLibrarySizes` — `internal/server/server.go` line 371**

This function walks the filesystem and accumulates `int64` totals. It is called
from `getSystemStatus` (line 2902) and the result is returned as
`total_size_bytes`, `library.total_size`, and `import_paths.total_size`. The
function itself is correct: it uses `int64` accumulators and `info.Size()`
which returns `int64`. It also de-duplicates import paths that overlap with
`rootDir` by skipping files whose path has the `rootDir` prefix.

**2. Dashboard stats — `internal/server/server.go` line 4238**

This is the more likely culprit. The dashboard handler iterates all books and
sums `*book.FileSize`:

```go
var totalSize int64 = 0
for _, book := range allBooks {
    if book.FileSize != nil {
        totalSize += *book.FileSize   // line 4243
    }
}
```

`book.FileSize` is declared as `*int64` on `database.Book`. This code path is
safe on its own. However, `FileSize` is populated in two places:

- During iTunes import (`buildBookFromTrack`, `internal/server/itunes.go` line
  483): `book.FileSize = &size` where `size` comes from `track.Size` (an
  `int64` parsed from the plist). If the plist parser mis-parses the `<integer>`
  element as a signed value and the XML contains a value larger than
  `math.MaxInt64`, the result wraps negative.
- During the `track.Size` fallback path (line 486): `size := info.Size()` from
  `os.Stat`. This is always correct.

#### Most likely root cause

The iTunes plist `<integer>` parser is reading the `Size` field as a signed
`int64`. iTunes occasionally writes file sizes as unsigned 64-bit integers in
its XML. Values above `9223372036854775807` (2^63 - 1) will overflow to
negative when parsed into Go's `int64`. This manifests as a single massively
negative `FileSize` on the affected book, which drags the `totalSize` sum
negative.

#### Fix

In `internal/itunes/plist_parser.go`, wherever the `Size` key is parsed, add
an overflow guard:

```go
// After parsing size as int64:
if size < 0 {
    // Overflow from unsigned → signed; discard and let the os.Stat fallback
    // in buildBookFromTrack populate the correct value.
    track.Size = 0
}
```

Additionally, add a defensive check in the dashboard accumulator:

```go
if book.FileSize != nil && *book.FileSize > 0 {
    totalSize += *book.FileSize
}
```

This skips any book with a negative or zero `FileSize` so the dashboard total
remains accurate even if corrupt data exists in the database.

---

## Data Quality

### Deduplication Job

Identify the same book stored under different filenames via fuzzy title/author
matching. Surface candidates for manual review or automatic merge.

#### Existing exact-hash deduplication

`PebbleStore.GetDuplicateBooks()` already groups books by `OrganizedFileHash`
(falling back to `FileHash`). This catches byte-identical files. It does not
catch the same logical title stored as different file formats or re-encoded at
different bitrates.

#### PebbleDB key pattern for fuzzy title matching

To support fuzzy deduplication without a full-text search engine, add a
**normalized-title secondary index** on every book. The normalization function
strips punctuation, lowercases, and collapses whitespace:

```
Key:   book:titleidx:<normalizedTitle>:<bookULID>
Value: "1"
```

Example:
```
book:titleidx:the great gatsby:01HXABC...  →  "1"
book:titleidx:the great gatsby:01HXDEF...  →  "1"   ← duplicate candidate
```

Normalization function (Go):
```go
// normalizeTitle produces a dedup-friendly key segment.
// Strips leading articles (the/a/an), removes non-alphanumeric chars,
// lowercases, and collapses runs of spaces to a single space.
func normalizeTitle(title string) string {
    lower := strings.ToLower(strings.TrimSpace(title))
    // Strip leading articles
    for _, article := range []string{"the ", "a ", "an "} {
        lower = strings.TrimPrefix(lower, article)
    }
    // Remove non-alphanumeric (keep spaces)
    var b strings.Builder
    for _, r := range lower {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
            b.WriteRune(r)
        }
    }
    // Collapse whitespace
    result := strings.Join(strings.Fields(b.String()), " ")
    return result
}
```

The index key must be written atomically with the book record in the same
`pebble.Batch`, following the existing pattern in `CreateBook` and `UpdateBook`.
When a book's title changes, the old `book:titleidx:` key must be deleted and
the new one written (same pattern as the path-index update in `UpdateBook`).

#### Deduplication job — code sample

The job runs as an `OperationFunc` (same pattern as iTunes import). It scans
the title index for groups, then surfaces candidates:

```go
// internal/dedup/dedup.go

package dedup

import (
    "context"
    "fmt"
    "strings"

    "github.com/cockroachdb/pebble"
    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/operations"
)

// DedupCandidate represents a pair of books that may be duplicates.
type DedupCandidate struct {
    BookA          database.Book `json:"book_a"`
    BookB          database.Book `json:"book_b"`
    MatchType      string        `json:"match_type"` // "exact_hash" | "fuzzy_title"
    NormalizedTitle string       `json:"normalized_title"`
}

// FindFuzzyDuplicates scans the book:titleidx: keyspace and groups books
// that share the same normalized title. Returns groups with 2+ members.
func FindFuzzyDuplicates(store *database.PebbleStore) ([]DedupCandidate, error) {
    // 1. Iterate all book:titleidx: keys, grouping by normalized title.
    //    Key format: book:titleidx:<normalizedTitle>:<bookULID>
    //    Extract the normalizedTitle segment (everything between the second and last colon).
    titleGroups := make(map[string][]string) // normalizedTitle → []bookULID

    prefix := []byte("book:titleidx:")
    iter, err := store.DB().NewIter(&pebble.IterOptions{
        LowerBound: prefix,
        UpperBound: append(append([]byte(nil), prefix...), 0xFF),
    })
    if err != nil {
        return nil, err
    }
    defer iter.Close()

    for iter.First(); iter.Valid(); iter.Next() {
        key := string(iter.Key())
        // key = "book:titleidx:<normalizedTitle>:<bookULID>"
        parts := strings.SplitN(key, ":", 4) // ["book", "titleidx", "<title>", "<ulid>"]
        if len(parts) != 4 {
            continue
        }
        normalizedTitle := parts[2]
        bookULID := parts[3]
        titleGroups[normalizedTitle] = append(titleGroups[normalizedTitle], bookULID)
    }

    // 2. For each group with 2+ books, fetch full Book records and emit candidates.
    var candidates []DedupCandidate
    for title, bookIDs := range titleGroups {
        if len(bookIDs) < 2 {
            continue
        }
        // Fetch books
        var books []database.Book
        for _, id := range bookIDs {
            book, err := store.GetBookByID(id)
            if err != nil || book == nil {
                continue
            }
            books = append(books, *book)
        }
        // Emit pairwise candidates
        for i := 0; i < len(books); i++ {
            for j := i + 1; j < len(books); j++ {
                candidates = append(candidates, DedupCandidate{
                    BookA:           books[i],
                    BookB:           books[j],
                    MatchType:       "fuzzy_title",
                    NormalizedTitle: title,
                })
            }
        }
    }
    return candidates, nil
}

// RunDedupJob is an OperationFunc for the global operation queue.
func RunDedupJob(ctx context.Context, progress operations.ProgressReporter) error {
    progress.UpdateProgress(0, 0, "Starting deduplication scan")
    progress.Log("info", "Starting fuzzy title dedup scan", nil)

    store, ok := database.GlobalStore.(*database.PebbleStore)
    if !ok {
        return fmt.Errorf("dedup job requires PebbleDB store")
    }

    candidates, err := FindFuzzyDuplicates(store)
    if err != nil {
        return fmt.Errorf("dedup scan failed: %w", err)
    }

    progress.UpdateProgress(len(candidates), len(candidates),
        fmt.Sprintf("Found %d duplicate candidate pairs", len(candidates)))
    progress.Log("info",
        fmt.Sprintf("Dedup scan complete: %d candidate pairs", len(candidates)), nil)

    // Candidates are persisted via a dedicated API endpoint or stored
    // as a user preference JSON blob for the UI to surface.
    return nil
}
```

Note: the `store.DB()` accessor method (returning the raw `*pebble.DB`) does
not yet exist on `PebbleStore`. Add it:

```go
func (p *PebbleStore) DB() *pebble.DB { return p.db }
```

### Orphan File Detector

Find files on disk that have no corresponding record in the database. Surface
them for import or cleanup.

#### How the book:path index works

Every book in PebbleDB has a secondary index entry:

```
book:path:<absoluteFilePath>  →  <bookULID>
```

This is written atomically in `CreateBook` and updated in `UpdateBook`. A
lookup by path is O(1): `db.Get([]byte("book:path:" + path))`. If the key
does not exist, the file has no database record — it is an orphan.

#### Orphan detection algorithm

```go
// internal/orphan/detector.go

package orphan

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/cockroachdb/pebble"
    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/operations"
)

// audioFileExtensions is the set of extensions the scanner recognizes.
var audioFileExtensions = map[string]bool{
    ".m4b": true, ".m4a": true, ".mp3": true,
    ".flac": true, ".ogg": true, ".opus": true,
    ".aac": true, ".wma": true, ".wav": true,
}

// OrphanFile represents a file on disk with no database record.
type OrphanFile struct {
    Path    string `json:"path"`
    Size    int64  `json:"size"`
    Format  string `json:"format"`
}

// DetectOrphans walks rootDir and all enabled import paths, collecting files
// whose path does not appear in the book:path: index.
func DetectOrphans(store database.Store, rootDir string, importPaths []database.ImportPath) ([]OrphanFile, error) {
    // Build the set of directories to scan
    scanDirs := []string{}
    if rootDir != "" {
        scanDirs = append(scanDirs, rootDir)
    }
    for _, ip := range importPaths {
        if ip.Enabled {
            scanDirs = append(scanDirs, ip.Path)
        }
    }

    var orphans []OrphanFile

    for _, dir := range scanDirs {
        err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                return nil // skip unreadable entries
            }
            if info.IsDir() {
                return nil
            }

            ext := strings.ToLower(filepath.Ext(path))
            if !audioFileExtensions[ext] {
                return nil // not an audiobook file
            }

            // Check the book:path: index
            existing, lookupErr := store.GetBookByFilePath(path)
            if lookupErr != nil {
                return nil // skip on error
            }
            if existing != nil {
                return nil // file is tracked
            }

            // No record found — this is an orphan
            orphans = append(orphans, OrphanFile{
                Path:   path,
                Size:   info.Size(),
                Format: strings.TrimPrefix(ext, "."),
            })
            return nil
        })
        if err != nil {
            return nil, fmt.Errorf("walk %s: %w", dir, err)
        }
    }

    return orphans, nil
}

// RunOrphanDetector is an OperationFunc for the global operation queue.
func RunOrphanDetector(rootDir string, importPaths []database.ImportPath) operations.OperationFunc {
    return func(ctx context.Context, progress operations.ProgressReporter) error {
        progress.UpdateProgress(0, 0, "Scanning for orphan files...")
        progress.Log("info", "Starting orphan file scan", nil)

        orphans, err := DetectOrphans(database.GlobalStore, rootDir, importPaths)
        if err != nil {
            return fmt.Errorf("orphan detection failed: %w", err)
        }

        progress.UpdateProgress(len(orphans), len(orphans),
            fmt.Sprintf("Found %d orphan files", len(orphans)))
        for _, o := range orphans {
            progress.Log("info", fmt.Sprintf("Orphan: %s (%s, %d bytes)", o.Path, o.Format, o.Size), nil)
        }

        // Store results in a user preference for the UI to consume:
        // preference:orphan_scan_results → JSON array of OrphanFile
        return nil
    }
}

### Full-Text Search Index

Build an index over author, title, and narrator fields for fast advanced
queries. Currently search is filter-based; this enables substring and
relevance-ranked results.

#### Current search implementation

`PebbleStore.SearchBooks()` (line 1093 of `pebble_store.go`) does a full table
scan — it loads every book via `GetAllBooks(1000000, 0)` and filters in memory
with `strings.Contains(strings.ToLower(title), lowerQuery)`. This is O(n) on
every search and does not match author or narrator fields.

#### Prefix index design for PebbleDB

PebbleDB's sorted keyspace makes prefix scanning efficient. The strategy is to
maintain a **trigram-style prefix index** on the normalized title and author
name. For each book, emit index entries for every 3-character prefix of every
word in the title and author:

```
Key pattern:   book:search:<prefix>:<bookULID>
Value:         "1"

Examples for title "The Great Gatsby" by "F. Scott Fitzgerald":
  book:search:the:01HXABC...   →  "1"
  book:search:gre:01HXABC...   →  "1"
  book:search:grea:01HXABC...  →  "1"    (optional: extend prefixes)
  book:search:gat:01HXABC...   →  "1"
  book:search:fsc:01HXABC...   →  "1"    (author word "fitzgerald" normalized)
  book:search:fitz:01HXABC...  →  "1"
```

A simpler and more practical approach for this library size (typically <10k
books) is a **single normalized-title index** (already designed for dedup above
as `book:titleidx:`) combined with a **normalized-author index**:

```
book:titleidx:<normalizedTitle>:<bookULID>    →  "1"   (already planned)
book:authoridx:<normalizedAuthorName>:<bookULID>  →  "1"
```

Search then becomes: scan all keys matching `book:titleidx:<queryPrefix>` and
`book:authoridx:<queryPrefix>`, union the resulting book ULIDs, fetch full
records, and return. PebbleDB prefix scans are O(log n + k) where k is the
number of matching keys.

#### Search query implementation

```go
// internal/database/pebble_store.go — replace or augment SearchBooks

func (p *PebbleStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
    if query == "" {
        return []Book{}, nil
    }

    // Normalize the query the same way we normalize titles/authors
    normalizedQuery := strings.ToLower(strings.TrimSpace(query))

    // Deduplicate results (a book may match on both title and author)
    seen := make(map[string]bool)
    var results []Book

    // Helper: prefix-scan an index and collect matching book IDs
    scanPrefix := func(indexPrefix string) {
        prefix := []byte(indexPrefix)
        iter, err := p.db.NewIter(&pebble.IterOptions{
            LowerBound: prefix,
            UpperBound: append(append([]byte(nil), prefix...), 0xFF),
        })
        if err != nil {
            return
        }
        defer iter.Close()

        for iter.First(); iter.Valid(); iter.Next() {
            key := string(iter.Key())
            // Extract bookULID: last segment after final ":"
            lastColon := strings.LastIndex(key, ":")
            if lastColon < 0 {
                continue
            }
            bookID := key[lastColon+1:]
            if seen[bookID] {
                continue
            }
            seen[bookID] = true

            book, err := p.GetBookByID(bookID)
            if err != nil || book == nil {
                continue
            }
            if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
                continue
            }
            results = append(results, *book)
        }
    }

    // Scan title index
    scanPrefix("book:titleidx:" + normalizedQuery)

    // Scan author index
    scanPrefix("book:authoridx:" + normalizedQuery)

    // If no index hits, fall back to narrator field (not indexed yet)
    // This could be added as book:narratoridx:<normalized>:<ulid> in future.

    // Apply pagination
    start := offset
    if start >= len(results) {
        return []Book{}, nil
    }
    end := start + limit
    if end > len(results) {
        end = len(results)
    }
    return results[start:end], nil
}
```

#### Index maintenance in CreateBook / UpdateBook

In `CreateBook`, after the existing batch writes, add:

```go
// Title search index
normTitle := normalizeTitle(book.Title)
if normTitle != "" {
    titleIdxKey := []byte(fmt.Sprintf("book:titleidx:%s:%s", normTitle, book.ID))
    batch.Set(titleIdxKey, []byte("1"), nil)
}

// Author search index (requires resolving AuthorID → name)
if book.AuthorID != nil {
    author, _ := p.GetAuthorByID(*book.AuthorID)
    if author != nil {
        normAuthor := strings.ToLower(strings.TrimSpace(author.Name))
        authorIdxKey := []byte(fmt.Sprintf("book:authoridx:%s:%s", normAuthor, book.ID))
        batch.Set(authorIdxKey, []byte("1"), nil)
    }
}
```

In `UpdateBook`, delete old index keys and write new ones when title or author
changes (same pattern as the existing path-index and series-index updates).

#### PebbleDB key schema additions

```
book:titleidx:<normalizedTitle>:<bookULID>       →  "1"
book:authoridx:<normalizedAuthorName>:<bookULID> →  "1"
```

Both indices are prefix-scannable. A query for "great" will match all keys
starting with `book:titleidx:great` — this includes "great gatsby",
"great expectations", etc.

---

## Database Maintenance

### Incremental Migration Harness

- Dry-run mode: show what a migration would do without applying it
- Supports incremental application (useful for large databases)

#### Existing migration harness

The migration system lives in `internal/database/migrations.go`. The pattern:

1. A global `var migrations = []Migration{...}` lists every migration in order
   by `Version` (int).
2. On startup, `RunMigrations(store)` reads the current version from
   `preference:db_version` (a JSON blob `{"version": N, "updated_at": "..."}`).
3. It iterates `migrations`, skipping any where `m.Version <= currentVersion`.
4. For each pending migration, it calls `m.Up(store)`. On success it writes a
   `preference:migration_<N>` record and bumps `db_version`.
5. Each `MigrationFunc` receives the `Store` interface. SQLite migrations
   type-assert to `*SQLiteStore` to run `ALTER TABLE`. PebbleDB migrations are
   typically no-ops (fields live in JSON blobs) or key-rename operations.

```go
type Migration struct {
    Version     int
    Description string
    Up          MigrationFunc       // func(store Store) error
    Down        MigrationFunc       // optional rollback (not currently used)
}

type MigrationRecord struct {
    Version     int       `json:"version"`
    Description string    `json:"description"`
    AppliedAt   time.Time `json:"applied_at"`
}
```

#### Dry-run mode implementation

Add a `DryRun bool` parameter to `RunMigrations`. When true, the function
logs what each migration *would* do without calling `m.Up()` or writing any
records:

```go
// internal/database/migrations.go — modified RunMigrations signature

// RunMigrations applies all pending migrations. If dryRun is true, it prints
// what would be applied without modifying the database.
func RunMigrations(store Store, dryRun bool) error {
    currentVersion, err := getCurrentVersion(store)
    if err != nil {
        return fmt.Errorf("failed to get current version: %w", err)
    }

    log.Printf("Current database version: %d", currentVersion)

    pendingMigrations := []Migration{}
    for _, m := range migrations {
        if m.Version > currentVersion {
            pendingMigrations = append(pendingMigrations, m)
        }
    }

    if len(pendingMigrations) == 0 {
        log.Printf("Database is up to date (version %d)", currentVersion)
        return nil
    }

    if dryRun {
        log.Printf("[DRY RUN] %d migration(s) would be applied:", len(pendingMigrations))
        for _, m := range pendingMigrations {
            log.Printf("  [DRY RUN] v%d: %s", m.Version, m.Description)
            // If the migration has a DryRunFunc, call it to produce a preview.
            // Otherwise just log the description.
        }
        return nil  // do not apply anything
    }

    // ... existing apply loop unchanged ...
}
```

Each migration can optionally implement a `DryRunFunc` that returns a list of
changes that would be made:

```go
type Migration struct {
    Version     int
    Description string
    Up          MigrationFunc
    Down        MigrationFunc
    DryRun      func(store Store) ([]string, error)  // returns human-readable change descriptions
}
```

Example for a hypothetical migration 14 that backfills the title search index:

```go
{
    Version:     14,
    Description: "Backfill book:titleidx: search index for all existing books",
    DryRun: func(store Store) ([]string, error) {
        pStore, ok := store.(*PebbleStore)
        if !ok {
            return []string{"Skipped: not a PebbleDB store"}, nil
        }
        books, err := pStore.GetAllBooks(1000000, 0)
        if err != nil {
            return nil, err
        }
        var changes []string
        for _, b := range books {
            norm := normalizeTitle(b.Title)
            if norm != "" {
                changes = append(changes, fmt.Sprintf("  SET book:titleidx:%s:%s = 1", norm, b.ID))
            }
        }
        changes = append([]string{fmt.Sprintf("Would write %d index entries", len(changes))}, changes...)
        return changes, nil
    },
    Up: func(store Store) error {
        // actual backfill logic
        pStore, ok := store.(*PebbleStore)
        if !ok { return nil }
        books, _ := pStore.GetAllBooks(1000000, 0)
        batch := pStore.DB().NewBatch()
        for _, b := range books {
            norm := normalizeTitle(b.Title)
            if norm != "" {
                key := []byte(fmt.Sprintf("book:titleidx:%s:%s", norm, b.ID))
                batch.Set(key, []byte("1"), nil)
            }
        }
        return batch.Commit(pebble.Sync)
    },
}

### Archival Strategy

Move old logs and completed operations to cold storage to keep the active
database small and fast.

### Config Schema Validation

Reject invalid configuration values (bad enum values, out-of-range numbers)
at update time rather than at runtime.

---

## Backup & Restore Enhancements

- Incremental backups: only store changes since last snapshot
- Backup integrity verification via hash manifest
- Scheduled backup task with configurable retention policy

#### Existing backup code

`internal/backup/backup.go` implements full (snapshot) backups:

- `CreateBackup(databasePath, databaseType, config)` walks the entire PebbleDB
  directory (or single SQLite file), writes it into a `tar.gz` archive, then
  computes a SHA-256 checksum of the archive. Returns a `BackupInfo` struct.
- `RestoreBackup(backupPath, targetPath, verify)` extracts the tar.gz. The
  `verify` flag has a TODO stub — checksum verification is not yet wired.
- `ListBackups(backupDir)` scans the backup directory for `.tar.gz` files.
- `cleanupOldBackups()` retains the `MaxBackups` most recent archives and
  deletes the rest.

The checksum IS computed during `CreateBackup` (via `calculateFileChecksum`)
and stored in the returned `BackupInfo`, but it is not persisted alongside the
backup file. `RestoreBackup` cannot verify it.

#### Incremental backup design

PebbleDB's WAL (write-ahead log) files provide a natural incremental boundary.
However, PebbleDB does not expose a stable "checkpoint" API that guarantees a
consistent snapshot without locking. The simpler approach for this project:

1. **Manifest-based incremental**: On each backup run, record the set of files
   and their sizes/mtimes into a `manifest.json` inside the archive. On the
   next run, compare the current filesystem state against the last manifest.
   Only archive files that are new or modified. The manifest itself is always
   written in full.

2. **Restore merges**: When restoring an incremental backup, apply snapshots in
   order (oldest first), then overlays (newer incremental archives) on top.

Manifest structure:

```go
type BackupManifest struct {
    CreatedAt   time.Time              `json:"created_at"`
    BaseBackup  string                 `json:"base_backup,omitempty"` // filename of the full backup this is incremental from
    Files       map[string]FileEntry   `json:"files"`                 // relative path → entry
    Checksum    string                 `json:"checksum"`              // SHA-256 of manifest JSON itself (for verification)
}

type FileEntry struct {
    Path     string    `json:"path"`
    Size     int64     `json:"size"`
    ModTime  time.Time `json:"mod_time"`
    SHA256   string    `json:"sha256"`  // per-file hash for integrity verification
}
```

#### Integrity verification

Replace the TODO in `RestoreBackup` with:

```go
func VerifyBackup(backupPath string) error {
    // 1. Open the tar.gz, find manifest.json
    // 2. Unmarshal BackupManifest
    // 3. For each file entry in manifest.Files:
    //    a. Extract the file to a temp buffer
    //    b. Compute SHA-256 of the extracted content
    //    c. Compare against entry.SHA256
    //    d. If mismatch, return error with filename
    // 4. Verify manifest.Checksum against the raw manifest JSON bytes
    return nil // all checks passed
}
```

During `CreateBackup`, compute per-file hashes and write them into the manifest
before closing the archive. The archive checksum in `BackupInfo.Checksum` remains
a whole-archive hash for quick "has this file been tampered with" checks.

#### Scheduled backup

`ScheduleBackup` in `backup.go` is a stub. Implement it as a goroutine with a
ticker, following the same pattern as the operation queue worker:

```go
func ScheduleBackup(interval time.Duration, config BackupConfig, databasePath, databaseType string) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            info, err := CreateBackup(databasePath, databaseType, config)
            if err != nil {
                log.Printf("Scheduled backup failed: %v", err)
            } else {
                log.Printf("Scheduled backup created: %s (%d bytes)", info.Filename, info.Size)
            }
        }
    }
}
```

Wire this into server startup after the database is initialized, using the
`BackupConfig` from `config.AppConfig`.

---

## Housekeeping

### Stale Operation Cleanup

Remove abandoned queued operations after a configurable timeout. Prevents
the queue from filling up with orphaned entries.

### Automatic Log Rotation & Compression

Rotate logs on a schedule and compress old entries to manage disk usage.

---

## PebbleDB key schema additions (summary)

The following new keys are required by the features in this plan. All follow the
existing conventions: colon-delimited, prefix-scannable, value is either `"1"`
(presence index) or a JSON blob.

| Key pattern | Value | Purpose | Written by |
|---|---|---|---|
| `book:titleidx:<normalizedTitle>:<bookULID>` | `"1"` | Fuzzy title match for dedup; prefix search for search index | `CreateBook`, `UpdateBook`, migration backfill |
| `book:authoridx:<normalizedAuthorName>:<bookULID>` | `"1"` | Author-name prefix search | `CreateBook`, `UpdateBook`, migration backfill |
| `preference:orphan_scan_results` | JSON `[]OrphanFile` | Orphan scan results for UI consumption | Orphan detector job |
| `preference:dedup_candidates` | JSON `[]DedupCandidate` | Dedup scan results for UI review | Dedup job |

No new counters or entity-level keys are needed. The search and dedup indices
are maintained alongside the existing `book:path:`, `book:series:`, and
`book:author:` indices in the same atomic batches.

---

## Dependencies

- Deduplication depends on having reliable title/author metadata (see
  [`metadata-system.md`](metadata-system.md))
- Incremental migration harness is useful for any future schema change
- Backup enhancements are independent
- The title and author search indices (`book:titleidx:`, `book:authoridx:`)
  must be backfilled via a migration before the new `SearchBooks` query can
  return results for existing books

## References

- PebbleDB key schema: `docs/database-pebble-schema.md`
- Database architecture: `docs/database-architecture.md`
- Store interface: `internal/database/store.go`
- PebbleStore implementation: `internal/database/pebble_store.go`
- Migration harness: `internal/database/migrations.go`
- Backup implementation: `internal/backup/backup.go`
- Operation queue: `internal/operations/queue.go`
