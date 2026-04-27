<!-- file: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17d-audiobook-handlers.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8a3b5d92-7e4a-4f1c-a8be-bda02c394675 -->

# BOT TASK: 4.17d — Audit audiobook handlers for service-delegation gaps

**TODO ID:** 4.17d
**Companion human design:** [`docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md`](../specs/2026-04-27-fetch-path-audit-design.md)
**Pattern reference:** identical to [`4.17b`](2026-04-27-fetch-path-4-17b-metadata-handlers.md) — read it first.

This is an **audit-only, docs-only PR**.

## Branch

```
docs/4-17d-audiobook-handlers-audit
```

## Files

Audit targets:
- `internal/server/audiobooks_handlers.go`
- `internal/server/audiobook_service.go` (server-package wrapper)
- `internal/server/entities_handlers.go` (related)
- Any other `audiobook*.go` in the server package

Service to delegate to:
- `internal/audiobookservice/` (if it exists) OR `audiobook_service.go` itself if no extracted package yet
- `internal/metafetch/service.go` for fetch paths
- `internal/dedup/engine.go` for dedup paths

Append to (must already exist from 4.17b/c):
- `docs/audits/2026-04-27-handler-service-delegation.md`

## Common divergences to flag in this file set

- Inline metadata fetch (should delegate to `metafetch.Service` per 4.17a/b)
- Inline external-ID lookups when `database.Store.GetExternalIDMapping` exists
- Manual book-state recomputation when `readstatus.Engine` (or equivalent) has it
- Direct PebbleDB / SQLite calls where a Store interface method exists
- Per-user filter logic duplicated outside `audiobook_service.go` (this was the bug fixed on 2026-04-24 — flag any new instances)

## Step-by-step

Same shape as 4.17b. List → verdict → append section → file follow-ups.

This file set is the largest. Expect 15–25 handler rows. Pace the audit:

- If a handler is purely CRUD on the database (e.g. `getAudiobookByID`), verdict = `no-fetch` and move on.
- If a handler does any external lookup, scoring, or apply-pipeline work, scrutinize.

## Definition of done

- [ ] Audit file gains a populated `## audiobook handlers (4.17d)` section
- [ ] All handlers in the file set represented
- [ ] Divergences → TODO entries (likely several — this is the largest surface)
- [ ] PR title: `docs(audit): audiobook handlers delegation audit (4.17d)`
- [ ] CHANGELOG prepended
- [ ] TODO.md `4.17` parent task gets a status update line: `Audit complete; <N> divergences filed as 4.17e..4.17?`

## When to STOP

NEEDS_REVIEW if:

- More than 8 divergences found. The bot should not file a giant batch of follow-ups without human review of priorities.
- A handler's purpose isn't clear from name + body. Surface the ambiguity rather than guessing the verdict.
