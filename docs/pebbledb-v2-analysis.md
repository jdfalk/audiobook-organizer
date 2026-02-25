<!-- file: docs/pebbledb-v2-analysis.md -->
<!-- version: 1.0.0 -->
<!-- guid: a2b3c4d5-e6f7-8901-2345-6789abcdef01 -->
<!-- last-edited: 2026-02-25 -->

# PebbleDB v2 Upgrade Analysis

## 1. Current State

The audiobook organizer uses **PebbleDB v1.1.5** via `github.com/cockroachdb/pebble`.

| File | Pebble API calls | Role |
|------|--:|------|
| `internal/database/pebble_store.go` | 109 | Primary data store (books, authors, series, ops) |
| `internal/openlibrary/store.go` | 10 | Open Library dump import/lookup |
| `internal/database/settings.go` | 2 | Settings persistence |
| `cmd/diagnostics.go` | 2 | DB diagnostics CLI |
| Test files | ~30 | Unit and coverage tests |

The database uses a colon-delimited prefix key scheme (`b:`, `a:`, `s:`, `idx:book:`, etc.) with JSON values. All writes go through Pebble batches for atomicity.

v1.1.5 creates databases at **format version 16** (`FormatVirtualSSTables`).

## 2. Format Versions (14-19)

PebbleDB format versions are monotonically increasing. Each version enables new on-disk features. Once a database is upgraded, it cannot be opened by older Pebble versions.

| Format | Name | What It Enables |
|--------|------|-----------------|
| 14 | `FormatFlushableIngest` | SST ingestion via the memtable path, avoiding write stalls during ingest. Minimum format for v2. |
| 15 | `FormatPrePebblev1Marked` | Internal marker for pre-v1 sstables. Bookkeeping for compaction correctness. |
| 16 | `FormatVirtualSSTables` | Virtual SSTables -- multiple logical tables backed by a single physical file. Enables excise (hole-punching) and shared storage. This is the default for v1.1.5. |
| 17 | `FormatSyntheticPrefixSuffix` | Virtual SSTables can have synthetic key prefixes/suffixes applied without rewriting data. Enables efficient key-space remapping. |
| 18 | `FormatSSTableValueBlocks` | Large values stored in separate blocks within SSTables. Iterators that only need keys skip value blocks entirely, improving scan performance for index-only queries. |
| 19 | `FormatColumnarBlocks` | Columnar SSTable block format. Keys and values stored in column-oriented layout for better compression ratios and faster prefix iteration. Latest format in v2. |

### Relevance to This Project

- **Format 16 (current)**: Already have virtual SSTable support. No immediate need.
- **Format 17**: Synthetic prefix/suffix could help if we ever do key-space partitioning or multi-tenant prefixing, but not needed now.
- **Format 18**: Value blocks would benefit our prefix scans over index keys (`idx:book:author:...`) where we only need to check key existence (value is just `"1"`). Marginal gain since our values are small.
- **Format 19**: Columnar blocks provide the best compression for our prefix-heavy key layout. Worth adopting.

## 3. v2 Features

### Columnar Blocks (Format 19)

Stores SSTable block data in column-oriented format rather than row-oriented. Each column (key prefix, key suffix, value) is compressed independently.

**Impact for us**: Our keys share long common prefixes (`idx:book:author:`, `idx:book:series:`, etc.). Columnar layout compresses these prefixes significantly better. Read performance for prefix scans improves because the engine can skip irrelevant columns.

**Expected benefit**: 10-30% reduction in on-disk size. Measurable improvement on prefix scans over large collections.

### Virtual SSTables

Already available at format 16 (our current version). Virtual SSTables let Pebble represent slices of a physical SSTable as independent logical tables without copying data.

**Impact for us**: Enables faster `DeleteRange` via excise (used in our `Reset()` method). Background compaction is more efficient.

### Synthetic Prefix/Suffix (Format 17)

Virtual SSTables can have a prefix or suffix applied to all their keys without rewriting the physical data.

**Impact for us**: Not immediately useful. Could matter if we add multi-tenant support (`tenant:ID:` prefix) -- data could be shared across tenants at the storage level while appearing to have different key prefixes. This is a future consideration only.

### SSTable Value Blocks (Format 18)

Separates values from keys within SSTable data blocks. When an iterator only needs keys (e.g., checking existence, counting), it skips loading values entirely.

**Impact for us**: Many of our index keys map to trivial values (`"1"` or a ULID string). The benefit is marginal for small values but becomes meaningful at scale. Prefix scans over `idx:book:tag:` or `idx:book:author:` would skip value I/O.

### External File Ingestion

v2 has improved support for ingesting pre-built SST files directly into the LSM tree, bypassing the memtable and WAL.

**Impact for us**: The Open Library bulk import currently writes 12M+ records via `batch.Set()` with commits every 5000 records. Building sorted SST files and ingesting them would be roughly 10x faster. This is the single biggest practical win from v2.

### EventuallyFileOnlySnapshot

Creates a snapshot that initially references memtable data but eventually becomes file-only, releasing memtable memory.

**Impact for us**: Useful for long-running background operations (library scans, bulk metadata fetch) that need a consistent view without holding memory. Currently not a bottleneck, but good hygiene for larger libraries.

### Better Compaction Scheduling

`ConcurrencyLimitScheduler` provides control over background compaction I/O.

**Impact for us**: During large imports or scans, compaction can compete with foreground reads. Being able to limit compaction concurrency during active use and increase it during idle periods is a quality-of-life improvement.

## 4. Migration Path and Risks

### Migration Path

1. **Update import path**: `github.com/cockroachdb/pebble` becomes `github.com/cockroachdb/pebble/v2`. Mechanical find/replace across 6 source files and 2 test files.
2. **Fix compilation errors**: Some Options fields may have moved. `go build ./...` will surface these.
3. **Set format version**: Set `FormatMajorVersion: pebble.FormatColumnarBlocks` in Options for new databases.
4. **Add format ratchet**: After `pebble.Open()`, call `db.RatchetFormatMajorVersion(pebble.FormatColumnarBlocks)` to upgrade existing databases on first open.
5. **Test**: Full test suite, manual verification with existing database.

### Database Compatibility

- v1.1.5 databases are at format 16
- v2 minimum is format 13
- **No data migration needed** -- v2 opens v1.1.5 databases directly
- Format ratchet to 19 is one-way: once upgraded, the database cannot be opened by v1 binaries

### Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| API breakage in Options or Iterator | Medium | Compile-time errors, fixable in 1-2 hours |
| Format ratchet prevents downgrade | Low | Document in release notes; backup before upgrade |
| New format introduces bugs | Low | CockroachDB runs this in production; well-tested |
| Open Library SST ingestion complexity | Medium | Can defer to a follow-up; batch.Set() still works |
| Test flakiness from parallel global state | Pre-existing | Not v2-related; fix independently |

### Rollback Plan

If v2 causes issues after deployment:
1. If format was NOT ratcheted: revert to v1 binary, database opens fine.
2. If format WAS ratcheted: restore from backup, revert to v1 binary.

Recommendation: back up the Pebble data directory before first run with v2 binary.

## 5. Performance Implications

### Positive

- **Columnar blocks**: Better compression ratio on our prefix-heavy keys. Estimated 10-30% smaller on-disk footprint.
- **Prefix iteration**: Columnar format allows skipping irrelevant key components during scans. Our `idx:` scans should be faster.
- **Bulk import**: SST ingestion for Open Library dumps could be 10x faster (minutes instead of tens of minutes for 12M records).
- **Memory**: EventuallyFileOnlySnapshot reduces memory pressure during background operations.

### Neutral

- **Point lookups**: `Get()` performance is unchanged. Most of our primary entity access (`b:ULID`, `a:ULID`) is point lookups.
- **Write throughput**: Batch write performance is comparable between v1 and v2 for our workload sizes.

### Negative

- **First compaction after ratchet**: When the database format is upgraded, existing SSTables continue to use the old format. New SSTables use the new format. Full benefit comes after a full compaction cycle. For our database sizes (typically under 1GB), this completes in seconds.

### Benchmarking Plan

Before and after migration, measure:
1. Open Library import time (12M records)
2. Prefix scan latency for `idx:book:author:` with 10K+ books
3. Database size on disk
4. Memory usage during library scan operation

## 6. Recommendation

**Upgrade to v2.** The migration is low-risk and the effort is modest (8-12 hours total).

### Priority Order

1. **Import path update + compilation fixes** (Steps 1-2, ~2 hours): Get building on v2. This is the foundation.
2. **Format version + ratchet** (Steps 3-4, ~30 minutes): Enable columnar blocks for new and existing databases.
3. **Open Library SST ingestion** (Step 5, ~4 hours): The biggest single performance win. Can be done as a follow-up PR.
4. **EventuallyFileOnlySnapshot for background ops** (Step 6, ~2 hours): Nice-to-have, can defer.
5. **Blob storage for cover art** (future): Only relevant once cover art feature is implemented.

### When to Do It

This is not urgent. The project is pre-1.0 and the current v1.1.5 works fine. Good timing would be:
- After Docker deployment is working (higher priority for daily-driver use)
- Before the Open Library import is needed at scale (SST ingestion is the key win)
- As part of a dependency update sweep

### What NOT to Do

- Do not adopt synthetic prefix/suffix (format 17 feature) -- no use case yet
- Do not store audio files or large blobs in Pebble -- filesystem is better for media files
- Do not ratchet format in CI/test environments unless all developers are on v2
