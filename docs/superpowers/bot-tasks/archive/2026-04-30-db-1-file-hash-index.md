<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-1-file-hash-index.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-123456789ef0 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-1 — Add Unique Index on (file_hash, library_id)

**TODO ID:** DB-1
**Audience:** burndown bot
**Branch:** `fix/db-file-hash-index`
**PR title:** `fix(database): add unique index on (file_hash, library_id)`

---

## What This Task Does

Adds a database migration that creates a `UNIQUE INDEX` on `(file_hash, library_id)`
in the `audiobooks` table (or equivalent). This prevents duplicate audiobook
records for the same physical file.

---

## What NOT to Do

- **Do NOT drop** or modify existing columns.
- **Do NOT add** the index inline in existing DDL — add it as a new numbered
  migration file.
- **Do NOT change** the application code to handle the unique constraint yet — just
  add the migration.
- **Do NOT use** `CREATE UNIQUE INDEX` if the table already has such an index (check
  first).

---

## Read First

1. Find the migration files:

```bash
find /Users/jdfalk/.worktrees/audiobook-eval -name '*.sql' -o -name '*migration*' \
  | grep -v node_modules | sort | head -30
```

2. Find the highest-numbered migration to know what number to use next.
3. `internal/database/` — find where migrations are applied (e.g., `migrate.go`,
   `schema.go`). Understand: are migrations embedded SQL files, or Go code?
4. Check the audiobooks table schema:

```bash
grep -rn 'CREATE TABLE.*audiobooks\|file_hash\|library_id' \
  internal/database/ | head -20
```

---

## Steps

### Step 1 — Verify the index doesn't already exist

```bash
grep -rn 'file_hash\|UNIQUE.*audiobooks' internal/database/ | head -20
```

If a unique index already exists, skip to the checklist and mark this task N/A.

### Step 2 — Determine migration format

Look at an existing migration file (from Step 1 above). Migrations may be:
- `.sql` files in a `migrations/` directory with numeric prefixes
- Go strings in a `schema.go` file
- Embedded via `go:embed`

### Step 3 — Create the migration

**If SQL files:** Create the next-numbered `.sql` file:

```sql
-- +migrate Up
CREATE UNIQUE INDEX IF NOT EXISTS uq_audiobooks_file_hash_library
    ON audiobooks (file_hash, library_id);

-- +migrate Down
DROP INDEX IF EXISTS uq_audiobooks_file_hash_library;
```

Use the correct Up/Down format for the migration tool in use.

**If Go code:** Find the `schema` or `migrations` slice and append:

```go
`CREATE UNIQUE INDEX IF NOT EXISTS uq_audiobooks_file_hash_library
     ON audiobooks (file_hash, library_id)`,
```

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go test ./internal/database/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/db-file-hash-index
# add the migration file or the modified schema file
git add .
git commit -m "fix(database): add unique index on (file_hash, library_id)

Prevents duplicate audiobook records for the same physical file by
adding a UNIQUE INDEX on audiobooks(file_hash, library_id).
Added as a numbered migration so existing databases are upgraded safely.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/db-file-hash-index
gh pr create \
  --title "fix(database): add unique index on (file_hash, library_id)" \
  --body "Adds unique index to prevent duplicate audiobook records. DB hygiene DB-1."
```

---

## Checklist

- [ ] Migration file (or schema entry) created
- [ ] Migration uses `IF NOT EXISTS` to be idempotent
- [ ] Includes a Down migration (if migration tool supports it)
- [ ] `go build ./...` passes
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
