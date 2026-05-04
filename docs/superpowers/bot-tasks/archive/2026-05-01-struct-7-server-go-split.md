<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-7-server-go-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: b8c9d0e1-f2a3-4567-bcde-890123456789 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-7 — Split `server.go` (3401 lines → 6 files)

**Priority:** High  
**Effort:** Large (mechanical move — no logic changes)  
**Branch:** `refactor/struct-7-server-go-split`

---

## Why This Matters

`internal/server/server.go` is **3401 lines** mixing server construction, metadata
helpers, indexing logic, startup/lifecycle, route registration, and path utilities.
Splitting it makes the responsibilities clear and each section reviewable independently.

**Evidence:**
```bash
wc -l internal/server/server.go
# ~3401
```

---

## What This Task Does

Split `server.go` into 6 files by responsibility. **No logic changes** — only move
functions. Package name stays `package server`.

---

## What NOT to Do

- **Do NOT** change any function signatures or logic.
- **Do NOT** rename any functions or exported types.
- **Do NOT** split the `Server` struct itself — it stays in `server.go`.
- **Do NOT** touch test files.

---

## What Stays in `server.go`

These declarations **must remain** in `server.go` since they define the package's
core types and are referenced everywhere:

- All `type` declarations: `Server`, `ServerConfig`, `aiParser`,
  `enrichedBookResponse`, `authorEntry`, `narratorEntry`, `activityServiceLogger`,
  `serverScanHooks`, `serverOrganizeHooks`, `seriesPruneResult`,
  `seriesPrunePreviewGroup`, `seriesPrunePreviewResult`, `bulkFetchMetadataRequest`,
  `bulkFetchMetadataResult`
- All `var`/`const` at file scope: `cachedLibrarySize`, `cachedImportSize`,
  `cachedSizeComputedAt`, `cacheLock`, `appVersion`, `librarySizeCacheTTL`
- `NewServer` (constructor, ~line 888) — keep here
- `Store` method (~line 868)
- `publishEvent` (~line 879)
- `RecordActivity` (~line 770)

---

## Target File Layout

### File 1: `internal/server/server_helpers.go`
Small utility helpers used across the package. Functions to move:
- `SetVersion` (~74)
- `resetLibrarySizeCache` (~79)
- `stringPtr` (~88)
- `intPtrHelper` (~92)
- `boolPtr` (~96)
- `ptrStr` (~3409)
- `stringVal` (~274)
- `intVal` (~281)
- `nonEmpty` (~675)
- `calculateLibrarySizes` (~700)

### File 2: `internal/server/server_metadata.go`
Metadata state encoding/decoding and book enrichment helpers. Functions to move:
- `metadataStateKey` (~114)
- `decodeMetadataValue` (~118)
- `encodeMetadataValue` (~129)
- `loadLegacyMetadataState` (~141)
- `loadMetadataState` (~158)
- `saveMetadataState` (~194)
- `decodeRawValue` (~246)
- `updateFetchedMetadataState` (~257)
- `resolveAuthorAndSeriesNames` (~288)
- `batchFetchBookAuthorsAndNarrators` (~339)
- `enrichBookForResponseSingle` (~391)
- `enrichBookForResponse` (~400)
- `buildComparisonValuesFromMetadata` (~477)
- `buildComparisonValuesFromBook` (~511)
- `buildComparisonValuesFromActivityLog` (~553)
- `buildMetadataProvenance` (~594)
- `applyOrganizedFileMetadata` (~682)

### File 3: `internal/server/server_search.go`
Search index management. Functions to move:
- `SearchIndex` (~1337)
- `safeWriteDeps` (~1345)
- `buildSearchIndexIfEmpty` (~1361)
- `IndexBookByID` (~1424)
- `DeleteIndexedBook` (~1438)
- `OnBookScanned` (~1452)
- `OnImportDedup` (~1465)
- `OnCollision` (~1477)
- `fireDedupOnImport` (~1526)

### File 4: `internal/server/server_lifecycle.go`
Startup, route setup, and stale operation management. Functions to move:
- `resumeInterruptedOperations` (~1541)
- `Start` (~1721)
- `perm` (~2292)
- `itunesSvcGuard` (~2303)
- `setupRoutes` (~2314)
- `isStaleOperationStatus` (~3006)
- `operationStartedOrCreatedAt` (~3015)
- `collectStaleOperations` (~3022)
- `failStaleOperations` (~3049)
- `GetDefaultServerConfig` (~3399)

### File 5: `internal/server/server_middleware.go`
CORS, protected paths, session helpers. Functions to move:
- `corsMiddleware` (~2813)
- `filesCommonDir` (~2856)
- `isProtectedPath` (~2876)
- `loadDismissedDedupGroups` (~2922)
- `saveDismissedDedupGroups` (~2939)
- `triggerITunesSync` (~2955)

### File 6: `internal/server/server_title_helpers.go`
Title and series extraction helpers. Functions to move:
- `extractSeriesNameForDedup` (~3082)
- `computeSeriesPrunePreview` (~3133)
- `stripChapterFromTitle` (~3224)
- `stripSubtitle` (~3282)
- `extractTitleFromSegmentFilename` (~3315)
- `reassignExternalIDsForFiles` (~3347)

---

## Steps

### Step 1 — Baseline check

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./internal/server/...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok'
wc -l internal/server/server.go
```

### Step 2 — Identify all top-level declarations in server.go

```bash
grep -n '^type\|^var\|^const\|^func ' internal/server/server.go | head -60
```

Anything that's a `type`, `var`, or `const` that multiple other files will need —
leave in `server.go`. Functions in the lists above get moved.

### Step 3 — Create the 6 new files

For each file listed above:
1. Create it with the version header.
2. Add `package server` at top.
3. Copy the listed functions (not cut yet).
4. Add only the imports those functions need.
5. `go build ./internal/server/...` — fix errors.

Header template:
```go
// file: internal/server/server_XXX.go
// version: 1.0.0
// guid: <generate-a-new-uuid>
// last-edited: 2026-05-01

package server
```

### Step 4 — Remove functions from server.go

After all 6 files build cleanly, delete the moved function bodies from `server.go`.
Leave all `type`, `var`, `const` declarations, `NewServer`, `Store`, `publishEvent`,
and `RecordActivity`.

### Step 5 — Final build + test

```bash
go build ./internal/server/...
go build ./...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

### Step 6 — Commit and open PR

```bash
git checkout -b refactor/struct-7-server-go-split
git add internal/server/server*.go
git commit -m "refactor(server): split server.go into 6 responsibility files

Splits the 3401-line server.go into:
- server_helpers.go (util/pointer helpers)
- server_metadata.go (metadata state, book enrichment)
- server_search.go (search index management)
- server_lifecycle.go (startup, routes, stale ops)
- server_middleware.go (CORS, path guards, iTunes sync)
- server_title_helpers.go (title/series extraction)

Types, vars, consts, NewServer stay in server.go.
No logic changes. Structure audit STRUCT-7.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-7-server-go-split
gh pr create \
  --title "refactor(server): split server.go into 6 responsibility files" \
  --body "Splits 3401-line file into 6 focused files. Types/constructor remain in server.go. Structure audit STRUCT-7."
```

---

## Checklist

- [ ] 6 new files created with version headers
- [ ] `Server` struct, all types, vars, consts, `NewServer` remain in `server.go`
- [ ] `go build ./internal/server/...` clean
- [ ] `go build ./...` clean
- [ ] Tests pass
- [ ] PR opened on branch `refactor/struct-7-server-go-split`
