<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p6-03-deprecate-legacy-routes.md -->
<!-- version: 1.0.0 -->
<!-- guid: 609b8342-7786-4ba6-9e52-927ea8ca013c -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Add deprecation headers to legacy /api/v1/dedup/*

**Pipeline phase:** P6
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P4-02` — must be merged before this task starts
- `task:P5-01` — must be merged before this task starts
- `task:P5-02` — must be merged before this task starts
- `task:P5-03` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P4-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P4-02"; exit 0; }
count=$(gh pr list --label "task:P5-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P5-01"; exit 0; }
count=$(gh pr list --label "task:P5-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P5-02"; exit 0; }
count=$(gh pr list --label "task:P5-03" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P5-03"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p6-03-deprecate-legacy-routes
```

## Label

```bash
gh label create "task:PIPE-P6-03" --color "1d76db" --description "Bot task: Add deprecation headers to legacy /api/v1/dedup/*" 2>/dev/null || true
```

## What This Does

Adds `Deprecation: true` and `Sunset: <date+90d>` headers to every legacy
`/api/v1/dedup/*` handler in `internal/server/dedup_handlers.go`. Adds a
banner to the legacy UI dedup pages pointing to the new tab.

DOES NOT delete the legacy code. Deletion lives in a follow-up bot-task two
release cycles later.

## Definition of Done

- [ ] Every handler in `dedup_handlers.go` emits both headers
- [ ] UI banner present on legacy dedup pages
- [ ] CHANGELOG updated


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): add deprecation headers to legacy /api/v1/dedup/* (PIPE-P6-03)" \
  --body "Implements PIPE-P6-03 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P6-03"
```
