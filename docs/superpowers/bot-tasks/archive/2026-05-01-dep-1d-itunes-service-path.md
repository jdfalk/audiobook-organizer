<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1d-itunes-service-path.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEP-1d — Remove deprecated `Book.ITunesPath` from `internal/itunes/service`

**TODO ID:** DEP-1d
**Audience:** burndown bot
**Branch:** `fix/dep-1d-itunes-service-path`
**PR title:** `fix(itunes): replace deprecated Book.ITunesPath in itunes/service package`

---

## What This Task Does

Removes ~14 deprecated `Book.ITunesPath` usages (staticcheck SA1019) from the
`internal/itunes/service` package:

| File | Lines (approx) | Pattern |
|------|----------------|---------|
| `importer.go` | 582, 583, 784, 787, 838, 841 | Reads and fallback writes |
| `path_reconcile.go` | 148, 149, 152 | Read current + write new |
| `path_repair.go` | 433, 434, 437 | Read current + write new |
| `writeback_batcher.go` | 299, 302 | Fallback read |

**Important:** This file also has many CORRECT usages of `bf.ITunesPath` /
`f.ITunesPath` / `bookFile.ITunesPath` — those are on `BookFile`, not `Book`.
Only change usages where the variable is a `*database.Book` (named `book`, `b`,
`existing`, or `books[i]`).

---

## What NOT to Do

- **Do NOT** change `bf.ITunesPath`, `f.ITunesPath`, `bookFile.ITunesPath` — those are already correct.
- **Do NOT** touch track_provisioner.go — its `bookFile.ITunesPath` usage is correct.
- **Do NOT** remove `Book.ITunesPath` from the struct.
- **Do NOT** change the database schema.

---

## Read First

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Find all Book.ITunesPath usages (variable names: book, b, existing, books[i])
grep -n '\.ITunesPath' internal/itunes/service/importer.go \
  internal/itunes/service/path_reconcile.go \
  internal/itunes/service/path_repair.go \
  internal/itunes/service/writeback_batcher.go

# Understand the struct difference
grep -n 'ITunesPath' internal/database/store.go | grep -v '//' | head -10
```

The `Book.ITunesPath` is `*string` (pointer). The `BookFile.ITunesPath` is
`string` (value). If you see `book.ITunesPath != nil` or `*book.ITunesPath`
it's the deprecated one.

---

## Step-by-step

### Step 1 — Fix `writeback_batcher.go` (easiest — start here)

```bash
sed -n '290,310p' internal/itunes/service/writeback_batcher.go
```

Lines 299–302 are a fallback: when a `Book` has no `BookFile` records, it falls
back to `book.ITunesPath`. Replace with: skip the write-back for that book (the
Book without BookFiles has no iTunes entry to update).

```go
// BEFORE:
if book.ITunesPath != nil && *book.ITunesPath != "" {
    updates = append(updates, writeback.LocationUpdate{
        NewLocation: *book.ITunesPath,
    })
}

// AFTER — simply omit the fallback; books without BookFiles have no iTunes path:
// (delete the if block entirely)
```

### Step 2 — Fix `path_reconcile.go` lines 148–152

```bash
sed -n '140,160p' internal/itunes/service/path_reconcile.go
```

The code reads `b.ITunesPath` to get the current path, then writes a new one.
The `BookFile` `bf` is already in scope (it's in the same loop). Use
`bf.ITunesPath` for the current-path read. For the write, set `bf.ITunesPath`
(which is already done on line 171):

```go
// BEFORE:
if b.ITunesPath != nil {
    current = *b.ITunesPath
}
...
b.ITunesPath = &wantBookITunesPath

// AFTER — use the BookFile (bf) that's already in scope:
current = bf.ITunesPath   // bf is already the matching file
// DELETE the b.ITunesPath = &wantBookITunesPath line
// bf.ITunesPath = want already happens below
```

### Step 3 — Fix `path_repair.go` lines 433–437

```bash
sed -n '425,445p' internal/itunes/service/path_repair.go
```

Same pattern: reads `book.ITunesPath` for current, writes new. A `BookFile` `bf`
should be in scope. If not, look up the book's BookFiles. Apply the same
read-from-bf, write-to-bf pattern.

### Step 4 — Fix `importer.go` lines 582–583, 784–787, 838–841

```bash
sed -n '575,595p' internal/itunes/service/importer.go
sed -n '778,795p' internal/itunes/service/importer.go
sed -n '832,845p' internal/itunes/service/importer.go
```

- Lines 582–583: `existing.ITunesPath` where `existing` is a `*Book`. Check if a
  matching `BookFile` for `existing` is already available, or fetch it via
  `GetBookFiles(existing.ID)`.
- Lines 784–787 and 838–841: `books[i].ITunesPath` fallbacks similar to
  writeback_batcher. If `books[i]` has no BookFile with a PID+path, skip the
  location update.

For each block, apply: read from BookFile if available, skip if not.

### Step 5 — Check staticcheck

```bash
staticcheck ./internal/itunes/... 2>&1 | grep 'ITunesPath\|SA1019'
```

Expected: zero SA1019 lines in the four changed files.

### Step 6 — Build and test

```bash
go build ./...
go test ./internal/itunes/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

Both must pass. If tests fail, read the error and fix.

### Step 7 — Bump version headers

Increment patch version in each changed file. Update last-edited date.

### Step 8 — Commit and open PR

```bash
git checkout -b fix/dep-1d-itunes-service-path
git add internal/itunes/service/
git commit -m "fix(itunes): replace deprecated Book.ITunesPath in itunes/service package

Removes ~14 SA1019 warnings across importer.go, path_reconcile.go,
path_repair.go, and writeback_batcher.go. Reads/writes ITunesPath via
BookFile instead of the deprecated Book-level pointer field.
Re-audit DEP-1d.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dep-1d-itunes-service-path
gh pr create \
  --title "fix(itunes): replace deprecated Book.ITunesPath in itunes/service package" \
  --body "Removes ~14 SA1019 warnings from internal/itunes/service. Uses BookFile.ITunesPath throughout. Re-audit DEP-1d."
```

---

## Checklist

- [ ] `staticcheck ./internal/itunes/...` shows zero SA1019 for deprecated `Book.ITunesPath`
- [ ] `go build ./...` clean
- [ ] `go test ./internal/itunes/...` passes
- [ ] Version headers bumped on all changed files
- [ ] PR opened with correct branch and title
