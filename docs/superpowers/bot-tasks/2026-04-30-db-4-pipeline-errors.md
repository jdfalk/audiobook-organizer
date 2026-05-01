<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-4-pipeline-errors.md -->
<!-- version: 1.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0123-defa-456789012bc3 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-4 — Return Errors from Pipeline Save Steps

**TODO ID:** DB-4
**Audience:** burndown bot
**Branch:** `fix/pipeline-save-errors`
**PR title:** `fix(database): propagate errors from pipeline save steps`

---

## What This Task Does

Finds all places in `internal/database/` where `ExecContext` or similar write calls
have their errors silently ignored (assigned to `_` or discarded), and changes them
to return the error to the caller.

---

## What NOT to Do

- **Do NOT add** new function signatures — just change `_ =` or missing error checks
  to `if err != nil { return ... }`.
- **Do NOT wrap** errors that are already wrapped — just `return err` or
  `return fmt.Errorf("context: %w", err)`.
- **Do NOT change** methods that already check their errors.
- **Do NOT change** read methods — only write methods.

---

## Read First

Search for discarded errors in database write functions:

```bash
grep -n '_ = .*ExecContext\|_ = .*Exec(\|_ = .*Insert\|_ = .*Update\|_ = .*Delete' \
  internal/database/ -r | head -30
```

Also look for `ExecContext` calls not followed by error checks:

```bash
grep -n -A1 'ExecContext\|\.Exec(' internal/database/ -r | grep -v 'if err\|err :=\|err =' | head -30
```

---

## Steps

### Step 1 — Enumerate all discarded errors

Run the grep above. Create a mental list of each location. A typical discarded
error looks like:

```go
_, _ = db.ExecContext(ctx, someSQL, args...)
// or
db.ExecContext(ctx, someSQL, args...)  // return value completely ignored
```

### Step 2 — Fix each occurrence

For each discarded error:

```go
// Before:
_, _ = s.db.ExecContext(ctx, updateLastPlayedSQL, bookID)

// After:
if _, err := s.db.ExecContext(ctx, updateLastPlayedSQL, bookID); err != nil {
    return fmt.Errorf("update last played: %w", err)
}
```

If the function signature currently returns nothing (e.g., `func ... () {`), change
it to return `error`. Update all call sites.

### Step 3 — Update call sites

For each function whose signature changed to return `error`, search for its call
sites:

```bash
grep -rn 'FunctionName(' internal/ cmd/ | head -20
```

Update callers to check the returned error. If a caller is a background goroutine,
log it:

```go
if err := s.store.UpdateLastPlayed(ctx, bookID); err != nil {
    log.Printf("UpdateLastPlayed: %v", err)
}
```

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/database/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/pipeline-save-errors
git add internal/database/
git commit -m "fix(database): propagate errors from pipeline save steps

Replaces discarded-error patterns (_ = ExecContext) in database write
methods with proper error returns. Updates call sites to check errors.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/pipeline-save-errors
gh pr create \
  --title "fix(database): propagate errors from pipeline save steps" \
  --body "Replaces silent error discards in DB write methods with proper error propagation. DB hygiene DB-4."
```

---

## Checklist

- [ ] No `_ = ...ExecContext` or `_ = ...Exec(` patterns remain in `internal/database/`
- [ ] All fixed methods return `error`
- [ ] Call sites updated to check returned errors
- [ ] Background goroutine callers log errors instead of ignoring
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
