<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-06-stage-filename-match.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9679e35a-1c84-4b17-baf0-d4cc69f49f99 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: filename / path heuristics

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
feat/dedup-pipeline-p1-06-stage-filename-match
```

## Label

```bash
gh label create "task:PIPE-P1-06" --color "1d76db" --description "Bot task: Pipeline stage: filename / path heuristics" 2>/dev/null || true
```

## What This Does

Lowest-weight signal. Emits `KindFilenameMatch` for any pair of books whose normalized basenames or parent dir names exceed Jaccard 0.7.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_filename_match.go`
2. **Create** `internal/maintenance/jobs/stage_filename_match_test.go`

## Implementation outline

- Reuses `strings.ToLower` + non-alnum stripping logic already used in
  the legacy `dedup-books` job.
- Confidence = 0.40.


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageFilenameMatch
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
  --title "feat(dedup): pipeline stage: filename / path heuristics (PIPE-P1-06)" \
  --body "Implements PIPE-P1-06 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-06"
```
