# Chai Migration Plan: Task Breakdown for Parallel Execution

## Overview

Full migration from Pebble manual indexing to Chai SQL. 78% code reduction across 9,300 lines. Designed for parallel Haiku/Sonnet execution with minimal cross-dependencies.

---

## PHASE 1: Foundation Setup (Blockers for all phases)

These must complete first; all other work depends on them.

### Task 1.1: Database Schema Design
**Scope**: Define Chai schema for books, authors, series, files, relationships
**Owner**: Sonnet (complex design)
**Deliverable**: `schema.sql` with tables, indexes, constraints
**Time**: 2-3 hours

Files to create:
- `internal/database/schema.sql`: Full schema with PRIMARY KEY, FOREIGN KEY, CREATE INDEX
- `internal/database/migration.go`: Schema initialization function

Acceptance criteria:
- [ ] Books table with all fields from Book struct
- [ ] Authors/series/files tables with relationships
- [ ] Indexes on frequently filtered columns (series_id, author_id, is_primary_version, marked_for_deletion)
- [ ] Schema matches current Pebble key structure (can be reversed)

### Task 1.2: Chai Integration Layer
**Scope**: Create wrapper to initialize Chai DB with Pebble backend
**Owner**: Haiku (straightforward integration)
**Deliverable**: `chai_integration.go` with connection + initialization
**Time**: 1 hour

Files to create:
- `internal/database/chai_integration.go`: Open Chai DB, initialize schema
- Tests: `chai_integration_test.go`

Acceptance criteria:
- [ ] NewChaiDB() function opens/initializes database
- [ ] Schema is created on first run
- [ ] Can execute test query and get results
- [ ] Cleanup on Close()

---

## PHASE 2: Aggregation Function Migration (High Parallelism)

These are independent and can run in parallel. Each follows same pattern: 
1. Implement SQL version
2. Add tests comparing Pebble vs Chai results
3. Benchmark both
4. Add feature flag to switch implementations

### Task 2.1: GetAllSeriesBookCounts → SQL
**Owner**: Haiku
**Time**: 1 hour
**Files**: `pebble_store.go`, tests

Pattern:
```sql
SELECT series_id, COUNT(*) as count
FROM books
WHERE series_id IS NOT NULL AND is_primary_version = true AND marked_for_deletion = false
GROUP BY series_id
```

Acceptance:
- [ ] GetAllSeriesBookCounts_Chai(ctx) function
- [ ] Test: both functions return identical results
- [ ] Benchmark: time both on 50K books
- [ ] Feature flag: UseChaiDB bool to switch

### Task 2.2: GetAllAuthorBookCounts → SQL
**Owner**: Haiku
**Time**: 1 hour
**Files**: `pebble_store.go`, tests

Pattern:
```sql
SELECT author_id, COUNT(*) as count
FROM book_authors
WHERE marked_for_deletion = false
GROUP BY author_id
```

Acceptance: [Same as 2.1]

### Task 2.3: GetAllSeriesFileCounts → SQL
**Owner**: Haiku
**Time**: 1.5 hours
**Files**: `pebble_store.go`, tests

Pattern (JOIN):
```sql
SELECT b.series_id, COUNT(f.id) as file_count
FROM books b
LEFT JOIN book_files f ON b.id = f.book_id
WHERE b.is_primary_version = true AND marked_for_deletion = false
GROUP BY b.series_id
```

Acceptance: [Same as 2.1]

### Task 2.4: GetAllAuthorFileCounts → SQL
**Owner**: Haiku
**Time**: 1.5 hours
**Files**: `pebble_store.go`, tests

Acceptance: [Same as 2.1]

### Task 2.5: CountFiles → SQL
**Owner**: Haiku
**Time**: 1 hour
**Files**: `pebble_store.go`, tests

Current: 77 lines (two-phase scan)
SQL: 8 lines

Pattern:
```sql
SELECT COUNT(*) as file_count
FROM book_files
WHERE book_id IN (
  SELECT id FROM books 
  WHERE marked_for_deletion = false AND is_primary_version = true
)
```

Acceptance: [Same as 2.1]

---

## PHASE 3: List/Filter Functions (Medium Parallelism)

Some interdependencies exist. Safe to parallelize 2-3 at a time.

### Task 3.1: GetAllBooks → SQL
**Owner**: Sonnet (complex filtering logic)
**Time**: 2 hours
**Files**: `pebble_store.go`, tests, `audiobooks_handlers.go` (update calls)

Current: GetAllBooks iterates "book:0" to "book:;", filters manually
SQL: Single query with WHERE/LIMIT/OFFSET

Acceptance:
- [ ] GetAllBooks_Chai(limit, offset, filter map) returns same results as Pebble
- [ ] Tests verify pagination works correctly
- [ ] Tests verify each filter type works (MarkedForDeletion, IsPrimaryVersion, etc.)
- [ ] Benchmark on 50K books

### Task 3.2: GetBooksBySeriesID → SQL
**Owner**: Haiku
**Time**: 1.5 hours
**Depends on**: 3.1 (uses same pattern)
**Files**: `pebble_store.go`, tests

Pattern:
```sql
SELECT b.* FROM books b
WHERE b.series_id = ? AND b.is_primary_version = true AND marked_for_deletion = false
ORDER BY b.title
```

Acceptance: [Same as 3.1]

### Task 3.3: GetBooksByAuthorID → SQL
**Owner**: Haiku
**Time**: 1.5 hours
**Depends on**: 3.1
**Files**: `pebble_store.go`, tests

Pattern (requires book_authors table):
```sql
SELECT b.* FROM books b
JOIN book_authors ba ON b.id = ba.book_id
WHERE ba.author_id = ? AND b.is_primary_version = true AND marked_for_deletion = false
ORDER BY b.title
```

Acceptance: [Same as 3.1]

### Task 3.4: Remove Denormalized Indexes
**Owner**: Sonnet (careful refactoring)
**Time**: 2 hours
**Depends on**: 3.3 (after JOIN works)
**Files**: `pebble_store.go` (SetBook, UpdateBook, DeleteBook), tests

Current: book:series:<series_id> and book:author:<author_id> store full Book JSON
After: Use book_authors table, remove dual-write logic

Refactoring:
- Remove JSON marshaling for index keys
- Remove from SetBook (lines 1920-1938)
- Remove from UpdateBook (lines 2108-2146)
- Remove from DeleteBook (lines 2548+)
- Add tests: book_authors table properly synchronized

Acceptance:
- [ ] SetBook, UpdateBook, DeleteBook no longer write denormalized indexes
- [ ] GetBooksBySeriesID, GetBooksByAuthorID still work (use SQL JOIN)
- [ ] No orphaned index entries

---

## PHASE 4: Utility Functions (Low Priority)

These can proceed in parallel once schema is stable.

### Task 4.1: GetAllSeries → SQL
**Owner**: Haiku
**Time**: 1 hour

Pattern:
```sql
SELECT * FROM series ORDER BY name
```

### Task 4.2: GetAllAuthors → SQL
**Owner**: Haiku
**Time**: 1 hour

Pattern:
```sql
SELECT * FROM authors ORDER BY name
```

### Task 4.3: GetAllImportPaths → SQL
**Owner**: Haiku
**Time**: 1 hour

### Task 4.4: GetAllBlockedHashes → SQL
**Owner**: Haiku
**Time**: 1 hour

### Task 4.5: GetAllUserPreferences → SQL
**Owner**: Haiku
**Time**: 1 hour

### Task 4.6-4.10: Remaining GetAll* functions
**Owner**: Haiku (5 more functions)
**Time**: 1 hour each

---

## PHASE 5: Clean Up & Testing

### Task 5.1: Remove Feature Flags
**Owner**: Haiku
**Time**: 1 hour
**Depends on**: All aggregation tests passing consistently

Switch all functions to use Chai by default, remove Pebble fallbacks.

### Task 5.2: Remove Pebble Manual Index Prefixes
**Owner**: Sonnet
**Time**: 1 hour
**Depends on**: All functions migrated

Remove:
- author:name:* prefix logic
- series:name:* prefix logic
- All custom index key building in SetAuthor, SetSeries, etc.

### Task 5.3: Integration Tests
**Owner**: Sonnet
**Time**: 2 hours
**Depends on**: All migrations complete

Tests:
- [ ] Create/read/update/delete book → all queries return correct results
- [ ] Pagination works with large datasets
- [ ] Filters (series_id, author_id, etc.) work correctly
- [ ] Aggregation functions match expectations

### Task 5.4: Performance Benchmarks
**Owner**: Haiku
**Time**: 1.5 hours
**Depends on**: All migrations complete

Benchmarks:
- [ ] GetAllSeriesBookCounts: 50K books
- [ ] ListBooks: pagination performance
- [ ] CountFiles: large file counts
- [ ] Document speedup: x% faster

---

## Parallel Execution Strategy

### Round 1 (Day 1) - Foundation
- Task 1.1 (Sonnet) + Task 1.2 (Haiku) **in parallel**

### Round 2 (Day 2) - Aggregations
- Tasks 2.1, 2.2, 2.3, 2.4, 2.5 **all in parallel** (5 Haiku agents)

### Round 3 (Day 3) - Filtering
- Task 3.1 (Sonnet, blocks others)
- After 3.1 done: Tasks 3.2, 3.3 **in parallel** (2 Haiku)
- Task 3.4 (Sonnet) after 3.3 done

### Round 4 (Day 4) - Utilities & Cleanup
- Tasks 4.1-4.10 **all in parallel** (10 Haiku)
- Task 5.1 (Haiku) + Task 5.2 (Sonnet) after tests pass **in parallel**
- Task 5.3 (Sonnet) + Task 5.4 (Haiku) **in parallel**

---

## Success Criteria

- [ ] All functions return identical results to Pebble version
- [ ] Benchmarks show ≥10x improvement on aggregations
- [ ] pebble_store.go reduced to ≤2,000 lines (from 9,300)
- [ ] All tests passing (unit + integration)
- [ ] No denormalized index prefixes in final code
- [ ] Production deployment: smooth switchover with feature flag

---

## Rollback Plan

Each task has a feature flag. If any migration shows performance regression:
1. Set UseChaiDB = false in that function
2. Keep both implementations until root cause fixed
3. Benchmark and profile to understand difference
4. Re-implement SQL query or fix indexes

---

## Timeline Estimate

| Phase | Parallel Tasks | Sequential Blockers | Duration |
|-------|---|---|---|
| 1. Foundation | 2 tasks (1 hr each) | None | **~1.5 hours** |
| 2. Aggregations | 5 tasks (1 hr each) | Task 1 complete | **~1 hour** (5× parallel) |
| 3. Filtering | 4 tasks, 2 sequential | Task 1 + 2 complete | **~4 hours** (3.1→3.2/3.3 parallel→3.4) |
| 4. Utilities | 10 tasks (1 hr each) | Task 1 complete | **~1 hour** (10× parallel) |
| 5. Cleanup | 4 tasks, mixed serial | All above complete | **~3 hours** |
| **Total** | **25 tasks** | | **~10-12 hours** |

With 5 Haiku + 2 Sonnet agents running in optimal parallel: **~2-3 days wall-clock time**.

---

## Next Steps

1. Create 25 GitHub Issues from this plan
2. Assign 1.1 and 1.2 to Sonnet/Haiku → execute Phase 1
3. After 1.2 passes tests, spawn 5× Haiku agents for Phase 2
4. Stagger Phase 3 → 5 as earlier phases complete
5. Weekly: benchmark against production load test
