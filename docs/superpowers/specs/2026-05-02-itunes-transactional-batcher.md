<!-- file: docs/superpowers/specs/2026-05-02-itunes-transactional-batcher.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a4f7b32-1c5e-4d6a-8f29-3b7c8e5d6f01 -->
<!-- last-edited: 2026-05-02 -->

# iTunes Transactional Write-Back Batcher (WAL Design)

> **Status:** design — pre-implementation
> **Supersedes:** the debounce-only `WriteBackBatcher` in
> `internal/itunes/service/writeback_batcher.go` (v5.0.0).
> **Companion safety net:** `MaxRemovesPerFlush=50` circuit-breaker
> shipped in PR #679 (already live in production).

## 1. Why

The current batcher is a debounce queue. Once a flush starts there is
no recovery: if the resulting write corrupts the ITL or removes the
wrong tracks, the only undo is the rolling 5-deep `.bak` file ring,
and you get one all-or-nothing rollback per flush.

That's not enough. We need to be able to:

1. **Replay** the exact sequence of intended changes against a known
   pristine ITL (e.g. the golden master) for forensics.
2. **Roll back a single transaction** while preserving every later
   transaction (e.g. "undo the 09:14 batch but keep the 09:15 batch").
3. **Trace** every PID-level change back to the user action, job, or
   sync that produced it (so when something goes wrong we can answer
   "what called EnqueueRemove for this PID, and why?").
4. **Two-way sync** — record changes that arrive from iTunes itself
   (the user edits a track in the app) into the same log so our DB
   converges to the user's intent rather than overwriting it.
5. **Bound risk** by capping how much work lands in any single
   transaction commit, independent of how big the in-memory queue is.

This spec keeps the existing `EnqueueAdd / EnqueueRemove / Enqueue`
public surface and the existing `SafeWriteITL` mutation primitive. It
slots a write-ahead log + transaction commit pipeline between them.

## 2. Vocabulary

| Term            | Meaning                                                                              |
| --------------- | ------------------------------------------------------------------------------------ |
| **Op**          | One ITL-level action: `add_track`, `remove_track`, `update_location`, `update_meta`. |
| **Batch**       | The set of Ops collected during one `BatchWindow` (default 1 minute).                |
| **Transaction** | One or more Batches committed atomically against the ITL; one journal row.           |
| **Journal**     | Append-only durable log of every Transaction (and its planned Ops).                  |
| **Rollback**    | Re-applying a Transaction's `inverse_ops` plan against the live ITL.                 |
| **Reconcile**   | Reading the ITL on startup / on demand and emitting Ops for any drift.               |

## 3. Public API (unchanged surface, new semantics)

The four `Enqueue*` methods stay. They now:

1. Validate the Op (PID format, location not empty, etc).
2. Write the Op to the **pending batch** in memory **and** to the
   `pending_ops` table (durable — survives crash).
3. Notify the batch-window timer.

`Stop()` flushes the current batch into a `pending_committed`
Transaction, fsyncs it, and exits cleanly. On next startup, any
`pending_committed` Transactions resume from where they were (idempotent
apply).

New public methods:

| Method                                    | Purpose                                                                 |
| ----------------------------------------- | ----------------------------------------------------------------------- |
| `Rollback(txID string) error`             | Generate inverse Ops for a committed Transaction and apply.             |
| `ListTransactions(since time.Time)`       | Operator query for the dashboard.                                       |
| `Replay(txID string, target string)`      | Re-apply a Transaction against a target ITL (default: current live).    |
| `ReconcileFromITL() (TxID, error)`        | Diff live ITL vs DB → emit `external_change` Ops → wrap in Transaction. |

## 4. Timing model

| Knob                       | Default | Meaning                                                                                           |
| -------------------------- | ------- | ------------------------------------------------------------------------------------------------- |
| `BatchWindow`              | 60s     | How long a Batch accumulates Ops before being sealed.                                             |
| `CommitInterval`           | 300s    | How often sealed Batches are folded into a Transaction and committed to the ITL.                  |
| `BackPressureThreshold`    | 100     | If pending Ops exceed this, halve `CommitInterval` floor at 60s.                                  |
| `MaxOpsPerTransaction`     | 500     | Hard cap. Excess Ops are split into multiple sequentially-numbered Transactions in the same flush.|
| `MaxRemovesPerTransaction` | 50      | Reuses `MaxRemovesPerFlush` constant. A Transaction whose remove count exceeds this is REFUSED.   |

Adaptive policy:

```text
backlog := count(pending_ops where state = 'queued')
interval := CommitInterval
if backlog >= BackPressureThreshold:
    interval := max(BatchWindow, CommitInterval / 2)
if backlog >= 5 * BackPressureThreshold:
    interval := BatchWindow                        // commit on every batch boundary
sleep(interval); commit_pending()
```

## 5. Storage

A new SQLite database `audiobook-organizer-itunes-wal.db` lives next
to the iTunes write-back ITL (same dir, group-writable). Pebble is
**not** used — we want SQL for ad-hoc operator queries and atomic
multi-row writes via `BEGIN IMMEDIATE`.

```sql
-- file: schema/itunes_wal.sql
-- version: 1.0.0

CREATE TABLE pending_ops (
    op_id          TEXT PRIMARY KEY,                  -- ULID
    enqueued_at    INTEGER NOT NULL,                  -- unix ns
    op_type        TEXT NOT NULL CHECK(op_type IN
                       ('add_track','remove_track','update_location','update_meta','external_change')),
    pid            TEXT,                              -- nullable for adds w/ no PID yet
    payload_json   TEXT NOT NULL,                     -- the full Op struct
    enqueued_by    TEXT NOT NULL,                     -- 'user:<uid>', 'job:<name>', 'sync', 'reconcile'
    state          TEXT NOT NULL DEFAULT 'queued'     -- queued | sealed | committed | rolled_back | rejected
                       CHECK(state IN ('queued','sealed','committed','rolled_back','rejected')),
    batch_id       TEXT,                              -- set when sealed
    tx_id          TEXT,                              -- set when committed
    error          TEXT
);
CREATE INDEX idx_pending_ops_state ON pending_ops(state);
CREATE INDEX idx_pending_ops_pid   ON pending_ops(pid);
CREATE INDEX idx_pending_ops_tx    ON pending_ops(tx_id);

CREATE TABLE transactions (
    tx_id           TEXT PRIMARY KEY,                 -- ULID
    sealed_at       INTEGER NOT NULL,                 -- unix ns
    committed_at    INTEGER,                          -- unix ns; null if never committed
    state           TEXT NOT NULL DEFAULT 'planned'   -- planned | applied | failed | rolled_back
                       CHECK(state IN ('planned','applied','failed','rolled_back')),
    op_count        INTEGER NOT NULL,
    remove_count    INTEGER NOT NULL,
    pre_itl_sha256  TEXT,                             -- sha256 of ITL just before apply
    post_itl_sha256 TEXT,                             -- sha256 of ITL just after apply (success only)
    pre_itl_backup  TEXT,                             -- absolute path to .bak-<tx_id> snapshot
    inverse_plan    TEXT,                             -- JSON: ops needed to undo this tx
    error           TEXT,
    rolled_back_by  TEXT                              -- tx_id of the rollback Transaction, if any
);
CREATE INDEX idx_transactions_state ON transactions(state);
CREATE INDEX idx_transactions_time  ON transactions(committed_at);
```

### 5.1 Transaction lifecycle

```text
queued ──seal──> sealed ──commit──> committed ─┬─> applied  (terminal, may be rolled_back)
                                               └─> failed   (terminal, never partially applied — see §7)
```

## 6. Inverse Op generation (the "before-state")

To roll back a Transaction we need to reconstruct the live state
before it ran. The trick is doing this **without** keeping the entire
ITL in memory.

For each Op type:

| Forward Op            | Inverse plan                                                                                                                                |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `add_track`           | `remove_track(pid)` — PID is generated at apply time, captured in `inverse_plan` after success.                                             |
| `remove_track`        | Need full track payload to re-add. Captured **at seal time** by reading the live ITL and snapshotting the relevant `mhit` blob to `inverse_plan`. |
| `update_location`     | Capture old `location` at seal time → inverse is `update_location(pid, old_location)`.                                                      |
| `update_meta`         | Capture old metadata fields (only the ones being changed) at seal time → inverse is `update_meta(pid, old_fields)`.                         |
| `external_change`     | Emitted by reconcile; **not invertible** (we only mirror what iTunes already did). Stored for audit, marked `inverse_plan = null`.          |

Seal-time reads are cheap because the live ITL is already memory-mapped
during commit. The inverse plan is JSON-serialized and stored on the
Transaction row, **not** the Op row, because rollback is per-Transaction.

## 7. Commit pipeline (the only path that mutates the ITL)

```text
1. SELECT op_id, payload_json FROM pending_ops WHERE state = 'sealed' ORDER BY enqueued_at;
2. Group sealed Ops into Transactions with hard caps (MaxOps, MaxRemoves). Excess → next-tx.
3. For each Transaction in order:
   a. BEGIN IMMEDIATE on the WAL DB.
   b. INSERT row into transactions (state='planned', pre_itl_sha256, pre_itl_backup).
      pre_itl_backup is a hard link to the live ITL via os.Link (free).
   c. Build inverse_plan by reading current ITL state for each forward Op (see §6).
      Persist inverse_plan to the transactions row.
   d. Call SafeWriteITL with the forward Op set. SafeWriteITL already does
      backup → write-temp → validate → rename → validate (PR-original behavior).
   e. On success:
        UPDATE transactions SET state='applied', committed_at=now, post_itl_sha256=...;
        UPDATE pending_ops  SET state='committed', tx_id=...;
        COMMIT.
   f. On any error in (d):
        UPDATE transactions SET state='failed', error=...;
        UPDATE pending_ops  SET state='rejected', tx_id=..., error=...;
        COMMIT (so the failure is recorded), then ABORT the loop.
        SafeWriteITL guarantees the ITL is unchanged or restored from .bak.
4. fsync the WAL DB.
```

**Crash invariants** (verified by the startup recovery routine):

- A Transaction in state `planned` with no matching ITL change ⇒
  reset to `failed`, mark its Ops `rejected` with reason
  `"crash_during_apply"`. SafeWriteITL's atomic rename guarantees the
  ITL is in a known-valid state.
- Ops in state `committed` whose Transaction is missing ⇒ impossible
  by FK; assert + abort startup.
- Ops in state `sealed` with no batch_id ⇒ revert to `queued`.

## 8. Rollback

```text
Rollback(tx_id):
  1. SELECT * FROM transactions WHERE tx_id = ? AND state = 'applied'.
     Else error.
  2. Build forward Ops from inverse_plan.
  3. Wrap into a NEW Transaction (state='planned', annotated as rollback-of-X).
  4. Run the standard commit pipeline against this new Transaction.
  5. On success:
       UPDATE original tx SET state='rolled_back', rolled_back_by=<new tx_id>.
```

A rollback Transaction itself can be rolled back (chain support). The
`rolled_back_by` chain is auditable.

## 9. Two-way sync (reconcile)

`ReconcileFromITL()`:

1. Parse the live ITL → enumerate (PID, location, mod-time, key meta).
2. Diff against `book_files` rows (the same table the existing batcher
   reads).
3. For each delta:
   - **PID present in ITL, missing in DB**: emit `external_change`
     Op with payload `{kind: "user_added", pid, location, meta}`.
     Handler in the post-commit hook re-imports the track.
   - **PID in DB, missing in ITL**: emit `external_change` Op with
     payload `{kind: "user_removed", pid}`. Handler clears the PID
     in book_files (does NOT delete the book — user removed it from
     iTunes, not from the library).
   - **Location/meta differs**: emit `external_change` payload
     `{kind: "user_edited", pid, fields: {...}}`. Handler updates the
     DB to match. **Does not write back** — the change came from
     iTunes, writing it back would be a no-op and a re-entry risk.
4. Wrap all `external_change` Ops in a single audit-only Transaction
   (state='applied' immediately, no SafeWriteITL call, no inverse_plan).

Reconcile runs:

- On service startup (after WAL recovery completes).
- On a 15-minute timer (configurable).
- On demand via `POST /api/v1/itunes/reconcile`.

## 10. HTTP surface

| Method | Path                                            | Auth scope             | Purpose                                                                  |
| ------ | ----------------------------------------------- | ---------------------- | ------------------------------------------------------------------------ |
| GET    | `/api/v1/itunes/wal/transactions`               | LibraryView            | List recent Transactions (paginated, filter by state).                   |
| GET    | `/api/v1/itunes/wal/transactions/:tx_id`        | LibraryView            | Detail incl. ops + inverse_plan summary.                                 |
| POST   | `/api/v1/itunes/wal/transactions/:tx_id/rollback` | LibraryEditMetadata + admin | Roll back a Transaction. Body `{confirm: "ROLLBACK", dry_run: bool}`. |
| POST   | `/api/v1/itunes/wal/reconcile`                  | LibraryEditMetadata    | Manual reconcile. Body `{dry_run: bool}`.                                |
| GET    | `/api/v1/itunes/wal/pending`                    | LibraryView            | Backlog stats: queued / sealed counts, oldest age.                       |

All write endpoints honor `ITUNES_WRITEBACK_DRYRUN` (logs but no ITL change).

## 11. Migration

1. Ship the WAL schema + Transaction commit pipeline behind feature flag
   `itunes.wal_enabled` (default: false).
2. With flag off, the existing debounce batcher runs unchanged. With
   flag on, every `Enqueue*` call dual-writes (memory queue + pending_ops
   row) and the commit timer drives flush instead of the debounce timer.
3. Soak in dry-run for one full week.
4. Default the flag to true; remove the debounce code path in a followup
   PR after another week.

No DB-schema changes outside the new SQLite file. `book_files` is
untouched.

## 12. Out of scope (future)

- Multi-host write-back (the WAL is single-host; HA needs a different design).
- A per-user UI for browsing Transaction history (the JSON endpoints
  are enough for the operator dashboard initially).
- Generic undo for non-iTunes mutations (book deletes, metadata edits).
  The WAL pattern would extend cleanly but is a separate scope.

## 13. Testing strategy

| Layer        | Tests                                                                                                             |
| ------------ | ----------------------------------------------------------------------------------------------------------------- |
| Unit         | Per-Op-type seal-time inverse-plan correctness against in-memory ITL fixture.                                     |
| Unit         | Commit pipeline simulated crash between (d) and (e) — verify recovery sets state=failed, ITL unchanged.           |
| Unit         | Rollback chain: apply tx A, rollback A → A2, rollback A2 → A3 = same state as after A.                            |
| Integration  | Real SafeWriteITL against a copy of the golden master; flush 10 transactions, rollback the middle one, verify ITL state matches replay-without-middle. |
| Integration  | Reconcile round-trip: edit a track in a copy of the live ITL via raw write, run ReconcileFromITL, assert the DB updated and no write-back fires. |
| Property     | Generated random Op sequences applied vs replayed must produce byte-identical post_itl_sha256.                    |

## 14. Implementation breakdown

The bot-task split lives in `docs/superpowers/bot-tasks/2026-05-02-wal-*.md`
(to be created). Outline:

1. `wal-1-schema-and-store` — SQLite schema, store wrapper, migration runner.
2. `wal-2-pending-ops-dual-write` — wire `Enqueue*` to also write `pending_ops`, behind feature flag.
3. `wal-3-seal-and-commit-pipeline` — batch sealer + Transaction committer, hooked into existing flush().
4. `wal-4-inverse-plan-generation` — per-Op-type before-state capture.
5. `wal-5-rollback-endpoint` — HTTP + handler.
6. `wal-6-reconcile-from-itl` — diff + external_change Ops + handlers.
7. `wal-7-startup-recovery` — replay sealed/committed-but-unapplied state.
8. `wal-8-operator-endpoints` — GET endpoints, pending stats.
9. `wal-9-default-flag-on` — flip default, observability check.
10. `wal-10-remove-debounce-path` — delete the legacy code.

Each bot-task ≤ 150 LOC of changes, with explicit prereqs and tests.
