<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p5-01-identification-tab-shell.md -->
<!-- version: 1.0.0 -->
<!-- guid: f406ce35-f4a1-4899-97c0-5b354e2065e1 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: React Identification tab shell + library-health panel

**Pipeline phase:** P5
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P4-01` — must be merged before this task starts
- `task:P4-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P4-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P4-01"; exit 0; }
count=$(gh pr list --label "task:P4-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P4-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p5-01-identification-tab-shell
```

## Label

```bash
gh label create "task:PIPE-P5-01" --color "1d76db" --description "Bot task: React Identification tab shell + library-health panel" 2>/dev/null || true
```

## What This Does

Adds a top-level "Identification" route + tab. The shell is empty other than
the **Library Health** panel: pie of identity-score buckets, table of stages
with availability + queue depth, and a "Recompute all (dry-run)" button.

## Files to Create / Edit

1. **Create** `web/src/pages/Identification/index.tsx`
2. **Create** `web/src/pages/Identification/LibraryHealth.tsx`
3. **Edit** `web/src/App.tsx` (or the central router) — register the route
4. **Edit** the side-nav component — add the link

## Definition of Done

- [ ] Route renders without crashing on an empty library
- [ ] Pie + stage table use `useAsyncAction` hook (existing pattern)
- [ ] Vitest snapshot test passes


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): react identification tab shell + library-health panel (PIPE-P5-01)" \
  --body "Implements PIPE-P5-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P5-01"
```
