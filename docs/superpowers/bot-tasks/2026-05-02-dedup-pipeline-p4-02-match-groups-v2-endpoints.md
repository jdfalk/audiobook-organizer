<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p4-02-match-groups-v2-endpoints.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8125d125-2861-4bdd-80f9-ddc53425b540 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: /api/v1/dedup/v2/match-groups/* endpoints

**Pipeline phase:** P4
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P2-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P2-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P2-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p4-02-match-groups-v2-endpoints
```

## Label

```bash
gh label create "task:PIPE-P4-02" --color "1d76db" --description "Bot task: /api/v1/dedup/v2/match-groups/* endpoints" 2>/dev/null || true
```

## What This Does

Implements the v2 match-group surface (spec §7) parallel to the legacy
`/api/v1/dedup/*` routes (which remain wired through Phase 6).

## Endpoints

```
GET  /api/v1/dedup/v2/match-groups?state=open&limit=&cursor=
GET  /api/v1/dedup/v2/match-groups/:id
POST /api/v1/dedup/v2/match-groups/:id/resolve
GET  /api/v1/dedup/v2/stats
POST /api/v1/dedup/v2/recompute
```

`POST .../resolve` body:
```json
{ "action": "merge|dismiss|split",
  "canonical_book_id": "…",
  "members": ["…","…"] }
```

## Definition of Done

- [ ] Cursor-based pagination
- [ ] `merge` calls into the existing `internal/merge.Service` (do NOT
      duplicate merge logic)
- [ ] `dismiss` and `split` only mutate match-group state; books untouched
- [ ] Forever-store sees a `LogMatch` entry with the chosen `decision`


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): /api/v1/dedup/v2/match-groups/* endpoints (PIPE-P4-02)" \
  --body "Implements PIPE-P4-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P4-02"
```
