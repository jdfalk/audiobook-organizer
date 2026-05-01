<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-2-begin-tx-sqlite.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-234567890fa1 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-2 — Wrap SaveBook Pipeline in a Transaction (SQLite)

**TODO ID:** DB-2
**Audience:** burndown bot
**Branch:** `fix/db-begin-tx-sqlite`
**PR title:** `fix(database): wrap SaveBook pipeline in a SQLite transaction`

---

## What This Task Does

Wraps the multi-step `SaveBook` pipeline in `internal/database/sqlite_store.go`
(or equivalent) in a single `BEGIN … COMMIT` transaction so that a failure midway
does not leave partially-written book records.

---

## What NOT to Do

- **Do NOT add** a transaction to read-only query methods — only write pipelines.
- **Do NOT nest** transactions if `SaveBook` is already called inside a transaction.
  Add a guard check: if a tx is already in progress, skip wrapping.
- **Do NOT commit** before all sub-steps have completed.
- **Do NOT ignore** the rollback error — log it.

---

## Read First

1. `internal/database/sqlite_store.go` — find `SaveBook` (or `UpsertBook`,
   `CreateBook`, whichever is the primary write path). Read it fully. Count how
   many separate `db.Exec` or `db.ExecContext` calls it makes. These are the calls
   that need to be in a transaction.
2. Check if `SaveBook` already uses a transaction:

```bash
grep -n 'Begin\|BeginTx\|Tx\|ROLLBACK\|COMMIT' internal/database/sqlite_store.go | head -20
```

3. Identify the `*sql.DB` or `*sqlx.DB` field name on the store struct.

---

## Steps

### Step 1 — Wrap SaveBook in a transaction

Find `SaveBook` (the exact function name may differ). Replace the function body
pattern from:

```go
func (s *SQLiteStore) SaveBook(ctx context.Context, book Book) error {
    _, err := s.db.ExecContext(ctx, insertBookSQL, ...)
    if err != nil { return err }
    _, err = s.db.ExecContext(ctx, insertAuthorsSQL, ...)
    if err != nil { return err }
    // ... more execs
    return nil
}
```

To:

```go
func (s *SQLiteStore) SaveBook(ctx context.Context, book Book) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("SaveBook begin tx: %w", err)
    }
    defer func() {
        if err != nil {
            if rbErr := tx.Rollback(); rbErr != nil {
                log.Printf("SaveBook rollback: %v", rbErr)
            }
        }
    }()

    if _, err = tx.ExecContext(ctx, insertBookSQL, ...); err != nil {
        return fmt.Errorf("SaveBook insert book: %w", err)
    }
    if _, err = tx.ExecContext(ctx, insertAuthorsSQL, ...); err != nil {
        return fmt.Errorf("SaveBook insert authors: %w", err)
    }
    // ... convert all s.db.ExecContext calls to tx.ExecContext

    if err = tx.Commit(); err != nil {
        return fmt.Errorf("SaveBook commit: %w", err)
    }
    return nil
}
```

**Important:** The `defer` uses the named `err` return variable. Ensure the
function signature uses a named return: `(err error)`.

### Step 2 — Apply the same pattern to DeleteBook if it has multi-step deletes

```bash
grep -n 'DeleteBook\|delete.*book\|DeleteAudio' internal/database/sqlite_store.go | head -10
```

If `DeleteBook` makes multiple `db.Exec` calls, wrap it in the same tx pattern.

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/database/... -v 2>&1 | tail -30
```

### Step 4 — Commit and open PR

```bash
git checkout -b fix/db-begin-tx-sqlite
git add internal/database/sqlite_store.go
git commit -m "fix(database): wrap SaveBook pipeline in a SQLite transaction

Wraps the multi-step book insert (book row + authors + narrators + tags)
in a single BEGIN/COMMIT transaction. A failure in any step now rolls
back the entire record rather than leaving partial data.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/db-begin-tx-sqlite
gh pr create \
  --title "fix(database): wrap SaveBook pipeline in a SQLite transaction" \
  --body "Prevents partial book records on write failure. DB hygiene DB-2."
```

---

## Checklist

- [ ] `SaveBook` wrapped in `BeginTx` / `Commit` / `Rollback`
- [ ] All `s.db.ExecContext` calls inside SaveBook converted to `tx.ExecContext`
- [ ] `defer rollback` pattern uses named return `err`
- [ ] Rollback errors are logged, not swallowed
- [ ] `DeleteBook` also wrapped if it has multi-step deletes
- [ ] `go build ./...` passes
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
