<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-03-stage-chromaprint-segments.md -->
<!-- version: 1.0.0 -->
<!-- guid: e2938b61-22e8-49d7-9c62-ea740aff7321 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: chromaprint segments

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
feat/dedup-pipeline-p1-03-stage-chromaprint-segments
```

## Label

```bash
gh label create "task:PIPE-P1-03" --color "1d76db" --description "Bot task: Pipeline stage: chromaprint segments" 2>/dev/null || true
```

## What This Does

Calls `internal/fingerprint.FileSegments` (existing) to compute the 7 segments. Stores them on the FingerprintStore record AND emits one `KindChromaprintSegment` signal per segment plus one `KindChromaprintFull`.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_chromaprint_segments.go`
2. **Create** `internal/maintenance/jobs/stage_chromaprint_segments_test.go`

## Implementation outline

- Use `fingerprint.Available()` to gate the stage.
- For each segment: also call `fpStore.LookupByChromaprintSegment` and, if a
  match is found, emit a per-pair signal carrying the matched SHA in
  `EvidenceJSON`. The matrix uses these to build match groups.
- Failure: backend missing → stage marks itself unavailable; matrix degrades.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageChromaprintSegments
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
  --title "feat(dedup): pipeline stage: chromaprint segments (PIPE-P1-03)" \
  --body "Implements PIPE-P1-03 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-03"
```
