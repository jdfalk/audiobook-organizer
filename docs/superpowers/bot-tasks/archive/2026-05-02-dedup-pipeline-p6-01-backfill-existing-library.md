<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p6-01-backfill-existing-library.md -->
<!-- version: 1.0.0 -->
<!-- guid: fd6a02a1-701e-49fc-a348-fc1f36afe382 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Backfill pipeline for every existing book

**Pipeline phase:** P6
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
feat/dedup-pipeline-p6-01-backfill-existing-library
```

## Label

```bash
gh label create "task:PIPE-P6-01" --color "1d76db" --description "Bot task: Backfill pipeline for every existing book" 2>/dev/null || true
```

## What This Does

A one-shot maintenance job (`pipeline-backfill`) that enqueues the
`identification-pipeline` for every book whose `signal_revision = 0`, in
batches of 50 with a configurable inter-batch sleep so the operation queue
isn't starved.

## Definition of Done

- [ ] Resumable
- [ ] Reports progress (`current/total`) on each batch boundary
- [ ] Once finished, every book has a non-zero `signal_revision` and at least
      one row in `identity_results`


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): backfill pipeline for every existing book (PIPE-P6-01)" \
  --body "Implements PIPE-P6-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P6-01"
```
