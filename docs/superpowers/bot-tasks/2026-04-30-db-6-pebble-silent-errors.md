<!-- file: docs/superpowers/bot-tasks/2026-04-30-db-6-pebble-silent-errors.md -->
<!-- version: 1.0.0 -->
<!-- guid: f6a7b8c9-d0e1-2345-fabc-678901234de5 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: DB-6 — Surface Silent Errors in PebbleDB Write Methods

**TODO ID:** DB-6
**Audience:** burndown bot
**Branch:** `fix/pebble-silent-errors`
**PR title:** `fix(database): surface silent errors in PebbleDB write methods`

---

## What This Task Does

Finds all PebbleDB write calls in `internal/database/pebble_store.go` (or
equivalent) where errors are silently discarded, and changes them to return or log
the error.

---

## What NOT to Do

- **Do NOT change** the PebbleDB library or version.
- **Do NOT change** read methods — only write methods (`Set`, `Delete`, `Apply`,
  `Batch.Commit`).
- **Do NOT add** panics — return errors or log them.

---

## Read First

Find the PebbleDB store file:

```bash
find /Users/jdfalk/.worktrees/audiobook-eval/internal/database -name '*.go' \
  | xargs grep -l 'pebble\|Pebble' 2>/dev/null
```

Read it. Search for silent error patterns:

```bash
grep -n '_ = .*\.Set\|_ = .*\.Delete\|_ = .*Commit\|\.Set(.*); *$' \
  internal/database/pebble_store.go | head -20
```

---

## Steps

### Step 1 — Enumerate silent errors

Run the grep above. Common patterns in PebbleDB code:

```go
// Pattern A: discarded error
_ = s.db.Set(key, value, pebble.Sync)

// Pattern B: error used but not returned
if err := s.db.Set(key, value, pebble.Sync); err != nil {
    // no return or log here — just falls through
}

// Pattern C: batch commit error discarded
_ = batch.Commit(pebble.Sync)
```

### Step 2 — Fix Pattern A (discarded return)

```go
// After:
if err := s.db.Set(key, value, pebble.Sync); err != nil {
    return fmt.Errorf("pebble Set %q: %w", key, err)
}
```

If the function cannot return an error (e.g., it's in a goroutine), log instead:

```go
if err := s.db.Set(key, value, pebble.Sync); err != nil {
    log.Printf("ERROR: pebble Set %q: %v", key, err)
}
```

### Step 3 — Fix Pattern C (batch commit)

```go
// After:
if err := batch.Commit(pebble.Sync); err != nil {
    return fmt.Errorf("pebble batch commit: %w", err)
}
```

### Step 4 — Update function signatures if needed

If fixing a method requires it to return `error` where it previously didn't, update
the interface in `internal/database/store.go` and all implementations.

### Step 5 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/database/... -v 2>&1 | tail -20
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/pebble-silent-errors
git add internal/database/
git commit -m "fix(database): surface silent errors in PebbleDB write methods

Replaces discarded errors (_, _ = db.Set) in PebbleDB store with
proper error returns. Updates interface and callers where needed.
Prevents silent data loss on PebbleDB write failures.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/pebble-silent-errors
gh pr create \
  --title "fix(database): surface silent errors in PebbleDB write methods" \
  --body "PebbleDB write errors were being silently discarded. Now returned or logged. DB hygiene DB-6."
```

---

## Checklist

- [ ] No `_ = s.db.Set`, `_ = s.db.Delete`, `_ = batch.Commit` patterns remain
- [ ] All write errors returned or logged
- [ ] Interface and callers updated where signatures changed
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened with correct branch and title
