<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p5-03-match-groups-table.md -->
<!-- version: 1.0.0 -->
<!-- guid: 75605846-4d6d-42c0-8aea-9274511b9e65 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Match-groups table with inline resolve

**Pipeline phase:** P5
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P5-01` — must be merged before this task starts
- `task:P4-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P5-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P5-01"; exit 0; }
count=$(gh pr list --label "task:P4-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P4-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p5-03-match-groups-table
```

## Label

```bash
gh label create "task:PIPE-P5-03" --color "1d76db" --description "Bot task: Match-groups table with inline resolve" 2>/dev/null || true
```

## What This Does

Adds the match-groups table: filters by `strongest_kind` / score range,
expandable rows with the per-pair signal breakdown, and a one-click resolve
dropdown (merge / dismiss / split).

## Definition of Done

- [ ] Server-side cursor pagination
- [ ] Optimistic UI on resolve (rollback on 4xx/5xx)
- [ ] E2E Playwright spec for the merge happy path


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): match-groups table with inline resolve (PIPE-P5-03)" \
  --body "Implements PIPE-P5-03 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P5-03"
```
