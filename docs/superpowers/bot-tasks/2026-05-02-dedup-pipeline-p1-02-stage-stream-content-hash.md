<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-02-stage-stream-content-hash.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8569d9f1-92b6-437b-89a0-0b6464765fd3 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: per-stream audio content hash

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
feat/dedup-pipeline-p1-02-stage-stream-content-hash
```

## Label

```bash
gh label create "task:PIPE-P1-02" --color "1d76db" --description "Bot task: Pipeline stage: per-stream audio content hash" 2>/dev/null || true
```

## What This Does

Hashes only the audio stream payload (skipping container/metadata) so two files that are identical re-encodes still match. Uses ffmpeg (`ffmpeg -i in.m4b -map 0:a -c:a copy -f md5 -`).

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_stream_content_hash.go`
2. **Create** `internal/maintenance/jobs/stage_stream_content_hash_test.go`

## Implementation outline

- Shell out to ffmpeg; parse the MD5 line.
- Emit `KindStreamContentHash` with score=1.0, confidence=1.0 when matched.
- Failure: ffmpeg missing → emit nothing; mark `stage_unavailable` metric.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageStreamContentHash
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
  --title "feat(dedup): pipeline stage: per-stream audio content hash (PIPE-P1-02)" \
  --body "Implements PIPE-P1-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-02"
```
