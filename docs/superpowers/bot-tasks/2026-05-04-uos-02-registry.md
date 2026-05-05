<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-02-registry.md -->
<!-- version: 1.0.0 -->
<!-- guid: 02c3d4e5-6f7a-8b9c-ad1e-2f3a4b5c6d7e -->
<!-- last-edited: 2026-05-04 -->

# UOS-02 — Registry shell + dispatcher + in-process worker pool

**Companion human spec:** §1 (OperationDef contract), §3 (worker pool
& dispatcher), §15 (defaults). Read both before starting.

## Branch

```
feat/uos-02-registry
```

## Goal

Create the in-memory registry that owns OperationDef registration,
dispatch, and the in-process worker pool. Subprocess execution is NOT
in this PR (UOS-03). Reporter contract is finalized in UOS-04 — for
now, define the bare-minimum reporter interface needed by the worker
loop and stub the DB-write side.

## Files to add

1. `internal/operations/registry/types.go`
   - `OperationDef` struct EXACTLY as in spec §1.
   - `ResumePolicy`, `Priority`, `ActorMode`, `Capability` types as
     enumerated in spec §1 and §5.
   - `EventSubscription` struct as in spec §6.4.
   - `Phase` struct (just `Name string` for now; phase semantics in
     UOS-03's reporter).

2. `internal/operations/registry/registry.go`
   - `Registry` struct holding:
     - `defs map[string]OperationDef` (registration table)
     - `running map[string]*runHandle` (in-flight runs by op id)
     - `pluginRunning map[string]int` (plugin → count of running ops)
     - `concurrencyKeys map[string]string` (key → op id of holder)
     - `dispatch chan struct{}` (signals dispatcher to re-evaluate)
     - per-mutex protection
   - `func New(store database.Store, logger *slog.Logger) *Registry`
   - `func (r *Registry) RegisterOp(def OperationDef) error`:
     - Returns error if `def.ID` already registered.
     - Returns error if `def.ResumePolicy == ResumeUnspecified`.
     - Returns error if `def.Run` is nil.
     - Persists to `op_definitions_v2` (upsert by id). On startup the
       definition table is rebuilt from registrations; orphan rows
       (no longer registered) are deleted.
   - `func (r *Registry) EnqueueOp(ctx context.Context, defID string, params any, opts ...EnqueueOption) (string, error)`:
     - Looks up def. Validates params against `def.ParamsSchema` if
       set.
     - Generates ULID, inserts row in `operations_v2` with
       `status='queued'`, fills `parent_id`/`actor_user_id`/
       `trace_id`/`span_id`/`parent_span_id` from
       `EnqueueOption`s and ctx.
     - Pings dispatcher.
   - `func (r *Registry) Cancel(opID string) error`:
     - Finds run handle, calls its `cancel()`.
     - Updates `operations_v2.status = 'canceled'` if still queued
       (no worker grabbed it yet). If running, status will be
       updated by the worker after Run returns.
   - `func (r *Registry) ActiveDefs() []OperationDef` — for
     introspection endpoints in UOS-06.
   - `func (r *Registry) Shutdown(ctx context.Context) error` —
     waits for workers; on timeout, marks remaining running ops as
     `interrupted_*` per their `ResumePolicy` (see spec §1.1) and
     returns.

3. `internal/operations/registry/dispatcher.go`
   - `func (r *Registry) runDispatcher(ctx context.Context)` — reads
     from `r.dispatch`, walks queued ops in priority order
     (high→normal→low, then `queued_at ASC`), applies the gating
     rules from spec §3 in order, dispatches eligible ops to the
     worker pool. Re-evaluates on every tick (tick interval: 100ms)
     OR on a signal.

4. `internal/operations/registry/worker.go`
   - `type runHandle struct { id, defID string; cancel context.CancelFunc; abandoned bool }`
   - `func (r *Registry) startWorker(ctx context.Context, slot int)` —
     long-running goroutine; reads from a `nextRun chan *queuedRun`
     channel populated by the dispatcher.
   - On each run:
     1. Build per-run ctx with cancel + the op timeout.
     2. Update `operations_v2.status='running'`, `started_at=NOW`.
     3. Construct a `Reporter` (stub for now — see below).
     4. Call `def.Run(runCtx, params, reporter)` with `defer recover()`.
     5. On return: set status to `completed` / `failed` / `canceled`
        per outcome and reporter state.
   - Watchdog interactions are out of scope for this PR (UOS-08).
   - For ops with `Isolate: true`: this PR returns
     `ErrSubprocessNotImplemented` — UOS-03 wires it.

5. `internal/operations/registry/reporter.go` — STUB version only.
   - `type Reporter interface` matching spec §4 signatures.
   - `type stubReporter struct { ... }` writes to a buffered slice;
     real DB writes land in UOS-03/04.
   - `func newStubReporter(opID string) Reporter` returns the stub.

6. Tests:
   - `internal/operations/registry/registry_test.go`:
     - `RegisterOp` rejects nil Run, duplicate ID, unspecified
       ResumePolicy.
     - `EnqueueOp` validates params against schema.
     - `Cancel` on queued op marks it canceled; on running op,
       cancels ctx.
   - `internal/operations/registry/dispatcher_test.go`:
     - Single op dispatches.
     - Two ops with same `ConcurrencyKey` serialize.
     - Two ops with different `ConcurrencyKey` run concurrently.
     - `MaxConcurrent=1` for a plugin caps plugin throughput.
     - `DependsOn` blocks dispatch until the dependency ends.
     - Priority ordering: high before normal before low at same
       queued_at.
   - `internal/operations/registry/worker_test.go`:
     - Successful Run sets status=completed.
     - Run returning error sets status=failed.
     - Run panicking sets status=failed with error message.
     - Cancel during running op sets ctx.Err()=Canceled.

## Files to edit

1. `internal/server/server.go` — wire the registry into Server lifecycle:
   `server.opRegistry = registry.New(store, logger)`. Do not yet
   register any plugins (that comes from each plugin's Register call
   in their own bot-task).

## Hard rules

- All struct fields MUST match spec §1 names and types.
- The dispatcher MUST evaluate ops in priority then queued_at order.
- Skip-and-continue is NOT skip-and-pause; a queued op skipped because
  its concurrency key is held MUST be re-evaluated when that key
  releases.
- ResumePolicy enforcement at restart-time is NOT in this PR (it is in
  UOS-08); but the registry MUST refuse `ResumeUnspecified` registrations
  immediately.
- No subprocess execution in this PR.
- Reporter fully implemented in UOS-03/04. Stub only here.

## Acceptance criteria

- [ ] `go test ./internal/operations/registry/...` passes; ≥80%
      coverage.
- [ ] `make build` passes.
- [ ] `make ci` passes.
- [ ] Property test (use `pgregory.net/rapid`): for any random
      sequence of `RegisterOp + EnqueueOp + Cancel`, registry state
      remains internally consistent (no negative running counts; no
      leaked concurrency keys).

## PR title

```
feat(uos): registry shell + dispatcher + in-process worker pool
```
