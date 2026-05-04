<!-- file: docs/superpowers/bot-tasks/2026-04-30-log-2-fileops.md -->
<!-- version: 1.0.0 -->
<!-- guid: e1f2a3b4-c5d6-7890-efab-123456789cd0 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: LOG-2 — Replace fmt.Printf with Structured Logging in fileops

**TODO ID:** LOG-2
**Audience:** burndown bot
**Branch:** `fix/log-fileops-structured`
**PR title:** `fix(server): replace fmt.Printf with structured logging in fileops`

---

## What This Task Does

Replaces all unstructured `fmt.Printf`, `fmt.Println`, and bare print calls in
`internal/server/fileops.go` (file copy/move/delete operations) with the project's
structured logger.

---

## What NOT to Do

- **Do NOT change** file operation logic — only logging calls.
- **Do NOT use** `fmt.Printf` after this change.
- **Do NOT remove** error logging — only convert its format.

---

## Read First

1. Read `internal/server/fileops.go`:

```bash
grep -n 'fmt\.Print\|fmt\.Fprintf\|log\.Print' internal/server/fileops.go | head -20
```

2. Check the structured logger import used elsewhere in `internal/server/`:

```bash
grep -n 'slog\.\|log\.' internal/server/server.go | head -10
```

---

## Steps

### Step 1 — Find all print calls

```bash
grep -n 'fmt\.Print\|fmt\.Fprintf.*Stderr\|log\.Print\b' internal/server/fileops.go
```

### Step 2 — Replace with structured calls

Match the project's existing style in `internal/server/`. Example using `slog`:
```go
// Before:
fmt.Printf("Copying %s to %s\n", src, dst)

// After:
slog.Info("copying file", "src", src, "dst", dst)
```

```go
// Before:
fmt.Printf("Error copying file: %v\n", err)

// After:
slog.Error("copying file", "src", src, "dst", dst, "error", err)
```

### Step 3 — Clean up imports

Remove the `fmt` import if no longer needed for non-log purposes.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/server/... -run TestFile -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/log-fileops-structured
git add internal/server/fileops.go
git commit -m "fix(server): replace fmt.Printf with structured logging in fileops

Converts fmt.Print* calls in fileops.go to structured logger calls.
File operation events are now machine-parseable and consistent with
the rest of the application.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/log-fileops-structured
gh pr create \
  --title "fix(server): replace fmt.Printf with structured logging in fileops" \
  --body "Converts fileops fmt.Printf calls to structured logging. Logging fix LOG-2."
```

---

## Checklist

- [ ] No `fmt.Printf`, `fmt.Println` in `fileops.go`
- [ ] Structured logger used with key-value pairs
- [ ] `fmt` import removed if no longer needed
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] PR opened with correct branch and title
