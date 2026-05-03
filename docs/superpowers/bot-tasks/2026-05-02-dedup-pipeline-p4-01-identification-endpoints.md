<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p4-01-identification-endpoints.md -->
<!-- version: 1.0.0 -->
<!-- guid: 88fc487b-93c9-4dd9-840a-4dd4391d7ef6 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: /api/v1/identification/* HTTP endpoints

**Pipeline phase:** P4
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
feat/dedup-pipeline-p4-01-identification-endpoints
```

## Label

```bash
gh label create "task:PIPE-P4-01" --color "1d76db" --description "Bot task: /api/v1/identification/* HTTP endpoints" 2>/dev/null || true
```

## What This Does

Implements the read + recompute endpoints from spec §7 under
`/api/v1/identification/`. All routes require the existing
`audiobook.read` permission for GETs and `dedup.manage` for POSTs (add the
permission in `internal/auth/permissions.go` if missing).

## Files to Create / Edit

1. **Create** `internal/server/identification_handlers.go`
2. **Create** `internal/server/identification_handlers_test.go`
3. **Edit** `internal/server/server.go` — register the new mux subtree

## Endpoints

```
GET  /api/v1/identification/books/:id
GET  /api/v1/identification/books/:id/signals
POST /api/v1/identification/books/:id/recompute
POST /api/v1/identification/books/:id/recompute/:stage
GET  /api/v1/identification/fingerprints/sha/:sha
GET  /api/v1/identification/fingerprints/chromaprint?fp=…&min=0.85
```

## Definition of Done

- [ ] Each endpoint has a unit test using `httptest`
- [ ] POST endpoints enqueue the `identification-pipeline` job (don't run sync)
- [ ] 404 vs 200 distinguished correctly
- [ ] Permissions enforced


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): /api/v1/identification/* http endpoints (PIPE-P4-01)" \
  --body "Implements PIPE-P4-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P4-01"
```
