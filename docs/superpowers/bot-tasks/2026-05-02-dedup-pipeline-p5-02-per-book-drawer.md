<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p5-02-per-book-drawer.md -->
<!-- version: 1.0.0 -->
<!-- guid: fec85726-bd07-4e4e-ac7e-430c1f26d140 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Per-book identification drawer

**Pipeline phase:** P5
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P5-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P5-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P5-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p5-02-per-book-drawer
```

## Label

```bash
gh label create "task:PIPE-P5-02" --color "1d76db" --description "Bot task: Per-book identification drawer" 2>/dev/null || true
```

## What This Does

Adds a drawer that opens from any book card showing the per-stage timeline,
each signal as a chip with score / confidence / weight, and a "Recompute…"
menu that POSTs to `/api/v1/identification/books/:id/recompute`.

## Definition of Done

- [ ] Renders timeline ordered by `computed_at` desc
- [ ] Chips colored by contribution sign (positive vs negative)
- [ ] Empty-state ("no signals yet — pipeline never ran for this book")


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): per-book identification drawer (PIPE-P5-02)" \
  --body "Implements PIPE-P5-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P5-02"
```
