<!-- file: docs/specs/pd-1-subprocess-isolation-implementation-plan.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8e2c4b7d-3a5f-4e1c-9d6a-2f8b1c5e7a09 -->
<!-- last-edited: 2026-05-29 -->

# PD-1 — Subprocess isolation RPC: implementation plan

**Source spec:** [`subprocess-isolation-rpc.md`](subprocess-isolation-rpc.md)
**Status:** Plan only. Implementation deferred per handoff (multi-day).
**Estimated effort:** 4–6 working days end-to-end, stage-gated.

## Constraint reminder

`Isolate: true` was reverted twice because PebbleDB is single-writer
and child processes can't `pebble.Open()`. The fix is to keep the DB
handle in the parent and proxy `database.Store` calls over a unix
socket. This plan stages that work safely.

## Stage 0 — Worktree + branch (15 min)

```bash
git worktree add ../abo-subproc-rpc -b feat/subprocess-isolation-rpc main
cd ../abo-subproc-rpc
```

Write `PLAN.md` at worktree root mirroring this file. **Do not edit
code until user approves the staged plan.**

## Stage 1 — RPC scaffolding, no Store yet (~1 day)

**Goal:** prove the wire format and re-exec flow with a dummy op.

Files created:
- `internal/operations/registry/rpc/server.go` — net/rpc server bound
  to unix socket; exposes `Ping(ctx) error` only.
- `internal/operations/registry/rpc/client.go` — `Dial(sock string)`,
  `Ping()` wrapper.
- `internal/operations/registry/childmode.go`:
  - `IsChildMode() bool` — checks `os.Getenv("UOS_SOCKET") != ""`.
  - `RunChildMode() int` — dials, pings, exits 0.
- `cmd/audiobook-organizer/main.go`: top-level guard
  ```go
  if registry.IsChildMode() { os.Exit(registry.RunChildMode()) }
  ```
  placed BEFORE any flag parsing or DB init.

Tests:
- `rpc_test.go` — start server in-process, dial, ping, assert.
- `subprocess_test.go` — `exec.Command(os.Args[0], ...)` with
  `UOS_SOCKET=$path` env; assert exit 0 + ping observed server-side.

Gate-out:
- `make ci` green
- `ABO_OP_ISOLATION` env var introduced (default `off`); no behaviour
  change yet.

## Stage 2 — StoreProxy + read-only methods (~1.5 days)

**Goal:** child can call `Store.GetBookByID`, `Store.GetBookFilesForIDs`,
`Store.ListBookIDs` over the socket, transparently.

Files:
- `internal/operations/registry/rpc/store_service.go` — server-side
  type wrapping the real `database.Store`; one RPC method per read
  signature.
- `internal/operations/registry/rpc/store_proxy.go` — client-side
  `StoreProxy` struct that satisfies the read subset of `Store`.

Decisions to lock first:
- **Codec:** start with `encoding/gob` (stdlib, no schema gen).
  Revisit protobuf if profile shows codec dominates.
- **Per-call latency budget:** 200µs p50, 1ms p99. Add benchmark in
  Stage 2 that fails CI if breached.
- **Type compat:** `Book`, `BookFile` already serialise to JSON in
  the API layer. Gob requires registered concrete types; add
  `gob.Register` calls in `rpc/init.go`.

Tests:
- Round-trip every read method against an in-memory fake Store.
- Benchmark `BenchmarkStoreProxy_GetBookByID` — fail if > 1ms p99.

Gate-out: bench passes, `make ci` green.

## Stage 3 — Write methods + batching (~1 day)

Add:
- `UpdateBook`, `UpdateBookFile`, `UpdateBooksBatch`,
  `UpdateBookFilesBatch`, `PutExternalIDMapping`, … (exactly the
  write methods the 7 isolated ops use, no more).

Batching:
- New Store interface methods `UpdateBooksBatch([]Book)` and
  `UpdateBookFilesBatch([]BookFile)`. Each batches into one Pebble
  `Batch` on the parent. This is the load-bearing call for
  fingerprint rescan (308K files); per-file RPC would be 80µs ×
  308K = 25s of pure overhead.
- Refactor `acoustid.fingerprint-rescan` Run to call
  `UpdateBookFilesBatch` every 500 files (not per file).

Tests:
- Write-method round-trip vs in-process Store: identical effect on
  fake.
- Batch correctness: 1000-element batch, verify all applied + order.

Gate-out: `make ci` green; manual run of `fingerprint-rescan` in dev
against a 1K-book fixture completes in < 2× in-process baseline.

## Stage 4 — Dispatcher integration (~0.5 day)

`internal/operations/registry/dispatcher.go`:

```go
func (d *Dispatcher) dispatch(opID string) {
    def := d.lookupDef(opID)
    if def.Isolate && os.Getenv("ABO_OP_ISOLATION") == "parent_rpc" {
        d.runIsolated(opID, def)  // NEW
        return
    }
    d.runInProcess(opID, def)
}
```

`runIsolated`:
1. Allocate `sock := filepath.Join(runtimeDir, "abo-op-"+opID+".sock")`
2. Bind `net.Listen("unix", sock)`; register `StoreService`.
3. `exec.Command(self)` with env `UOS_SOCKET=sock`,
   `UOS_OP_ID=opID`, `UOS_OP_DEF=<gob-encoded def metadata>`.
4. Wait for child handshake (RPC `Hello` with opID); timeout 5s →
   kill + fall back to in-process + log.
5. Monitor `cmd.Wait()` in a goroutine; on non-zero exit, mark op
   failed with `child_exit_code` + `child_rss_max`; do not auto-retry.

Tests:
- Integration: dispatch a real op def whose `Run` updates one book;
  assert (a) child exits 0, (b) parent DB reflects the update, (c)
  socket file cleaned up.
- Failure injection: child panics → op marked failed, no parent
  crash.

## Stage 5 — Re-enable Isolate per op (~1 day, staged in prod)

Per op, in this order (least → most invasive):

1. `maintenance.compact-activity` — small, low-throughput
2. `itunes.scan` — read-heavy, low write rate
3. `acoustid.scan` — read-heavy
4. `dedup.full-rescan` — read + occasional write
5. `metadata.bulk-fetch` — write-heavy, batched
6. `itunes.relink` — write-heavy
7. `acoustid.fingerprint-rescan` — highest throughput; only after
   batching benchmark green in prod

Each re-enable is its own PR. Prod soak ≥ 24h between PRs.

Rollback per op: PR sets `Isolate: false` again, redeploy. Flag
`ABO_OP_ISOLATION=off` kills isolation globally without redeploy.

## Stage 6 — Telemetry + cleanup (~0.5 day)

- Sample child RSS every 30s via `os/exec` rusage; emit slog field
  `child_max_rss_mb`.
- On parent shutdown, send `SIGTERM` to all live children, wait 5s,
  `SIGKILL`. Cleanup socket files.
- Doc: update `docs/operations/isolation.md` with the flag, rollback,
  and "when to set Isolate: true on a new op" checklist.

## Acceptance criteria (overall)

1. `Isolate: true` on all 7 ops, default `ABO_OP_ISOLATION=parent_rpc`
   in prod.
2. Fingerprint rescan over 308K files within 2× of pre-isolation
   wall time.
3. Forced child OOM in dev (`stress-ng --vm`) does not crash parent;
   op marked failed.
4. p99 round-trip latency < 1ms for proxy reads on warm socket.

## Risks + mitigations

| Risk | Mitigation |
|---|---|
| Gob version skew between binaries | Parent + child are the same binary (re-exec self). Skew impossible. |
| Socket file orphaned on parent crash | Cleanup at startup: scan `runtimeDir` for `abo-op-*.sock`, unlink. |
| Per-call latency dominates fingerprint | Stage 3 batching mandatory; bench gate in CI. |
| Context cancel doesn't propagate | Heartbeat ping every 5s; child cancels ctx after 2 missed. |
| Child writes to Pebble accidentally | StoreProxy is the ONLY Store impl visible to child; no `pebble.Open` import in child path (assert via build tag or codepath audit). |

## Out of scope (deferred)

- Cross-host distribution
- Plugin/user-supplied ops
- seccomp / namespace sandboxing
- Swapping Pebble for a multi-writer store
