<!-- file: docs/superpowers/bot-tasks/2026-05-04-async-embed-batch-api.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9d3f8c12-6b54-4a7e-b29c-8f1d5e3a4b7c -->
<!-- last-edited: 2026-05-04 -->

# BOT TASK: Use OpenAI Batch API for nightly embedding re-scans

## Branch

```
feat/embed-batch-api
```

## Problem

`Engine.FullScan` (post PR #698) batches embedding requests into 64-input
chunks against the synchronous `/v1/embeddings` endpoint. That is the right
shape for interactive paths (metadata scorer, dedup-on-import) but wasteful
for the once-a-night "re-embed every primary book" workload:

- Sync requests pay full price even at 1AM when latency does not matter.
- A 10K-book FullScan still ties up an OpenAI request slot for several
  minutes and competes with foreground traffic.

OpenAI's Batch API supports `endpoint=/v1/embeddings` with the same
50% discount and 24h SLA used today for chat completions. We already have
the universal batch poller infrastructure (it routes results by metadata
tag) — embeddings would slot in alongside dedup-LLM and metadata batches.

## Goal

Add an async embed path that submits the full primary-book set as one
Batch job, polls for completion via the existing universal poller, and
writes results back to the `embeddings` table + content-hash cache.

The synchronous `EmbedBatch` path stays for interactive callers.

## Files to add / change

- **NEW** `internal/ai/embedding_batch.go` — JSONL build / submit / parse
  helpers mirroring `internal/ai/openai_batch.go` but pinned to
  `BatchNewParamsEndpointV1Embeddings`.
- **EDIT** `internal/dedup/engine.go` — add `EmbedBooksAsync(ctx, ids)`
  that returns a batch ID instead of vectors.
- **EDIT** universal batch poller (search `metadata tag` routing) — add a
  handler that, on completion, decodes embedding JSONL, joins results to
  book IDs (via custom_id), upserts the embeddings table, and mirrors to
  chromem.
- **NEW** `POST /api/v1/dedup/embed-async` route — creates a tracked
  Operation that submits the batch and returns immediately.
- **NEW** scheduled job `nightly-embed-refresh` (cron-like, opt-in) that
  invokes the async path during quiet hours.

## Constraints

- Batch limits: 50K requests / 200MB upload per batch. 10K books × ~120
  bytes/request fits in one batch with room to spare. If the library
  grows past ~40K, chunk into multiple batches keyed by ULID prefix.
- Custom IDs: each JSONL line's `custom_id` MUST be the book ID — that is
  the only way the result handler can join vectors back to books.
- The content-hash cache key (`text_hash`) is computed at submit time so
  the result handler does not need to re-derive the text. Store the hash
  alongside the custom_id (e.g., `bookID|hash`) or in a sidecar map keyed
  by batch ID.
- Per-book hash check still applies: if `book.text_hash` matches at
  submit time, do NOT include the book in the batch. The async path is
  for *new* embeddings only, same as sync.
- The Operation row must be resumable: store the batch ID in
  `result_data` so a server restart can re-attach to the in-flight batch
  via the universal poller instead of dropping it on the floor.

## Open questions

1. Cron syntax vs. manual nightly trigger: integrate with the existing
   scheduler (search `internal/scheduler` / `internal/scheduled`) or
   add a one-off ticker. Prefer the existing scheduler.
2. Should the sync FullScan stay primary, or auto-fall back to async
   when the queue is over a threshold? Probably keep them separate to
   avoid surprises — the user picks.

## Test strategy

- Unit: a fake batch client returns canned JSONL; assert vectors land in
  the embeddings table and chromem mirror, with correct text_hash.
- Integration: submit a batch with 3 books, mock poller fires completion
  after a tick, assert the Operation transitions queued → running →
  completed and the books are now embedded.
- Resume test: kill the server mid-batch, restart, assert the universal
  poller picks up the batch ID from `operations.result_data` and
  completes the write-back.

## Rollback

Additive route + new job. Revert the branch — sync path is untouched.
