<!-- file: docs/superpowers/bot-tasks/2026-04-30-log-4-progressbar.md -->
<!-- version: 1.0.0 -->
<!-- guid: a3b4c5d6-e7f8-9012-abcd-345678901ef2 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: LOG-4 — Remove Terminal Progress Bar from Scanner

**TODO ID:** LOG-4
**Audience:** burndown bot
**Branch:** `fix/scanner-remove-progressbar`
**PR title:** `fix(scanner): remove terminal progress bar, use structured log events`

---

## What This Task Does

Removes the terminal progress-bar dependency (e.g., `schollz/progressbar`,
`cheggaaa/pb`, or similar) from the scanner and replaces progress-bar updates with
structured log events. This prevents garbled output when the server logs to a file
or to systemd journal.

---

## What NOT to Do

- **Do NOT remove** scan progress reporting entirely — emit log events instead.
- **Do NOT add** a new progress-bar library.
- **Do NOT change** the scan algorithm or file-walking logic.
- **Do NOT change** the SSE event channel that sends progress to the frontend — this
  is separate from terminal progress bars.

---

## Read First

1. Find the progress-bar usage in the scanner:

```bash
grep -rn 'progressbar\|pb\.New\|uiprogress\|mpb\.\|tqdm' \
  internal/scanner/ | head -20
```

2. Read the surrounding code to understand how progress is currently tracked
   (total files, current index, etc.).
3. Check `go.mod` to confirm which library is imported:

```bash
grep 'progressbar\|cheggaaa\|schollz\|pb\.' go.mod
```

---

## Steps

### Step 1 — Identify all progress-bar usage

```bash
grep -n 'progressbar\|pb\.' internal/scanner/scanner.go internal/scanner/*.go 2>/dev/null
```

Note every method call: `New(total)`, `Add(1)`, `Finish()`, etc.

### Step 2 — Replace with structured log events

Replace:
```go
bar := progressbar.NewOptions(total, progressbar.OptionSetDescription("Scanning..."))
for _, file := range files {
    processFile(file)
    bar.Add(1)
}
bar.Finish()
```

With:
```go
slog.Info("scan started", "total_files", total)
for i, file := range files {
    processFile(file)
    if i%100 == 0 || i == total-1 {
        slog.Info("scan progress", "processed", i+1, "total", total,
            "pct", (i+1)*100/total)
    }
}
slog.Info("scan complete", "total_files", total)
```

Emit a progress log every 100 files (adjust frequency as needed).

### Step 3 — Remove the import

Remove the progress-bar import from `import (...)`.

### Step 4 — Remove from go.mod (if no longer used elsewhere)

Check if the library is used anywhere else:
```bash
grep -rn 'progressbar\|cheggaaa' internal/ cmd/ | grep -v 'go.sum'
```

If no other usages, remove from `go.mod`:
```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go mod tidy
```

### Step 5 — Verify

```bash
go build ./...
go vet ./...
go test ./internal/scanner/... -v 2>&1 | tail -20
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/scanner-remove-progressbar
git add internal/scanner/ go.mod go.sum
git commit -m "fix(scanner): remove terminal progress bar, use structured log events

Replaces the terminal progress bar library in the scanner with
structured slog events (scan started / progress / complete).
Prevents garbled output when logging to files or systemd journal.
SSE-based frontend progress reporting is unchanged.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/scanner-remove-progressbar
gh pr create \
  --title "fix(scanner): remove terminal progress bar, use structured log events" \
  --body "Removes terminal progress bar dependency. Uses structured log events instead. Frontend SSE progress unchanged. Logging fix LOG-4."
```

---

## Checklist

- [ ] Progress-bar library import removed from scanner file(s)
- [ ] Progress-bar calls replaced with structured log events
- [ ] Progress logged every ~100 files (not on every file)
- [ ] `go mod tidy` run if library removed from `go.mod`
- [ ] SSE progress channel code left unchanged
- [ ] `go build ./...` passes
- [ ] `go test ./internal/scanner/...` passes
- [ ] PR opened with correct branch and title
