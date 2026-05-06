<!-- file: docs/superpowers/bot-tasks/2026-04-30-scan-1-walkdir.md -->
<!-- version: 1.0.0 -->
<!-- guid: d6e7f8a9-b0c1-2345-defa-678901234bc5 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SCAN-1 — Replace filepath.Walk with filepath.WalkDir in Scanner

**TODO ID:** SCAN-1
**Audience:** burndown bot
**Branch:** `perf/scanner-walkdir`
**PR title:** `perf(scanner): replace filepath.Walk with filepath.WalkDir`

---

## What This Task Does

Replaces `filepath.Walk` calls in `internal/scanner/` with `filepath.WalkDir`.
`filepath.WalkDir` is more efficient because it passes `fs.DirEntry` (no extra
`os.Stat` call per file) instead of `os.FileInfo`.

---

## What NOT to Do

- **Do NOT change** which files are included or excluded — only the walk mechanism.
- **Do NOT change** the scan algorithm or file processing logic.
- **Do NOT use** `filepath.Walk` in new code.

---

## Read First

1. Find all `filepath.Walk` calls:

```bash
grep -rn 'filepath\.Walk\b' internal/scanner/ | head -20
```

2. Read the walk callback function. Note what fields of `os.FileInfo` it uses
   (e.g., `info.Name()`, `info.IsDir()`, `info.Size()`, `info.ModTime()`).

---

## Steps

### Step 1 — Understand the current signature

`filepath.Walk` callback:
```go
filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
    if info.IsDir() { ... }
    if info.Name() == ... { ... }
    return nil
})
```

### Step 2 — Convert to filepath.WalkDir

`filepath.WalkDir` callback uses `fs.DirEntry` instead of `os.FileInfo`:
```go
filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
    if err != nil {
        return err
    }
    if d.IsDir() { ... }        // same as info.IsDir()
    if d.Name() == ... { ... }  // same as info.Name()
    // If you need full file info (e.g., size, modtime):
    // info, err := d.Info()
    // if err != nil { return err }
    // info.Size(), info.ModTime()
    return nil
})
```

For each piece of `os.FileInfo` used, either use the `fs.DirEntry` method directly
(`.Name()`, `.IsDir()`, `.Type()`) or call `d.Info()` when the full stat is needed.

### Step 3 — Add the import

Add `"io/fs"` to the import block if not already present.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/scanner/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b perf/scanner-walkdir
git add internal/scanner/
git commit -m "perf(scanner): replace filepath.Walk with filepath.WalkDir

filepath.WalkDir avoids an extra os.Stat call per file by passing
fs.DirEntry instead of os.FileInfo. On large libraries (10k+ files)
this reduces syscall overhead noticeably.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin perf/scanner-walkdir
gh pr create \
  --title "perf(scanner): replace filepath.Walk with filepath.WalkDir" \
  --body "Replaces filepath.Walk with the more efficient filepath.WalkDir. Reduces extra os.Stat calls on large libraries. Scanner efficiency SCAN-1."
```

---

## Checklist

- [ ] All `filepath.Walk` calls in `internal/scanner/` replaced with `filepath.WalkDir`
- [ ] Callback updated to use `fs.DirEntry` parameter
- [ ] `d.Info()` called only when full stat data is actually needed
- [ ] `"io/fs"` added to imports
- [ ] Scan results unchanged (same files processed)
- [ ] `go build ./...` passes
- [ ] `go test ./internal/scanner/...` passes
- [ ] PR opened with correct branch and title
