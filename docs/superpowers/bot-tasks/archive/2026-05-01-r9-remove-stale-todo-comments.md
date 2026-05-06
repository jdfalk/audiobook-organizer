<!-- file: docs/superpowers/bot-tasks/2026-05-01-r9-remove-stale-todo-comments.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: R-9 — Remove stale `// TODO: Implement in N1-2` comments

**TODO ID:** R-9
**Audience:** burndown bot
**Branch:** `fix/r9-remove-stale-todos`
**PR title:** `chore(database): remove stale TODO comments from GetAuthorsByBookIDs and GetNarratorsByBookIDs`

---

## What This Task Does

Removes two stale `// TODO: Implement in N1-2` comments from
`internal/database/sqlite_store.go` at the start of `GetAuthorsByBookIDs`
(line ~6913) and `GetNarratorsByBookIDs` (line ~6946).

Both functions are **fully implemented** — the TODO was left from when the bot-task
spec (N1-2) said "implement later". The implementation is there; only the comment is wrong.

---

## Step-by-step

### Step 1 — Verify both functions are implemented

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -n 'TODO: Implement in N1-2' internal/database/sqlite_store.go
```

Expected: two lines (one for each function). Both functions below those lines
should have full query bodies (not just `panic("not implemented")`).

```bash
# Confirm the functions are real (should show SELECT queries, not panics)
sed -n '6912,6960p' internal/database/sqlite_store.go
```

### Step 2 — Delete only the TODO comment lines

Delete the line `// TODO: Implement in N1-2` above `GetAuthorsByBookIDs` and
above `GetNarratorsByBookIDs`. Do not change any other lines.

### Step 3 — Build

```bash
go build ./internal/database/...
```

Expected: clean.

### Step 4 — Bump version header on sqlite_store.go

Increment `<!-- version: X.Y.Z -->` by patch. Update `<!-- last-edited: -->`.

### Step 5 — Commit and open PR

```bash
git checkout -b fix/r9-remove-stale-todos
git add internal/database/sqlite_store.go
git commit -m "chore(database): remove stale TODO comments from batch query functions

GetAuthorsByBookIDs and GetNarratorsByBookIDs are fully implemented.
Remove the leftover 'TODO: Implement in N1-2' comments that no longer
apply. Re-audit R-9.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/r9-remove-stale-todos
gh pr create \
  --title "chore(database): remove stale TODO comments from batch query functions" \
  --body "Removes 2 stale TODO comments from sqlite_store.go. Both functions are fully implemented. Re-audit R-9."
```

---

## Checklist

- [ ] Both `// TODO: Implement in N1-2` lines removed
- [ ] Both functions still have their full query implementations
- [ ] `go build ./internal/database/...` clean
- [ ] Version header bumped on sqlite_store.go
- [ ] PR opened with correct branch and title
