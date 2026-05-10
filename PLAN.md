<!-- file: PLAN.md -->
<!-- version: 3.0.0 -->
<!-- guid: 7d8e9f10-2345-4abc-9def-0123456789ab -->
<!-- last-edited: 2026-05-10 -->

# Plan: UOS Bell + Select-All + iTunes Dialog + Bulk Metadata Fix

## Goal

Fix four regressions after UOS-13/14 cleanup:
1. Bell shows no operations (SSE never opened, no initial load on mount)
2. Select-all across pages silently capped at 500
3. iTunes sync dialog lacks select-all
4. Library bulk metadata fetch not resumable

## Root Causes

1. `openSSE()` never called — entire SSE path is dead; page reload loses all op state
2. `BridgeQueue` writes `Plugin: "legacy"` — ops shown as wrong type; also no progress updates
3. `ParsePaginationParams` hard-cap at 500 — select-all endpoint returns at most 500 IDs
4. Library bulk fetch does `Promise.all(N × fetchBookMetadata)` in-browser — not an op

## Files To Change

### Backend
- `internal/operations/bridge.go` — fix Plugin/DefID fields; forward progress to v2 store
- `internal/server/server_lifecycle.go` — register `GET /api/v1/audiobooks/ids` + bulk_metadata_fetch def
- `internal/server/handlers_audiobooks.go` (or nearby) — new IDs-only handler
- `internal/server/metadata_batch_candidates.go` — add `bulk_metadata_fetch` op func registered via opRegistry
- `internal/operations/registry/` — register the new op def

### Frontend
- `web/src/App.tsx` — call `openSSE()` + `loadFromServer()` after auth confirmed; cleanup with `closeSSE()`
- `web/src/services/api.ts` — add `getAudiobookIds()` and `startBulkMetadataFetch()`
- `web/src/pages/Library.tsx` — use IDs endpoint for select-all; convert bulk fetch to v2 op
- `web/src/components/settings/ITunesImport.tsx` — add "Select All (N)" button with filtered IDs

## Ordered Steps

### Step 1 — Wire SSE + initial load in App.tsx
Add useEffect after auth check:
- `useOperationsStore.getState().openSSE()` — opens SSE connection
- `useOperationsStore.getState().loadFromServer()` — loads recent ops on mount
- Return `() => useOperationsStore.getState().closeSSE()` for cleanup

### Step 2 — Fix BridgeQueue plugin field + progress forwarding
- `Plugin: opType` (was "legacy")
- `DefID: "bridge." + opType` (was "legacy." + opType)
- Wrap ProgressReporter to call `b.v2Store.UpdateOpProgressV2(id, current, total, message)` on
  each progress update so the timeline shows real progress

### Step 3 — New `/api/v1/audiobooks/ids` endpoint
`GET /api/v1/audiobooks/ids` — accepts same filter/sort params as getBooks but returns only:
`{"data": {"ids": [...string], "total": N}}`
No pagination cap — IDs only so result set is cheap. Register in server_lifecycle.go.

### Step 4 — Library select-all uses IDs endpoint
`handleSelectAllItems` → calls `api.getAudiobookIds(filters)`, stores returned IDs in
`selectedAudiobookIds: string[]`. Existing `selectedAudiobooks` used only for display;
batch op calls use `selectedAudiobookIds`.

### Step 5 — iTunes dialog "Select All"
Add "Select All (N)" button that calls `getAudiobookIds({ hasItunesId: true })` and
merges all returned IDs into `browseSelected`.

### Step 6 — Bulk metadata fetch as v2 op
Register op def `bulk_metadata_fetch` with opRegistry (plugin "builtin"):
- Params: `{"book_ids": [...]}` 
- Resumable via `high_water_progress` (index of last processed book)
- Op func reuses existing per-book metadata fetch logic
Frontend: replace Library's `Promise.all` bulk fetch with `POST /api/v1/operations/v2`

## Test Strategy
- `go test ./internal/operations/...` and `go test ./internal/server/...`
- `make test`
- Manual verification: scan → bell shows "Library Scan"; reload → bell still shows running op

## Rollback
- Bridge change: non-breaking (type label improvement only)
- New endpoint: additive
- App.tsx change: openSSE guarded by null-check — calling twice is a no-op
- Frontend bulk fetch: old in-browser path removed; if v2 op fails, users can't fetch in bulk
