<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-01-stage-sha256-full.md -->
<!-- version: 1.0.0 -->
<!-- guid: 18e1deb3-05a6-4199-8f6c-7aa1333ccf1c -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: full SHA-256 file hash

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
feat/dedup-pipeline-p1-01-stage-sha256-full
```

## Label

```bash
gh label create "task:PIPE-P1-01" --color "1d76db" --description "Bot task: Pipeline stage: full SHA-256 file hash" 2>/dev/null || true
```

## What This Does

Implements the mandatory SHA-256 stage. Reuses `scanner.ComputeFileHash` (`internal/scanner/scanner.go:1761`) — DO NOT reimplement chunking. Emits `KindSHAExact` with score=1.0, confidence=1.0. Also calls `fingerprintStore.Upsert` so the forever-store sees this SHA.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_sha256_full.go`
2. **Create** `internal/maintenance/jobs/stage_sha256_full_test.go`

## Implementation outline

- Inject `database.FingerprintStore` and `*signals.Store` via the
  job's `InjectStore`-style hook (extend `internal/maintenance/job.go` if
  needed; see existing `EnqueuerInjectable` pattern).
- For each book: compute SHA → `Upsert` into FingerprintStore →
  `Insert` a Signal{Kind: KindSHAExact, Value: sha, Score: 1.0, Confidence: 1.0,
  Source: "sha256-full"}.
- Failure: file unreadable → log warn, no signal emitted, pipeline continues.
- Failure: 50 GB file → already streamed by ComputeFileHash, no special handling needed.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageSha256Full
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
  --title "feat(dedup): pipeline stage: full sha-256 file hash (PIPE-P1-01)" \
  --body "Implements PIPE-P1-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-01"
```
