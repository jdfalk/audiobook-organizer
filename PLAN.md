# Plan: Review Metadata Matches — fetch-all + client-side filter/sort

## Problem
Endpoint paginates server-side over all 5851 cached results in insertion order;
filters (Hide Applied/Rejected/No Match, Min Confidence, Match Language) run
client-side per page. Matched rows are scattered across 24 pages and never
auto-float to page 1.

## Goal
Server returns the full cached set in one call; client owns sort/filter/page.

## Changes
1. `internal/server/metadata_cached_handlers.go` — `getCacheReviewResults`:
   - `limit=0` (or omitted) returns all rows.
   - Sort by status (matched first, then no_match, then applied), stable
     within group.
2. `web/src/services/api.ts` — allow `limit=0`.
3. `web/src/components/audiobooks/MetadataReviewDialog.tsx`:
   - Fetch once on open with limit=0.
   - Compute `totalPages` from filtered length.
   - Paginate `filteredResults` client-side via slice.
   - Update chip labels to drop "(page)" suffix where counts are global.
   - Drop the auto-advance-when-page-empty effect.

## Test
- `make build` (Go + TS).
- Manual: open Review dialog → matched rows fill page 1; toggling hide filters
  no longer refetches.

## Rollback
Single PR.
