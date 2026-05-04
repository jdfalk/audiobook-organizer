<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1c-server-itunes-path.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEP-1c — Remove deprecated `Book.ITunesPath` from server handlers

**TODO ID:** DEP-1c
**Audience:** burndown bot
**Branch:** `fix/dep-1c-server-itunes-path`
**PR title:** `fix(server): replace deprecated Book.ITunesPath reads in server handlers`

---

## What This Task Does

Removes 6 deprecated `Book.ITunesPath` read usages (staticcheck SA1019) from:

- `internal/server/itl_rebuild.go` — lines 161, 162, 190, 191
- `internal/server/metadata_batch_candidates.go` — lines 301, 302

Both files read `book.ITunesPath` (a `*string`) to get the iTunes path for a book.
The correct source is `BookFile.ITunesPath` (a `string`) via `GetBookFiles`.

---

## What NOT to Do

- **Do NOT** touch any other server files.
- **Do NOT** touch `maintenance_fixups.go` — its `bf.ITunesPath` usages are already on BookFile (correct).
- **Do NOT** remove `Book.ITunesPath` from the struct.

---

## Read First

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# See the deprecated lines
sed -n '155,200p' internal/server/itl_rebuild.go
sed -n '295,310p' internal/server/metadata_batch_candidates.go

# Understand BookFile.ITunesPath
grep -n 'ITunesPath' internal/database/store.go | grep -v '//'

# Understand GetBookFiles signature
grep -n 'GetBookFiles' internal/database/store.go | head -3
```

---

## Step-by-step

### Step 1 — Fix `itl_rebuild.go` lines 161–162

Read lines 155–170 first:

```bash
sed -n '155,170p' internal/server/itl_rebuild.go
```

The code checks `book.ITunesPath != nil && *book.ITunesPath != ""` to decide
whether to use the path. Replace with BookFile lookup:

```go
// BEFORE:
if book.ITunesPath != nil && *book.ITunesPath != "" {
    wantLoc = *book.ITunesPath
}

// AFTER:
if bfs, bfErr := s.store.GetBookFiles(book.ID); bfErr == nil && len(bfs) > 0 && bfs[0].ITunesPath != "" {
    wantLoc = bfs[0].ITunesPath
}
```

If `s.store` is not the right receiver, check the function signature to find the
Store parameter.

### Step 2 — Fix `itl_rebuild.go` lines 190–191

Read lines 185–200:

```bash
sed -n '185,200p' internal/server/itl_rebuild.go
```

Apply the same pattern as Step 1.

### Step 3 — Fix `metadata_batch_candidates.go` lines 301–302

Read lines 295–310:

```bash
sed -n '295,310p' internal/server/metadata_batch_candidates.go
```

Same pattern — replace `book.ITunesPath` read with `GetBookFiles`:

```go
// BEFORE:
if book.ITunesPath != nil {
    info.ITunesPath = *book.ITunesPath
}

// AFTER:
if bfs, bfErr := s.store.GetBookFiles(book.ID); bfErr == nil && len(bfs) > 0 {
    info.ITunesPath = bfs[0].ITunesPath
}
```

### Step 4 — Check staticcheck

```bash
staticcheck ./internal/server/... 2>&1 | grep 'ITunesPath\|SA1019'
```

Expected: no SA1019 ITunesPath lines in itl_rebuild.go or metadata_batch_candidates.go.

### Step 5 — Build and test

```bash
go build ./...
go test ./internal/server/... -timeout 120s -run 'ITunes\|Rebuild\|BatchCand' 2>&1 | grep -E 'FAIL|ok|---'
```

If no matching tests, just run `go build ./...` and confirm it passes.

### Step 6 — Bump version headers

Increment patch version in both changed files. Update last-edited date.

### Step 7 — Commit and open PR

```bash
git checkout -b fix/dep-1c-server-itunes-path
git add internal/server/itl_rebuild.go internal/server/metadata_batch_candidates.go
git commit -m "fix(server): replace deprecated Book.ITunesPath reads in server handlers

Removes 6 SA1019 warnings in itl_rebuild.go (4 reads) and
metadata_batch_candidates.go (2 reads). Uses GetBookFiles to source
ITunesPath from BookFile instead of the deprecated Book field.
Re-audit DEP-1c.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dep-1c-server-itunes-path
gh pr create \
  --title "fix(server): replace deprecated Book.ITunesPath reads in server handlers" \
  --body "Removes 6 SA1019 warnings. Uses GetBookFiles for ITunesPath sourcing. Re-audit DEP-1c."
```

---

## Checklist

- [ ] `staticcheck ./internal/server/...` shows no SA1019 for `Book.ITunesPath` in the two changed files
- [ ] `go build ./...` clean
- [ ] Version headers bumped on changed files
- [ ] PR opened with correct branch and title
