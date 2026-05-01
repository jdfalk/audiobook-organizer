<!-- file: docs/codebase-evaluation.md -->
<!-- version: 2.0.1 -->
<!-- guid: 9a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-05-01 -->

# Audiobook Organizer — Codebase Evaluation

**Original audit:** 2026-04-30 | **Re-audit:** 2026-05-01  
**Scope:** Full repository at commit HEAD (Go backend + React/TypeScript frontend)  
**Tooling:** `go vet`, `go build`, `staticcheck`, source analysis, live test runs

---

## Table of Contents

1. [2026-05-01 Re-Audit — New Findings](#2026-05-01-re-audit--new-findings)
2. [2026-04-30 Original Findings — Status Update](#2026-04-30-original-findings--status-update)
3. [Critical Issues 🔴 (original)](#critical-issues-)
4. [Security Issues 🔐 (original)](#security-issues-)
5. [Major Issues 🟠 (original)](#major-issues-)
6. [Minor Issues 🟡 (original)](#minor-issues-)
7. [Frontend (original)](#frontend)
8. [Testing (original)](#testing)
9. [Dependencies (original)](#dependencies)
10. [Architecture Overview (original)](#architecture-overview)
11. [Appendix — Raw Metrics](#appendix--raw-metrics)

---

## 2026-05-01 Re-Audit — New Findings

**Date:** 2026-05-01  
**Commit:** `f67246b7` (HEAD → main)  
**Summary:** 38 original bot-tasks were implemented across PRs #587–#627. Build is clean (`go build ./...`, `go vet ./...`). However, **two test packages have regressions** and staticcheck surfaces a fresh set of dead-code, deprecated-field, and context-propagation issues.

New findings: **9** (High: 2, Medium: 4, Low: 3)

---

### R-1 — Unit tests failing: `GetAllBookSummaries` not mocked in server tests ❌ **High**

**File:** `internal/server/audiobook_service_unit_test.go`  
**Evidence:**
```
TestAudiobookService_GetAudiobooks_EmptyResult — FAIL
TestAudiobookService_GetAudiobooks_StoreError — FAIL
TestAudiobookService_InvalidateBookCaches_ClearsCache — FAIL
… (11+ tests total)

mock: I don't know what to return because the method call was unexpected.
  GetAllBookSummaries(int,int)
```

`audiobook_service.go:673` now calls `store.GetAllBookSummaries(…)` (added by PROJ-1/PROJ-2), but the unit tests in `audiobook_service_unit_test.go` still set up `Mock.On("GetAllBooks")`. The implementation changed, the tests did not. `make ci` reports `FAIL github.com/jdfalk/audiobook-organizer/internal/server`.

**Fix:** Update `audiobook_service_unit_test.go` to mock `GetAllBookSummaries` instead of `GetAllBooks` wherever the service now uses the summary path. Also add `Mock.On("GetUserBookState")` stubs where needed.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-test-1-fix-audiobook-service-tests.md`

---

### R-2 — `TestStoreAdditionalCoverageSQLite` failing in database package ❌ **High**

**File:** `internal/database/` (package-level failure)  
**Evidence:** `177.437s FAIL github.com/jdfalk/audiobook-organizer/internal/database`

The database test suite takes 177 s and exits with FAIL. Likely causes: new schema migrations not covered by test fixtures, or an assertion on a method whose signature changed. This blocks coverage reporting for the entire database layer.

**Fix:** Run `go test ./internal/database/... -v -run TestStoreAdditionalCoverageSQLite` to get the full error message, then fix the fixture or assertion. Check whether any migration added since the last test run requires an updated in-memory schema.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-test-2-fix-database-test-coverage.md`

---

### R-3 — ~~Secret still committed to `config.yaml`~~ ✅ **Not a finding** (false positive)

**File:** `config.yaml:10`

`openai_api_key: sk-test12345678` is an **intentional example placeholder** showing contributors what an OpenAI API key looks like. It is not a real secret. Do not flag or remove this value.

---

### R-4 — Deprecated `ITunesPath` field used in 13+ production locations ⚠️ **Medium**

**Files / Evidence (`staticcheck` SA1019):**
- `internal/itunes/service/importer.go:582,583,784,838,841`
- `internal/itunes/service/path_reconcile.go:148,149,152`
- `internal/itunes/service/path_repair.go:433,434,437`
- `internal/itunes/service/writeback_batcher.go:299,302`
- `internal/metafetch/service.go:3667,3680`
- `internal/metafetch/batch.go:53,54`

`Book.ITunesPath` is marked `// Deprecated: use book_files.itunes_path instead`. The deprecation note exists in `internal/database/store.go:154`. All 13+ call sites need to migrate to reading/writing via `BookFile.ITunesPath` and the `book_files` table.

**Fix:** Migrate each call site to use the `book_files` relation. Remove `ITunesPath` from the `Book` struct once all usages are gone.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-dep-1-migrate-itunes-path-field.md`

---

### R-5 — Dead code: unused functions, vars, and consts (`staticcheck` U1000) ⚠️ **Medium**

**Evidence:**
- `internal/config/persistence.go:769` — `legacySaveConfigToDatabase_REMOVED` unused func (nolint comment does not suppress U1000)
- `internal/database/pebble_store.go:6964` — `bookTagKeyspace` unused var
- `internal/database/sqlite_store.go:87` — `bookSummarySelectColumnsQualified` unused const
- `internal/itunes/service/importer.go:1065` — `linkAsVersion` unused func
- `internal/database/pebble_store.go:236,238` — `counterValue` and `value` assigned but never used (SA4006)
- `internal/metadata/enhanced.go:187,191` — `ok` from map lookups never used (SA4006)

Dead code increases binary size, confuses reviewers, and can indicate logic gaps (e.g., `bookSummarySelectColumnsQualified` may be intended for a query that never uses it).

**Fix:** Delete or use each dead symbol. For `bookSummarySelectColumnsQualified`, wire it into the query it was created for.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-dead-1-remove-unused-code.md`

---

### R-6 — `activity_store.go` uses `context.Background()` in 8 transaction sites ⚠️ **Medium**

**File:** `internal/database/activity_store.go`  
**Evidence:**
- Line 318: `s.db.BeginTx(context.Background(), nil)` inside `Summarize()`
- Line 665: `s.db.BeginTx(context.Background(), nil)` inside `CompactByDay()`
- Lines 328, 345, 352, 685, 737, 745, 773: `tx.ExecContext(context.Background(), …)`

`Summarize` and `CompactByDay` are called from HTTP handlers and scheduled jobs. Using `context.Background()` means these long-running compaction transactions cannot be cancelled if the caller's context is done. The same fix was applied to `sqlite_store.go` (DB-2) but not to `activity_store.go`.

**Fix:** Add `ctx context.Context` parameters to `Summarize` and `CompactByDay`; pass `ctx` through to all `BeginTx` / `ExecContext` calls.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-ctx-4-activity-store.md`

---

### R-7 — Unbounded `GetAllBooks(0, 0)` in background jobs — OOM risk ⚠️ **Medium**

**Files / Evidence:**
- `internal/server/archive_sweep.go:27` — `GetAllBooks(0, 0)`
- `internal/server/audiobook_service.go:783` — `GetAllBooks(0, 0)` in `EnrichAudiobooksWithNames`
- `internal/server/writeback_outbox.go:76` — `GetAllBooks(0, 0)`
- `internal/database/pebble_store.go:2163,2183` — `GetAllBooks(0, 0)` in search-index build
- (20 total call sites with `0` limit or `100000`+ hard-coded cap)

For libraries with 50,000+ books (the project cleaned 68K → 10.9K), loading all books in one slice can exhaust heap memory. The proper pattern is cursor-based pagination (batch size 1,000, loop until empty).

**Fix:** Replace `GetAllBooks(0, 0)` calls in long-running background jobs with a paginated loop: `for offset := 0; ; offset += batchSize { books, _ = store.GetAllBooks(batchSize, offset); if len(books) == 0 { break } }`.

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-perf-1-paginate-getallbooks.md`

---

### R-8 — Remaining `fmt.Printf` / `log.Printf` in library packages 🟡 **Low**

**Files / Evidence:**
- `internal/database/sqlite_store.go:404` — `fmt.Printf("Warning: series deduplication failed: …")`
- `internal/database/sqlite_store.go:807` — `fmt.Printf("Deduplicated %d series records\n", …)`
- `internal/playlist/playlist.go:94` — `fmt.Printf("Generated playlist: %s\n", …)`
- `internal/organizer/organizer.go:68` — `fmt.Printf("Warning: failed to clean temporary …")`
- `internal/database/pebble_store.go:93,119,134,754,783` — bare `log.Printf` calls
- `internal/database/migrations.go:395–433` — bare `log.Printf` / `log.Println` calls
- `internal/database/sqlite_store.go:2813,3164` — `log.Printf` in transaction rollback paths
- `internal/database/metadata_fetch_cache.go:88` — `log.Printf` warning

LOG-1..4 bot-tasks addressed `fmt.Printf` in `tagger`, `fileops`, `backup`, and `scanner` packages. Several library packages were missed.

**Fix:** Replace remaining `fmt.Printf` / bare `log.Printf` with structured logging (`log/slog` or the project's `logger.Logger` interface where available).

**Bot-task spec:** `docs/superpowers/bot-tasks/2026-05-01-log-5-remaining-printf.md`

---

### R-9 — Stale `// TODO: Implement in N1-2` comments after implementation 🟡 **Low**

**Files / Evidence:**
- `internal/database/sqlite_store.go:6913` — `// TODO: Implement in N1-2` above `GetAuthorsByBookIDs` which IS implemented
- `internal/database/sqlite_store.go:6946` — same above `GetNarratorsByBookIDs`
- `internal/metadata/enhanced.go:188,192` — `// TODO: Resolve author/series name to ID` — unactioned for multiple audit cycles

These stale TODOs cause reviewer confusion and obscure genuinely open work.

**Fix:** Remove the two stale N1-2 comments. Create separate tickets for the metadata `enhanced.go` TODOs if they are intentional deferrals.

---

### R-10 — Capitalized error strings in metadata packages (`staticcheck` ST1005) 🟡 **Low**

**Files / Evidence:**
- `internal/metadata/audible.go:150,159,180`
- `internal/metadata/audnexus.go:115,185`
- `internal/metadata/googlebooks.go:115`
- `internal/metadata/hardcover.go:260,275`
- `internal/metadata/openlibrary.go:171,234,308`
- `internal/metadata/wikipedia.go:127`

Go convention (and `staticcheck` ST1005): error strings should not be capitalized or end with punctuation. This matters when errors are wrapped with `fmt.Errorf("fetch failed: %w", err)` — the result is a double-capital sentence mid-chain.

**Fix:** Lowercase the first letter of each error string (e.g., `"Failed to fetch"` → `"failed to fetch"`).

---

## 2026-04-30 Original Findings — Status Update

The following table records the disposition of every finding from the 2026-04-30 audit. Bot-tasks MOCK-1 through SCAN-2 and FE-1 through FE-10 were all implemented in PRs #587–#627.

| ID | Finding | Status | Notes |
|----|---------|--------|-------|
| C-1 | Mock interface drift: `GetDuplicateFilesByHash` | ✅ DONE | MOCK-1: mock regenerated |
| C-2 | Mock interface drift: `MergeChapterBooks` | ✅ DONE | MOCK-2: CI gate added |
| C-3 | N+1 queries in `enrichBookForResponse` | ✅ DONE | N1-1..4: batch fetch + enrich rewrite |
| C-4 | N+1 queries in `EnrichAudiobooksWithNames` | ✅ DONE | N1-1..4 |
| C-5 | Secret committed to `config.yaml` | ✅ NOT A FINDING | `sk-test12345678` is an intentional example placeholder, not a real key |
| S-1 | `BrowseDirectory` unrestricted filesystem | ✅ DONE | SEC-1: `defaultBrowseAllowPrefixes` |
| S-2 | Auth disabled by default | ✅ DONE | SEC-2: startup warning added |
| S-3 | Rate limiting disabled by default | ✅ DONE | SEC-3: startup warning added |
| S-4 | `books.file_hash` missing index | ✅ DONE | FS-1: unique index migration added |
| S-5 | Rate limiter O(N) cleanup | ✅ DONE | SEC-4: lazy eviction |
| M-1 | Global `GetGlobalStore()` anti-pattern | ⚠️ PARTIAL | DI migration still in progress; 103+ calls remain |
| M-2 | `context.Background()` in HTTP handlers | ✅ DONE | CTX-1/2/3: context threaded through major paths |
| M-3 | `db.Begin()` instead of `BeginTx()` | ✅ DONE | DB-2: all SQLite Begin replaced |
| M-4 | `SavePhaseData` errors silently discarded | ✅ DONE | DB-4: pipeline error propagation |
| M-5 | `recomputeDurationMap` failure ignored | ✅ DONE | DB-6: pebble silent errors addressed |
| M-6 | `RecordPathChange` silently ignored | ✅ DONE | DB-6 |
| M-7 | Monolithic `internal/server` package | ⚠️ PARTIAL | Still flat; middleware sub-package exists |
| M-8 | `maintenance_fixups.go` 6,304 lines | ⚠️ PARTIAL | Now 6,400 lines (grew); still unsplit |
| M-9 | `pebble_store.go` 8,031 lines | ⚠️ PARTIAL | Now 8,324 lines; still unsplit |
| M-10 | `sqlite_store.go` 6,509 lines | ⚠️ PARTIAL | Now 6,976 lines; still unsplit |
| M-11 | `metafetch/service.go` 3,932 lines | ⚠️ PARTIAL | Now 3,932 lines; unchanged |
| M-12 | `fmt.Printf` in library packages | ⚠️ PARTIAL | LOG-1..4 fixed tagger/fileops/backup/scanner; 8 still remain → R-8 |
| M-13 | `RowsAffected()` error ignored | ✅ DONE | DB-2 scope |
| M-14 | `bookSelectColumns` selects all 50+ columns | ✅ DONE | PROJ-1/2: BookSummary projected query |
| N-1 | `filepath.Walk` → `filepath.WalkDir` | ✅ DONE | SCAN-1 |
| N-2 | `progressbar` in server context | ✅ DONE | SCAN-2: removed, structured log events |
| N-3 | 24-hour cache TTL no active invalidation | ⚠️ PARTIAL | No change observed |
| N-4 | `WriteTimeout=0` for SSE | ⚠️ PARTIAL | By design; no idle heartbeat guard added |
| N-5 | `time.Parse` errors silently discarded | ✅ DONE | DB-5 |
| N-6 | Rate limiter O(N) cleanup | ✅ DONE | SEC-4 (same as S-5) |
| N-7 | `json.Marshal` errors discarded in AI pipeline | ✅ DONE | DB-4 scope |
| N-8 | `db.Begin()` without context in `activity_store.go` | ⚠️ PARTIAL | `BeginTx` changed but `context.Background()` still used → R-6 |
| N-9 | No gzip compression middleware | ✅ DONE | COMP-1/SRV-1: gin-contrib/gzip added |
| N-10 | `malformed_m4b_remux.go` ignores WalkDir error | ⚠️ PARTIAL | Walk replaced but `_ =` still on return |
| N-11 | `RevokeSession` error ignored | ⚠️ PARTIAL | Unchanged |
| N-12 | `SetSetting` return ignored in remux | ⚠️ PARTIAL | Unchanged |
| N-13 | Hard-coded cover source allowlist | ⚠️ PARTIAL | No change |
| N-14 | Positional `Scan()` fragile to schema changes | ⚠️ PARTIAL | Not changed |
| N-15 | CHANGELOG / TODO update tracking | ✅ DONE | Regular updates observed |
| F-1 | `Library.tsx` God Component 3,372 lines | ✅ DONE | FE-1/2/3: FilterPanel + BookGrid + BatchToolbar extracted |
| F-2 | `Settings.tsx` God Component 4,166 lines | ✅ DONE | FE-4/5/6: three tab components extracted |
| F-3 | 17 `useEffect` in `Library.tsx` | ⚠️ PARTIAL | Component split helps; effect count not verified post-split |
| F-4 | 108 `console.log` calls | ✅ DONE | FE-7: replaced/removed |
| F-5 | Error boundary only at top level | ✅ DONE | FE-8: per-page error boundaries |
| F-6 | `useCallback`/`useMemo` not systematic | ⚠️ PARTIAL | No explicit task addressed this |
| F-7 | Inline styles mixed with MUI `sx` | ⚠️ PARTIAL | No explicit task addressed this |
| F-8 | Frontend coverage thresholds too low (15%) | ⚠️ PARTIAL | FE-10 added thresholds but they remain at 15%/10% |
| F-9 | SSE connection status in `console.log` | ✅ DONE | FE-7 scope |
| F-10 | `localStorage` keys as magic strings | ✅ DONE | FE-8/9: typed constants |
| T-1 | Broken mock | ✅ DONE | MOCK-1 |
| T-2 | CI doesn't enforce mock freshness | ✅ DONE | MOCK-2: `check-mock-fresh` target |
| T-3 | Frontend coverage thresholds too low | ⚠️ PARTIAL | Same as F-8 |
| T-4 | No integration tests for N+1 | ⚠️ PARTIAL | No integration test suite added |
| T-5 | `testdata/` binary files in git | ⚠️ PARTIAL | `.gitignore` not updated for LFS |
| T-6 | No `staticcheck` in `make ci` | ✅ DONE | MOCK-2: `staticcheck` added to CI |
| D-1 | `go-sqlite3` requires CGO | ⚠️ PARTIAL | No migration to pure-Go SQLite |
| D-2 | `progressbar` CLI dep in server code | ✅ DONE | SCAN-2: dependency removed |
| D-3 | `quic-go` possibly unused | ⚠️ PARTIAL | Still in `go.mod` |
| D-4 | `chromem-go` in-memory only | ⚠️ PARTIAL | Still in use |
| D-5 | `axios` and `fetch` side-by-side | ⚠️ PARTIAL | No consolidation |
| D-6 | MUI v5 superseded by v6 | ⚠️ PARTIAL | Still on v5 |
| D-7 | `node_modules/` in repo root | ✅ DONE | Not tracked in git (verified) |
| D-8 | Go version mismatch `go.mod` vs docs | ⚠️ PARTIAL | `go.mod` says `1.24.0`; runtime is 1.25+ |
| A-1 | Dual database backend burden | ⚠️ PARTIAL | Architectural; no consolidation started |
| A-2 | `Store` interface 580+ methods | ⚠️ PARTIAL | Width unchanged; sub-interfaces not added |
| A-3 | No pagination on some list endpoints | ⚠️ PARTIAL | Partially addressed by `GetAllBookSummaries`; several unbounded calls remain → R-7 |

---

---

## Critical Issues 🔴

### C-1 — Mock interface drift: `GetDuplicateFilesByHash` missing

**File:** `internal/database/mocks/mock_store.go`  
**Severity:** 🔴 Critical — entire test suite fails to compile

`MockStore` does not implement `GetDuplicateFilesByHash`, which was added to the `Store` interface in `internal/database/store.go`. `go vet ./...` and `staticcheck` both produce the same compile-level error across 9+ packages:

```
internal/database/mocks/mock_store.go: MockStore does not implement
  database.Store (missing GetDuplicateFilesByHash method)
```

Affected packages: `metadata`, `scanner`, `server`, `organizer`, `merge`, `versions`, `operations`, `diagnostics`, `itunes/service`, `cmd`.

**Fix:** Re-run `mockery --name Store --dir internal/database --output internal/database/mocks`. The `docs/MOCKERY_GUIDE.md` documents the required invocation.

---

### C-2 — Mock interface drift: potentially missing `MergeChapterBooks`

**File:** `internal/database/mocks/mock_store.go`  
**Severity:** 🔴 Critical (same root cause as C-1)

In addition to `GetDuplicateFilesByHash`, any interface method added after the last `mockery` run will be absent. Both issues stem from the same systemic failure: the mock is not re-generated after interface additions. `make ci` should fail loudly when this happens.

---

### C-3 — N+1 queries in `enrichBookForResponse` (per-book 4-5 DB calls)

**File:** `internal/server/server.go:334–406`  
**Severity:** 🔴 Critical for large libraries

`enrichBookForResponse` is called for every book in a list response. For each book it makes:
1. `GetBookAuthors(bookID)` — 1 query
2. `GetAuthorByID(id)` for each author — N nested queries
3. `GetBookNarrators(bookID)` — 1 query
4. `GetNarratorByID(id)` for each narrator — M nested queries
5. `GetBooksByMetadataSourceHash(hash)` — 1 query

For a 1,000-book library with 2 authors + 1 narrator per book, this produces **~4,000 DB round-trips per page request**. Even with SQLite's low overhead this serialises all work and will lock the database connection pool.

**Fix:** Add batch-fetch methods (`GetAuthorsByBookIDs`, `GetNarratorsByBookIDs`) and do a single query + in-memory group-by, or store author/narrator names denormalised in the books row (they already have `author_name`/`narrator_name` columns — use those).

---

### C-4 — N+1 queries in `EnrichAudiobooksWithNames` loop

**File:** `internal/server/audiobook_service.go:782`  
**Severity:** 🔴 Critical

`EnrichAudiobooksWithNames` calls `resolveAuthorAndSeriesNames` inside a loop over every book. Each invocation makes separate DB lookups. This duplicates the C-3 pattern in the service layer.

---

### C-5 — Secret committed to config file

**File:** `config.yaml:~11`  
**Severity:** ~~🔴 Critical~~ ✅ **Not a finding**

```yaml
openai_api_key: sk-test12345678
```

This is an **intentional example placeholder** showing contributors the format of an OpenAI API key. It is not a real secret and should not be removed or flagged by security scanners.

---

## Security Issues 🔐

### S-1 — `BrowseDirectory` allows unrestricted server filesystem access

**File:** `internal/server/filesystem_service.go:35–79`  
**Handler:** `internal/server/filesystem_handlers.go:43`  
**Severity:** 🔐 High

`BrowseDirectory` calls `filepath.Abs(path)` and then `os.ReadDir(absPath)` with no restriction on where `absPath` can be. Any authenticated user can supply `/etc`, `/root`, `/home`, `/var`, etc. and enumerate the server filesystem. This is especially dangerous when `enable_auth: false` (the default — auth is opt-in).

**Fix:** Validate that `absPath` is a subdirectory of a configured allow-list (e.g., the user's home dir + configured import paths). Reject paths that escape the allowed set.

---

### S-2 — Authentication disabled by default

**File:** `internal/server/server.go:2262–2278`, `config.yaml`  
**Severity:** 🔐 High

`enable_auth` defaults to `false`. On a fresh deployment with default config, no authentication is required for any API endpoint. Combined with S-1, this means any network-adjacent user can browse the server's entire filesystem.

**Fix:** Reverse the default: require explicit `enable_auth: false` to opt out of authentication. Or at minimum, log a prominent startup warning when auth is disabled.

---

### S-3 — API rate limiting disabled by default

**File:** `internal/server/server.go:2262–2278`, `internal/config/config.go`  
**Severity:** 🔐 Medium

`api_rate_limit_per_minute` defaults to `0` (disabled). Only `auth_rate_limit_per_minute` has a non-zero default (10 RPM). A deployment without explicit config has no API-level rate limiting, exposing the server to enumeration and DoS.

**Fix:** Set a reasonable non-zero default (e.g., 120 RPM) and allow opt-out rather than opt-in.

---

### S-4 — `books` table missing index on `file_hash` column

**File:** `internal/database/migrations.go:601–603`  
**Related:** `internal/database/sqlite_store.go:1957`  
**Severity:** 🔐 Medium (DoS via table-scan amplification)

`GetBookByFileHash` executes `SELECT … FROM books WHERE file_hash = ?`. The migration creates indexes on `original_file_hash` and `organized_file_hash` (lines 602–603) but **not** on `file_hash`. On a large library this forces a full table scan on every dedup check and file-import lookup.

**Fix:** Add `CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash) WHERE file_hash IS NOT NULL` in a new migration step.

---

### S-5 — Rate limiter O(N) cleanup holds write lock on every request

**File:** `internal/server/middleware/ratelimit.go`  
**Severity:** 🔐 Low / Performance

The per-IP rate limiter iterates all `entries` to delete expired keys on **every request** while holding a write mutex:

```go
for key, entry := range r.entries {
    if now.Sub(entry.lastSeen) > r.idleTTL {
        delete(r.entries, key)
    }
}
```

With many unique IPs, this O(N) scan blocks all concurrent requests during cleanup. A standard lazy-eviction (only evict the accessed key) or a periodic background cleanup goroutine should be used instead.

---

## Major Issues 🟠

### M-1 — Global store anti-pattern (103+ uses of `GetGlobalStore()`)

**File:** `internal/database/store.go:892–900` (global state definition)  
**Callsites:** 103+ in non-test production code  
**Examples:** `internal/server/server.go`, `internal/server/audiobook_service.go`, `internal/server/filesystem_handlers.go`

`database.GetGlobalStore()` acts as a global singleton. A comment in `server.go` acknowledges "the global is being phased out per the 4.4 DI migration", but the migration is far from complete. Global state makes unit testing without mocks impossible, hides dependencies, and causes race conditions when the global is mutated in tests. `config.AppConfig` has the same anti-pattern.

**Fix:** Continue the DI migration — pass `database.Store` through constructors; do not access globals in handlers.

---

### M-2 — `context.Background()` in HTTP handler paths

**Files:** 30+ instances across `internal/server/` (non-test, non-mock)  
**Examples:**
- `internal/server/audiobook_update_service.go:173`
- `internal/server/openlibrary_service.go:181`
- `internal/server/filesystem_handlers.go:165`

HTTP handlers receive a `*gin.Context` that embeds the request context. Using `context.Background()` instead of `c.Request.Context()` means: DB/network calls continue after the client disconnects, goroutine leaks accumulate when clients timeout, and distributed tracing spans lose the parent.

**Fix:** Replace `context.Background()` with `c.Request.Context()` in all handler code. For goroutines that must outlive the request, derive from a `context.WithTimeout(context.Background(), maxDuration)` and store it explicitly.

---

### M-3 — `db.Begin()` instead of `db.BeginTx()` throughout SQLite layer

**File:** `internal/database/sqlite_store.go` (14 `db.Begin()` calls)  
**Examples:** Lines 1270, 1294, 2414, 2615, 4452, 4562, …

`database/sql`'s `Begin()` does not accept a context, so these transactions cannot be cancelled when the caller's context is done. The entire transaction will run to completion even if the requesting client has already disconnected.

**Fix:** Replace all `s.db.Begin()` with `s.db.BeginTx(ctx, nil)` where `ctx` is propagated from the caller.

---

### M-4 — `SavePhaseData` errors silently discarded in AI scan pipeline

**File:** `internal/server/ai_scan_pipeline.go:395, 406, 446, 457, 510, 520, 543, 553, 607`  
**Severity:** 🟠 Major

All 9+ calls to `pm.scanStore.SavePhaseData(...)` use `_ =` to discard errors. If phase data fails to save, the pipeline continues silently; the UI shows an incomplete or empty scan result with no indication of failure. Same pattern for `UpdateOperationStatus`, `UpdateScanStatus`, etc.

---

### M-5 — `recomputeDurationMap` failure silently ignored

**File:** `internal/database/pebble_store.go:4958`

```go
_ = p.recomputeDurationMap(bookNumericID)
```

If duration recomputation fails, the book's total duration will be incorrect indefinitely. This affects sorting by duration and UI display. At minimum the error should be logged.

---

### M-6 — `RecordPathChange` silently ignored

**Files:**
- `internal/database/pebble_store.go:1584`
- `internal/database/sqlite_store.go:2692`

Path history is used for rename tracking / revert. Silent failure means the audit trail for file moves is incomplete with no notification.

---

### M-7 — Monolithic `internal/server` package (150+ files)

**Directory:** `internal/server/`

`internal/server` contains 150+ `.go` files in a single package. The package has no internal sub-package structure; every symbol is visible to every other file. This makes the dependency graph opaque, coupling maintenance_fixups to realtime to metadata to filesystem. Large refactors become risky because a change anywhere could affect the entire server package.

**Fix:** Establish sub-packages: `internal/server/handlers`, `internal/server/middleware`, `internal/server/services`. The middleware sub-package already exists but most code is still in the flat package.

---

### M-8 — `maintenance_fixups.go` is 6,304 lines

**File:** `internal/server/maintenance_fixups.go`

One Go source file at 6,304 lines contains dozens of unrelated maintenance endpoint handlers. It should be split into domain-specific files (audio handlers, tag repair handlers, library maintenance handlers, etc.). At current size, git blame and code navigation are impractical.

---

### M-9 — `pebble_store.go` is 8,031 lines

**File:** `internal/database/pebble_store.go`

8,031 lines in a single Go file is the largest production Go source file in the repo. All PebbleDB store methods — books, authors, series, playlists, playback, AI scans, operations, tags, settings, embeddings — are in one file. Build times for this file are disproportionately long and incremental compilation offers no benefit.

**Fix:** Split by domain: `pebble_books_store.go`, `pebble_authors_store.go`, `pebble_operations_store.go`, etc.

---

### M-10 — `sqlite_store.go` is 6,509 lines

**File:** `internal/database/sqlite_store.go`

Same concern as M-9. The file spans books CRUD, authors, series, narrators, tags, playback, path history, metadata versioning, book segments, and more. All at 6,509 lines in a single file.

---

### M-11 — `metafetch/service.go` is 3,932 lines

**File:** `internal/metafetch/service.go`

The metadata fetch service is at version `4.63.0`, indicating it has been continually grown rather than split. It handles Open Library fetching, AI ranking, ISBN enrichment, cover art, author disambiguation, and more. Each of these is a distinct responsibility.

---

### M-12 — `fmt.Printf` used in library packages instead of structured logging

**Files:**
- `internal/database/sqlite_store.go:316, 715` — warnings to stdout on dedup
- `internal/tagger/tagger.go:48, 55, 93, 104, 115` — operation output to stdout
- `internal/fileops/safe_operations.go:137, 144, 207`
- `internal/backup/backup.go:119, 194, 367`
- `internal/organizer/organizer.go:68`

Library packages (not CLI entry points) write to `stdout` via `fmt.Printf`. In a server context, this interleaves with Gin's structured logger output and is impossible to filter or redirect. The project uses `log` and has structured logging infrastructure — these should use it.

---

### M-13 — `RowsAffected()` error ignored in transactions

**File:** `internal/database/sqlite_store.go:1285`

```go
rows, _ := result.RowsAffected()
```

`RowsAffected()` can return an error (driver-dependent). The ignored error means a failed segment lookup silently reports 0 rows affected, which triggers the "segment not found" error path — but for the wrong reason.

---

### M-14 — `bookSelectColumns` always selects all 50+ columns

**File:** `internal/database/sqlite_store.go:26–71`

The `bookSelectColumns` constant is a 50+ column `SELECT` used in every single book query — including list endpoints, search endpoints, and lightweight ID lookups. There is no projection or partial-select variant. For list endpoints this transfers significantly more data than needed from SQLite to Go.

**Fix:** Introduce a `bookSummaryColumns` variant for list/search use cases that omits large text fields (`description`, `full_text`, `raw_metadata`, etc.).

---

## Minor Issues 🟡

### N-1 — `filepath.Walk` should be `filepath.WalkDir`

**File:** `internal/scanner/scanner.go` (primary walk), `internal/server/malformed_m4b_remux.go:52`

`filepath.Walk` calls `os.Lstat` on every entry, even when the entry's `FileInfo` is already known. `filepath.WalkDir` (Go 1.16+) avoids the redundant stat call. On large libraries this is measurable. The project uses Go 1.26.

---

### N-2 — `progressbar` CLI library used in server context

**File:** `internal/scanner/scanner.go:286`  
**Dependency:** `github.com/schollz/progressbar/v3`

`progressbar` writes ANSI escape sequences to `os.Stderr`. In a server process with no terminal, this produces garbage output to the error log. The scanner already uses SSE (`internal/realtime`) for progress reporting — the progressbar should be removed.

---

### N-3 — 24-hour TTL on list/dedup/facets caches with no active invalidation

**File:** `internal/cache/cache.go`  
**Users:** `internal/server/audiobook_service.go`, `internal/server/dedup_service.go`

All three caches use 24-hour TTLs. If cache invalidation on write is not consistent, stale data persists for up to a day. The cache is a generic LRU with no domain awareness — a write to a single book could invalidate the entire list cache unnecessarily, or miss invalidation entirely.

---

### N-4 — `WriteTimeout=0` on server for SSE streams

**File:** `internal/server/server.go:3316–3318`

`WriteTimeout` is set to `0` (no timeout) to support SSE long-polling. A hung SSE client (e.g., due to a network issue that doesn't surface as a TCP RST) holds the connection indefinitely. Consider using a per-request idle timeout (application-level heartbeat + disconnect on missed ping) rather than disabling the server-level write timeout entirely.

---

### N-5 — `time.Parse` errors silently discarded in SQLite store

**File:** `internal/database/sqlite_store.go:4274–4275, 4307, 4342, 4408–4409`

Multiple `time.Parse` calls discard the error silently. When a stored timestamp is malformed, the resulting `time.Time` is the zero value, which corrupts sort order and display formatting without any log entry.

---

### N-6 — Rate limiter uses O(N) cleanup on every request

Already noted in **S-5**, but also a correctness concern: the cleanup iterates entries under a write lock, blocking concurrent requests from any IP during cleanup.

---

### N-7 — `ai_scan_pipeline.go` discards `json.Marshal` errors

**File:** `internal/server/ai_scan_pipeline.go:334, 359`

```go
rolesJSON, _ = json.Marshal(s.Roles)
```

`json.Marshal` can fail for types with circular references or channel fields. Discarding the error means malformed JSON may be stored in the scan phase data.

---

### N-8 — `db.Begin()` (without context) used in `activity_store.go`

**File:** `internal/database/activity_store.go:299, 642`

Same issue as M-3 but in a separate file outside `sqlite_store.go`.

---

### N-9 — No response compression middleware

**File:** `internal/server/server.go` (middleware stack)

The server returns JSON API responses without gzip/brotli compression. For large library listings (hundreds of books with metadata), uncompressed JSON can be 1–2 MB per response. Gin has a built-in gzip middleware (`github.com/gin-contrib/gzip`) that can reduce wire size by 70–80% with one import.

---

### N-10 — `malformed_m4b_remux.go` silently ignores `filepath.WalkDir` return

**File:** `internal/server/malformed_m4b_remux.go:52`

```go
_ = filepath.WalkDir(root, func(...) error { ... })
```

Walk errors (e.g., permission denied on a directory) are silently dropped, so the remux may process fewer files than expected with no indication.

---

### N-11 — `session.RevokeSession` error silently ignored on auth failure

**File:** `internal/server/middleware/auth.go:129`

```go
_ = store.RevokeSession(token)
```

If a token is invalid and the session cannot be revoked, the failure is silently ignored. The client is still rejected (correct), but the dangling session record will persist until TTL expiry.

---

### N-12 — `SetSetting` return ignored in malformed_m4b_remux

**File:** `internal/server/malformed_m4b_remux.go:91`

```go
_ = store.SetSetting(malformedRemuxKey, "true", "bool", false)
```

If the setting fails to save, the remux process will re-run on next startup.

---

### N-13 — `isAllowedCoverSource` list is hard-coded; no wildcard support

**File:** `internal/server/covers.go:114–`

The allowed cover proxy sources are a hard-coded string slice. Adding a new CDN or changing Audible's image host requires a code change + deployment.

---

### N-14 — `bookSelectColumns` uses positional scan, not named columns

**File:** `internal/database/sqlite_store.go:26–71`

The 50+ column scan uses positional `Scan()` parameters. Any schema migration that adds a column in the middle will silently corrupt every scanned book. Named column scanning (`sqlx` or explicit maps) is safer.

---

### N-15 — Missing CHANGELOG / TODO update tracking

**Files:** `CHANGELOG.md`, `TODO.md`

Per `CLAUDE.md` policy: "After completing any feature/fix: update CHANGELOG, update TODO, and commit before moving on." Several recently merged areas (AI scan pipeline, mock drift, DI migration) are not reflected in the CHANGELOG.

---

## Frontend

### F-1 — `Library.tsx` is a God Component (3,372 lines, 90 useState, 17 useEffect)

**File:** `web/src/pages/Library.tsx`  
**Metrics:** 3,372 lines, 90 `useState` declarations, 17 `useEffect` hooks

This single component manages: pagination, search/filter state, audiobook listing, batch operations, SSE subscription, scan/organize operations, tag management, server-side import paths, and URL state sync. It is untestable as a unit and has 17 `useEffect` hooks that interact with each other in non-obvious ways.

**Fix:** Extract sub-components:
- `BookGrid` / `BookList` (display)
- `FilterPanel` (filter state)
- `BatchOperationsToolbar` (batch edit, tag, rate)
- `OperationProgressPane` (scan/organize SSE)
- `LibraryURLSync` (URL ↔ state synchronisation)

---

### F-2 — `Settings.tsx` is a God Component (4,166 lines, 61 useState)

**File:** `web/src/pages/Settings.tsx`  
**Metrics:** 4,166 lines, 61 `useState` declarations

The largest frontend file combines: general settings, audio path management, backup config, OpenAI settings, metadata sources, database maintenance, cover art settings, and more. Multiple unrelated domains share state and render in one component tree.

---

### F-3 — 17 `useEffect` calls in `Library.tsx` create fragile dependency chains

**File:** `web/src/pages/Library.tsx:342, 360, 474, 481, 488, 499, 508, 518, 546, 550, 778, 784, 791, 873, 892, 905, 911`

Each `useEffect` maintains its own state synchronisation. Some use `useRef` flags (`isInitialMount`, `isInternalUpdate`) to suppress re-runs — a pattern indicating the state model is too complex for the component. Stale closure bugs are likely given the volume of interdependent effects.

---

### F-4 — 108 `console.log` calls in frontend production code

**Files:** Across `web/src/` (grep: 108 matches)  
**Examples:**
- `web/src/pages/Library.tsx:457, 1155, 1531` — debug logs in SSE and sort handlers
- `web/src/pages/Settings.tsx:1849–1857` — multiple debug logs in settings handlers

All debug `console.log` calls should be replaced with a conditional logger that no-ops in production (`import.meta.env.PROD`), or removed entirely. These slow rendering and leak internal state to browser devtools.

---

### F-5 — Error boundary only at top level (`App`)

**File:** `web/src/main.tsx`

`<ErrorBoundary>` wraps the entire app but there are no per-route or per-section boundaries. An unhandled React rendering error in any one page component crashes the entire application.

**Fix:** Add error boundaries around each route and each major section (Library, Settings, BookDetail).

---

### F-6 — `useCallback`/`useMemo` not systematic in large list components

**File:** `web/src/pages/Library.tsx`, `web/src/pages/BookDetail.tsx`

`useCallback` and `useMemo` are used in some places but not systematically. With 90 `useState` values, re-renders of `Library.tsx` re-create many callbacks on every state change, even those that don't depend on the changed state. This causes child components to re-render unnecessarily.

---

### F-7 — Inline styles mixed with MUI `sx` prop

**File:** Multiple components (20+ inline `style=` attributes found)

Mixing MUI's `sx` theming API with raw `style={{…}}` attributes bypasses MUI's style system, making theme changes (dark mode, spacing overrides) incomplete.

---

### F-8 — Test coverage thresholds are ineffectively low

**File:** `web/vite.config.ts`

```ts
thresholds: { statements: 15, lines: 15, branches: 10, functions: 15 }
```

15% statement/line and 10% branch coverage is essentially no coverage enforcement. In a project this large with this many state interactions, meaningful thresholds should be 60–70%+ with explicit exclusions for generated code and third-party wrappers.

---

### F-9 — SSE connection status logged to `console.log` in production

**File:** `web/src/pages/Library.tsx:499`

```ts
console.log('EventSource connection established');
```

SSE reconnection events are common (network hiccups, server restarts). Each reconnection logs to the browser console, adding noise and potentially leaking connection timing information in screenshots/recordings.

---

### F-10 — `localStorage` keys are magic strings scattered across Library.tsx

**File:** `web/src/pages/Library.tsx:~826`

`localStorage.getItem('library_page')` and similar calls use bare string keys. A typo in one location silently fails. Centralise `localStorage` key names in a typed constant object.

---

## Testing

### T-1 — Broken mock (see C-1, C-2)

All Go unit tests fail to compile. This has presumably been the state for multiple commits without CI enforcement of `go test ./...`.

---

### T-2 — `Makefile` `ci` target should enforce mock freshness

**File:** `Makefile`

The `make ci` target runs tests, but there is no step that verifies `mock_store.go` is up-to-date with the current `Store` interface. Add a `make generate` step (runs mockery) and compare output to committed file; fail CI if they differ.

---

### T-3 — Frontend coverage thresholds too low (see F-8)

See F-8 above.

---

### T-4 — No integration tests for critical paths (N+1 queries)

**Directory:** `tests/`

The test suite has unit tests and E2E Playwright tests but no integration tests that exercise the `enrichBookForResponse` path with a real SQLite database and a large book set. N+1 regressions would not be caught.

---

### T-5 — `testdata/` contains binary files tracked in git

**Directory:** `testdata/`

Binary test assets (audio files, cover images) tracked in git increase clone size and slow CI. Use `git-lfs` or download test fixtures as part of test setup.

---

### T-6 — No `make lint` or `staticcheck` step in `make ci`

**File:** `Makefile`

`make ci` runs tests and coverage but does not invoke `staticcheck` or `golangci-lint`. Static analysis findings (like the mock drift) would be caught on every PR if added to the CI target.

---

## Dependencies

### D-1 — `go-sqlite3` requires CGO (`mattn/go-sqlite3`)

**File:** `go.mod`

`github.com/mattn/go-sqlite3` requires CGO. This means: cross-compilation requires a cross-compiler, Docker images must have a C toolchain, and builds without CGO (`CGO_ENABLED=0`) fail silently (fallback to no-op store). The separate `Dockerfile.build-cgo` exists to handle this, but it adds build complexity.

**Alternative:** `modernc.org/sqlite` is a pure-Go SQLite port that does not require CGO.

---

### D-2 — `schollz/progressbar` is a CLI-only dependency used in server code

**File:** `go.mod`, `internal/scanner/scanner.go:286`

A CLI progress bar library is imported by a server package. This is a build-time smell: any package that imports `internal/scanner` transitively depends on a terminal-display library.

---

### D-3 — `quic-go` (HTTP/3) adds significant CGO/complexity overhead

**File:** `go.mod`

`github.com/quic-go/quic-go` is included but HTTP/3 support is not documented as a feature. If it is unused, removing it reduces the build surface and dependency attack surface.

---

### D-4 — `chromem-go` (embeddings) uses in-memory storage only

**File:** `go.mod`, embeddings integration

`github.com/philippgille/chromem-go` stores vectors in memory (or serialized to disk manually). For large libraries with thousands of embeddings, this does not scale. Consider `pgvector` or a dedicated vector store if embedding search is a roadmap feature.

---

### D-5 — Frontend: `axios` and native `fetch` used side-by-side

**File:** `web/package.json`, `web/src/api/`

Both `axios` and native `fetch` (via the generated OpenAPI client) are used. This creates two error-handling patterns, two interceptor systems, and two request-cancellation APIs.

---

### D-6 — MUI v5 — known to be superseded by MUI v6

**File:** `web/package.json`

MUI v5 (`@mui/material: ^5`) is the current dependency. MUI v6 introduced a new styling system (`sx`→CSS variables) with significant performance improvements. Not blocking, but a future migration path should be planned.

---

### D-7 — `node_modules/` present in repository root

**Directory:** `node_modules/`

`node_modules/` appears in the repository directory listing. If this is committed to git (not `.gitignore`d), it dramatically inflates repo size and should be removed.

---

### D-8 — Go version mismatch between `go.mod` and documentation

**File:** `go.mod` (declares `go 1.24.0`), `.github/copilot-instructions.md` (states `Go 1.25`), `CLAUDE.md` note (references 1.26 in runtime)

The declared Go version in `go.mod` (`go 1.24.0`) is inconsistent with the runtime in use. `go.mod` should declare the minimum version actually required to build the project.

---

## Architecture Overview

### A-1 — Dual database backend increases maintenance burden

The codebase maintains two complete database backend implementations (`PebbleStore` and `SQLiteStore`) that must be kept in sync via a 580+ method `Store` interface. The mock at 42,949 lines is a direct consequence. This is the fundamental driver of C-1/C-2 (mock drift), M-9 (pebble_store.go size), M-10 (sqlite_store.go size), and D-1 (CGO requirement).

If PebbleDB is the default and SQLite is opt-in, consider: does the project need both? If SQLite is not widely used in production, consolidating on PebbleDB (or a pure-Go SQLite) would cut the implementation surface by ~50%.

---

### A-2 — `Store` interface has 580+ methods — too wide

**File:** `internal/database/store.go`

The single `Store` interface covers books, authors, narrators, series, playlists, playback, operations, AI scans, tags, settings, embeddings, path history, activity, diagnostics, and more. This width is the root cause of the 42,949-line mock.

**Fix:** Split into domain-specific sub-interfaces (`BookStore`, `AuthorStore`, `OperationStore`, etc.) that compose into `Store`. Components can depend on narrow interfaces and mocks become small and focused.

---

### A-3 — No pagination on `GetAllImportPaths` and similar list endpoints

**File:** `internal/server/filesystem_handlers.go:92`

`GetAllImportPaths()` returns all records without pagination. For most uses (import path management) this is acceptable — the user is unlikely to have thousands of paths — but the pattern is replicated in several "get all" endpoints that could grow unboundedly.

---

## Appendix — Raw Metrics

| Metric | Value |
|--------|-------|
| `go vet` errors | Compile failures (9+ packages, mock drift) |
| `go build ./...` | ✅ Clean |
| Total Go source files | ~350 |
| Total Go lines | ~200,000+ |
| Largest Go file | `mock_store.go` — 42,949 lines |
| Largest production Go file | `pebble_store.go` — 8,031 lines |
| `context.Background()` (non-test) | 30+ instances |
| `_ =` error discards (non-test) | 30+ instances (pebble + sqlite + server) |
| `database.GetGlobalStore()` (non-test) | 103+ calls |
| `fmt.Printf` in library packages | 15+ instances |
| Goroutine spawns (non-test) | 137 |
| Mutex/lock instances | 57 |
| TODO/FIXME/HACK comments (Go) | 52 |
| Frontend source files | ~80 |
| `web/src/pages/Library.tsx` | 3,372 lines, 90 useState, 17 useEffect |
| `web/src/pages/Settings.tsx` | 4,166 lines, 61 useState |
| `console.log` calls (frontend) | 108 |
| Frontend inline `style=` attributes | 20+ |
| Vite coverage thresholds | 15% statements/lines, 10% branches |

### 2026-05-01 Re-Audit Metrics

| Metric | Value |
|--------|-------|
| `go build ./...` | ✅ Clean |
| `go vet ./...` | ✅ Clean |
| `staticcheck ./...` | ⚠️ 55 findings (unused code, deprecated fields, error strings) |
| Failing test packages | 2 (`internal/server`, `internal/database`) |
| Secret still committed | ✅ Not a finding — `config.yaml:10` `sk-test12345678` is an intentional example placeholder |
| `context.Background()` remaining (non-test) | ~20 instances (mostly database layer) |
| `fmt.Printf` / `log.Printf` remaining in library pkgs | ~12 instances |
| Deprecated `ITunesPath` usages | 13 (SA1019) |
| Unused code items | 7 (U1000/SA4006) |
| `GetAllBooks(0,0)` unbounded calls | 20+ |
| Frontend MUI bundle size | 443 KB (uncompressed) |
| Frontend test count | 21 test files |
| Vite coverage thresholds | Unchanged at 15%/10% |

---

*Original report: 2026-04-30. Re-audit: 2026-05-01. Each finding includes specific file paths and line numbers for reproducibility.*
