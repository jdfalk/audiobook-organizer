<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-13-frontend-single.md -->
<!-- version: 1.0.0 -->
<!-- guid: b3b4c5d6-e7f8-9a0b-1c2d-3e4f5a6b7c8d -->
<!-- last-edited: 2026-05-04 -->

# UOS-13 — Frontend single-source (drop dual-source)

**Companion human spec:** §9.

## Branch

```
feat/uos-13-frontend-single
```

## Goal

Remove the dual-source merging logic from `useOperationsStore`. After
PR 12, every op flows through the v2 path. The store now reads only
from v2 (`/operations/timeline` + SSE). The v1 endpoints stay live
but are never called by the UI.

## Files to edit

1. `web/src/stores/useOperationsStore.ts`:
   - Delete v1 polling.
   - Delete dedupe-by-id logic.
   - Keep SSE subscription, timeline initial-load, terminal-state
     30-min retention.
2. `web/src/services/api.ts`:
   - Mark v1 wrappers `getActiveOperations`, `getRecentCompletedOperations`
     deprecated; UI no longer calls them.
3. `web/src/pages/ActivityLog.tsx`:
   - Remove residual references to v1-shaped fields.
   - Trigger lineage display: now that every op has a `parent_id`,
     render the tree with full indentation/collapse controls. Use a
     simple recursive render — no third-party tree component.
4. `web/src/components/layout/OperationsIndicator.tsx`:
   - Remove residual v1-shape handling.

## Hard rules

- Do NOT delete v1 endpoints in this PR (UOS-14 does that).
- Do NOT delete the v1 wrappers in `api.ts` yet — they may be used
  by other code paths that we discover during cleanup.
- Tree rendering for trigger lineage: indent by depth; collapse
  default-closed for ops with ≥3 children; expand-all/collapse-all
  buttons in the panel header.

## Acceptance criteria

- [ ] `npx tsc --noEmit` clean.
- [ ] `npx vitest run` passes.
- [ ] Manual: trigger an `itunes.import`, observe child
      `acoustid.backfill` ops cascade in the timeline tree.
- [ ] Manual: refresh the Activity page during an active scan,
      confirm tree is reconstructed correctly with parent-child
      relationships.

## PR title

```
feat(uos): drop frontend dual-source; v2 is sole source
```
