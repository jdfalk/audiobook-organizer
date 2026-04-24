# AI Jobs Batch Migration — Design

**Date:** 2026-04-24
**Branch:** `feat/aijobs-batch-migration`
**Status:** Design approved, pending implementation plan

## Problem

Large-scale LLM work (dedup review, metadata review, bulk scanner pipeline) calls the OpenAI synchronous `chat/completions` endpoint and hits 429 `insufficient_quota` on maintenance runs. The repo already has mature batch-API infrastructure (`internal/ai/openai_batch.go` + `internal/server/batch_poller.go`) used by `author_dedup`, `author_review`, `diagnostics`, and `pipeline` flows, but four code paths still call `client.Chat.Completions.New` synchronously and drive bulk workloads through them.

Goal: **no bulk-scale LLM work touches the synchronous chat-completions endpoint.** Interactive, single-item, user-is-waiting flows keep the sync path.

## Non-Goals

- Quota-aware throttling — quota exhaustion is an operator concern, surfaced via alerts.
- Automatic retry on submission failure — operator retriggers.
- Cross-batch deduplication of in-flight work items — caller responsibility.
- Migrating or refactoring the existing batch flows that already work (`author_dedup`, `author_review`, `diagnostics`, `pipeline`) — they stay as-is; we add a layer beside them.

## Key Design Decisions

| Decision | Choice | Reason |
|---|---|---|
| Scope | All sync sites, sync allowed only for declared `Interactive` callers | Matches rule "any large-scale job uses batch" while preserving latency for UI flows |
| Routing | Caller declares intent (`Interactive` \| `Bulk`) at each call site | Auditable via grep; immune to "N loops of 1-item sync" bypass bug |
| Result handling | Hybrid: unified `ai_jobs` tracking table + per-feature `Apply(item, result)` callback | Centralizes observability/status; feature-specific result logic stays local |
| Error handling | Fail-loud at job level, per-row tolerance within a batch | Partial progress on 10k-row batches; operator attention for submission failures |

## Architecture

```
┌─ Caller (Bulk) ─┐     ┌─ aijobs.Submit(type, items, callback) ─┐
│ dedup_review    │────▶│  1. Insert ai_jobs row (pending)       │
│ metadata_review │     │  2. Build JSONL, upload, create batch  │
│ openai_parser   │     │  3. Update row: batch_id, submitted    │
└─────────────────┘     └────────────────────────────────────────┘
                                         │
                                         ▼
                        ┌─ BatchPoller (existing) ─┐
                        │  On completion:          │
                        │  - Download results      │
                        │  - Dispatch by metadata  │
                        │    type → aijobs.Dispatch│
                        └──────────────────────────┘
                                         │
                                         ▼
                ┌─ aijobs.Dispatch (generic) ─┐
                │  1. Parse rows              │
                │  2. For each row:           │
                │     recover + Apply(row)    │
                │     on err: append to       │
                │     row_errors (cap 100)    │
                │  3. Update ai_jobs row      │
                └─────────────────────────────┘
```

### Components

**1. `ai_jobs` table (migration 52) — tracking only**

```
id              TEXT PK         (ULID)
type            TEXT NOT NULL   (e.g., "dedup_review", "metadata_review")
batch_id        TEXT            (OpenAI batch id; null until submitted)
custom_id_prefix TEXT NOT NULL  (for filtering result rows)
status          TEXT NOT NULL   (pending|submitted|completed|completed_with_errors|failed|expired)
item_count      INT NOT NULL
success_count   INT             (populated on completion)
error_count     INT             (populated on completion)
row_errors      JSONB           ([{custom_id, error, raw_ref}], cap 100)
error_msg       TEXT            (job-level failure message)
submitted_at    TIMESTAMP
completed_at    TIMESTAMP
created_at      TIMESTAMP NOT NULL
```

Index on `(status, created_at)` for "in-flight jobs" queries and on `(type, created_at)` for per-feature history.

**2. `internal/ai/aijobs` package**

- `Submit[Item, Result](ctx, type, items, build, apply) (jobID, err)` — generic entry point
  - `build(item) openai.BatchRequestInputItem` — produces one JSONL row
  - `apply(item, result) error` — called at completion with the parsed response for that custom_id
- Internal registry maps `type` string → `apply` callback (registered on package init or explicit `aijobs.Register`)
- `Dispatch(ctx, batchID, outputFileID)` — wired into `BatchPoller` as a single new handler type `"aijobs"`. Reads the ai_jobs row by batch_id, loads items from persisted payload ref, runs apply per row.

**3. Payload persistence**

Items submitted to `aijobs.Submit` must be recoverable at dispatch time (minutes/hours later, possibly after server restart). Store the serialized item slice in an object-store-like location keyed by job_id. Simplest for this repo: a new `ai_job_payloads` table with `(job_id, items_json BLOB)`. Feature callbacks deserialize their specific `Item` type.

**4. `BatchPoller` wiring**

Register a single new handler in `batch_poller.go`:

```go
s.batchPoller.RegisterHandler("aijobs", func(ctx context.Context, batchID, outputFileID string) error {
    return aijobs.Dispatch(ctx, store, parser, batchID, outputFileID)
})
```

All unified-layer batches carry `metadata.type == "aijobs"` and a sub-type in `metadata.subtype` (or encoded in custom_id_prefix). The `aijobs` dispatcher reads the `ai_jobs` row for the actual feature type and routes to the registered callback.

### Caller Intent Contract

Every `client.Chat.Completions.New` call site must either:
- Live in an `Interactive*`-named function whose godoc says "sync, user-waiting flow", OR
- Live behind `aijobs.Submit(...)`.

No function offers both in the same path. The enforcement test (`TestNoUnmarkedChatCompletionCallers`) greps `internal/ai/**/*.go` for `Chat.Completions.New` and fails CI if any call site is outside the allow-list or lacks a `// PRIORITY: Interactive` marker on the enclosing function.

### Migration Mapping (8 sync sites)

| File:Line | Current caller | Priority | Action |
|---|---|---|---|
| `dedup_review.go:124` | Maintenance `dedup_llm_review` | Bulk | Migrate (reference impl) |
| `metadata_llm_review.go:147` | Batch metadata apply | Bulk | Migrate |
| `openai_parser.go:136` (ParsePath) | Scan pipeline | Bulk | Migrate |
| `openai_parser.go:238` (ParseSeries) | Scan pipeline | Bulk | Migrate |
| `openai_parser.go:329` (ParseMetadata) | Scan pipeline | Bulk | Migrate |
| `openai_parser.go:391` (ParseCoverImage) | Scan pipeline + UI | **Split** into `ParseCoverImageInteractive` + `ParseCoverImageBulk` |
| `openai_parser.go:544` | TBD (Phase 1.4 audit) | Bulk (likely) | Migrate |
| `openai_parser.go:678` | TBD (Phase 1.4 audit) | Bulk (likely) | Migrate |

## Error Handling

**Submission failures (quota, auth, malformed JSONL):** `aijobs.Submit` returns error synchronously, marks row `failed`, emits audit log, surfaces in Diagnostics. No auto-retry.

**Per-row errors:** `Dispatch` wraps every `apply(item, result)` in `defer recover() + err capture`. Errors append to `row_errors` (cap 100; overflow counted in `error_count`). Successful rows commit per-row — no all-or-nothing. Final status `completed` if `error_count == 0`, else `completed_with_errors`.

**Batch expiry (24h):** Poller marks `expired`, no retry.

## Observability

- Structured logs at each state transition (`submit`, `submitted`, `completed`, `row_error`, `failed`) with `job_id`, `type`, `item_count`.
- `GET /api/v1/ai-jobs` — list rows, filters by `type`/`status`.
- Diagnostics page: "AI Jobs" panel — in-flight count, last 50 jobs, expand for row errors.

## Testing Strategy

**Unit (per feature):** Build round-trip, Apply success + malformed-result paths; mock `aijobs.Submit` to verify callers pass correct type/items.

**Unit (aijobs package):** Submit inserts row → uploads → creates batch → updates row (mock OpenAI); Dispatch fixture with 98 success + 2 apply errors → correct counts + row_errors; panic in apply is recovered; row_errors cap; terminal states (`failed`, `expired`) handled.

**Integration:** End-to-end with mock OpenAI server — submit dedup batch → poller → dispatch → DB state matches golden fixture.

**Enforcement:** `TestNoUnmarkedChatCompletionCallers` as described above.

**Out of scope:** no live OpenAI in CI; no load tests; no UI Playwright beyond existing render test.

## Work Decomposition

Sequential phases; tasks within a phase run in parallel where marked.

### Phase 1 — Foundation + reference migration (serial)

- **1.1** Migration 52 (`ai_jobs` + `ai_job_payloads` tables) + `internal/database/ai_jobs_store.go` accessor.
- **1.2** `internal/ai/aijobs` package: `Submit`, `Register`, `Dispatch`, in-memory registry. Wire as `aijobs` handler in `batch_poller.go`.
- **1.3** `TestNoUnmarkedChatCompletionCallers` (allow-list initially contains all 8 current sites; tightens as migrations land).
- **1.4** Audit `openai_parser.go` lines 544 & 678 — trace callers, confirm priority, record findings at the bottom of this design doc.
- **1.5** Migrate `dedup_review.go` as the reference implementation (sync path deleted; Bulk path calls `aijobs.Submit`; unit tests; allow-list updated).

### Phase 2 — Parallel migrations (3 Haiku agents)

Each agent copies the pattern from Phase 1.5 (`dedup_review.go` is the sibling template):

- **2.1** Migrate `metadata_llm_review.go`.
- **2.2** Migrate `openai_parser.go` sites 136 / 238 / 329.
- **2.3** Migrate `openai_parser.go` site 391 (with interactive/bulk split) and 544 / 678 per Phase 1.4 findings.

Each task is self-contained: read the sibling Build/Apply, mirror it for the target feature, delete (or split) the sync path, add a unit test, update the allow-list.

### Phase 3 — Observability + cleanup (serial)

- **3.1** `GET /api/v1/ai-jobs` endpoint + handler.
- **3.2** Diagnostics UI "AI Jobs" panel (React/TS).
- **3.3** Audit `internal/server/ai_scan_pipeline.go` and `bench.go` for remaining direct `client.Batches.New` calls — migrate to `aijobs.Submit` where appropriate or document why raw stays.
- **3.4** End-to-end integration test with mock OpenAI.

## Open Items (resolved during Phase 1.4)

### Phase 1.4 audit results

- `openai_parser.go:544` — function: `reviewAuthorBatch`. Callers: `internal/server/ai_handlers.go:571`, `internal/server/ai_scan_pipeline.go:409`. **Status: OUT OF SCOPE.** These are sync fallback helpers for the existing `author_dedup` batch flow (see `CreateBatchAuthorDedup` in openai_batch.go). The batch flow itself is explicitly excluded from migration per the Non-Goals section. Their allow-list entries are permanent.

- `openai_parser.go:678` — function: `discoverAuthorBatch`. Callers: `internal/server/ai_handlers.go:635`, `internal/server/ai_scan_pipeline.go:470`, `internal/server/ai_scan_pipeline.go:680`. **Status: OUT OF SCOPE.** These are sync fallback helpers for the existing `author_review` batch flow (see `CreateBatchAuthorReview` in openai_batch.go). The batch flow itself is explicitly excluded from migration per the Non-Goals section. Their allow-list entries are permanent.
