<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-05-identity-results-schema.md -->
<!-- version: 1.0.0 -->
<!-- guid: cd773164-7ebc-47e2-92c2-b0def115d5c6 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Add identity_results table to main DB

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
feat/dedup-pipeline-p0-05-identity-results-schema
```

## Label

```bash
gh label create "task:PIPE-P0-05" --color "1d76db" --description "Bot task: Add identity_results table to main DB" 2>/dev/null || true
```

## What This Does

Adds the `identity_results` table that stores the matrix output for every book.

## Files to Create / Edit

1. **Edit** `internal/database/migrations.go` — add the table

## Step 1 — Migration

Add inside `migrateSchema` near the dedup_signals migration:

```go
`CREATE TABLE IF NOT EXISTS identity_results (
    book_id          TEXT PRIMARY KEY,
    identity_score   REAL NOT NULL,
    signal_revision  INTEGER NOT NULL,
    decided_at       DATETIME NOT NULL,
    summary_json     TEXT NOT NULL
)`,
`ALTER TABLE books ADD COLUMN signal_revision INTEGER NOT NULL DEFAULT 0`,
```

> The `ALTER TABLE` will fail on second run; wrap with the existing
> "column-already-exists" tolerant helper in this file (search
> `addColumnIfMissing` or the closest equivalent — there is one used for the
> itunes columns).

## Step 2 — Test

```go
// add to internal/database/migrations_test.go (or a new file if missing)
func TestIdentityResultsMigration(t *testing.T) {
    db := newTestSQLiteDB(t) // existing helper
    _, err := db.Exec(`INSERT INTO identity_results (book_id, identity_score, signal_revision, decided_at, summary_json)
        VALUES ('b1', 0.92, 1, datetime('now'), '{}')`)
    require.NoError(t, err)
}
```

## Verify

```bash
go test ./internal/database/ -run TestIdentityResultsMigration
```

## Definition of Done

- [ ] Table created with all 5 columns
- [ ] `books.signal_revision` column added (idempotent)
- [ ] Test passes


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): add identity_results table to main db (PIPE-P0-05)" \
  --body "Implements PIPE-P0-05 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-05"
```
