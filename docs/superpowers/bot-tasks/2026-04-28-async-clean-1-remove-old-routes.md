<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-clean-1-remove-old-routes.md -->
<!-- version: 1.0.0 -->
<!-- guid: f3a4b5c6-d7e8-9012-fabc-456789012345 -->

# BOT TASK: Remove Old Synchronous Maintenance Routes

**TODO ID:** ASYNC-CLEAN-1  
**Audience:** burndown bot  
**⚠️ LAST — depends on ALL wave tasks being merged.**

## Prerequisites

ALL of the following must be merged before starting this task:

```bash
for label in task:ASYNC-CORE-2 task:ASYNC-W1-1 task:ASYNC-W1-2 task:ASYNC-W1-3 task:ASYNC-W1-4 \
             task:ASYNC-W2-1 task:ASYNC-W2-2 task:ASYNC-W2-3 task:ASYNC-W2-4 \
             task:ASYNC-W3-1 task:ASYNC-W3-2 task:ASYNC-W3-3 task:ASYNC-W3-4 task:ASYNC-W3-5; do
  count=$(gh pr list --label "$label" --state merged --json number | jq 'length')
  if [ "$count" -eq 0 ]; then
    echo "UNMET: $label"
    exit 0
  fi
done
echo "All prerequisites met — proceeding."
```

## Branch

```
feat/async-clean-1-remove-old-maintenance-routes
```

## Label

```bash
gh label create "task:ASYNC-CLEAN-1" --color "d93f0b" --description "Bot task: remove old synchronous maintenance routes" 2>/dev/null || true
```

## What This Does

Removes the 13 old synchronous handler methods from `maintenance_fixups.go` and
their route registrations from `server.go`. By this point every job is implemented
as a `MaintenanceJob` and the unified dispatcher handles all traffic.

Also removes any hardcoded maintenance buttons from the frontend that were
superseded by the dynamic "Manual Fixes" section (ASYNC-CORE-4).

## Routes to Remove from `server.go`

Find and delete these route registrations (exact lines will vary; search by path):

```
protected.POST("/maintenance/fix-read-by-narrator", ...)
protected.POST("/maintenance/cleanup-series", ...)
protected.POST("/maintenance/backfill-book-files", ...)
protected.POST("/maintenance/cleanup-empty-folders", ...)
protected.POST("/maintenance/cleanup-organize-mess", ...)
protected.POST("/maintenance/fix-author-narrator-swap", ...)
protected.POST("/maintenance/fix-version-groups", ...)
protected.POST("/maintenance/enrich-book-files", ...)
protected.POST("/maintenance/fix-book-file-paths", ...)
protected.POST("/maintenance/dedup-books", ...)
protected.POST("/maintenance/refetch-missing-authors", ...)
protected.POST("/maintenance/fix-library-states", ...)
protected.POST("/maintenance/recompute-itunes-paths", ...)
```

Keep the following routes — they are NOT converted (fast/bounded or dev-only):
```
protected.POST("/maintenance/cleanup-backups", ...)      // stays sync
protected.POST("/maintenance/generate-itl-tests", ...)   // dev-only, stays sync
protected.POST("/maintenance/scan-composer-tags", ...)   // already async via queue
```

## Handler Methods to Remove from `maintenance_fixups.go`

Delete these methods entirely (their logic now lives in `internal/maintenance/jobs/`):

- `fixReadByNarrator`
- `cleanupSeries`
- `backfillBookFiles`
- `cleanupEmptyFolders`
- `cleanupOrganizeMess`
- `fixAuthorNarratorSwap`
- `fixVersionGroups`
- `enrichBookFiles`
- `fixBookFilePaths`
- `dedupBooks`
- `refetchMissingAuthors`
- `fixLibraryStates`
- `recomputeItunesPaths`

## Frontend Cleanup

In `web/src/components/system/MaintenanceTab.tsx`, remove any hardcoded buttons
that called the old routes (e.g. fetch calls to `/api/v1/maintenance/fix-read-by-narrator`).
The dynamic "Manual Fixes" section (from ASYNC-CORE-4) already covers these.

Also remove any dead API functions from `web/src/services/api.ts` that called
the old maintenance endpoints directly.

## Step-by-Step

1. Search `server.go` for each old route path and delete the `protected.POST(...)` line
2. Search `maintenance_fixups.go` for each handler function and delete the entire method
3. Search `api.ts` for calls to the old paths and remove those functions
4. Search `MaintenanceTab.tsx` for any hardcoded calls to old paths and remove them
5. Run `go build ./...` — fix any compilation errors (unused imports, etc.)
6. Run `cd web && npx tsc --noEmit` — fix any TS errors
7. Run `make ci` — must pass

## Verify

```bash
make ci
# Confirm old routes no longer exist:
grep -r "fix-read-by-narrator\|cleanup-series\|backfill-book-files" internal/server/server.go && echo "FAIL: old routes still present" || echo "OK"
# Confirm new dispatcher still works:
curl -s http://localhost:8080/api/v1/maintenance/jobs | jq '.jobs | length'
# Should return 13 (one per converted job)
```

## Definition of Done

- All 13 old route registrations deleted from `server.go`
- All 13 old handler methods deleted from `maintenance_fixups.go`
- Dead API functions removed from `api.ts`
- Hardcoded old-path calls removed from `MaintenanceTab.tsx`
- `make ci` passes (all tests green, 80% coverage)
- `GET /api/v1/maintenance/jobs` still returns 13 jobs
- `POST /api/v1/maintenance/jobs/:id` still works for all 13 jobs

## PR Instructions

```bash
gh label create "task:ASYNC-CLEAN-1" --color "d93f0b" --description "Bot task: remove old synchronous maintenance routes" 2>/dev/null || true
gh pr create \
  --title "chore(maintenance): remove 13 old synchronous maintenance routes" \
  --body "All converted handlers now live as registered MaintenanceJobs. Removes the old per-route boilerplate from server.go and maintenance_fixups.go. Dead API client functions and hardcoded frontend buttons also removed. (ASYNC-CLEAN-1)" \
  --label "task:ASYNC-CLEAN-1"
```
