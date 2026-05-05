<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-06-sse-timeline.md -->
<!-- version: 1.0.0 -->
<!-- guid: 46a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c -->
<!-- last-edited: 2026-05-04 -->

# UOS-06 — SSE event hub + timeline endpoint

**Companion human spec:** §9, §10.

## Branch

```
feat/uos-06-sse-timeline
```

## Goal

1. Implement `/api/v1/operations/timeline` (HTTP).
2. Implement `/api/v1/operations/events` (SSE).
3. Wire the registry's existing `bus` interface (defined in
   UOS-03/04) to a real EventHub.
4. Wire the introspection endpoints from spec §10.

## Files to add

1. `internal/operations/registry/bus.go`:
   - `EventHub` struct with subscriber set, `Publish` method, and
     SSE streaming method.
   - `Publish(ctx, eventName, payload any)`:
     - Resolves OperationDefs whose `Triggers` match (exact or
       wildcard). For each match, calls `EnqueueOp` with the
       payload as params + parent metadata per spec §6.3.
     - Fans out to SSE subscribers on the `op.<eventName>` channel.

2. `internal/server/operations_v2_handlers.go`:
   - `GET /api/v1/operations/timeline?since=15m`: SQL select from
     `operations_v2` ordered by `started_at DESC NULLS LAST,
     queued_at DESC` with logs tail. Returns the spec §9 shape.
   - `GET /api/v1/operations/events`: SSE handler that subscribes to
     EventHub and writes `data: <json>\n\n` per event. Handles
     reconnect via `Last-Event-ID`.
   - `GET /api/v1/operations/v2/:id`: full op + tail of logs.
   - `GET /api/v1/operations/v2/:id/logs?tail=N`: paginated logs.
   - `POST /api/v1/operations/v2`: trigger an op (params: `def_id`,
     `params`).
   - `DELETE /api/v1/operations/v2/:id`: cancel.
   - `GET /api/v1/op-defs`: list all OperationDefs.
   - `GET /api/v1/op-defs/:id`: single OperationDef.
   - All use `httputil.RespondWithSuccess` so the response is wrapped
     in `{ data: ... }`.

3. `internal/server/operations_v2_handlers_test.go`:
   - Each endpoint has at least one happy-path test.
   - Timeline test: insert 3 ops at different completion times, query
     `since=10m`, assert ordering and inclusion.
   - SSE test: connect, publish 5 events, assert all 5 received in
     order.

## Files to edit

1. `internal/server/server_lifecycle.go` — register the new routes
   under `/api/v1/operations/v2`, `/api/v1/operations/timeline`,
   `/api/v1/operations/events`, `/api/v1/op-defs`.

2. `web/src/services/api.ts` — uncomment the v2 endpoint wrapper
   added in UOS-05; remove the 404 fallback (endpoint exists now).
   Add SSE client.

3. `web/src/stores/useOperationsStore.ts`:
   - On mount, open SSE connection.
   - Patch store on each event:
     - `op.created` → add
     - `op.updated` → patch entry
     - `op.log` → append to entry's log slice (cap 500)
     - `op.error` → derived; UI uses for badges
     - `op.terminal` → set terminal state; entry remains 30 minutes
       wall-clock
   - On SSE drop: re-call timeline + reconnect.

## Hard rules

- The timeline endpoint MUST return ops ordered as spec §9 dictates
  (started_at DESC NULLS LAST, queued_at DESC).
- The SSE handler MUST send periodic heartbeats (every 30s) to defeat
  proxy idle timeouts.
- The bus `Publish` MUST honor parent inheritance per spec §6.3 for
  every triggered op.
- Trigger payload validation against the receiving OperationDef's
  `ParamsSchema` is the registry's job — bus calls EnqueueOp, and
  EnqueueOp validates.

## Acceptance criteria

- [ ] `go test ./internal/operations/registry/... ./internal/server/...`
      passes.
- [ ] `npx vitest run` passes.
- [ ] Manual: trigger an op via POST, see it in the bell within 1s
      (SSE), refresh, see it in Activity page (timeline endpoint),
      cancel, see status flip via SSE.
- [ ] Manual: kill the SSE connection (close the browser tab and
      reopen); store reconstructs from timeline endpoint; no data
      gap.

## PR title

```
feat(uos): SSE event hub + /operations/timeline + introspection endpoints
```
