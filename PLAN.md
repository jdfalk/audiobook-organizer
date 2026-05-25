# Burn Chai, Rise go-memdb

<!-- file: PLAN.md -->
<!-- version: 1.0.0 -->
<!-- last-edited: 2026-05-24 -->

## Goal

Replace the ChaiSQL embedded SQL layer with an in-memory query/index layer built on `github.com/hashicorp/go-memdb`. PebbleDB remains the source of truth and persistence layer. The memdb layer is rebuilt from Pebble on startup and kept in sync via write-through.

**Why:** Chai is dev-stage software, lacks JOINs, has type-system issues (int64 overflow), single-developer maintenance, and has caused multiple production incidents. go-memdb is HashiCorp-maintained, used in Consul/Vault/Nomad, pure Go, native int64, MVCC for concurrent reads, and a perfect fit for our 50K-row workload that fits trivially in RAM.

## Affected files

### New files
- `internal/database/memdb_schema.go` — schema definitions for books, authors, series, book_files, narrators, import_paths, etc.
- `internal/database/memdb_store.go` — `MemStore` wrapper with query methods replacing `_Chai` functions
- `internal/database/memdb_sync.go` — write-through helpers (replaces `chai_sync.go`)
- `internal/database/memdb_warmup.go` — startup population from Pebble (replaces Chai backfill)
- `internal/database/memdb_store_test.go` — unit tests for query methods
- `internal/database/memdb_integration_test.go` — integration tests against real Pebble

### Files to modify
- `internal/database/pebble_store.go` — replace `_Chai` method delegations with memdb calls (10+ methods)
- `internal/database/types.go` (or wherever `PebbleStore` struct lives) — add `mem *MemStore`, remove `chai *ChaiDB`, `UseChaiDB bool`
- Wherever `NewPebbleStore` is — initialize memdb + warmup instead of Chai
- API handlers calling `_Chai` methods — no signature change needed (methods stay on `PebbleStore`)

### Files to delete (Phase 4)
- `internal/database/chai_schema.go`
- `internal/database/chai_sync.go`
- `internal/database/chai_integration.go`
- `internal/database/chai_store.go`
- `internal/database/chai_*_test.go` (~10 test files)
- `internal/database/poc_chai_test.go`
- Any chai admin endpoint / backfill handler
- `github.com/chaisql/chai` from `go.mod`

### Surface area (audit baseline)
- **87 `_Chai`-named definitions** in non-test code
- **29 call sites / references** in non-test code
- ~10 test files dedicated to Chai

## Phased Approach

### Phase 1 — Foundation (no behavior change)
1. Add `go-memdb` dependency
2. Write `memdb_schema.go` covering all entities currently in Chai: `books`, `authors`, `series`, `book_files`, `narrators`, `book_authors`, `book_narrators`, `import_paths`, `author_aliases`, `blocked_hashes`, `user_preferences`, `works`
3. Write `MemStore` struct wrapping `*memdb.MemDB`
4. Write `memdb_warmup.go` — single-pass scan of Pebble that bulk-inserts into all tables
5. Wire `MemStore` into `PebbleStore` struct (added alongside, not replacing yet)
6. Initialize and warm on `NewPebbleStore`; gate behind `UseMemDB` flag (default false)
7. Add `Close()` cleanup
8. **Verification:** `make build` passes, startup warmup logs entity counts matching Pebble counts

### Phase 2 — Write-through sync
1. Write `memdb_sync.go` with helpers: `upsertBookToMem`, `deleteBookFromMem`, `upsertSeriesToMem`, etc.
2. Add write-through calls to all PebbleStore mutators that currently call `UpsertBookToChaiDB`:
   - `CreateBook`, `UpdateBook`, `DeleteBook`
   - `CreateBookFile`, `UpdateBookFile`, `DeleteBookFile`
   - `CreateSeries`, `UpdateSeries`, `DeleteSeries`
   - `CreateAuthor`, `UpdateAuthor`, `DeleteAuthor`
   - `SetBookAuthors`, `SetBookNarrators`
   - All other entity mutators
3. Run side-by-side with Chai write-through (both update) — no behavior change yet
4. **Verification:** create/update/delete a book in dev, query memdb directly, confirm row matches Pebble

### Phase 3 — Migrate read queries one at a time
Each query gets:
- A new memdb implementation
- A feature-flag toggle (`UseMemDBForX`) to switch between Chai and memdb
- A unit test proving identical output to Chai
- A benchmark proving it's faster

Order (easiest → hardest):
1. `GetAllSeries` — flat scan
2. `GetAllAuthors`
3. `GetAllImportPaths`
4. `GetAllAuthorAliases`
5. `GetAllWorks`
6. `CountFiles` — single aggregate
7. `GetAllSeriesBookCounts` — group-by aggregate
8. `GetAllAuthorBookCounts`
9. `GetAllSeriesFileCounts` — cross-table aggregate (books + book_files)
10. `GetAllAuthorFileCounts`
11. `GetBooksBySeriesID` — filtered scan with limit/offset
12. `GetBooksByAuthorID`
13. `GetAllBooks` — full filter map (hardest — many filter combinations)

After each migration, the corresponding `_Chai` call is removed from `PebbleStore` and the feature flag is removed.

### Phase 4 — Burn it down
Once all 13 reads + all write-throughs are on memdb:
1. Remove `UseChaiDB` flag and all Chai write-throughs
2. Remove `BackfillChaiFromPebble` handler + admin endpoint
3. Delete `chai_*.go` and `chai_*_test.go` files
4. Remove `chai` field from `PebbleStore`
5. Remove `github.com/chaisql/chai` from `go.mod`; `go mod tidy`
6. Update `CHANGELOG.md`, `TODO.md`, `.claude/notes/chai-sql-reference.md` (mark as historical)
7. Update `CLAUDE.md` / memory: Chai removed, memdb is the query layer

## Test strategy

### Per-query parity testing
For each migrated query, write a test that:
1. Seeds Pebble with deterministic data (use existing test helpers)
2. Runs both `_Chai` and `_Mem` versions
3. Asserts identical output (use `reflect.DeepEqual` or sorted comparison for unordered maps)

### Crash recovery test
1. Create books in Pebble
2. Tear down memdb in-process
3. Re-initialize and re-warm
4. Verify all queries return identical results

### Concurrency test
1. Start 4 goroutines doing reads against memdb
2. Start 1 goroutine doing writes
3. Assert no readers block, no data race (run with `-race`)

### Memory baseline
Capture `runtime.MemStats` heap allocation at startup with full dataset loaded. Expect ~50–100MB for 50K books + relations. Document in PR.

### Benchmark suite
Reuse existing `Benchmark*_Chai` benchmarks; add `Benchmark*_Mem` mirrors. Expect memdb to be 10–100x faster on aggregations.

### Build gates
- `make build` passes after every phase
- `make test` (Go backend) passes after every phase
- `make ci` (80% coverage gate) passes before merge

## Rollback

Each phase is independently revertable:
- **Phase 1:** memdb is initialized but unused; just don't enable it. Zero risk.
- **Phase 2:** dual write-through. If memdb writes fail, log warning, Pebble still authoritative. Setting `UseMemDB=false` reverts to pre-Phase-2 behavior.
- **Phase 3:** per-query feature flags allow per-method rollback. If `GetAllBooks_Mem` is buggy, flip `UseMemDBForGetAllBooks=false`, Chai handles it.
- **Phase 4:** the burn-it-down phase is the only non-trivially-reversible one. Tag the commit immediately before deletion as `pre-chai-removal` so a single revert restores the Chai code.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| memdb memory footprint too large at scale | Low | Measure during Phase 1; 50K books × ~2KB/row = ~100MB. Even 5x growth is fine. |
| Write-through perf regression | Low | memdb writes are O(log n) on a radix tree; negligible vs Pebble fsync cost. |
| Schema indexer mismatch with our types | Medium | Per-table parity tests in Phase 3 catch this. |
| Forgetting a write-through somewhere | Medium | Audit all `_Chai` write call sites in Phase 2; add `make test` invariant: after any mutator, memdb counts == Pebble counts. |
| Crash mid-write (Pebble succeeds, memdb skipped) | Low | memdb is rebuilt from Pebble on startup. Next restart reconciles. |

## Out of scope

- Persistence for memdb (intentionally — Pebble persists; memdb is derived state)
- Migrating the Pebble store itself (still authoritative)
- Touching the SQLite store, Bleve search, chromem-go embeddings, NutsDB activity log
- Adding new query methods that don't exist today

## Success criteria

1. Zero `github.com/chaisql/chai` imports anywhere in the repo
2. All previous Chai queries return identical results via memdb
3. Aggregation query latency improved (target: 10x+ on `GetAllSeriesBookCounts`)
4. No data inconsistency between Pebble and memdb after any operation
5. Production deploy completes without "integer out of range" errors
6. `make ci` green

## Estimated effort

- Phase 1: 1 session (~2 hours)
- Phase 2: 1 session (~2 hours)
- Phase 3: 2–3 sessions (per-query work; ~1 hour each for 13 queries, parallelizable)
- Phase 4: 1 session (~1 hour cleanup + verification)

**Total: 5–7 focused sessions, deliverable as one PR per phase (4 PRs).**
