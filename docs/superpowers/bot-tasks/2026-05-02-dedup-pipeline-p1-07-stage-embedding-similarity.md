<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-07-stage-embedding-similarity.md -->
<!-- version: 1.0.0 -->
<!-- guid: 56aa40d0-3dad-461e-982f-1e9bbb50b977 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: chromem embedding similarity

**Pipeline phase:** P1
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-04` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-04" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-04"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p1-07-stage-embedding-similarity
```

## Label

```bash
gh label create "task:PIPE-P1-07" --color "1d76db" --description "Bot task: Pipeline stage: chromem embedding similarity" 2>/dev/null || true
```

## What This Does

Reuses the existing `EmbeddingStore` / `ChromemEmbeddingStore` (`internal/database/embedding_store.go`, `chromem_embedding_store.go`). Emits one `KindEmbeddingSimilarity` per pair with cosine ≥ 0.80.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_embedding_similarity.go`
2. **Create** `internal/maintenance/jobs/stage_embedding_similarity_test.go`

## Implementation outline

- Lookup vector for the book; if missing, request a backfill via
  existing `embedding_backfill` machinery.
- Confidence = 0.65.
- Failure: chromem store unavailable → emit nothing.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageEmbeddingSimilarity
go vet ./internal/maintenance/jobs/...
```

## Definition of Done

- [ ] Job registers itself via `init()` (`maintenance.Register(&Job{})`)
- [ ] Job emits exactly one `signals.Signal` per `(book_id, kind, value)` it sees
- [ ] Failure mode (see master spec §3.4) is handled — never panics, never blocks the pipeline
- [ ] Test asserts: (a) no signal on empty input, (b) correct signal on happy path, (c) graceful no-op on missing dependency
- [ ] `make build-api` succeeds


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): pipeline stage: chromem embedding similarity (PIPE-P1-07)" \
  --body "Implements PIPE-P1-07 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-07"
```
