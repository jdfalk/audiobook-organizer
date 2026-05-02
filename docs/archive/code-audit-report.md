<!-- file: docs/code-audit-report.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d2e3f4-a5b6-7c8d-9e0f-1a2b3c4d5e6f -->

# Go Codebase Audit Report — audiobook-organizer

**Date:** 2026-04-04  
**Scope:** `internal/server/`, `internal/database/`, `internal/scanner/`, `internal/itunes/`, `internal/organizer/`, `internal/tagger/`, `internal/playlist/`  
**Auditor:** Claude Sonnet 4.6 (automated static analysis)

---

## Executive Summary

The codebase is well-structured but shows clear signs of evolutionary growth: a legacy raw-SQL layer that predates the `Store` interface, several packages that are CLI-only stubs never invoked by the HTTP server, utility functions that have been re-implemented 3–6 times across packages, and a `server.go` file of 9,000+ lines that contains dozens of handler functions alongside business logic. The issues below are ordered by impact. Resolving P0 and P1 items would eliminate dead code paths that add confusion without value, while P2 items represent consolidation opportunities that reduce future maintenance burden.

---

## P0 — Dead Code: Functions Defined but Never Called

### 1. `(s *Server).startITunesSyncScheduler` — Dead function

- **File:** `internal/server/server.go:2262`
- **Issue:** This method launches a background iTunes sync goroutine. The unified `TaskScheduler` (`scheduler.go`) already manages iTunes sync via the `itunes_sync` task. `startITunesSyncScheduler` is **never called** anywhere — not in `NewServer()`, not in `Start()`, not in `scheduler.go`. The `triggerITunesSync` method it calls IS used, but only through the scheduler's `TriggerFn`.
- **Evidence:** `grep startITunesSyncScheduler` returns only the definition at line 2262.
- **Action:** Delete `startITunesSyncScheduler` (lines 2260–2286). `triggerITunesSync` (line 2289) is still called from the scheduler and must be kept.

### 2. `(aus *AudiobookUpdateService).ApplyUpdatesToBook` — Test-only method

- **File:** `internal/server/audiobook_update_service.go:85`
- **Issue:** This exported method applies a flat `map[string]any` payload to a `*database.Book` struct. It is called only from tests (`audiobook_update_service_test.go`, `service_layer_test.go`). The actual production update path goes through `UpdateAudiobook(id, payload)` which builds an `AudiobookUpdate` struct directly — `ApplyUpdatesToBook` is bypassed.
- **Evidence:** No call site in non-test `.go` files.
- **Action:** Delete the method. The internal field-extraction pattern it tests is already covered by `UpdateAudiobook` integration tests.

### 3. `(aus *AudiobookUpdateService).ValidateRequest` — Test-only method

- **File:** `internal/server/audiobook_update_service.go:28`
- **Issue:** Exported method that validates `id` non-empty and `payload` non-empty. Only called from tests. Production code in `UpdateAudiobook` does these checks inline at lines 117–120.
- **Action:** Delete the method; inline validation is sufficient.

### 4. `stringFromSeries` — Test-only helper

- **File:** `internal/server/server.go:589`
- **Issue:** Converts `*database.Series` to `any`. Only referenced in `server_extra_test.go:36,39`. No production call sites.
- **Action:** Move to the test file or delete.

### 5. `tagger.UpdateSeriesTags` + all sub-functions — Placeholder stubs

- **File:** `internal/tagger/tagger.go` (entire file, 118 lines)
- **Issue:** `UpdateSeriesTags()` queries `database.DB` (legacy raw SQL) and calls `updateFileTags()` which dispatches to `updateM4BTags`, `updateMP3Tags`, `updateFLACTags`. All three tag-writing functions contain only `fmt.Printf("Would update...")` and `return nil` — they are pure stubs that do nothing. The CLI `tag` command calls this, but since it's a no-op, it has no effect.
- **Evidence:** `internal/tagger/tagger.go:87-117` — all three functions just print and return nil.
- **Action:** Either implement them with actual tag-writing (the separate `tagger/embed_cover.go` shows the pattern) or deprecate the `tag` CLI command and delete `tagger.go`. The `tagger` package's `EmbedCoverArt` function (embed_cover.go) is real and must be kept.

### 6. `internal/database/audiobooks.go` — Legacy functions bypassed by server

- **File:** `internal/database/audiobooks.go` (full file)
- **Issue:** Contains `GetAudiobooks`, `GetAudiobookByID`, `UpdateAudiobook`, `DeleteAudiobook`, `GetOrCreateAuthor`, `GetOrCreateSeries` — all using `database.DB` (raw `*sql.DB`). The HTTP server **never calls any of these**; it uses `database.GlobalStore.GetBookByID()`, `database.GlobalStore.UpdateBook()`, etc. Only `audiobooks_test.go` and `coverage_test.go` call these functions.
- **Evidence:** `grep 'database\.UpdateAudiobook\|database\.GetAudiobooks\|database\.GetAudiobookByID'` across non-test files returns 0 results.
- **Action:** Deprecate this file. Long-term, merge the test coverage into `sqlite_store_test.go` / `pebble_store_test.go` and delete `audiobooks.go`. Short-term, add a `//go:build ignore` tag with a comment explaining it's legacy.

### 7. `internal/database/web.go` functions — Almost entirely bypassed

- **File:** `internal/database/web.go`
- **Issue:** Functions `CreateOperation`, `GetOperationByID`, `UpdateOperationStatus`, `UpdateOperationError`, `GetRecentOperations`, `AddOperationLog`, `GetOperationLogs`, `GetUserPreference`, `SetUserPreference`, `GetAllUserPreferences` all use raw `database.DB`. The server uses `database.GlobalStore.*` equivalents exclusively.
- **Exception:** `database.GetImportPaths()` is called once at `server.go:1101` but only when `database.DB != nil` (SQLite path), gating a transcode cleanup ticker that never fires in production (PebbleDB). Should be replaced with `database.GlobalStore.GetAllImportPaths()`.
- **Action:** Replace the one call site in `server.go:1101` to use `GlobalStore`, then delete or `//go:build ignore` `web.go`.

---

## P1 — Duplicate Utility Functions

### 8. Pointer-conversion helpers duplicated across packages

Six separate implementations of the same "wrap scalar in pointer" pattern:

| Function | File | Line | Used where |
|---|---|---|---|
| `stringPtr(s string) *string` | `internal/server/server.go` | 78 | server package widely |
| `stringPtr(s string) *string` | `internal/scanner/scanner.go` | 1705 | scanner package |
| `stringPtrValue(s string) *string` | `internal/scanner/scanner.go` | 1700 | scanner package |
| `intPtrHelper(i int) *int` | `internal/server/server.go` | 82 | server package |
| `intPtr(value int) *int` | `internal/server/itunes.go` | 1859 | itunes handlers |
| `intPtr(i int) *int` | `internal/scanner/scanner.go` | 1709 | scanner package |
| `boolPtr(b bool) *bool` | `internal/server/server.go` | 86 | server package |

**Action:** Create `internal/util/pointers.go` (or use Go 1.22+ `ptr.To[T]`). Consolidate to a single implementation. Update callers. The scanner and server packages importing a shared utility is a clean dependency direction.

### 9. String-deref helpers duplicated six times

All perform the same `if p == nil { return "" }; return *p` operation:

| Function | File | Line |
|---|---|---|
| `derefStr(s *string) string` | `internal/server/audiobook_service.go` | 135 |
| `derefString(p *string) string` | `internal/server/metadata_fetch_service.go` | 679 |
| `stringVal(p *string) any` | `internal/server/server.go` | 264 |
| `ptrStr(p *string) string` | `internal/server/server.go` | 8737 |
| `stringDeref(s *string) string` | `internal/server/maintenance_fixups.go` | 267 |
| `derefStrDisplay(s *string) string` | `internal/server/changelog_service.go` | 156 |

`stringVal` returns `any` while the rest return `string` — it's a slight variant for JSON map building. Still, five of the six are identical.

**Action:** Consolidate the five identical `string` return variants to a single `ptrStr` (or `derefStr`) in `server.go` or `internal/util`. Delete the rest. Keep `stringVal` in `server.go` since it serves a different purpose (returns `any` for provenance maps).

### 10. `ExtractStringField` / `ExtractIntField` / `ExtractBoolField` duplicated on two services

Both `AudiobookUpdateService` (`audiobook_update_service.go:39-67`) and `ConfigUpdateService` (`config_update_service.go:33-69`) define identical `ExtractStringField`, `ExtractBoolField`, `ExtractIntField` methods. The implementations are byte-for-byte identical.

**Action:** Extract these three methods to a standalone `payloadExtractor` type or package-level functions in `internal/server/payload_helpers.go`. Both services embed or call the helper.

### 11. `sanitizeFilename`/`sanitizePath` (organizer) vs `sanitizePathComponent` (server/path_format)

- `internal/organizer/organizer.go:320` — `sanitizeFilename` strips control chars, `..`, invalid chars, limits length to 200
- `internal/server/path_format.go:144` — `sanitizePathComponent` replaces a different set of chars, no length limit

These serve the same purpose (make a path component filesystem-safe) but have different behavior — specifically `sanitizeFilename` prevents path traversal by replacing `..` with `_` and limits filename length, while `sanitizePathComponent` is more minimal.

**Action:** Audit whether both are needed for their respective callers, then decide on a canonical version. The organizer's version is more defensive. Consider making `organizer.sanitizeFilename` exported and reusing it in `path_format.go`, or having `path_format.go` import `organizer` (if that doesn't create circular deps).

---

## P2 — Multiple Paths for the Same Operation

### 12. Two "merge series" implementations with different behavior

- `mergeSeriesGroup(store, keepID, mergeIDs)` at `maintenance_fixups.go:545` — synchronous, no operation record, no change log, just moves books and deletes series
- `(s *Server).mergeSeriesGroup(c *gin.Context)` at `server.go:4456` — async operation via queue, records `OperationChange` entries, supports custom rename, invalidates caches

The maintenance handler (`handleCleanupSeries`) calls the standalone function. The API endpoint (`POST /series/merge`) calls the async version. Both are called from different contexts so they legitimately need different behavior, BUT the core "move all books from series A to series B" logic is duplicated. 

**Action:** Extract the core book-reassignment loop into a shared `reassignBooksToSeries(store, fromID, toID int) error` function. Both callers use it. The async handler wraps it with progress reporting and change logs; the maintenance handler calls it directly.

### 13. `searchAudiobooks` is a subset of `listAudiobooks`

- `GET /audiobooks?search=...` → `listAudiobooks` — supports `search`, `author_id`, `series_id`, filters, sort, pagination
- `GET /audiobooks/search?q=...` → `searchAudiobooks` — only supports `q` (basic search), same pagination

`searchAudiobooks` (server.go:1843) calls `audiobookService.GetAudiobooks` with a `q` string. `listAudiobooks` calls the same with a `search` string. Both return the same enriched response shape.

**Action:** Delete `searchAudiobooks`. Update any frontend callers from `/audiobooks/search?q=` to `/audiobooks?search=`. The route is registered at line 1407 and can be removed from `setupRoutes`.

### 14. `/work` (inline) duplicates `/works` (service-backed)

- `GET /works` → `listWorks` at server.go:3704 — delegates to `workService.ListWorks()`, returns `{items, count}` without books
- `GET /work` → `listWork` at server.go:8574 — inline, fetches `GetAllWorks()` then for each work fetches `GetBooksByWorkID()`, returns `{items, total}` with books embedded

These serve different purposes (one is a plain work list, one is a work-with-books view) but the code comment says `/work` is "alternative singular form for compatibility" — meaning it exists for backward compatibility, not as a genuinely different endpoint.

**Action:** Document which frontend route uses `/work` vs `/works`. If the embedded-books behavior is genuinely needed, add `?include_books=true` to `/works` and delete `/work`. Otherwise just remove it.

### 15. Dual activity log endpoints

- `GET /system/activity-log` → `getSystemActivityLog` at server.go:2404 — calls `GlobalStore.GetSystemActivityLogs()`, SQLite-only implementation
- `GET /activity` → `listActivity` at activity_handlers.go:34 — calls `ActivityService`, uses the separate `activity.db` SQLite store, supports full filtering

The newer `ActivityService` is the correct layer for all activity queries. The `/system/activity-log` endpoint predates it and uses the old schema (`GetSystemActivityLogs`).

**Action:** Deprecate `/system/activity-log`. The route is registered at server.go:1588. Remove it after confirming no frontend component uses it (the dashboard and changelog use `/activity`).

---

## P3 — Inconsistent Use of Legacy vs Current Patterns

### 16. `internal/playlist/playlist.go` uses `database.DB` directly

- **File:** `internal/playlist/playlist.go:38,104,177,...`
- **Issue:** `GeneratePlaylistsForSeries()` queries `database.DB` (legacy raw SQL). This is the function called by the CLI `playlist` command. With PebbleDB, `database.DB` is nil and this function will silently fail.
- **Action:** Either rewrite `GeneratePlaylistsForSeries` to use `database.GlobalStore`, or deprecate the `playlist` CLI command since playlists are not part of the current HTTP API feature set. The current HTTP API has no playlist endpoints.

### 17. `internal/tagger/tagger.go:19` uses `database.DB`

- **File:** `internal/tagger/tagger.go:19`
- **Issue:** `UpdateSeriesTags` queries `database.DB`. Same problem as #16 above — will silently fail with PebbleDB. Also covered by #5 since this function is all-stub anyway.
- **Action:** Covered by fix for item #5.

### 18. Transcode cleanup ticker gated on `database.DB != nil`

- **File:** `internal/server/server.go:1100-1107`
- **Issue:** `if database.DB != nil` guards a call to `database.GetImportPaths()` (from `web.go`) and starts transcode cleanup tickers. With PebbleDB in production, `database.DB` is nil, so transcode temp files are **never cleaned up**.
- **Action:** Replace `database.DB != nil` guard and `database.GetImportPaths()` with `database.GlobalStore.GetAllImportPaths()`. Remove the `database.DB` check entirely — the store is always initialized before `Start()` is called.

### 19. `intPtr` defined in both `scanner.go` and `server/itunes.go`

- `internal/scanner/scanner.go:1709` — `func intPtr(i int) *int`
- `internal/server/itunes.go:1859` — `func intPtr(value int) *int`

The scanner's `intPtr` is only used within `scanner.go`. The itunes file's `intPtr` is used within `itunes.go`. Both should be replaced by the shared helper from item #8.

---

## P4 — Minor / Style Issues

### 20. `error_handler.go` helpers almost entirely unused

- **File:** `internal/server/error_handler.go`
- Functions `RespondWithBadRequest`, `RespondWithNotFound`, `RespondWithInternalError`, `RespondWithConflict`, `RespondWithForbidden`, `RespondWithUnauthorized`, `RespondWithSuccess`, `RespondWithList`, `RespondWithCreated`, `RespondWithOK`, `RespondWithNoContent`, `HandleBindError`, `EnsureNotNil`, `logErrorWithContext` (14 functions)
- **Usage:** Only `RespondWithOK` is called once (server.go:6761). All others are used only in `error_handler_test.go`.
- **Issue:** The server uses `internalError(c, msg, err)` and `c.JSON(http.StatusXXX, ...)` directly throughout. These helpers were added as a framework layer but never adopted.
- **Action:** Either (a) migrate all handlers to use these consistently (improves uniformity), or (b) delete them and keep only `internalError`. Option (a) is the better long-term choice but requires touching many files.

### 21. `GetPlaylistBySeriesID(0)` in `healthCheck` is a hack

- **File:** `internal/server/server.go:1764`
- **Code:** `if playlists, err := database.GlobalStore.GetPlaylistBySeriesID(0); err == nil && playlists != nil {`
- **Comment:** `// legacy placeholder (0 unlikely valid series)`
- **Issue:** This is nonsense code in the health check — it calls a playlist query with series ID 0 to get a `playlistCount` that is always `1` if any playlist exists with seriesID=0 (unlikely). The comment admits it's wrong.
- **Action:** Remove the `playlistCount` field from the health check response entirely, or query `GetPlaylistCount()` if that's an interface method, or remove the playlist count from `/health` and `/api/v1/health`.

### 22. `server.go` is 9,000+ lines — needs decomposition

- **File:** `internal/server/server.go`
- **Issue:** The file contains the Server struct definition, route setup, 100+ handler methods, and many utility functions. A file of this size makes navigation, code review, and merge conflicts extremely difficult.
- **Note:** The service layer migration (phase 3) already extracted logic into `*_service.go` files. The remaining handlers in `server.go` should be extracted into domain-specific handler files (e.g., `book_handlers.go`, `author_handlers.go`, `maintenance_handlers.go`).
- **Action:** This is a refactoring task, not a bug. Suggested decomposition:
  - `book_handlers.go` — listAudiobooks, getAudiobook, updateAudiobook, deleteAudiobook, restoreAudiobook, etc.
  - `author_handlers.go` — listAuthors, mergeAuthors, splitCompositeAuthor, etc.
  - `series_handlers.go` — listSeries, mergeSeriesGroup, deduplicateSeriesHandler, etc.
  - `operation_handlers.go` — startScan, startOrganize, listOperations, etc.
  - `maintenance_handlers.go` — handleWipe, handleFixReadByNarrator, etc.

---

## Summary Table

| # | File(s) | Issue | Priority | Action |
|---|---|---|---|---|
| 1 | server.go:2262 | `startITunesSyncScheduler` never called | P0 | Delete |
| 2 | audiobook_update_service.go:85 | `ApplyUpdatesToBook` test-only | P0 | Delete |
| 3 | audiobook_update_service.go:28 | `ValidateRequest` test-only | P0 | Delete |
| 4 | server.go:589 | `stringFromSeries` test-only | P0 | Delete or move to test |
| 5 | tagger/tagger.go | All tag-write functions are stubs | P0 | Implement or delete |
| 6 | database/audiobooks.go | Legacy functions bypassed by server | P0 | Deprecate/delete |
| 7 | database/web.go | Most functions bypassed; one call needs fix | P0 | Fix call site, then deprecate |
| 8 | server.go, scanner.go, itunes.go | `stringPtr`/`intPtr`/`boolPtr` 6+ duplicates | P1 | Consolidate to util package |
| 9 | Multiple server files | `derefStr` and variants duplicated 6 times | P1 | Consolidate to one |
| 10 | audiobook_update_service.go, config_update_service.go | `ExtractXxxField` methods duplicated | P1 | Extract to shared helpers |
| 11 | organizer.go, server/path_format.go | Two sanitize-path implementations | P1 | Audit and consolidate |
| 12 | maintenance_fixups.go:545, server.go:4456 | Series merge logic duplicated | P2 | Extract shared core |
| 13 | server.go:1843 | `searchAudiobooks` duplicates `listAudiobooks` | P2 | Delete endpoint |
| 14 | server.go:8574 | `/work` duplicates `/works` | P2 | Deprecate one |
| 15 | server.go:2404 | `/system/activity-log` duplicates `/activity` | P2 | Deprecate legacy endpoint |
| 16 | playlist/playlist.go | Uses `database.DB` directly (PebbleDB incompatible) | P3 | Rewrite or deprecate |
| 17 | tagger/tagger.go | Uses `database.DB` directly | P3 | Covered by #5 |
| 18 | server.go:1100 | Transcode cleanup never fires with PebbleDB | P3 | Fix guard condition |
| 19 | scanner.go, itunes.go | `intPtr` duplicated | P3 | Consolidate |
| 20 | error_handler.go | 14 `RespondWith*` helpers unused in production | P4 | Adopt or delete |
| 21 | server.go:1764 | `GetPlaylistBySeriesID(0)` in health check | P4 | Remove hack |
| 22 | server.go | 9,000+ line file | P4 | Decompose into handler files |

---

## Recommended Action Plan

**Sprint 1 (Low risk, high value — ~4 hours):**
- Delete `startITunesSyncScheduler` (#1)
- Delete `ApplyUpdatesToBook`, `ValidateRequest` on AudiobookUpdateService (#2, #3)
- Delete `stringFromSeries` or move to test file (#4)
- Remove `searchAudiobooks` endpoint and route (#13)
- Fix transcode cleanup ticker guard (#18)
- Remove `GetPlaylistBySeriesID(0)` hack from health check (#21)

**Sprint 2 (Consolidation — ~1 day):**
- Create `internal/util/pointers.go` with `StringPtr`, `IntPtr`, `BoolPtr` (#8)
- Consolidate `derefStr` variants to `ptrStr` in server package (#9)
- Extract `ExtractStringField/IntField/BoolField` to shared helpers (#10)
- Add `//go:build ignore` and deprecation notice to `database/audiobooks.go` and `database/web.go` (#6, #7)
- Fix `server.go:1101` to use `GlobalStore.GetAllImportPaths()` (#18)

**Sprint 3 (Structural refactoring — ~2 days):**
- Extract shared `reassignBooksToSeries` from both merge-series implementations (#12)
- Decide on `/work` vs `/works` and remove the redundant one (#14)
- Deprecate `/system/activity-log` endpoint (#15)
- Deprecate or implement the `playlist` CLI command (#16)
- Decide on tagger stub fate (#5)
- Begin decomposing `server.go` into handler files (#22)

---

*This report was generated by automated static analysis of commit `4e6bf61`. Line numbers are approximate due to file mutations between analysis steps.*
