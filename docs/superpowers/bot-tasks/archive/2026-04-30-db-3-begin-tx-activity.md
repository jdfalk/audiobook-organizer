<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-3-begin-tx-activity.md -->
<!-- version: 1.0.0 -->
<!-- guid: c3d4e5f6-a7b8-9012-cdef-345678901ab2 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-3 — Wrap Activity Store Writes in Transactions

**TODO ID:** DB-3
**Audience:** burndown bot
**Branch:** `fix/db-begin-tx-activity`
**PR title:** `fix(database): wrap activity store writes in transactions`

---

## What This Task Does

Applies the same transaction pattern from DB-2 to multi-step write pipelines in
`internal/database/activity_store.go` (or equivalent activity/progress DB file).

---

## What NOT to Do

- **Do NOT wrap** read-only methods in transactions.
- **Do NOT nest** transactions — check first whether the method already uses one.
- **Do NOT copy** DB-2's code blindly — `activity_store.go` may use a different
  `*sql.DB` field name or have different method names.
- **Do NOT change** the public method signatures.

---

## Read First

1. Identify the activity store file:

```bash
find /Users/jdfalk/.worktrees/audiobook-eval/internal/database -name '*.go' | xargs ls -1
```

2. Read the file that handles activity/progress records.
3. Search for multi-step writes (functions with 2+ `ExecContext` calls):

```bash
grep -n 'ExecContext\|Begin\|Commit' internal/database/activity_store.go | head -30
```

---

## Steps

### Step 1 — Identify multi-step write methods

List all functions in `activity_store.go` that call `ExecContext` more than once.
These are candidates for transaction wrapping.

Typical candidates:
- `LogActivity` (or `RecordActivity`, `CreateActivity`) — may insert into two tables
- `UpdatePlaybackPosition` — may update position and update last-played timestamp

### Step 2 — Wrap each multi-step write

Apply the same pattern as DB-2 for each identified method:

```go
func (s *ActivityStore) LogActivity(ctx context.Context, a Activity) (err error) {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("LogActivity begin tx: %w", err)
    }
    defer func() {
        if err != nil {
            if rbErr := tx.Rollback(); rbErr != nil {
                log.Printf("LogActivity rollback: %v", rbErr)
            }
        }
    }()

    if _, err = tx.ExecContext(ctx, insertActivitySQL, ...); err != nil {
        return fmt.Errorf("LogActivity insert: %w", err)
    }
    // ... other execs using tx
    if err = tx.Commit(); err != nil {
        return fmt.Errorf("LogActivity commit: %w", err)
    }
    return nil
}
```

### Step 3 — Skip single-step writes

If a method only has one `ExecContext` call, a transaction adds no value. Leave
it as-is.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/database/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/db-begin-tx-activity
git add internal/database/activity_store.go
git commit -m "fix(database): wrap activity store writes in transactions

Wraps multi-step activity/progress insert pipelines in
BEGIN/COMMIT/ROLLBACK transactions to prevent partial writes.
Single-step writes are unchanged.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/db-begin-tx-activity
gh pr create \
  --title "fix(database): wrap activity store writes in transactions" \
  --body "Applies transaction wrapping to activity/progress DB writes. DB hygiene DB-3."
```

---

## Checklist

- [ ] All multi-step write methods in `activity_store.go` wrapped in transactions
- [ ] Single-step write methods left unchanged
- [ ] Named return `err` used with `defer rollback`
- [ ] Rollback errors logged, not swallowed
- [ ] `go build ./...` passes
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
