# Slow-Paths Cleanup (post-memdb)

## Goal

Move three remaining slow handlers off Pebble full-scans onto memdb. After PR #1135 fixed the `sort_by=title` library timeout, these are what's left making the dashboard/library page feel sluggish:

| Endpoint | Now | Cause | Target |
|---|---|---|---|
| `library_counts` recompute (drives `/system/status`) | 78s every ~10 min | `computeLibraryStats` does a full `book:` + `book_file:` Pebble scan with JSON unmarshal | <500ms via memdb iteration |
| `/api/v1/import-paths` | ~20s for 4 rows | `CountBooksByPathPrefix` runs a full Pebble scan **per folder** | <100ms via cached `BooksByImportPath` map |
| `/api/v1/audiobooks/soft-deleted?limit=10000` | ~20s | `ListSoftDeletedBooks` full Pebble scan to find the ~0–dozens of soft-deleted rows | <50ms via memdb `marked_for_deletion=true` index |

`/system/status` itself is not slow when the cache is warm — fixing the recompute fixes the periodic spike.

## Affected files

- `internal/database/memdb_reads.go` — add three new memdb-backed reads:
  - `ComputeLibraryStats() (*LibraryStats, error)` — iterates `memTableBooks` and `memTableBookFiles` in memory; mirrors the field-for-field aggregation in `pebble_store.go:computeLibraryStats`.
  - `ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error)` — uses `txn.Get(memTableBooks, memIdxMarkedForDeletion, true)`.
  - `CountBooksByPathPrefix(prefix string) (int, error)` — iterates `memTableBooks` (no Pebble index exists for path-prefix; full memdb scan is still ~200× faster than Pebble + JSON unmarshal).
- `internal/database/pebble_store.go`:
  - `computeLibraryStats` — early-return through `p.mem().ComputeLibraryStats()` when memdb is published, fall back to existing Pebble scan otherwise.
  - `ListSoftDeletedBooks` — same pattern: memdb fast-path, Pebble fallback.
  - `CountBooksByPathPrefix` — same pattern.
- `internal/server/filesystem_handlers.go:listImportPaths` — drop the per-folder `CountBooksByPathPrefix` call. Replace with one `GetDashboardStats()` lookup and read `stats.BooksByImportPath[folders[i].ID]`. This is the biggest win — turns N scans into one cached map read.
- Tests:
  - `internal/database/memdb_reads_test.go` — add table-driven tests for each new read against a fixture memstore.
  - Existing `pebble_store_test.go` coverage for the original methods stays valid (the fallback path).

## Ordered steps

1. **memdb reads** — implement the three new methods in `memdb_reads.go` with tests. No production wiring yet; this is purely additive.
2. **wire `ComputeLibraryStats`** — change `pebble_store.computeLibraryStats` to prefer memdb. Verify the returned `LibraryStats` is identical to Pebble's by running `make test` + an ad-hoc `curl /system/status`.
3. **wire `ListSoftDeletedBooks` + `CountBooksByPathPrefix`** — same memdb-first pattern.
4. **rewrite `listImportPaths`** — pull counts from `GetDashboardStats().BooksByImportPath` instead of per-folder scans.
5. **smoke test locally** — `make build && make run-api`, hit each endpoint, confirm <500ms.
6. **commit, PR, ship via `/ship`** — deploy to prod and verify the `library_counts cache recomputed` log line drops from `duration_ms=78000` to <500.

## Test strategy

- `go test ./internal/database/... -run "TestMemStore_(ComputeLibraryStats|ListSoftDeleted|CountBooksByPathPrefix)" -v` — new unit coverage.
- `go test ./internal/database/... -run "TestPebbleStore_(ComputeLibraryStats|ListSoftDeleted|CountBooksByPathPrefix)" -v` — fallback path still works.
- `go test ./internal/server/... -run "TestImportPaths" -v` — handler still returns correct shape.
- `make test` — full backend suite.
- Post-deploy: `journalctl -u audiobook-organizer.service --since "5 min ago" | grep "library_counts cache recomputed"` — confirm duration_ms drops by ~150×.

## Rollback

Fast-path is gated by `p.mem() != nil` — if memdb is unpublished, falls through to the original Pebble code unchanged. Worst case: `gh pr revert <N>`. No schema or write-path changes, purely read-side.
