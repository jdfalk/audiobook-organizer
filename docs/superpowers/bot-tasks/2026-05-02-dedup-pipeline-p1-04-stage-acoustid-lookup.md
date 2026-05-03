<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-04-stage-acoustid-lookup.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6e828151-1c37-4921-991b-d42c67b7d913 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: AcoustID external lookup

**Pipeline phase:** P1
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-04` — must be merged before this task starts
- `task:P1-03` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-04" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-04"; exit 0; }
count=$(gh pr list --label "task:P1-03" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-03"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p1-04-stage-acoustid-lookup
```

## Label

```bash
gh label create "task:PIPE-P1-04" --color "1d76db" --description "Bot task: Pipeline stage: AcoustID external lookup" 2>/dev/null || true
```

## What This Does

Submits the chromaprint full fingerprint to acoustid.org and records the returned MBID + score. Throttled and retried via `internal/ai/aijobs` job runner.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_acoustid_lookup.go`
2. **Create** `internal/maintenance/jobs/stage_acoustid_lookup_test.go`

## Implementation outline

- Skip if no `chromaprint_full` exists for the book yet.
- Skip if `last_seen_at` < 24h (cache).
- Failure: HTTP 429 → exponential backoff, signal stays unemitted; matrix proceeds.
- Failure: HTTP 5xx > 3 retries → log warn, no signal emitted.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageAcoustidLookup
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
  --title "feat(dedup): pipeline stage: acoustid external lookup (PIPE-P1-04)" \
  --body "Implements PIPE-P1-04 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-04"
```
