<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p1-05-stage-tag-match.md -->
<!-- version: 1.0.0 -->
<!-- guid: b354480e-b5df-43c0-be61-85680df25a7f -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Pipeline stage: tag-based pairwise comparison

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
feat/dedup-pipeline-p1-05-stage-tag-match
```

## Label

```bash
gh label create "task:PIPE-P1-05" --color "1d76db" --description "Bot task: Pipeline stage: tag-based pairwise comparison" 2>/dev/null || true
```

## What This Does

Walks each book's title/author/narrator/duration/track-count and emits `KindTagMatch` signals against any other book in the library that is plausibly a match. Reuses normalized comparators in `internal/dedup/helpers.go`.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/stage_tag_match.go`
2. **Create** `internal/maintenance/jobs/stage_tag_match_test.go`

## Implementation outline

- Title comparator: existing normalized Levenshtein in
  `internal/dedup/helpers.go`.
- Duration must match within ±2 % (configurable threshold;
  `settings.dedup.duration_tolerance`).
- Score = weighted blend (title 0.5, author 0.25, narrator 0.15, duration 0.10).
- Confidence = 0.75 (constant).


## Verify

```bash
go test ./internal/maintenance/jobs/... -run StageTagMatch
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
  --title "feat(dedup): pipeline stage: tag-based pairwise comparison (PIPE-P1-05)" \
  --body "Implements PIPE-P1-05 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P1-05"
```
