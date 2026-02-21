# PebbleDB v2 Upgrade Plan

## Current State

- **Current version**: `github.com/cockroachdb/pebble v1.1.5`
- **Target version**: `github.com/cockroachdb/pebble/v2 v2.1.4` (latest as of 2026-01-21)
- **Import path change**: `github.com/cockroachdb/pebble` → `github.com/cockroachdb/pebble/v2`

## Files That Import Pebble

| File | Pebble API calls | Purpose |
|------|----------------:|---------|
| `internal/database/pebble_store.go` | 109 | Main data store (books, authors, series, operations, etc.) |
| `internal/openlibrary/store.go` | 10 | Open Library dump import/lookup |
| `internal/database/settings.go` | 2 | Settings persistence |
| `cmd/diagnostics.go` | 2 | DB diagnostics command |
| `internal/database/pebble_store_test.go` | ~20 | Tests |
| `internal/database/pebble_coverage_test.go` | ~10 | Coverage tests |

## What v2 Brings

### New Features Worth Adopting

1. **FormatColumnarBlocks (format 19)** — Columnar SSTable format for better compression and read performance on wide key spaces. Our store has many different key prefixes (`book:`, `author:`, `series:`, `op:`, etc.) — columnar blocks could improve prefix-heavy iteration.

2. **DeleteRange improvements** — We already use `DeleteRange` in `Reset()`. v2 has better excise support that could make bulk deletes faster.

3. **Blob file support** — Could be useful for storing cover art or audio metadata blobs directly in Pebble instead of external files.

4. **EventuallyFileOnlySnapshot** — Useful for background operations (bulk metadata fetch, organization) that need a consistent view without holding memory.

5. **Better compaction scheduling** — `ConcurrencyLimitScheduler` for controlling background I/O during imports.

6. **External file ingestion** — Could enable importing pre-sorted SSTs for bulk Open Library dump loading instead of individual `Set()` calls.

### Breaking Changes

1. **Import path**: `github.com/cockroachdb/pebble` → `github.com/cockroachdb/pebble/v2`
2. **Minimum format version**: v2 requires at least `FormatFlushableIngest` (format 13). Databases created with v1.1.5 should already be at format 16+, so this is likely fine.
3. **No RocksDB compat**: v2 drops RocksDB format support. Not relevant to us.
4. **API changes**: Some Options fields may have moved or been renamed. Need to verify.

## Migration Steps

### Step 1: Update Import Path (Mechanical)

```bash
# Update go.mod
go get github.com/cockroachdb/pebble/v2@v2.1.4

# Update all imports
find . -name '*.go' -exec sed -i '' 's|"github.com/cockroachdb/pebble"|"github.com/cockroachdb/pebble/v2"|g' {} +

# Remove old dependency
go mod tidy
```

**Files to update** (6 source files + 2 test files):
- `internal/database/pebble_store.go`
- `internal/database/settings.go`
- `internal/openlibrary/store.go`
- `cmd/diagnostics.go`
- `internal/database/pebble_store_test.go`
- `internal/database/pebble_coverage_test.go`

### Step 2: Fix Any API Breakages

After updating imports, run `go build ./...` and fix compilation errors. Known areas to check:

- `pebble.Open()` — Options struct may have new/changed fields
- `pebble.IterOptions` — Check for renamed fields
- `pebble.ErrNotFound` — Should still exist
- `pebble.NoSync` / `pebble.Sync` — Write options may have changed
- `pebble.NewBatch()` — Batch API may have additions
- `db.NewIter()` — Iterator creation may have new signatures
- `closer.Close()` — Value closer pattern should be the same

### Step 3: Set Optimal Format Version

In `pebble_store.go` `NewPebbleStore()` and `openlibrary/store.go` `NewOLStore()`, update Options:

```go
db, err := pebble.Open(path, &pebble.Options{
    FormatMajorVersion: pebble.FormatColumnarBlocks, // format 19, latest
    // ... existing options
})
```

This enables the newest SSTable format for better compression.

### Step 4: Optimize Open Library Bulk Import

**Current approach** (`internal/openlibrary/store.go`):
- Reads gzipped TSV line by line
- Parses JSON per line
- Calls `batch.Set()` per record
- Commits batch every 5000 records

**v2 optimization using `IngestExternalFiles`**:
- Parse dump into sorted SST files using `sstable.Writer`
- Ingest pre-built SSTs directly — bypasses memtable and WAL
- ~10x faster for bulk imports of millions of records

```go
import "github.com/cockroachdb/pebble/v2/sstable"

// Build sorted SST file from dump records
writer, _ := sstable.NewWriter(file, sstable.WriterOptions{
    TableFormat: sstable.TableFormatColumnarBlocks,
})
// Write sorted key-value pairs
writer.Set(key, value)
writer.Close()

// Ingest the SST
db.Ingest([]string{sstFilePath})
```

**Key constraint**: Keys must be written in sorted order within each SST. Since dump records aren't sorted by our key scheme, either:
- Sort in memory (fine for authors ~12M records, may need chunking for editions ~40M)
- Write multiple SSTs with non-overlapping key ranges (one per prefix type)

### Step 5: Add Blob Storage for Cover Art

v2's blob file support could replace the current filesystem-based cover art storage:

```go
// Store cover art as blob
blobKey := []byte("blob:cover:" + bookID)
db.Set(blobKey, coverImageBytes, pebble.Sync)
```

Benefits:
- Single backup target (just the Pebble DB directory)
- Atomic with book metadata
- Pebble handles compaction and space reclamation

### Step 6: Use EventuallyFileOnlySnapshot for Background Operations

For long-running operations (bulk metadata fetch, library organization):

```go
snap := db.NewEventuallyFileOnlySnapshot([]pebble.KeyRange{
    {Start: []byte("book:"), End: []byte("book:\xff")},
})
defer snap.Close()

// Read from snapshot while writes continue
snap.WaitForFileOnlySnapshot(ctx, 30*time.Second)
iter, _ := snap.NewIter(&pebble.IterOptions{})
```

This allows reads without holding memtable references, reducing memory pressure during imports.

### Step 7: Database Migration Path

Users upgrading from v1 to v2 need their existing Pebble databases to work:

1. v1.1.5 creates databases at format version 16 (`FormatVirtualSSTables`)
2. v2 minimum is format 13 (`FormatFlushableIngest`)
3. **No migration needed** — v1.1.5 databases are already compatible with v2

### Step 8: Post-Migration — Auto-Upgrade Format to Latest

After migrating to v2, add a startup ratchet in both `NewPebbleStore()` and `NewOLStore()` to upgrade existing databases to the latest format version. This unlocks columnar blocks and other v2 optimizations for databases created under older versions.

Add to `internal/database/pebble_store.go` in `NewPebbleStore()` and `internal/openlibrary/store.go` in `NewOLStore()`, immediately after `pebble.Open()`:

```go
db, err := pebble.Open(path, &pebble.Options{
    FormatMajorVersion: pebble.FormatColumnarBlocks, // request latest format for new DBs
})
if err != nil {
    return nil, err
}

// Upgrade existing databases to latest format
if db.FormatMajorVersion() < pebble.FormatColumnarBlocks {
    log.Printf("[INFO] Upgrading PebbleDB format from %d to %d (FormatColumnarBlocks)",
        db.FormatMajorVersion(), pebble.FormatColumnarBlocks)
    if err := db.RatchetFormatMajorVersion(pebble.FormatColumnarBlocks); err != nil {
        log.Printf("[WARN] Failed to upgrade PebbleDB format: %v", err)
        // Non-fatal — DB still works at old format
    }
}
```

**Why this matters:**
- `FormatMajorVersion` in `Options` only applies to **new** databases
- Existing databases opened with v2 retain their old format (e.g. 16) unless explicitly ratcheted
- `RatchetFormatMajorVersion` is a one-way upgrade — once upgraded, the DB can't be opened by older Pebble versions
- Format 19 (FormatColumnarBlocks) enables the best compression and read performance

**Rollback consideration:** Once ratcheted, the database cannot be downgraded. If a user needs to roll back to the v1 binary, they'd need to restore from backup. Document this in release notes.

## Test Plan

1. `go build ./...` compiles clean
2. `go test ./internal/database/...` — all PebbleStore tests pass
3. `go test ./internal/openlibrary/...` — OLStore tests pass
4. `go test ./cmd/...` — diagnostics tests pass
5. `go test ./...` — full suite passes
6. Manual test: open existing v1.1.5 database with v2 binary — should auto-upgrade
7. Benchmark: Open Library import speed before/after SST ingestion optimization

## Estimated Effort

| Step | Effort | Risk |
|------|--------|------|
| 1. Import path update | 30 min | Low — mechanical find/replace |
| 2. Fix API breakages | 1-2 hr | Medium — depends on API changes |
| 3. Format version | 10 min | Low |
| 4. Bulk import optimization | 3-4 hr | Medium — SST writer is new API |
| 5. Blob storage for covers | 2-3 hr | Low — additive feature |
| 6. File-only snapshots | 1-2 hr | Low — additive optimization |
| 7. Migration check | 30 min | Low |

**Total**: ~8-12 hours

## Pre-existing Issues to Fix First

These test failures exist on main and should be fixed before or during the upgrade:

1. `cmd/commands_test.go` — `stubStore` missing `CountAuthors` method (interface grew, stub didn't)
2. `internal/config` — `TestMetadataDefaults` expects old metadata source order
3. `internal/database/mocks` — `TestMockStore_Coverage` missing new Store methods
4. `internal/itunes` — 3 test failures (likely test data mismatch)
5. `internal/server` — `TestFilesystemService_CreateExclusion_NotDirectory` (pre-existing)

## References

- [PebbleDB GitHub](https://github.com/cockroachdb/pebble)
- [PebbleDB v2 Go Docs](https://pkg.go.dev/github.com/cockroachdb/pebble/v2)
- [Releases](https://github.com/cockroachdb/pebble/releases)
- Current Pebble store: `internal/database/pebble_store.go` (2715 lines, 109 Pebble API calls)
- OL store: `internal/openlibrary/store.go` (503 lines, 10 Pebble API calls)
