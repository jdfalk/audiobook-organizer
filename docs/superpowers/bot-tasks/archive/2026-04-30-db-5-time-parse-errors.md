<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-5-time-parse-errors.md -->
<!-- version: 1.0.0 -->
<!-- guid: e5f6a7b8-c9d0-1234-efab-567890123cd4 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-5 — Handle time.Parse Errors in DB Row Scanners

**TODO ID:** DB-5
**Audience:** burndown bot
**Branch:** `fix/db-time-parse-errors`
**PR title:** `fix(database): handle time.Parse errors in row scanners`

---

## What This Task Does

Finds all `time.Parse(...)` calls in `internal/database/` where the error is
silently ignored, and adds proper error handling so corrupt timestamp values don't
silently produce zero-value times.

---

## What NOT to Do

- **Do NOT change** the timestamp format string — only add error handling.
- **Do NOT use** `time.MustParse` or panic on parse failure — return the error or
  use a fallback with a warning log.
- **Do NOT add** error handling to code that already handles parse errors.

---

## Read First

Search for unhandled `time.Parse` calls in the database layer:

```bash
grep -n 'time\.Parse' internal/database/ -r | head -30
```

Look for patterns like:
```go
t, _ := time.Parse(layout, s)
// or
t, err := time.Parse(layout, s)
// (where err is never checked)
```

---

## Steps

### Step 1 — Find all time.Parse calls

```bash
grep -n 'time\.Parse\|time\.ParseInLocation' internal/database/ -r
```

For each occurrence, check the surrounding code to see if the error is checked.

### Step 2 — Fix discarded-error patterns

**Pattern A** — error assigned to `_`:
```go
// Before:
t, _ := time.Parse(time.RFC3339, row.CreatedAt)

// After:
t, err := time.Parse(time.RFC3339, row.CreatedAt)
if err != nil {
    return fmt.Errorf("parse CreatedAt %q: %w", row.CreatedAt, err)
}
```

**Pattern B** — if the function cannot return an error and a zero-value time is
acceptable, log a warning:
```go
t, err := time.Parse(time.RFC3339, row.CreatedAt)
if err != nil {
    log.Printf("WARN: parse CreatedAt %q: %v (using zero time)", row.CreatedAt, err)
    // t is already zero value, which is acceptable here
}
```

Prefer Pattern A for functions that can return an error. Use Pattern B only when
the caller cannot receive an error.

### Step 3 — Update callers if signatures changed

For any row-scanner function whose signature changed to return `error`, update its
callers (similar to DB-4 Step 3).

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/database/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/db-time-parse-errors
git add internal/database/
git commit -m "fix(database): handle time.Parse errors in row scanners

Replaces ignored errors from time.Parse calls in DB row scanners with
proper error returns or warning logs. Corrupt timestamp values now
surface instead of silently becoming zero times.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/db-time-parse-errors
gh pr create \
  --title "fix(database): handle time.Parse errors in row scanners" \
  --body "Surfaces time.Parse errors in DB row scanners. Prevents silent zero-time corruption. DB hygiene DB-5."
```

---

## Checklist

- [ ] No `time.Parse` calls with `_` error in `internal/database/`
- [ ] All fixes return the error OR log a warning (not silent)
- [ ] Callers updated if function signatures changed
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
