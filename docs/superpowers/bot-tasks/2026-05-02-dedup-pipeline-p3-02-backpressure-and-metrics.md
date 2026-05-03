<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p3-02-backpressure-and-metrics.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7027fea4-4aed-4d35-a829-c1901dfcb186 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Per-stage concurrency caps + Prometheus metrics

**Pipeline phase:** P3
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P3-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P3-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P3-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p3-02-backpressure-and-metrics
```

## Label

```bash
gh label create "task:PIPE-P3-02" --color "1d76db" --description "Bot task: Per-stage concurrency caps + Prometheus metrics" 2>/dev/null || true
```

## What This Does

Adds the per-stage worker pools (spec §5.1) and Prometheus metrics:
`pipeline_stage_duration_seconds{stage}`, `pipeline_signal_total{kind}`,
`pipeline_identity_score_bucket`, `pipeline_match_group_size_bucket`.

## Files to Create / Edit

1. **Edit** `internal/metrics/metrics.go` — register the four metrics.
2. **Edit** the coordinator from P3-01 — bound concurrency per stage via a
   `chan struct{}` semaphore initialized from config.
3. **Edit** `internal/config/config.go` — add `Pipeline.StageWorkers map[string]int`.

## Definition of Done

- [ ] Default worker counts: `sha256-full=NumCPU`, `chromaprint=max(1,NumCPU/2)`,
      `whisper=2`, `acoustid=1`.
- [ ] Metrics registered, exposed at `/metrics`.
- [ ] Test verifies an over-saturated queue blocks rather than DoSing the API.


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): per-stage concurrency caps + prometheus metrics (PIPE-P3-02)" \
  --body "Implements PIPE-P3-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P3-02"
```
