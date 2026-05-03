<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-06-match-groups-schema.md -->
<!-- version: 1.0.0 -->
<!-- guid: 96514375-993d-43d5-a6b0-6a0489344315 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Add dedup_match_groups + members tables

**Pipeline phase:** P0
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
feat/dedup-pipeline-p0-06-match-groups-schema
```

## Label

```bash
gh label create "task:PIPE-P0-06" --color "1d76db" --description "Bot task: Add dedup_match_groups + members tables" 2>/dev/null || true
```

## What This Does

Adds `dedup_match_groups` and `dedup_match_group_members` tables (spec §6).
These supersede the legacy `dedup_candidates` table; that table is migrated
into this one in Phase 6.

## Files to Create / Edit

1. **Edit** `internal/database/migrations.go`

## Step 1 — Migration

```go
`CREATE TABLE IF NOT EXISTS dedup_match_groups (
    id              TEXT PRIMARY KEY,
    canonical_book  TEXT NOT NULL,
    strongest_kind  TEXT NOT NULL,
    strongest_score REAL NOT NULL,
    signal_summary  TEXT NOT NULL,
    state           TEXT NOT NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    decided_by      TEXT,
    decided_at      DATETIME
)`,
`CREATE INDEX IF NOT EXISTS idx_match_groups_state ON dedup_match_groups(state)`,
`CREATE TABLE IF NOT EXISTS dedup_match_group_members (
    group_id   TEXT NOT NULL,
    book_id    TEXT NOT NULL,
    pair_score REAL NOT NULL,
    role       TEXT NOT NULL,
    PRIMARY KEY (group_id, book_id)
)`,
`CREATE INDEX IF NOT EXISTS idx_match_group_members_book ON dedup_match_group_members(book_id)`,
```

## Definition of Done

- [ ] Both tables and both indexes created
- [ ] Idempotent on re-run


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): add dedup_match_groups + members tables (PIPE-P0-06)" \
  --body "Implements PIPE-P0-06 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-06"
```
