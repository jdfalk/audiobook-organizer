<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p3-01-pipeline-coordinator-job.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d53336-fc72-4a8d-a457-c7b729e3ac89 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: identification-pipeline coordinator job

**Pipeline phase:** P3
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P2-01` — must be merged before this task starts
- `task:P2-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P2-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P2-01"; exit 0; }
count=$(gh pr list --label "task:P2-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P2-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p3-01-pipeline-coordinator-job
```

## Label

```bash
gh label create "task:PIPE-P3-01" --color "1d76db" --description "Bot task: identification-pipeline coordinator job" 2>/dev/null || true
```

## What This Does

A self-registering `MaintenanceJob` named `identification-pipeline` that
orchestrates Phase-1 stages for a book (or for the whole library), gathers
their `signals.Signal` output, runs the matrix, persists `IdentityResult` and
`MatchPair`s, and schedules Stage 9.

## Files to Create / Edit

1. **Create** `internal/maintenance/jobs/identification_pipeline.go`
2. **Create** `internal/maintenance/jobs/identification_pipeline_test.go`

## Implementation outline

```go
type IdentificationPipelineParams struct {
    BookID string `json:"book_id,omitempty"`
    Since  string `json:"since,omitempty"` // RFC3339; library mode
    Stages []string `json:"stages,omitempty"` // optional whitelist
}

func (j *IdentificationPipelineJob) Run(ctx context.Context, reporter operations.ProgressReporter, raw json.RawMessage, startFrom int) error {
    // 1. Resolve target books.
    // 2. For each book: skip stages whose output already exists for the current
    //    signal_revision; enqueue the rest; wait via a per-book sync.WaitGroup
    //    with a hard timeout.
    // 3. Gather signals via signals.Store.ListByBook(bookID).
    // 4. matrix.ComputeIdentity → write identity_results row.
    // 5. matrix.ComputeMatchPairs → matrix.ApplyPairs(groups, pairs).
    // 6. If identity_score < 0.85 and Whisper stage hasn't run, enqueue it
    //    and re-run matrix when its signal arrives (single retry only).
    // 7. Schedule Stage 9 trust-ladder action via P7-01 hook (no-op until P7).
}
```

The coordinator MUST NOT compute fingerprints itself — it only orchestrates
the per-stage jobs.

## Tests must cover

- Single-book run with all stages mocked → identity_results row written
- Library mode with `Since` filter → only books updated after the cutoff
- Re-run with same `signal_revision` is a no-op
- Whisper gating: stage skipped when identity_score from cheap stages ≥ 0.85
- Cancel: respects `reporter.IsCanceled()`

## Definition of Done

- [ ] Job appears in `GET /api/v1/maintenance/jobs`
- [ ] Resumable (CanResume=true) with checkpoint at end of each book
- [ ] All Phase-1 stage jobs are dispatched through the operations queue


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): identification-pipeline coordinator job (PIPE-P3-01)" \
  --body "Implements PIPE-P3-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P3-01"
```
