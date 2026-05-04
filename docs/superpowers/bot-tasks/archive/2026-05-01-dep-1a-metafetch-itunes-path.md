<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1a-metafetch-itunes-path.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEP-1a — Remove deprecated `Book.ITunesPath` from `internal/metafetch`

**TODO ID:** DEP-1a
**Audience:** burndown bot
**Branch:** `fix/dep-1a-metafetch-itunes-path`
**PR title:** `fix(metafetch): replace deprecated Book.ITunesPath with BookFile.ITunesPath`

---

## What This Task Does

Removes ~9 deprecated `Book.ITunesPath` usages (staticcheck SA1019) from:

- `internal/metafetch/batch.go` — lines 53–54
- `internal/metafetch/service.go` — lines 3667, 3680, 3682, 3804, 3847, 3858, 3860

These lines read or write `book.ITunesPath` (the deprecated `*string` field on the
`Book` struct). The correct field is `ITunesPath string` on `BookFile`.

---

## What NOT to Do

- **Do NOT** remove `Book.ITunesPath` from the struct.
- **Do NOT** change the database schema.
- **Do NOT** touch any other files beyond the two listed above.
- **Do NOT** modify test files (keep them passing by keeping external behaviour the same).

---

## Read First

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# See the exact lines
grep -n '\.ITunesPath' internal/metafetch/batch.go internal/metafetch/service.go | grep -v '_test'

# Understand Book.ITunesPath (deprecated) vs BookFile.ITunesPath (correct)
grep -n 'ITunesPath' internal/database/store.go | head -15

# Understand how to get BookFiles for a book
grep -n 'GetBookFiles' internal/database/store.go | head -5
```

---

## Step-by-step

### Step 1 — Read batch.go around lines 50–60

```bash
sed -n '45,65p' internal/metafetch/batch.go
```

Expected: code reads `book.ITunesPath` into a local info struct. Replace with a
`GetBookFiles` lookup:

```go
// BEFORE (around line 53):
if book.ITunesPath != nil {
    info.ITunesPath = *book.ITunesPath
}

// AFTER:
if bfs, bfErr := store.GetBookFiles(book.ID); bfErr == nil && len(bfs) > 0 {
    info.ITunesPath = bfs[0].ITunesPath
}
```

If `store` is not available in scope, check what parameter provides database access
at that call site (look at the function signature above line 53).

### Step 2 — Read service.go around the 7 affected lines

```bash
sed -n '3660,3690p' internal/metafetch/service.go
sed -n '3795,3870p' internal/metafetch/service.go
```

These are write paths where the code computes an iTunes path and stores it on the
book. Replace each write to `book.ITunesPath` with a write to the BookFile:

```go
// BEFORE:
itunesPath := ComputeITunesPath(somePath)
book.ITunesPath = &itunesPath

// AFTER — find the BookFile and update it instead:
itunesPath := ComputeITunesPath(somePath)
if bfs, bfErr := store.GetBookFiles(book.ID); bfErr == nil && len(bfs) > 0 {
    bfs[0].ITunesPath = itunesPath
    _ = store.UpdateBookFile(bfs[0].ID, &bfs[0])
}
// Do NOT set book.ITunesPath
```

If a `BookFile` (`bf` or `newBF`) is already in scope at that line, set its
`ITunesPath` directly without an extra `GetBookFiles` call.

### Step 3 — Check staticcheck

```bash
staticcheck ./internal/metafetch/... 2>&1 | grep 'ITunesPath'
```

Expected: no output. If there are still SA1019 lines, fix them.

### Step 4 — Build and test

```bash
go build ./...
go test ./internal/metafetch/... -timeout 60s 2>&1 | grep -E 'FAIL|ok|---'
```

Both must pass. If tests fail, read the error output and fix.

### Step 5 — Bump version headers on changed files

Every changed file must have its `<!-- version: X.Y.Z -->` header incremented
by a patch version (e.g. `1.4.0` → `1.4.1`). Also update `<!-- last-edited: 2026-05-01 -->`.

### Step 6 — Commit and open PR

```bash
git checkout -b fix/dep-1a-metafetch-itunes-path
git add internal/metafetch/
git commit -m "fix(metafetch): replace deprecated Book.ITunesPath with BookFile.ITunesPath

Removes ~9 SA1019 staticcheck warnings in internal/metafetch/batch.go
and service.go. Reads/writes ITunesPath via BookFile struct instead of
the deprecated Book.ITunesPath pointer field. Re-audit DEP-1a.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dep-1a-metafetch-itunes-path
gh pr create \
  --title "fix(metafetch): replace deprecated Book.ITunesPath with BookFile.ITunesPath" \
  --body "Removes ~9 SA1019 warnings from internal/metafetch. Part of DEP-1 migration. Re-audit finding R-4."
```

---

## Checklist

- [ ] `staticcheck ./internal/metafetch/...` shows zero SA1019 for `ITunesPath`
- [ ] `go build ./...` clean
- [ ] `go test ./internal/metafetch/...` passes
- [ ] Version headers bumped on all changed files
- [ ] PR opened with correct branch and title
