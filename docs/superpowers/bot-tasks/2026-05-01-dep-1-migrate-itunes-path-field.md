<!-- file: docs/superpowers/bot-tasks/2026-05-01-dep-1-migrate-itunes-path-field.md -->
<!-- version: 1.0.0 -->
<!-- guid: e4f5a6b7-c8d9-0e1f-2a3b-4c5d6e7f8a9b -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEP-1 — Migrate deprecated `ITunesPath` field usages to `book_files.itunes_path`

**TODO ID:** DEP-1  
**Audience:** burndown bot  
**Branch:** `fix/dep-itunes-path-migration`  
**PR title:** `fix(itunes): migrate deprecated ITunesPath field to book_files relation`

---

## What This Task Does

Removes all 13+ production usages of the deprecated `Book.ITunesPath` field
(flagged as SA1019 by `staticcheck`). Each usage should read/write `itunes_path`
via the `BookFile` struct and the `book_files` table instead.

---

## What NOT to Do

- **Do NOT** remove the `ITunesPath` field from the `Book` struct yet — other
  code may still rely on it being populated during scans.
- **Do NOT** change the database schema — only update the read/write access patterns.
- **Do NOT** change any non-deprecated code paths.
- **Do NOT** fix test files in this task — only production code.

---

## Read First

1. Find all deprecated usages:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -rn 'ITunesPath' --include='*.go' internal/ | grep -v '_test.go' | grep -v 'store.go' | grep -v 'sqlite_store.go' | head -30
```

2. Understand the `BookFile` struct:

```bash
grep -n 'ITunesPath\|BookFile\|book_files' internal/database/store.go | head -20
```

3. Understand how `BookFile` is loaded alongside a `Book`:

```bash
grep -n 'GetBookFiles\|book_files\|BookFile' internal/database/sqlite_store.go | head -20
```

---

## Affected Files

| File | Lines | Description |
|------|-------|-------------|
| `internal/itunes/service/importer.go` | 582,583,784,838,841 | Import path assignment |
| `internal/itunes/service/path_reconcile.go` | 148,149,152 | Path reconciliation |
| `internal/itunes/service/path_repair.go` | 433,434,437 | Path repair |
| `internal/itunes/service/writeback_batcher.go` | 299,302 | Write-back batcher |
| `internal/metafetch/service.go` | 3667,3680 | Metadata fetch |
| `internal/metafetch/batch.go` | 53,54 | Batch processing |

---

## Steps

### Step 1 — For each file, read and understand the usage context

```bash
sed -n '580,590p' internal/itunes/service/importer.go
sed -n '780,790p' internal/itunes/service/importer.go
sed -n '835,845p' internal/itunes/service/importer.go
```

### Step 2 — Replace `book.ITunesPath` reads with `bookFile.ITunesPath`

For read paths, first fetch the `BookFile`:
```go
// Before:
path := book.ITunesPath

// After — fetch the primary book file first:
files, err := store.GetBookFiles(ctx, book.ID)
if err != nil || len(files) == 0 {
    // handle missing file
}
path := files[0].ITunesPath
```

If a `BookFile` is already available in scope, use it directly.

### Step 3 — Replace `book.ITunesPath` writes with `UpdateBookFile`

For write paths:
```go
// Before:
book.ITunesPath = newPath
store.UpdateBook(ctx, book)

// After:
if len(files) > 0 {
    files[0].ITunesPath = newPath
    store.UpdateBookFile(ctx, files[0])
}
```

### Step 4 — Verify staticcheck passes

```bash
staticcheck ./internal/itunes/... ./internal/metafetch/... 2>&1 | grep 'ITunesPath'
```

No output = all deprecated usages removed.

### Step 5 — Build and test

```bash
go build ./...
go test ./internal/itunes/... ./internal/metafetch/... -timeout 60s 2>&1 | grep -E 'FAIL|ok'
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/dep-itunes-path-migration
git add internal/itunes/ internal/metafetch/
git commit -m "fix(itunes): migrate deprecated ITunesPath field to book_files relation

Replaces all 13+ production usages of the deprecated Book.ITunesPath
field (staticcheck SA1019) with reads/writes via BookFile.ITunesPath
and the book_files table. Re-audit finding R-4 / DEP-1.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dep-itunes-path-migration
gh pr create \
  --title "fix(itunes): migrate deprecated ITunesPath field to book_files relation" \
  --body "Removes 13+ SA1019 staticcheck warnings by migrating from Book.ITunesPath to BookFile.ITunesPath. Re-audit finding R-4."
```

---

## Checklist

- [ ] `staticcheck ./internal/itunes/... ./internal/metafetch/...` shows no SA1019 for ITunesPath
- [ ] `go build ./...` clean
- [ ] `go test ./internal/itunes/... ./internal/metafetch/...` passes
- [ ] PR opened with correct branch and title
