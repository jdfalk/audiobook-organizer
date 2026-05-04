<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1b-organizer-itunes-path.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEP-1b — Remove deprecated `Book.ITunesPath` from `internal/organizer`

**TODO ID:** DEP-1b
**Audience:** burndown bot
**Branch:** `fix/dep-1b-organizer-itunes-path`
**PR title:** `fix(organizer): stop writing deprecated Book.ITunesPath on organize`

---

## What This Task Does

Removes 1 deprecated `Book.ITunesPath` write (staticcheck SA1019) from:

- `internal/organizer/service.go` — **line 427–428**

The organizer already correctly writes `ITunesPath` to each `BookFile` record
(lines 444 and 839). Line 428 is an extra write to the deprecated `Book`-level
field that duplicates what the BookFile already stores.

---

## What NOT to Do

- **Do NOT** touch any other files.
- **Do NOT** touch lines 444 or 839 — those use `bf.ITunesPath` (BookFile) which is correct.
- **Do NOT** remove `Book.ITunesPath` from the struct.
- **Do NOT** change the database schema.

---

## Read First

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
sed -n '415,460p' internal/organizer/service.go
```

You will see something like:

```go
newITunesPath := orgSvc.ComputeITunesPath(targetPath)
book.ITunesPath = &newITunesPath   // <-- line 428: DEPRECATED, remove this
organizedState := "organized"
book.LibraryState = &organizedState
...
if _, err := orgSvc.db.UpdateBook(book.ID, book); err != nil {
```

---

## Step-by-step

### Step 1 — Confirm the line

```bash
sed -n '425,432p' internal/organizer/service.go
```

### Step 2 — Delete the two deprecated lines

Remove lines that:
1. Call `orgSvc.ComputeITunesPath(targetPath)` and assign to a variable used
   only for `book.ITunesPath`
2. Set `book.ITunesPath = &newITunesPath`

If `newITunesPath` is ONLY used to set `book.ITunesPath` (check the surrounding
code), remove the variable declaration too. If it's used elsewhere, just remove the
`book.ITunesPath` assignment.

### Step 3 — Check staticcheck

```bash
staticcheck ./internal/organizer/... 2>&1 | grep 'ITunesPath'
```

Expected: no output.

### Step 4 — Build and test

```bash
go build ./...
go test ./internal/organizer/... -timeout 60s 2>&1 | grep -E 'FAIL|ok|---'
```

### Step 5 — Bump version header on service.go

Increment `<!-- version: X.Y.Z -->` by patch in `internal/organizer/service.go`.
Update `<!-- last-edited: -->` to today.

### Step 6 — Commit and open PR

```bash
git checkout -b fix/dep-1b-organizer-itunes-path
git add internal/organizer/service.go
git commit -m "fix(organizer): stop writing deprecated Book.ITunesPath on organize

The organizer already writes ITunesPath to BookFile records (lines 444,
839). Remove the redundant write to the deprecated Book.ITunesPath
field. Fixes SA1019. Re-audit DEP-1b.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dep-1b-organizer-itunes-path
gh pr create \
  --title "fix(organizer): stop writing deprecated Book.ITunesPath on organize" \
  --body "Removes 1 SA1019 warning in organizer/service.go. BookFile.ITunesPath is already written on lines 444 and 839. Re-audit DEP-1b."
```

---

## Checklist

- [ ] `staticcheck ./internal/organizer/...` shows zero SA1019 for `ITunesPath`
- [ ] `go build ./...` clean
- [ ] `go test ./internal/organizer/...` passes
- [ ] Version header bumped on service.go
- [ ] PR opened with correct branch and title
