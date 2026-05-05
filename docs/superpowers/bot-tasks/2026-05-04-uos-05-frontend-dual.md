<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-05-frontend-dual.md -->
<!-- version: 1.0.0 -->
<!-- guid: 35f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b -->
<!-- last-edited: 2026-05-04 -->

# UOS-05 — Frontend dual-source store

**Companion human spec:** §9.

## Branch

```
feat/uos-05-frontend-dual
```

## Goal

Refactor `useOperationsStore` to read from BOTH the existing v1
(`/operations/active`, `/operations/recent`) and the new v2
(`/operations/timeline`) endpoints. During the migration, an op may
appear in either source. The store dedupes by id (v2 wins on
conflict). When v2 surfaces an op id that was previously seen in v1,
v1 row is replaced.

This PR does NOT yet wire SSE — that's UOS-06. Polling is sufficient
for this PR.

## Files to edit

1. `web/src/stores/useOperationsStore.ts` — refactor:
   - Single `operations` slice keyed by id.
   - `loadFromServer()` calls both endpoints in parallel (v1 may
     return empty if the timeline endpoint is missing — handle 404
     gracefully by ignoring v2 results until UOS-06 lands).
   - Polling cadence: 3s when bell open or Activity page mounted; 15s
     when neither. Reuse existing logic; do not introduce a new
     scheduler.
   - Existing `startPolling(opId, type)` API preserved for backward
     compat.

2. `web/src/services/api.ts` — add typed wrapper for new endpoint:
   ```ts
   export interface OperationV2 { ... }   // mirror operations_v2 columns
   export interface OperationTimelineResponse {
     operations: OperationV2[];
   }
   export async function getOperationTimeline(sinceMinutes = 15):
     Promise<OperationV2[]>;
   ```
   - Endpoint: `GET /api/v1/operations/timeline?since=<sinceMinutes>m`.
   - Unwrap `body.data.operations` per the lesson from PR #705.
   - Returns `[]` on 404 (timeline endpoint not yet deployed).

3. `web/src/components/layout/OperationsIndicator.tsx` — bell icon:
   - Read from the unified store; no other change.

4. `web/src/pages/ActivityLog.tsx`:
   - Read from the unified store.
   - Hierarchical render of trigger lineage: ops with the same
     `parent_id` indent under their parent. Implement as a flat list
     with a depth-prefix; no tree control yet.

5. Tests:
   - `web/src/stores/useOperationsStore.test.ts`:
     - Dedupes by id when same op appears in both sources.
     - v2 row wins on conflict.
     - `loadFromServer` survives v2 endpoint 404.
     - `loadFromServer` survives v1 endpoint 404.
   - `web/src/components/layout/OperationsIndicator.test.tsx`:
     - Existing tests still pass.

## Hard rules

- v2 endpoint may not exist yet (it ships in UOS-06). Code MUST handle
  404 / network error from v2 as "no v2 data available" and continue
  serving from v1.
- Do not delete or rename existing v1 wrappers in this PR.
- Do not introduce SSE yet; polling only.
- Frontend-side terminal-state retention is unchanged in this PR
  (existing 30s linger logic stays).

## Acceptance criteria

- [ ] `npx tsc --noEmit` clean.
- [ ] `npx vitest run` passes; all existing tests still pass.
- [ ] Manual: deploy with v2 endpoint stubbed to return 404, confirm
      bell + Activity page still render existing v1 data without
      console errors.

## PR title

```
feat(uos): frontend dual-source operations store
```
