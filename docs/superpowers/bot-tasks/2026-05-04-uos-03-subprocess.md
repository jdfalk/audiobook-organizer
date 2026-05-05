<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-03-subprocess.md -->
<!-- version: 1.0.0 -->
<!-- guid: 13d4e5f6-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->
<!-- last-edited: 2026-05-04 -->

# UOS-03 — Subprocess runner + Reporter contract

**Companion human spec:** §3 (subprocess worker), §4 (Reporter), §15.

## Branch

```
feat/uos-03-subprocess
```

## Goal

1. Replace UOS-02's stub Reporter with the real implementation: writes
   to `op_logs_v2`, `op_errors_v2`, `op_state_v2`, updates
   `operations_v2`, emits SSE events (SSE channel produced here, hub
   consumed in UOS-06).
2. Add subprocess runner used by `OperationDef.Isolate = true` ops.

## Files to add

1. `internal/operations/registry/reporter_db.go` — full Reporter
   implementation. EXACT methods from spec §4. Behavior:
   - `UpdateProgress`: updates `operations_v2` columns
     `progress_current`, `progress_total`, `progress_message`,
     `last_progress_at`. Emits `op.updated` SSE event.
   - `Log`: enqueues to a buffered DB writer (flush every 250ms or
     at 100 lines). Writes to `op_logs_v2`. If `level >=
     slog.LevelError`, ALSO writes to `op_errors_v2` (single write
     path; promotion happens here, not at callsite). Emits `op.log`
     SSE event.
   - `Logger()`: returns `*slog.Logger` with default attrs `op_id`,
     `def_id`, `plugin`, `trace_id`, `span_id`. Handler routes to
     (a) journalctl via existing log handler, (b) `Log` above,
     (c) SSE.
   - `Checkpoint(state any)`: gob-encodes state, upserts into
     `op_state_v2`. Updates `operations_v2.last_checkpoint_at`.
     Updates `high_water_progress` if current progress exceeds it.
   - `IsCanceled()`: reads `runCtx.Err() != nil`.
   - `RunPhase(ctx, name, fn)`: updates `operations_v2.current_phase
     = name`. Calls `fn(phaseReporter)`. Phase reporter is a thin
     wrapper that prefixes phase name into log attrs.
   - `Trigger(ctx, eventName, payload)`: publishes via
     `r.bus.Publish(ctx, eventName, payload)` with parent metadata
     pre-filled (uses spec §6.3 inheritance matrix). Bus interface
     is defined here; backing implementation in UOS-06.

2. `internal/operations/registry/subprocess.go` — subprocess runner.
   - On registry init, parse argv: if `os.Args[1] == "--operation-runner"`,
     enter child mode and execute the named def.
   - Parent path: when worker dispatches an `Isolate: true` op:
     1. Open a unix socket pair.
     2. Re-exec self with `--operation-runner <op_id>`.
     3. Send def_id + params over the socket as JSON.
     4. Capture child's stdout/stderr; route lines to Reporter as
        `info` (stdout) and `warn` (stderr).
     5. Receive Reporter calls from child over the socket and
        forward to the parent's Reporter for that op_id.
     6. Wait for child exit.
     7. On Cancel: SIGTERM, 30s grace, SIGKILL.

3. `internal/operations/registry/reporter_buffer.go` — buffered DB
   writer for log lines. Goroutine flushing on tick or threshold.
   Tests for backpressure and drop-on-shutdown behavior.

4. `internal/operations/registry/runner_main.go` — entrypoint for
   child-mode execution. Consumed by `cmd/root.go` change in this PR
   to dispatch `--operation-runner` early.

5. Tests:
   - Reporter unit tests: progress writes, log writes, error
     promotion, gob round-trip for Checkpoint, phase tracking.
   - Subprocess test: a fake op that prints to stdout and stderr;
     parent sees both as `info`/`warn` log lines.
   - Cancel-via-SIGTERM test: a fake op that ignores the socket
     signal; parent SIGKILLs after 30s; total cancel time bounded.
   - Buffer test: 1000 log lines flushed in ≤2 ticks; shutdown
     drains buffer; errors during flush retry once then drop with
     log.

## Files to edit

1. `cmd/root.go` — at the very top of `Execute()`, check argv for
   `--operation-runner` and dispatch to `runner_main.RunChild()`
   before any other initialization.
2. `internal/operations/registry/worker.go` — replace
   `ErrSubprocessNotImplemented` with the new subprocess runner.
3. `internal/operations/registry/reporter.go` — delete `stubReporter`
   in favor of the real one from `reporter_db.go`.

## Hard rules

- Subprocess child MUST initialize the same plugin set as the parent
  (so it can resolve `def.Run`). It does this by going through the
  normal `internal/plugins/plugins.go` import chain.
- Stdout/stderr capture in parent MUST tag each line with op_id; this
  is the auto-tagging promise from spec §9.
- Reporter calls from child are RPC'd over the socket using a tiny
  framed JSON protocol; do not pull in gRPC.
- DB writer is buffered to prevent hot-path latency from SQLite
  fsync. Loss on hard crash is acceptable; in-process panic should
  flush via `defer`.

## Acceptance criteria

- [ ] `go test ./internal/operations/registry/...` passes.
- [ ] Subprocess test: spawn 100 trivial subprocess ops, all succeed.
- [ ] Cancel test: subprocess ignoring SIGTERM is killed within
      30s+grace.
- [ ] Log test: subprocess emitting 10K lines via stdout has all
      10K visible in `op_logs_v2`, tagged with op_id.
- [ ] Reporter test: a Run that calls UpdateProgress 1K times produces
      exactly 1K `op.updated` SSE events on the bus.

## PR title

```
feat(uos): subprocess runner + Reporter implementation
```
