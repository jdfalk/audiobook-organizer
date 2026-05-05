<!-- file: docs/superpowers/bot-tasks/2026-05-05-uos-amendment-current-item.md -->
<!-- version: 1.0.0 -->
<!-- guid: e7f8a9b0-c1d2-3e4f-5a6b-7c8d9e0f1a2b -->
<!-- last-edited: 2026-05-05 -->

# UOS amendment — `Reporter.SetCurrentItem` for live current-item ticker

**Companion human spec:** `docs/superpowers/specs/2026-05-04-unified-operations-system.md` §1, §9.
**Depends on:** UOS-03 (Reporter), UOS-06 (SSE event hub + timeline endpoint) merged.

## Branch

```
feat/uos-current-item-ticker
```

## Goal

Add a Sonarr/Radarr-style "currently working on: *Foundation by Asimov*"
line to the bell icon and Activity-page op view. Updates fast (every
per-item tick of the op's loop) without paying DB cost per update, and
survives a page refresh or new browser tab.

## Contract addition

### `pkg/plugin/sdk/reporter.go` — add to `Reporter` interface:

```go
// SetCurrentItem sets the human-readable label for whatever the op
// is presently working on. Purely ephemeral: held in registry memory
// per-run, fanned out via SSE, included in the timeline endpoint
// snapshot for that run. NOT persisted to op_logs_v2 or
// operations_v2. Plugins can call this once per per-item iteration
// without measurable cost.
//
// Pass an empty string to clear the label (e.g. when the op moves
// from a per-item phase into a finalize phase).
SetCurrentItem(label string)
```

### Registry-side change (in `internal/operations/registry/`)

The per-run handle gains `currentItem string` (mutex-protected).
`SetCurrentItem` updates that field and emits an SSE event of type
`op.current_item` with payload `{ op_id, label }`.

The timeline endpoint (`GET /api/v1/operations/timeline`) and the
single-op endpoint (`GET /api/v1/operations/v2/:id`) both include
`current_item` in each running op's snapshot, sourced from registry
memory (NOT the DB).

## Frontend change (in `web/src/`)

### `useOperationsStore.ts`
- `OperationV2` type gains `current_item?: string`.
- SSE handler maps `op.current_item` events to `state.operations[id].current_item = payload.label`.

### `OperationsIndicator.tsx` (bell)
- Below the existing progress message line, render `current_item`
  in a smaller font with `noWrap + Tooltip(full)` so a long path
  doesn't blow out the dropdown width. Hidden when empty.

### `ActivityLog.tsx` (Activity page expanded view)
- Same treatment under the progress bar in the expanded card.

## Survival semantics (per spec discussion)

| Action | Visible after? |
|---|---|
| Page refresh | Yes — timeline endpoint returns it from registry memory |
| Logout / login | Yes — same path |
| New tab / window | Yes — same path |
| SSE reconnect | Yes — reconnect re-calls timeline |
| Server restart | No, briefly. Next per-item iteration repopulates within ms-to-seconds. Resumed ops re-call `SetCurrentItem` on their first per-item loop iteration. |

The brief gap on restart is acceptable: by definition this label is
about to change, so a momentary blank doesn't show stale data. If
restart-survival ever becomes important, the cheap retrofit is a
single column add to `operations_v2` flushed by the buffered DB
writer at 30s cadence — explicitly out of scope for this amendment.

## Test strategy

- Unit test (registry): `SetCurrentItem("Foo")` then `Get` returns
  `"Foo"`; sets emit one SSE event each.
- Reporter test: 1000 calls to `SetCurrentItem` produce 1000 SSE
  events and zero DB writes (verify by counting writes to
  `op_logs_v2` / `operations_v2`).
- Frontend test: `op.current_item` event patches the store entry's
  `current_item` field.
- Manual: register a fake op that loops over 100 items at 100ms
  each, calling `SetCurrentItem` per item; observe the bell ticker
  updating in real time; refresh; confirm the latest item appears
  immediately on reload.

## Acceptance criteria

- [ ] One new method on the `Reporter` interface.
- [ ] Zero new DB tables; zero new schema migrations.
- [ ] Timeline endpoint returns `current_item` for running ops.
- [ ] SSE emits `op.current_item` events.
- [ ] Bell + Activity render the line.
- [ ] All UOS unit/integration tests still pass.

## PR title

```
feat(uos): Reporter.SetCurrentItem live ticker
```
