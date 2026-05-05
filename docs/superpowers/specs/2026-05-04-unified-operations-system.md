<!-- file: docs/superpowers/specs/2026-05-04-unified-operations-system.md -->
<!-- version: 1.0.0 -->
<!-- guid: d2a8c3f1-5e7b-4f92-b6d1-8c3e2f1a9b4d -->
<!-- last-edited: 2026-05-04 -->

# Unified Operations System (UOS) — Design Spec

Status: **Approved for implementation** (Daisy, 2026-05-04)

## Why this exists

The current async-work infrastructure is fragmented across three layers
that don't agree:

- **Backend.** `OperationQueue` (in-memory map of `QueuedOperation`),
  `operations` DB table (status rows), several scheduled-job paths,
  ad-hoc `init()` goroutines, and direct fire-and-forget calls in some
  handlers.
- **Frontend.** `useOperationsStore` (zustand, bell icon),
  `ActivityLog.tsx` `useState` for the same data, plus per-page polling
  in places like `BookDedup.tsx`.
- **Plugin layer.** Every "plugin" (iTunes, Deluge, AcoustID) registers
  its async work differently — some via `s.queue.Enqueue`, some via
  scheduler entries, some via direct goroutines.

Concrete failures in production this week:

1. `reconcile_scan` ignored `ctx`, so cancellation didn't free workers.
   Two stuck reconcile_scans pinned both workers; AcoustID + everything
   else queued indefinitely. Worker count was 2.
2. Server restart auto-resumed the stuck reconcile_scans every time.
3. `getActiveOperations()` read `body.operations` instead of
   `body.data.operations` — the Activity-page Active Operations panel
   was permanently empty, the bell icon's auto-discovery was blind.
4. ffmpeg/chromaprint warnings from inside an op had zero op-id
   tagging; landed in journalctl as orphan log lines.
5. Completed ops vanished from the UI on refresh because terminal-state
   retention was client-only.
6. Three trigger paths (handler-emit, scheduler-emit, direct-goroutine)
   meant "every async thing the user can trigger" wasn't a coherent
   set. Asking "show me what's happening in the system right now"
   couldn't be answered authoritatively.

The fix is a from-scratch design that makes the registry the single
source of truth for async work, designed for plugin-extensibility from
day one.

## Goals

1. **One contract for all async work.** User-triggered, scheduled,
   event-triggered, scanner-emitted — all funnel through the same
   `OperationDef` registration.
2. **Every plugin registers its operations through the same SDK.**
   Today's iTunes/Deluge/AcoustID/dedup are first-party plugins using
   the contract. New first-party plugins (the file-error card, async
   embed batch, log tagging) and future contributed plugins use the
   same SDK.
3. **Operations cannot stall the system.** A misbehaving op (ignores
   `ctx`, never checkpoints, runs forever) hurts itself but never
   blocks the queue.
4. **Cancel actually cancels.** Cooperative for in-process; subprocess
   kill for ops that declare `isolate: true`.
5. **Every log line emitted inside an op is tagged with the op id.**
   ffmpeg, chromaprint, whisper, etc. — all auto-tagged via the
   subprocess wrapper.
6. **Terminal state survives a refresh.** The frontend reads from a
   server-authoritative timeline endpoint; SSE patches the store
   between refreshes.
7. **Triggered op chains are observable.** Activity page renders cause
   → effect as a tree.
8. **Plugin disable/enable is real.** Disabling stops new triggers and
   asks running ops to quiesce per their declared resume policy.
   History is preserved unless explicitly flushed.

Non-goals (deliberately out of scope):

- Runtime-loaded plugins (RPC subprocesses) in v1. Contract is designed
  so that's a vNext addition without breaking changes.
- Runtime-enforced capability narrowing in v1. Capabilities are
  declared and lint-checked; runtime enforcement is vNext.
- Distributed / multi-node operation. Single-process, single-host.

## Glossary

- **Plugin** — a self-contained subsystem that registers OperationDefs,
  declares capabilities and dependencies, and may publish/subscribe to
  events. Today's iTunes service, Deluge integration, AcoustID
  fingerprinter, embedding-based dedup, etc. are plugins.
- **OperationDef** — the static registration of an operation: its id,
  display name, run function, schedule, triggers, resume policy, etc.
  Defined once at plugin init time.
- **OperationRun** — one execution of an OperationDef. Has an id (ULID),
  state, progress, logs, parent reference. Persisted to `operations_v2`.
- **Registry** — the central in-memory and DB-backed object that owns
  every OperationDef, dispatches runs, enforces policies, and routes
  events.
- **Reporter** — the per-run handle a plugin's `Run` function uses to
  emit progress, logs, checkpoints. Wraps a context-bound `slog.Logger`
  and the DB writers.
- **Capability** — a coarse permission an OperationDef declares it
  needs (`library.read`, `network.openai`, `subprocess.spawn`, …).
  Declared statically; UI surfaces them; lint-enforced in v1; runtime-
  enforced in vNext.

## Architecture overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Plugin layer (compiled-in)                   │
│                                                                     │
│  iTunes Plugin   Deluge Plugin   AcoustID Plugin   Dedup Plugin ... │
│       │              │                 │               │            │
│       └──────────────┴───── pkg/plugin/sdk ─────┴──────┘            │
│                              (OperationDef, Reporter, Capabilities) │
└────────────────────────────────────┬────────────────────────────────┘
                                     │ Register at init()
                                     ▼
┌─────────────────────────────────────────────────────────────────────┐
│                            Registry                                 │
│                                                                     │
│  ┌───────────┐  ┌──────────┐  ┌───────────┐  ┌─────────────────┐    │
│  │ OpDef map │  │ Cron     │  │ Event bus │  │ Resource budgets│    │
│  └─────┬─────┘  └────┬─────┘  └─────┬─────┘  └────────┬────────┘    │
│        └─────────────┴──────────────┴─────────────────┘             │
│                                ▼                                    │
│                       Dispatcher (per-plugin                        │
│                       budget + concurrency-key                      │
│                       gating)                                       │
└────────────────────────────────────┬────────────────────────────────┘
                                     │
                ┌────────────────────┴────────────────────┐
                ▼                                         ▼
         ┌──────────────┐                          ┌──────────────┐
         │ In-process   │                          │ Subprocess   │
         │ runner pool  │                          │ runner       │
         │ (worker B)   │                          │ (isolate=true│
         │              │                          │  worker C)   │
         └───────┬──────┘                          └──────┬───────┘
                 │                                        │
                 └─────────────┬──────────────────────────┘
                               ▼
                       Reporter writes
                  ┌──────────────────────────┐
                  │ operations_v2  (state)   │
                  │ op_logs_v2     (logs)    │
                  │ op_errors_v2   (derived) │
                  │ op_state_v2    (resume)  │
                  └────────────┬─────────────┘
                               │
                               ▼
                        SSE event hub
                               │
                               ▼
              ┌───── Frontend (single zustand store) ─────┐
              │ Bell icon · Activity timeline · per-page  │
              │ are all renderings of the same store.     │
              └────────────────────────────────────────────┘
```

## Decision matrix (the 14 questions, locked answers)

| # | Topic | Decision | Source |
|---|---|---|---|
| 1 | Migration scope | **C (greenfield redesign with strangler-fig migration; section 11)** | Q1 |
| 2 | OperationDef contents | **C: id, run, priority, cancellable, resumable, concurrency_key, params_schema, permissions, capabilities, schedule, depends_on, triggers** | Q2 |
| 3 | Cancellation/isolation | **B + C: in-process cooperative + replacement-worker accounting; subprocess kill for `isolate: true`** | Q3 |
| 4 | Frontend pipeline | **C: server-authoritative `/operations/timeline` endpoint as source of truth; SSE for liveness** | Q4 |
| 5 | Resume policy | **B + optional C: explicit `resume_policy` per OperationDef, no default; phases optional for fine-grained resume** | Q5 |
| 6 | Watchdog/strikes | **B (now) + C (later): behavioural watchdog + auto-demotion in v1; quarantine + trust tiers in v2** | Q6 |
| 7 | Worker pool | **A + B: single global pool, per-plugin `max_concurrent` enforced at dispatch** | Q7 |
| 8 | Disable behavior | **C with ladder: quiesce per resume_policy, drain fallback, cancel hammer; history preserved** | Q8 |
| 9 | Logging | **A + C: structured slog from ctx for in-process; subprocess wrapper auto-tags stdout/stderr** | Q9 |
| 10 | Triggers | **C: hardcoded core vocabulary + plugin-namespaced extensions** | Q10 |
| 11 | Trigger inheritance | **Matrix in §6.3** | Q10 |
| 12 | Plugin loading | **B (compiled-in via `pkg/plugin/sdk`); D (RPC subprocesses) deferred to vNext, contract designed for it** | Q11 |
| 13 | Permissions | **B (declared + lint-checked) in v1; C (runtime-narrowed handles) in vNext** | Q12 |
| 14 | Schema | **C: plugin-owned migrations + stable core schema + `requires_core_schema` declaration** | Q13 |
| 15 | Migration ordering | **B with consolidated cutover after canary** | Q14 |

The "C-weighted-pool" worker model (Q7 alternative) is captured in
section 14.2 ("perfect-world-someday") for future reference and is
explicitly out of scope until every other software bug is fixed and
every feature is perfect.

## §1. The OperationDef contract

```go
// pkg/plugin/sdk/operation.go

type OperationDef struct {
    // Identity. Required.
    ID            string                  // globally unique, e.g. "acoustid.fingerprint-extract"
    Plugin        string                  // owning plugin, e.g. "acoustid"
    DisplayName   string                  // human-readable, shown in UI
    Description   string                  // 1-2 sentences for the plugin detail panel

    // Execution. Required.
    Run           func(ctx context.Context, params json.RawMessage,
                      reporter Reporter) error
    DefaultPriority Priority             // PriorityLow | PriorityNormal | PriorityHigh

    // Cancellation. Required.
    Cancellable   bool                    // false = registry's Cancel API rejects; true = ctx.Done() honored

    // Isolation. Required.
    Isolate       bool                    // true = subprocess; false = in-process goroutine
    Timeout       time.Duration           // 0 = use defaults (120m in-process, 6h subprocess); cap 24h

    // Resumability. Required (no default — must be explicit).
    ResumePolicy  ResumePolicy            // ResumeRestart | ResumeRequeue | ResumeDrop | ResumeAsk

    // Concurrency. Required.
    ConcurrencyKey   string               // ops with same key serialize; empty = no key, no serialization
    // MaxConcurrent is set on the Plugin, not the OperationDef
    // (see Plugin interface in §7). Per-OperationDef serialization
    // uses ConcurrencyKey above.

    // Inputs. Optional.
    ParamsSchema  *jsonschema.Schema      // if set, params validated before enqueue

    // Permissions. Optional.
    Permissions   []auth.Permission       // user perms required to trigger via API
    Capabilities  []Capability            // system capabilities the op needs (Q12)
    RunsAs        ActorMode               // ActorContext (default) | ActorSystem

    // Scheduling. Optional.
    Schedule      *cron.Spec              // if set, registry runs on this cron

    // Triggers. Optional.
    Triggers      []EventSubscription     // event names this op fires on

    // Dependencies. Optional.
    DependsOn     []string                // op ids that must NOT be running for this op to start

    // Phases. Optional, for fine-grained resume.
    Phases        []Phase                 // if set, registry tracks phase progress for resume
}

type ResumePolicy int

const (
    ResumeUnspecified  ResumePolicy = iota // forbidden — registry refuses registration
    ResumeRestart                          // restore from checkpoint, op handles serialization
    ResumeRequeue                          // re-run from zero (idempotent ops only)
    ResumeDrop                             // abandon on restart, mark `interrupted_dropped`
    ResumeAsk                              // surface in UI, wait for user choice
)

type Priority int

const (
    PriorityLow    Priority = 0
    PriorityNormal Priority = 1
    PriorityHigh   Priority = 2
)

type ActorMode int

const (
    ActorContext ActorMode = iota // run as the user/system that triggered (default)
    ActorSystem                   // run as system regardless of caller
)
```

### §1.1 Resume policy semantics

| Policy | On registry restart with an interrupted run | When used |
|---|---|---|
| `ResumeRestart` | Reload last `reporter.Checkpoint(state)`, call `Run` with `params + state`. Op resumes from where it stopped. | Long ops with meaningful incremental state (e.g. fingerprint backfill across N files) |
| `ResumeRequeue` | Discard checkpoint, re-run from zero. | Idempotent ops where re-running is cheap or required for correctness (e.g. cache rebuilds) |
| `ResumeDrop` | Mark run as `interrupted_dropped`, do not re-execute. Scheduler may re-fire on next tick. | Heavy ops that should not auto-resume across restarts (e.g. `reconcile_scan`, full library re-scans) |
| `ResumeAsk` | Surface in Activity UI as "interrupted, choose: resume / requeue / drop". | Rare; for ops where user judgement is needed |

`ResumeUnspecified` causes registry to **refuse registration with a
panic at plugin init**. There is no implicit default; plugin authors
must choose explicitly.

### §1.2 Phases (optional)

If `Phases` is non-empty, the registry takes over checkpoint tracking
at phase granularity. Plugin's `Run` function is structured as:

```go
Run: func(ctx context.Context, params json.RawMessage, r Reporter) error {
    if err := r.RunPhase(ctx, "enumerate", func(p PhaseReporter) error {
        // ... emits per-phase progress, optional fine-grained checkpoint
    }); err != nil { return err }

    if err := r.RunPhase(ctx, "hash", func(p PhaseReporter) error {
        // ...
    }); err != nil { return err }

    return nil
}
```

Registry persists current phase and within-phase checkpoint. On
restart, completed phases are skipped and the current phase resumes
from its last `p.Checkpoint(state)` call. Phases are an optional
refinement of `ResumeRestart` — they do not replace `ResumePolicy`.

### §1.3 Watchdog rules (Q6/B)

Registry runs a watchdog goroutine that, every 30s, walks running
runs and applies these rules:

- **Checkpoint freshness.** A run with `ResumePolicy = ResumeRestart`
  that has not called `Reporter.Checkpoint(state)` within
  `min_checkpoint_interval` (default 60s) for the last 5 consecutive
  minutes earns one **Strike: uncheckpointed** against its
  OperationDef. The current run is not killed; the strike is logged.
- **Progress liveness.** A run that has not called
  `Reporter.UpdateProgress(...)` for `progress_timeout` (default 5
  minutes) is killed: subprocess SIGTERM → 30s grace → SIGKILL;
  in-process cancels `ctx` and **spawns a replacement worker**
  (Q3/B). Earns a **Strike: stuck**.
- **Restart budget.** A run that has been resumed N≥3 times without
  reaching a higher progress-percent than its previous high-water
  mark earns a **Strike: infinite-restart** and is force-dropped
  (status `interrupted_dropped`). Future interruptions of this op
  are also force-dropped until the strike decays.

Strikes accumulate per-OperationDef in `op_strikes_v2`. Quarantine
(Q6/C) is **not implemented in v1**; the strike record is kept so
quarantine can be added in v2 without schema change.

## §2. Storage model

### §2.1 Tables (all in core schema)

```sql
-- Static registration mirror; populated at startup from compiled-in
-- plugins. Used for UI listing and capability inspection.
CREATE TABLE op_definitions_v2 (
    id              TEXT PRIMARY KEY,            -- e.g. "acoustid.fingerprint-extract"
    plugin          TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    description     TEXT NOT NULL,
    capabilities    TEXT NOT NULL,               -- JSON array
    permissions     TEXT NOT NULL,               -- JSON array
    cancellable     BOOLEAN NOT NULL,
    isolate         BOOLEAN NOT NULL,
    resume_policy   TEXT NOT NULL,
    schedule_cron   TEXT,                        -- nullable
    triggers        TEXT NOT NULL,               -- JSON array of event subs
    depends_on      TEXT NOT NULL,               -- JSON array
    phases          TEXT NOT NULL,               -- JSON array
    timeout_seconds INTEGER NOT NULL,
    registered_at   TIMESTAMP NOT NULL
);

-- One row per execution. Replaces `operations` table.
CREATE TABLE operations_v2 (
    id                  TEXT PRIMARY KEY,         -- ULID
    def_id              TEXT NOT NULL,            -- FK op_definitions_v2.id
    plugin              TEXT NOT NULL,
    parent_id           TEXT,                     -- FK self; for trigger lineage
    actor_user_id       TEXT,                     -- nullable (system runs)
    trace_id            TEXT NOT NULL,
    span_id             TEXT NOT NULL,
    parent_span_id      TEXT,
    status              TEXT NOT NULL,            -- queued|running|completed|failed|canceled|interrupted_dropped|interrupted_quiesced
    priority            INTEGER NOT NULL,
    progress_current    INTEGER NOT NULL DEFAULT 0,
    progress_total      INTEGER NOT NULL DEFAULT 0,
    progress_message    TEXT NOT NULL DEFAULT '',
    current_phase       TEXT,                     -- nullable; one of OperationDef.Phases
    params              TEXT NOT NULL DEFAULT '{}',  -- JSON
    error_message       TEXT,
    result_data         TEXT,                     -- JSON
    queued_at           TIMESTAMP NOT NULL,
    started_at          TIMESTAMP,
    completed_at        TIMESTAMP,
    last_progress_at    TIMESTAMP,
    last_checkpoint_at  TIMESTAMP,
    high_water_progress INTEGER NOT NULL DEFAULT 0,
    resume_count        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_operations_v2_status ON operations_v2(status, queued_at);
CREATE INDEX idx_operations_v2_parent ON operations_v2(parent_id);
CREATE INDEX idx_operations_v2_def    ON operations_v2(def_id, completed_at DESC);

-- Logs. Replaces `operation_logs` for new ops.
CREATE TABLE op_logs_v2 (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL,                   -- FK operations_v2.id
    level        TEXT NOT NULL,                   -- debug|info|warn|error
    message      TEXT NOT NULL,
    attrs        TEXT NOT NULL DEFAULT '{}',      -- JSON, structured
    created_at   TIMESTAMP NOT NULL
);
CREATE INDEX idx_op_logs_v2_op_time ON op_logs_v2(operation_id, created_at);

-- Errors are a derived view of op_logs_v2 where level='error', materialized
-- for fast filter & long-term retention.
CREATE TABLE op_errors_v2 (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL,
    plugin       TEXT NOT NULL,
    def_id       TEXT NOT NULL,
    message      TEXT NOT NULL,
    attrs        TEXT NOT NULL DEFAULT '{}',
    occurred_at  TIMESTAMP NOT NULL
);
CREATE INDEX idx_op_errors_v2_def     ON op_errors_v2(def_id, occurred_at DESC);
CREATE INDEX idx_op_errors_v2_plugin  ON op_errors_v2(plugin, occurred_at DESC);

-- Resume checkpoints. One row per op, overwritten on each Checkpoint call.
CREATE TABLE op_state_v2 (
    operation_id TEXT PRIMARY KEY,
    phase        TEXT,
    state_blob   BLOB NOT NULL,
    schema_version INTEGER NOT NULL,              -- plugin-controlled; bumped on incompatible state format changes
    written_at   TIMESTAMP NOT NULL
);

-- Watchdog strikes. Accumulates per-OperationDef.
CREATE TABLE op_strikes_v2 (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    def_id      TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    kind        TEXT NOT NULL,                    -- uncheckpointed|stuck|infinite-restart
    details     TEXT NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_op_strikes_v2_def_time ON op_strikes_v2(def_id, occurred_at DESC);

-- Plugin schema versioning (Q13/C).
CREATE TABLE plugin_schema_v2 (
    plugin               TEXT NOT NULL,
    migration_version    INTEGER NOT NULL,
    applied_at           TIMESTAMP NOT NULL,
    PRIMARY KEY (plugin, migration_version)
);

-- Core schema version (Q13/C).
-- Single row.
CREATE TABLE core_schema_meta_v2 (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    core_schema_version INTEGER NOT NULL
);
```

### §2.2 Retention

| Table | Retention | Rationale |
|---|---|---|
| `op_definitions_v2` | Forever (rebuilt from registrations on each startup) | Must reflect compiled-in plugin set |
| `operations_v2` | Forever | History; needed for trigger lineage and audit. UI windows are queries with `WHERE completed_at > X`. |
| `op_logs_v2` | 7 days post-completion, then deleted | Debug detail; bounded growth |
| `op_errors_v2` | Forever (until plugin removed with `--drop-data`) | First-class signal for the broken-files card and similar |
| `op_state_v2` | Until run reaches terminal state, then deleted | Resume-only; no value after completion |
| `op_strikes_v2` | 90 days | Watchdog needs ≥30 days of history; 90 gives headroom for v2 quarantine logic |

### §2.3 Plugin-owned tables

Plugins MAY create their own tables under namespace `<plugin>_*`
(e.g. `acoustid_fingerprints`, `dedup_candidates`). Migrations live
in `internal/plugins/<plugin>/migrations/NNN_<name>.sql`.

Rules:

- Plugin tables MAY have FKs to core schema tables (`books`, `authors`,
  etc.). Core MUST NOT have FKs to plugin-namespaced tables.
- Each migration declares its `up.sql` and `down.sql`. Both required.
- Migrations are applied per-plugin in version order, after core
  migrations, in plugin-name alphabetical order.
- A migration's failure rolls back that plugin's batch only; other
  plugins continue.
- A plugin enabled for the first time after migrations have shipped
  applies the full backlog at enable time (not at startup).

`OperationDef.RequiresCoreSchema` (top-level package field, not on the
def itself) declares the minimum core schema version each plugin
needs:

```go
// pkg/plugin/sdk/registration.go
type Plugin interface {
    Name() string
    RequiresCoreSchema() string   // e.g. ">=42"
    Register(reg Registry) error
}
```

If `core_schema_meta_v2.core_schema_version` does not satisfy the
plugin's requirement, registry refuses to enable the plugin and surfaces
a UI error: "Plugin X requires core schema ≥42, but the current core
schema is 41. Update the application to enable this plugin."

## §3. Worker pool & dispatcher (Q7)

Single global worker pool with N goroutines (default 8, configurable
via `OPERATION_QUEUE_WORKERS` env var). Each goroutine pulls from a
priority queue.

Dispatch rules (checked at dequeue time, in order):

1. **Plugin disabled?** Skip (op stays queued; will dispatch when
   re-enabled, or be reaped by quiesce logic if disable was a hard
   disable).
2. **Capability requirements satisfied?** (e.g. `network.openai`
   requires an OpenAI key configured.) If not, skip and surface a
   UI hint on the op row: "Waiting on: OpenAI API key".
3. **Plugin's `max_concurrent` already in use?** Skip; try the next
   queued op.
4. **`ConcurrencyKey` already running for another op?** Skip.
5. **Any `DependsOn` op currently running?** Skip.
6. **Otherwise:** dispatch.

The skip-and-continue logic uses an O(1) plugin-running-count map and
an O(1) concurrency-key map maintained by the dispatcher. A queued op
that gets skipped repeatedly does not starve — the dispatcher loops
through the queue in priority order on every dequeue tick (every 100ms
or on op-completion/enqueue events).

In-process worker (Q3/B):

- Calls `OperationDef.Run(ctx, params, reporter)` directly.
- `ctx` is cancelled when user clicks Cancel.
- If watchdog kills the run (progress timeout), worker abandons the
  goroutine (it stays running until it returns, but is no longer
  tracked) and the worker slot is **immediately reused** by spawning a
  replacement worker. There is a hard cap on outstanding "abandoned"
  goroutines per plugin (default 4); exceeding it refuses new
  dispatches for that plugin until they finish.

Subprocess worker (Q3/C, used when `OperationDef.Isolate = true`):

- Re-execs the host binary with a special flag (`--operation-runner`),
  passing the def id and params on stdin.
- Child process initializes the same plugin set and invokes
  `OperationDef.Run` directly in-process within the child.
- Cancellation = SIGTERM; 30-second grace; then SIGKILL.
- Stdout/stderr are captured by the parent and routed through the
  Reporter as `info` (stdout) and `warn` (stderr) log lines, tagged
  with the operation id. This is what auto-tags ffmpeg/chromaprint
  output without requiring plugin-author cooperation.
- Reporter calls in the child are RPC'd to the parent over a unix
  domain socket pair attached on launch. Parent persists state; child
  is stateless across runs.

## §4. Reporter contract

```go
// pkg/plugin/sdk/reporter.go

type Reporter interface {
    UpdateProgress(current, total int, message string) error
    Log(level slog.Level, msg string, attrs ...slog.Attr) error
    Logger() *slog.Logger                         // returns op-bound logger
    Checkpoint(state any) error                   // serializes via gob; bumps schema_version externally
    IsCanceled() bool

    // Phases
    RunPhase(ctx context.Context, name string, fn func(PhaseReporter) error) error

    // Sub-operation triggers (Q10 inheritance; section 6.3)
    Trigger(ctx context.Context, eventName string, payload any) error
}

type PhaseReporter interface {
    UpdateProgress(current, total int, message string) error
    Log(level slog.Level, msg string, attrs ...slog.Attr) error
    Checkpoint(state any) error
    IsCanceled() bool
}
```

`Logger()` returns a `*slog.Logger` with default attrs `op_id`,
`def_id`, `plugin`, `trace_id`, `span_id` already bound. The standard
slog pipeline writes to:

1. **journalctl** via the existing log handler, with structured prefix.
2. **op_logs_v2** via a buffered DB writer (flushes every 250ms or
   when buffer hits 100 lines).
3. **SSE event hub** as `op.log` events.

A line with `level >= slog.LevelError` is additionally written to
`op_errors_v2` by the same handler (single write path; promotion
happens in the handler, not at callsite).

## §5. Capability vocabulary (Q12)

```go
// pkg/plugin/sdk/capability.go

type Capability string

const (
    CapLibraryRead   Capability = "library.read"
    CapLibraryWrite  Capability = "library.write"
    CapFilesRead     Capability = "files.read"
    CapFilesWrite    Capability = "files.write"
    CapFilesExecute  Capability = "files.execute"

    CapNetworkOpenAI       Capability = "network.openai"
    CapNetworkAudible      Capability = "network.audible"
    CapNetworkOpenLibrary  Capability = "network.openlibrary"
    CapNetworkGoogleBooks  Capability = "network.googlebooks"
    CapNetworkITunes       Capability = "network.itunes"
    CapNetworkGeneric      Capability = "network.generic"

    CapScheduleCron  Capability = "schedule.cron"
    CapScheduleEvent Capability = "schedule.event"

    CapSubprocessSpawn Capability = "subprocess.spawn"
    CapDBMigrate       Capability = "db.migrate"
)
```

Adding a capability later is permitted; renaming is a contract break.

In v1, declarations are surfaced via the plugin-detail UI and via
`/api/v1/plugins/<name>/capabilities`. CI runs a linter
(`tools/cmd/oplint`) that walks plugin source and reports declared-vs-
used mismatches as build errors.

In vNext, the SDK exposes narrowed handles (`LibraryReader` vs
`LibraryReadWriter`) and the plugin's `ctx` only carries the handles
matching its declarations. Direct `os.Open` becomes a build error via
lint banning calls outside SDK-provided wrappers.

## §6. Triggers and the event bus (Q10)

### §6.1 Core event vocabulary (registry-owned, hardcoded)

```
book.imported           payload: { book_id, source }
book.updated            payload: { book_id, fields_changed }
book.deleted            payload: { book_id }
book.relocated          payload: { book_id, old_path, new_path }
library.scanned         payload: { books_added, books_updated }
metadata.applied        payload: { book_id, source }
version.merged          payload: { primary_id, merged_id }
file.imported           payload: { file_path, book_id }
operation.completed     payload: { op_id, def_id, status }
operation.failed        payload: { op_id, def_id, error }
plugin.enabled          payload: { plugin_name }
plugin.disabled         payload: { plugin_name }
```

Adding to this list is a core-schema-version bump (plugins declaring
`requires_core_schema: ">=N"` get a clean compatibility check).

### §6.2 Plugin-namespaced events

Plugins publish on their own namespace:
`acoustid.fingerprint.extracted`, `dedup.candidate.created`. Other
plugins subscribe explicitly via `Triggers: []EventSubscription{...}`.

Wildcard subscriptions (`acoustid.*`) are supported.

### §6.3 Trigger inheritance matrix

When event `E` fires and op `Op` is triggered by it, the run inherits
from the firing run (if any) as follows:

| Field | Inherited? | Override |
|---|---|---|
| `parent_id` | Always set to firing run's `id` | Cannot override |
| `actor_user_id` | Inherited by default | OperationDef may set `RunsAs: ActorSystem` to fall back to system actor |
| `trace_id` / `parent_span_id` | Always set; new `span_id` generated | Cannot override |
| `Cancellation cascade` | Cancelling parent cancels children | OperationDef may set `CancelWithParent: false` for cleanup ops |
| Event payload | Mandatory; handed to triggered op as `params` | Cannot override |
| `Priority` | Not inherited; uses OperationDef's `DefaultPriority` | n/a |
| `Permissions` | Triggered op runs as actor; if actor lacks permission, run is refused | OperationDef may set `RunsAs: ActorSystem` to bypass |
| Resource budget | Not inherited; each plugin's budget tracked independently | n/a |

### §6.4 Event publish/subscribe

```go
// pkg/plugin/sdk/events.go

type EventSubscription struct {
    EventName string                 // exact match or wildcard prefix (e.g. "acoustid.*")
    Filter    func(payload []byte) bool  // optional; coarse pre-dispatch filter
}

type Bus interface {
    Publish(ctx context.Context, event string, payload any) error
}
```

`Reporter.Trigger(ctx, eventName, payload)` is sugar that publishes
through the registry's bus with parent metadata pre-filled.

## §7. Plugin contract (Q11/B)

```go
// pkg/plugin/sdk/plugin.go

type Plugin interface {
    Name() string
    DisplayName() string
    Description() string
    Version() string                              // semver
    RequiresCoreSchema() string                   // e.g. ">=42"
    MaxConcurrent() int                           // per-plugin slot quota (Q7); default 4
    Migrations() []Migration                      // returned to registry; applied at enable time
    Register(reg Registry) error                  // calls reg.RegisterOp(...) for each OperationDef
    OnEnable(ctx context.Context) error           // optional setup
    OnDisable(ctx context.Context, mode DisableMode) error  // see §8
}

type Registry interface {
    RegisterOp(def OperationDef) error
    EnqueueOp(ctx context.Context, defID string, params any, opts ...EnqueueOption) (opID string, err error)
}
```

Plugins live under `internal/plugins/<name>/` and are discovered at
binary init time via a centralized import-and-register file
(`internal/plugins/plugins.go`) that imports each plugin package for
its side effects.

## §8. Disable behavior (Q8/C)

Disabling a plugin proceeds through a ladder:

1. **Stop scheduling.** Cron entries are removed; new triggers do not
   fire ops for this plugin. UI dims the plugin's user-facing buttons
   with a "plugin disabled" hint.
2. **Quiesce running ops.** For each running op owned by the plugin:
   - `ResumeRestart` → registry signals `quiesce`, expects op to call
     `Reporter.Checkpoint(state)` and return `ErrQuiesced` within
     `quiesce_grace` (30s).
   - `ResumeRequeue` → registry cancels via ctx; on next enable, op is
     re-queued (retains queued status).
   - `ResumeDrop` → registry cancels via ctx; run marked
     `interrupted_quiesced`; not re-queued.
   - `ResumeAsk` → run marked `interrupted_quiesced`; user choice
     surfaced when plugin is re-enabled.
3. **Drain.** If quiesce_grace expires for a `ResumeRestart` op, it
   degrades to drain — registry stops sending it new triggers but
   lets the in-flight call return naturally. UI shows "draining" with
   a "force-cancel" button.
4. **Cancel hammer.** If user clicks "force-cancel", or `force_grace`
   (default 5m) expires, the op is hard-cancelled (subprocess SIGKILL
   or in-process worker abandonment per §3).

Plugin removal (vs. disable) is stricter:
- All running ops cancelled immediately (cancel hammer).
- Cron entries and event subscriptions deleted.
- Plugin's history is **preserved** (read-only) unless the user invokes
  the API-only `DELETE /api/v1/plugins/<name>/history?confirm=true`,
  which truncates `operations_v2`, `op_logs_v2`, `op_errors_v2`,
  `op_state_v2`, `op_strikes_v2` rows for that plugin and runs the
  plugin's `down.sql` migrations.

## §9. Frontend pipeline (Q4/C)

Single zustand store: `useOperationsStore`. Bell icon, Activity page,
and per-page op displays are all renderings of the same store.

Data flow:

1. **Initial load** (page mount, post-refresh): `GET /api/v1/operations/timeline?since=15m`
   returns every op (running, recently completed, recently failed)
   within the window, ordered by start time, with each op carrying a
   tail of recent log lines (last 50). Replaces the entire store
   contents.
2. **Live updates** via SSE (`/api/v1/operations/events`):
   - `op.created` — add to store
   - `op.updated` — patch store entry (status, progress, message,
     phase)
   - `op.log` — append to store entry's logs (capped at 500 lines per
     op in client memory; full history available via
     `GET /api/v1/operations/<id>/logs`)
   - `op.error` — derived event; UI uses for badges/coloring without
     parsing log lines
   - `op.terminal` — sets terminal state; entry remains in store for
     30 minutes of wall-clock time, then drops on next periodic
     compaction
3. **Reconnect** (SSE drop): re-call timeline endpoint with
   `since=<last-event-time>` and replace the slice; resume SSE.

Bell icon renders: any op with `status in (queued, running)` plus any
op with `status terminal` whose `completed_at > now - 30s` (briefly
shows completion before fading).

Activity page renders: every op in the store, grouped by `parent_id`
to form trees. Click an op row to expand; expansion fetches the full
log via `GET /api/v1/operations/<id>/logs?tail=N` and subscribes to
that op's SSE channel for live tail.

The store is not authoritative for terminal-state retention; the
server-side timeline endpoint is. Refreshing the page always
reconstructs the visible window from the server.

The "in-memory DB" escape hatch noted by the user is acknowledged: if
SSE + timeline-endpoint pressure becomes a problem at scale (e.g. SQLite
contention on op_logs_v2 writes), the read path can be moved to an
in-memory cache (e.g. Redis or an in-process map) without changing the
client contract. v1 sticks with SQLite as the read source of truth.

## §10. API surface

```
# Triggering
POST   /api/v1/operations/v2                     - body { def_id, params }
                                                  → { id, status, ... }

# Query
GET    /api/v1/operations/timeline?since=15m     → { operations: [...] }
GET    /api/v1/operations/v2/:id                 → { ...full op + tail of logs }
GET    /api/v1/operations/v2/:id/logs?tail=500   → { logs: [...] }
GET    /api/v1/operations/v2/:id/state           → { state_blob, schema_version }  (debug-only)

# Live
GET    /api/v1/operations/events                 - SSE stream (filter via query: ?op_id=, ?plugin=)

# Lifecycle
DELETE /api/v1/operations/v2/:id                 - cancel (cooperative or subprocess kill)
POST   /api/v1/operations/v2/:id/resume          - explicit resume for ResumeAsk runs

# Plugins
GET    /api/v1/plugins                           → { plugins: [...] }
GET    /api/v1/plugins/:name                     → { ...full plugin def }
GET    /api/v1/plugins/:name/capabilities        → { capabilities: [...], satisfied: bool, missing: [...] }
POST   /api/v1/plugins/:name/disable             → 202
POST   /api/v1/plugins/:name/enable              → 202
DELETE /api/v1/plugins/:name/history?confirm=true → 200 (admin-only; truncates plugin history)

# Operation definitions (introspection)
GET    /api/v1/op-defs                           → { defs: [...] }
GET    /api/v1/op-defs/:id                       → { ...full def }
```

Old v1 endpoints (`/operations/active`, `/operations/recent`, etc.)
remain during migration; deleted in the cutover PR (§11).

## §11. Migration plan (Q14/B with consolidated cutover)

**Phase A — establish the new system (canary):**

1. Schema migrations for all `*_v2` tables (PR 1).
2. Registry shell + dispatcher + in-process worker pool (PR 2).
3. Subprocess runner + SDK reporter (PR 3).
4. SDK package `pkg/plugin/sdk/` with full contract (PR 4).
5. Frontend dual-source: `useOperationsStore` reads from both v1 and
   v2 endpoints, dedupes by id (PR 5).
6. SSE event hub plumbing (PR 6).
7. Migrate `embed-scan` as canary; new endpoint serves it; existing
   `/api/v1/dedup/embed` stays as a redirect to the new path (PR 7).
8. Watchdog + strikes tables + watchdog goroutine (PR 8).

**Confidence checkpoint** — soak the canary for ~1 week. Required
observations before proceeding:

- ≥3 successful runs of `embed-scan` to completion via v2.
- ≥1 deliberate cancellation that actually frees the worker within
  30s.
- ≥1 server restart while `embed-scan` is queued, verifying its
  `ResumePolicy` is honored.
- Zero orphaned workers (in-process replacement count) at steady
  state.
- SSE reconnect tested manually with no data loss.
- Logs land in `op_logs_v2` with op-id-tagged ffmpeg output (canary
  uses subprocess for the embedding API call).

User explicitly approves moving to phase B.

**Phase B — consolidated cutover:**

9. Plugin SDK extraction; convert `acoustid` + `dedup` plugins
   together (PR 9, ~8 ops).
10. Convert `itunes` plugin (PR 10, ~6 ops).
11. Convert `deluge` plugin (PR 11, ~4 ops).
12. Convert maintenance plugin (`reconcile_scan`, etc.) (PR 12, ~8
    ops). `reconcile_scan` declares `ResumePolicy: ResumeDrop` —
    structurally prevents today's bug from recurring.
13. Frontend single-source: drop dual-source merging; reads only from
    v2 (PR 13).
14. Old endpoint + old `OperationQueue` deletion (PR 14).
15. SDK extracted to `pkg/plugin/sdk` as its own importable package;
    docs page added with "how to write a plugin" (PR 15).

## §12. Test strategy

Per-PR test plan is in each bot-task doc. Spec-level guarantees:

- **Registry property tests** (using `pgregory.net/rapid`): for any
  random sequence of `RegisterOp + EnqueueOp + Cancel + Restart`, the
  resulting state of `operations_v2` is internally consistent (no
  running ops without a worker; no completed ops with a non-terminal
  status; no `ResumeDrop` ops in a queued state after restart).
- **Watchdog soak test**: a fake plugin with an OperationDef that
  declares `ResumeRestart` but never checkpoints, run 100 times in a
  test harness, must accumulate exactly 100 strikes and never crash
  the registry.
- **Subprocess kill test**: a fake op that ignores `ctx`, run with
  `Isolate: true`, MUST be killed within `30s + grace` of cancel.
- **In-process replacement worker test**: a fake op that ignores
  `ctx`, run with `Isolate: false`, MUST result in worker-slot
  reuse within `progress_timeout + grace`; abandoned-goroutine count
  is bounded by per-plugin cap.
- **End-to-end happy path**: Cypress/Playwright test that triggers
  `embed-scan`, observes bell icon increment, expands the op in the
  Activity panel, sees logs streaming, watches it complete, sees it
  remain in the timeline for 30 minutes.
- **Trigger lineage test**: fire a `book.imported` event, assert the
  triggered fingerprint-extract op has the import op as `parent_id`.

## §13. Observability & metrics

Each op execution emits Prometheus metrics (existing `internal/metrics`
package):

- `op_runs_total{def_id, status}` — counter
- `op_duration_seconds{def_id}` — histogram
- `op_queue_wait_seconds{def_id}` — histogram
- `op_strikes_total{def_id, kind}` — counter
- `op_workers_busy{kind=in_process|subprocess}` — gauge
- `op_workers_abandoned{plugin}` — gauge

Activity page exposes a "metrics" tab per OperationDef showing strike
history, recent error counts, average duration over the last 7 days.

## §14. Deferred / future work

### §14.1 vNext (post-v1)

- **Runtime-loaded plugins (Q11/D).** Subprocess-RPC harness that
  speaks the same OperationDef contract. Crash isolation; language-
  agnostic plugin authorship.
- **Runtime-enforced capabilities (Q12/C).** SDK refactor to provide
  narrowed handles (`LibraryReader` vs `LibraryReadWriter`); direct
  `os.Open` becomes lint-banned. Schedule when ≥3 contributed plugins
  exist or after one capability incident.
- **Quarantine + trust tiers (Q6/C).** Use accumulated strike data to
  auto-quarantine misbehaving OperationDefs; promote new ops from
  `unverified` to `trusted` after N successful runs without strikes.

### §14.2 "Perfect-world someday"

The following should not be touched until every other software bug in
the application is fixed and every feature is perfect:

- **Weighted concurrency-key worker pool (Q7/C).** Per-key slot
  allocation with weights; `whisper-transcribe: weight=4` consumes 4
  slots, `metadata-fetch-one: weight=1` consumes 1. Replaces the
  per-plugin `max_concurrent` model. Over-engineered for current
  scale (≤10 plugins, ≤30 ops); revisit only if we hit
  starvation/priority-inversion in production.

## §15. Defaults reference

| Setting | Default | Tunable via |
|---|---|---|
| Worker count | 8 | `OPERATION_QUEUE_WORKERS` env var |
| In-process op timeout | 120m | OperationDef.Timeout |
| Subprocess op timeout | 6h | OperationDef.Timeout |
| Hard timeout cap | 24h | not tunable |
| `min_checkpoint_interval` | 60s | OperationDef field |
| `progress_timeout` (watchdog) | 5m | OperationDef field |
| Quiesce grace | 30s | not tunable in v1 |
| Force grace (post-quiesce) | 5m | not tunable in v1 |
| Subprocess SIGTERM→SIGKILL grace | 30s | not tunable in v1 |
| Strike threshold (informational only in v1) | 3 strikes / 30 days | future config |
| Strike decay | 1 strike / 7 clean days | future config |
| Abandoned-goroutine cap per plugin | 4 | future config |
| Default per-plugin `max_concurrent` | 4 | OperationDef field |
| `op_logs_v2` retention | 7 days post-completion | future config |
| `op_errors_v2` retention | forever | per-plugin disable command |
| Timeline window default | 15 minutes | query param |
| Terminal-state visible duration in store | 30 minutes | client constant |

## §16. Acceptance criteria

The system ships when:

- [ ] All 14 PRs in §11 are merged.
- [ ] `embed-scan` has run >100 times via v2 in production with zero
      strike accumulations.
- [ ] User triggers an op from any UI surface and sees it appear in the
      bell icon AND the Activity timeline within 1s.
- [ ] User refreshes the Activity page mid-op; the op is still visible
      with progress.
- [ ] User cancels a long-running `Isolate: true` op; the subprocess is
      dead within 30s.
- [ ] `reconcile_scan` declares `ResumeDrop`; restarting the server
      does not re-resume an in-flight reconcile_scan.
- [ ] An ffmpeg warning emitted by AcoustID's fingerprint extractor is
      visible in the Activity-page log view, tagged with the op id.
- [ ] A plugin disable propagates correctly: scheduled ops stop firing;
      running ops quiesce per their `ResumePolicy`; history is
      preserved; re-enable resumes ops marked `ResumeAsk`.
- [ ] All 14 watchdog property tests pass.

## Open questions for review

None at spec time. All Q1-Q14 decisions are locked. If implementation
surfaces new questions, they go through the same decision-matrix
process — they are not allowed to be resolved by the implementation
agent ad-hoc.
