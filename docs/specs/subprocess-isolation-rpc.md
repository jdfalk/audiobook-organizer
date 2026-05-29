<!-- file: docs/specs/subprocess-isolation-rpc.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6a1e2c1d-7b9f-4a3c-9b14-3d5b7e0f1a44 -->
<!-- last-edited: 2026-05-29 -->

# Subprocess Operation Isolation — Parent-Mediated Store RPC

**Status:** Draft (post-incident, supersedes MAYDEPLOY-A)
**Author:** Claude (handoff from 2026-05-29 perf sprint)
**Owner:** TBD
**Related:** PR #1154 (registry double-dispatch fix), PR #1155 (Isolate disabled),
PR #1172 (child-mode wire-up attempt — reverted in #1181), PR #1181
(`Isolate: false` restored across 7 ops)

## Problem

`sdk.OperationDef.Isolate = true` was added so heavy/risky operations
(`acoustid.scan`, `metadata.bulk-fetch`, `itunes.scan`, …) could run in
a forked subprocess that gets killed if it OOMs or panics, without
taking the parent server down. The current child-mode handler
re-execs `audiobook-organizer --operation-runner <opID>` and tries to
open PebbleDB directly.

**This cannot work as designed.** PebbleDB enforces single-writer
semantics via an exclusive on-disk lock. The child process attempting
`pebble.Open(dir)` immediately fails with
`resource temporarily unavailable`. We deployed the wire-up twice
(#1172, #1181) and rolled back twice. `Isolate: true` is now disabled
across all 7 affected ops; `acoustid.scan` and friends run in-process.

That is the wrong long-term state: a single OOM/panic in a long
op (e.g. fingerprint rescan over 308K files) takes down the server.

## Goals

1. Re-enable per-op subprocess isolation for ops marked `Isolate: true`
2. Survive parent restarts gracefully (already covered by ResumePolicy)
3. Survive child crash/OOM without losing the operation queue
4. Stay compatible with PebbleDB single-writer constraint
5. Keep DB latency for child ops within ~2× of in-process (target: <5ms
   round-trip per Get/Put on warm sockets)

## Non-goals

- Cross-host distribution (single-node only)
- Generic plugin sandbox (only ops listed in registry, not user plugins)
- Network-transparent RPC (unix socket only)

## Design — Two viable options

### Option A: Parent-mediated Store RPC (recommended)

The child does NOT open PebbleDB. The parent exposes its existing
`database.Store` interface over a unix-socket JSON-RPC (or gob/protobuf)
endpoint. The child gets a `Store` proxy implementation that marshals
calls to the parent.

```
Parent process                      Child process (per op)
─────────────                       ──────────────────────
PebbleStore (real)                  StoreProxy (RPC client)
        ▲                                    │
        │                                    ▼
StoreRPCServer  ◄── unix socket ──►  net.Dial("unix", $UOS_SOCKET)
        ▲
        │
Registry.RunChildMode bootstraps the proxy, calls Run(ctx, params, reporter)
```

Per-op socket path: `$XDG_RUNTIME_DIR/abo-op-<opID>.sock`
or `/tmp/abo-op-<opID>.sock` on systems without XDG.

**Round-trip cost.** unix-socket RPC + JSON ≈ 80–200µs per call on
Linux. Bulk hot loops (fingerprint over 308K files) must batch — see
"Batching" below.

**Reporter / progress.** The same socket multiplexes a `Reporter`
channel back to the parent. Parent forwards to slog + activity log as
if the op ran in-process.

**Lifecycle.**
1. Parent's `Dispatcher.dispatch(opID)` (`internal/operations/registry/
   dispatcher.go`) sees `def.Isolate == true`. It:
   a. Allocates the socket path and binds a listener.
   b. Inserts the in-memory `runHandle` stub (Gate 0 from PR #1154).
   c. `exec.Command(self, "--operation-runner", opID)` with env
      `UOS_SOCKET=<path>`, `UOS_OP_DEF=<def-json>`.
   d. Waits for child handshake (≤5s, else kill + retry-in-process).
2. Child's `cmd/audiobook-organizer/main.go` checks
   `registry.IsChildMode()` BEFORE cobra. If true, dials the socket,
   identifies itself, receives the op def, builds a `runtime` with the
   `StoreProxy`, runs `def.Run(ctx, params, reporter)`, then exits.
3. Parent monitors child exit code. Non-zero or signal → mark op failed
   with diagnostic, no auto-restart (let ResumePolicy decide on next
   server start).

**Batching.** Add bulk methods to the Store interface used hot:
- `GetBookFilesForIDs([]string)` — already exists
- `UpdateBookFilesBatch([]BookFile)` — NEW. Coalesce per-file writes
  during fingerprint into batches of ~500.
- `ListBookIDs() []string` — already exists (PR #1189)

Operations that already use these (post-MAYDEPLOY-H) need no change.

### Option B: Read-only Pebble snapshot + write-back queue

Child opens the Pebble dir in read-only mode (`pebble.Open(dir,
&pebble.Options{ReadOnly: true})` is supported since Pebble v1.0). All
writes go into a write-back queue sent over the socket to the parent
for application.

**Pros:** child reads are zero-RPC latency (~µs).
**Cons:** read-only snapshot is frozen at open; child can't see writes
applied by other ops or the server. Either acceptable (single-writer
ops only) or fatal (concurrent index updates), per op. Mixed mode is
complex.

**Verdict:** Option A is simpler and fast enough if hot paths batch.
Pick Option B only if profiling shows Option A's RPC overhead dominates
in fingerprint rescan or scan ops.

## Concrete deliverables

1. `internal/operations/registry/rpc/` — new package, JSON-RPC server
   (parent) + client (`StoreProxy`, satisfies `database.Store`)
2. `internal/operations/registry/childmode.go` — `IsChildMode()`,
   `RunChildMode(opID, sock string) error`. Idempotent; safe to call
   before any cobra wiring.
3. `cmd/audiobook-organizer/main.go` — top-level branch:
   `if registry.IsChildMode() { os.Exit(registry.RunChildMode(...)) }`
4. Bulk methods on Store: `UpdateBookFilesBatch`, `UpdateBooksBatch`
   (already partial — formalise the interface)
5. Per-op `Isolate: true` re-enabled in: `acoustid.scan`,
   `acoustid.fingerprint-rescan`, `metadata.bulk-fetch`,
   `itunes.scan`, `itunes.relink`, `maintenance.compact-activity`,
   `dedup.full-rescan`
6. Tests:
   - `internal/operations/registry/rpc/rpc_test.go` — proxy round-trip
   - `internal/operations/registry/subprocess_test.go` — re-exec the
     test binary, run a tiny op end-to-end via real socket
7. Telemetry: child RSS sampled every 30s, parent logs OOM-kill if
   exit signal SIGKILL && rusage.MaxRSS > 80% of cgroup limit

## Rollback

Each delivery lands behind `ABO_OP_ISOLATION=parent_rpc | off`. Default
`off` until #6 lands all-green in CI + 24h soak in prod. Flip to
`parent_rpc` then watch RSS and op success rate. If regression, revert
flag with a single env change — no redeploy.

## Open questions

- Does JSON-RPC suffice or do we want protobuf? Recommendation: start
  with `net/rpc` + `encoding/gob` for stdlib-only deps.
- How do we plumb context cancellation through RPC? Parent closes the
  socket → child reads EOF → cancels its ctx. Add a heartbeat (5s) so
  the child notices parent death within 2 heartbeats.
- Should the proxy cache `GetBookByID` reads inside the child? Risk:
  staleness on long ops. Defer; revisit after measurement.

## Out of scope (file separately)

- Replacing Pebble with a multi-writer store (e.g. SQLite WAL,
  Badger v4) — that's a year of work and not justified by this
  isolation problem alone.
- Process-level sandboxing (seccomp / namespaces) — current threat
  model is "ops are our code, isolate for crash containment, not
  trust boundary".
