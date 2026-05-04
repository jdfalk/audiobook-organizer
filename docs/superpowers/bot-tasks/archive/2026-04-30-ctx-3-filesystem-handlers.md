<!-- file: docs/superpowers/bot-tasks/2026-04-30-ctx-3-filesystem-handlers.md -->
<!-- version: 1.0.0 -->
<!-- guid: c9d0e1f2-a3b4-5678-cdef-901234567ab8 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: CTX-3 — Thread Context Through Filesystem Handlers

**TODO ID:** CTX-3
**Audience:** burndown bot
**Branch:** `fix/ctx-filesystem-handlers`
**PR title:** `fix(server): thread context through filesystem handlers`

---

## What This Task Does

Replaces `context.Background()` calls in filesystem handler functions
(`internal/server/filesystem_handlers.go` and `internal/server/filesystem_service.go`)
with `c.Request.Context()` (the Gin request context).

---

## What NOT to Do

- **Do NOT change** filesystem read logic — only context threading.
- **Do NOT add** context to `os.ReadDir`, `os.Stat`, `filepath.Walk` — the stdlib
  does not support context cancellation for these. Only pass context to DB and
  external service calls.
- **Do NOT use** `context.TODO()` — use the actual request context.

---

## Read First

1. Read `internal/server/filesystem_handlers.go` and `filesystem_service.go`:

```bash
grep -n 'context\.Background' \
  internal/server/filesystem_handlers.go \
  internal/server/filesystem_service.go 2>/dev/null
```

2. For each `context.Background()` usage, identify what it's passed to — a DB
   call? An external HTTP call? Only replace those.
3. Check Gin handler signatures to confirm they use `*gin.Context` (which has
   `.Request.Context()`).

---

## Steps

### Step 1 — Find context.Background() usages in filesystem code

```bash
grep -n 'context\.Background()' \
  internal/server/filesystem_handlers.go \
  internal/server/filesystem_service.go
```

### Step 2 — Replace with request context

In Gin handlers, replace:
```go
ctx := context.Background()
```
with:
```go
ctx := c.Request.Context()
```

Where `c` is the `*gin.Context` parameter.

For service methods called from handlers, update the method to accept `ctx`:
```go
// Before:
func (s *Server) resolveImportPath(path string) (string, error) {
    _ = s.store.LogAccess(context.Background(), path)
    
// After:
func (s *Server) resolveImportPath(ctx context.Context, path string) (string, error) {
    _ = s.store.LogAccess(ctx, path)
```

Update the handler call site to pass `c.Request.Context()`.

### Step 3 — Do NOT add context to pure filesystem operations

```go
// Do NOT do this:
entries, err := os.ReadDir(ctx, absPath) // os.ReadDir doesn't take context

// This is fine as-is:
entries, err := os.ReadDir(absPath) // leave unchanged
```

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/server/... -run TestFilesystem -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/ctx-filesystem-handlers
git add internal/server/filesystem_handlers.go internal/server/filesystem_service.go
git commit -m "fix(server): thread context through filesystem handlers

Replaces context.Background() in filesystem handlers and service with
c.Request.Context(). DB calls and service calls in the filesystem
request path now respect request cancellation. Pure fs operations
(os.ReadDir, filepath.Walk) left unchanged.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/ctx-filesystem-handlers
gh pr create \
  --title "fix(server): thread context through filesystem handlers" \
  --body "Propagates Gin request context through filesystem handlers. Context fix CTX-3."
```

---

## Checklist

- [ ] No `context.Background()` in request-path filesystem handler/service code
- [ ] DB and external service calls in filesystem handlers use request context
- [ ] Pure `os.*` / `filepath.*` calls left unchanged (no context added)
- [ ] Gin handlers use `c.Request.Context()`
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] PR opened with correct branch and title
