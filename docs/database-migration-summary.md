<!-- file: docs/database-migration-summary.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2e3f4a5b-6c7d-8e9f-0a1b-2c3d4e5f6a7b -->

# Database Migration: SQLite3 → PebbleDB

## Summary

Successfully migrated the audiobook organizer from SQLite3 to PebbleDB as the primary database, with SQLite3 available as an opt-in legacy option.

## What Changed

### Architecture

1. **New Abstraction Layer** (`internal/database/store.go`)
   - `Store` interface defines all database operations
   - Supports multiple backends transparently
   - Common data structures for both implementations

2. **PebbleDB Implementation** (`internal/database/pebble_store.go`)
   - Pure Go key-value store
   - LSM tree architecture
   - Custom indexing for relationships
   - ~1150 lines of implementation

3. **SQLite Implementation** (`internal/database/sqlite_store.go`)
   - Wrapped existing SQL code
   - Implements Store interface
   - Maintains backward compatibility
   - ~650 lines of implementation

### Configuration

**New Configuration Options:**

```yaml
# .audiobook-organizer.yaml
database_type: pebble                        # "pebble" (default) or "sqlite"
database_path: audiobooks.pebble             # Changed from audiobooks.db
enable_sqlite3_i_know_the_risks: false       # Must be true for SQLite
```

**New Command-Line Flags:**

```bash
--db-type pebble                              # Database type
--db audiobooks.pebble                        # Database path
--enable-sqlite3-i-know-the-risks             # Scary flag for SQLite
```

### Code Changes

**Files Modified:**

- `internal/config/config.go` (v1.1.0 → v1.2.0)
  - Added `DatabaseType` and `EnableSQLite` fields
  - Set PebbleDB as default

- `cmd/root.go` (v1.1.0 → v1.2.0)
  - Updated all `database.Initialize()` → `database.InitializeStore()`
  - Updated all `database.Close()` → `database.CloseStore()`
  - Added database type and SQLite enable flags
  - Changed default database path to `audiobooks.pebble`

- `internal/database/web.go` (v1.0.0 → v1.1.0)
  - Removed duplicate type definitions
  - Types moved to `store.go` to avoid circular dependencies

**Files Created:**

- `internal/database/store.go` (v1.0.0)
  - Store interface (55 methods)
  - Common data structures (10 types)
  - InitializeStore() and CloseStore() functions

- `internal/database/pebble_store.go` (v1.0.0)
  - Complete PebbleDB implementation
  - Key schema with prefixes
  - Secondary indexes for relationships

- `internal/database/sqlite_store.go` (v1.0.0)
  - SQLite3 wrapper implementing Store interface
  - Preserves existing SQL schema and queries

- `docs/database-architecture.md` (v1.0.0)
  - Comprehensive documentation
  - Key schema reference
  - Migration guide

## Key Design Decisions

### 1. PebbleDB as Default

**Reasoning:**
- No CGO dependency (pure Go)
- Cross-compilation works everywhere
- Production-proven (CockroachDB)
- Simpler deployment

**Trade-offs:**
- No SQL queries (but we don't need them)
- Manual indexing required (implemented)

### 2. SQLite3 as Opt-in Only

**Reasoning:**
- Cross-compilation issues with CGO
- Most users don't need it
- Scary flag prevents accidental use

**Implementation:**
```go
if !enableSQLite && dbType == "sqlite" {
    return fmt.Errorf("SQLite3 is not enabled. To use SQLite3, you must explicitly enable it...")
}
```

### 3. Clean Abstraction Layer

**Interface-based design:**
```go
type Store interface {
    GetAllAuthors() ([]Author, error)
    CreateAuthor(name string) (*Author, error)
    // ... 53 more methods
}
```

**Benefits:**
- Easy to add new backends (BoltDB, BadgerDB, etc.)
- Testing with mock implementations
- Clear API contract

## PebbleDB Key Schema

### Prefix Strategy

```
author:<id>                          → Author JSON
author:name:<name>                   → author_id (index)
series:<id>                          → Series JSON
series:name:<name>:<author_id>       → series_id (index)
book:<id>                            → Book JSON
book:path:<path>                     → book_id (index)
book:series:<series_id>:<book_id>    → book_id (secondary index)
book:author:<author_id>:<book_id>    → book_id (secondary index)
```

### Example Data

```
author:1 → {"id":1,"name":"Brandon Sanderson"}
author:name:Brandon Sanderson → 1

series:1 → {"id":1,"name":"Mistborn","author_id":1}
series:name:Mistborn:1 → 1

book:1 → {"id":1,"title":"The Final Empire","series_id":1,"author_id":1,...}
book:path:/audiobooks/mistborn1.m4b → 1
book:series:1:1 → 1
book:author:1:1 → 1

counter:author → 2
counter:series → 2
counter:book → 2
```

### Operations

**Lookup by ID:** `O(log N)` - direct key access
**Lookup by name:** `O(log N)` - index lookup + data fetch
**List by series:** `O(K log N)` - range scan over `book:series:<id>:*`
**List all:** `O(N)` - range scan over `book:0` to `book:;`

## Migration Path

### For New Installations

Just use the defaults:
```bash
./audiobook-organizer scan --dir /audiobooks
# Uses PebbleDB automatically
```

### For Existing SQLite Users

Two options:

**1. Continue using SQLite (not recommended):**
```bash
./audiobook-organizer scan \
  --dir /audiobooks \
  --db-type sqlite \
  --db audiobooks.db \
  --enable-sqlite3-i-know-the-risks
```

**2. Switch to PebbleDB (recommended):**
```bash
# Your old SQLite database remains at audiobooks.db
# Just use new defaults - PebbleDB will create new database
./audiobook-organizer scan --dir /audiobooks

# Re-scan will rebuild metadata in PebbleDB
```

### Future: Automatic Migration

Not yet implemented, but planned:
```bash
./audiobook-organizer migrate \
  --from-sqlite audiobooks.db \
  --to-pebble audiobooks.pebble
```

## Testing

### Verified

✅ Project builds successfully
✅ No compilation errors
✅ All Store interface methods implemented
✅ PebbleDB initializes correctly
✅ SQLite wrapper maintains compatibility
✅ Configuration system updated
✅ Command-line flags working

### TODO

- [ ] End-to-end tests with PebbleDB
- [ ] Performance benchmarks (PebbleDB vs SQLite)
- [ ] Migration tool (SQLite → PebbleDB)
- [ ] Database backup/restore utilities
- [ ] Concurrent access stress tests

## Dependencies

### Added

```go
require (
    github.com/cockroachdb/pebble v1.1.5
    github.com/cockroachdb/errors v1.11.3
    github.com/DataDog/zstd v1.4.5
    github.com/golang/snappy v0.0.4
    github.com/klauspost/compress v1.16.0
)
```

### Retained

```go
require (
    github.com/mattn/go-sqlite3 v1.14.32  // Optional, for legacy support
)
```

## Performance Expectations

### PebbleDB

- **Writes**: Buffered, very fast (LSM)
- **Reads**: Fast for key lookups
- **Range scans**: Efficient (sequential disk access)
- **Memory**: Block cache (~16MB default)
- **Disk**: Compaction reclaims space automatically

### SQLite

- **Writes**: Good with WAL mode
- **Reads**: Excellent with indexes
- **Range scans**: Excellent with B-tree
- **Memory**: Page cache (~8MB default)
- **Disk**: VACUUM required to reclaim space

For our use case (mostly reads, infrequent writes), both perform well.

## Next Steps

1. **Test with real audiobook library**
   - Scan large collection (~1000+ books)
   - Verify metadata extraction
   - Check relationship indexing

2. **Performance benchmarking**
   - Compare PebbleDB vs SQLite
   - Measure scan times
   - Test concurrent access

3. **Documentation updates**
   - Update README.md with new defaults
   - Add troubleshooting guide
   - Document key schema

4. **Migration tool**
   - Export from SQLite
   - Import to PebbleDB
   - Verify data integrity

5. **Web interface integration**
   - Test REST API with PebbleDB
   - Verify all CRUD operations
   - Check concurrent access handling

## References

- [PebbleDB GitHub](https://github.com/cockroachdb/pebble)
- [Store Interface](../internal/database/store.go)
- [PebbleDB Implementation](../internal/database/pebble_store.go)
- [SQLite Implementation](../internal/database/sqlite_store.go)
- [Database Architecture Doc](database-architecture.md)
