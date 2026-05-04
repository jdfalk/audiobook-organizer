<!-- file: docs/superpowers/bot-tasks/2026-05-04-tag-operation-logs.md -->
<!-- version: 1.0.0 -->
<!-- guid: a4c8d92e-5b71-4f6a-8c3d-e9f2a4b5c6d7 -->
<!-- last-edited: 2026-05-04 -->

# BOT TASK: Tag operation log lines with the originating operation ID

## Branch

```
feat/op-id-log-tagging
```

## Problem

Operations like `dedup-acoustid-scan` spawn deep call chains
(`acoustid_backfill.go`, ffmpeg subprocess wrappers, fingerprint
extractors). When these emit log lines via the standard `log.Printf`
or even via `internal/logger`, the lines look like this in journalctl:

```
acoustid_backfill.go:55: [WARN] fingerprint: <path>: ffmpeg chromaprint
                                <path>@0.00: [mp3 @ 0x...] Failed to find
                                two consecutive MPEG audio frames.
```

There is no operation ID attached. When the user opens the operation in
the Activity page and clicks "expand to see logs", the in-DB
`operation_logs` table has whatever the operation's `ProgressReporter`
explicitly logged via `progress.Log(...)`, but every fmt.Printf and
log.Printf scattered through the implementation is invisible to that
view. They only show up in journalctl, ungrouped.

The user's request: any log line emitted while inside an operation's
goroutine should be tagged with that operation's ID, AND those lines
should land in `operation_logs` so the Activity-page log view actually
reflects what's happening inside the scan.

## Goal

1. Pipe `op.ID` into a context-bound logger that all internal calls use.
2. Funnel that logger's output to both (a) journalctl with `op=<id>`
   prefix and (b) the `operation_logs` table associated with the op.
3. Replace direct `log.Printf` / `fmt.Printf` inside operation
   implementations with structured `slog`-style calls that carry the op
   context.

## Files (likely)

- `internal/logger/logger.go` — add `WithOperation(id)` returning a
  scoped logger that prepends `op=<id>` and writes to operation_logs.
- `internal/operations/queue.go` — when dispatching an op, install the
  logger as the request-scoped logger via context (e.g.
  `ctx = logger.WithContext(ctx, opLogger)`). The reporter already
  exposes the logger via `LoggerFromReporter(progress)`.
- `internal/dedup/engine.go`, `internal/server/acoustid_backfill.go`,
  `internal/fingerprint/*.go`, etc. — replace bare `log.Printf` with
  `logger.FromContext(ctx).Warn/Info/...`. This is a parallel-sweep
  candidate.
- DB write side: extend OperationLogger to write each line into
  `operation_logs` so the Activity page's existing log-fetch endpoint
  surfaces them.

## Out of scope

- Changing how journalctl receives logs.
- Backporting old log lines.
- Catching ffmpeg's own stdout/stderr (separate concern — those would
  need explicit wiring through cmd.Stdout/cmd.Stderr).

## Rough plan

1. Land logger context plumbing in a small PR with a couple of
   high-traffic call sites (acoustid_backfill, fingerprint extractor)
   converted as proof.
2. Use `/parallel-sweep` to convert the remaining `log.Printf` sites
   inside operation funcs.
3. Add a vitest/Go test that asserts a sample operation produces
   `operation_logs` rows containing the expected lines.

## Test strategy

- Backend test: run a tiny operation that emits 3 log lines; assert all
  3 appear in `operation_logs` with the correct `operation_id`.
- Manual: run AcoustID scan, click the row in Activity, confirm the
  inline log view shows ffmpeg warnings prefixed with the op ID.

## Rollback

Logger plumbing and call-site rewrites are independent — revert by
branch. The DB rows accumulated during the rollout are harmless to
keep.
