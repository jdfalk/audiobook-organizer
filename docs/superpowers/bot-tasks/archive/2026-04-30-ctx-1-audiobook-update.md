<!-- file: docs/superpowers/bot-tasks/2026-04-30-ctx-1-audiobook-update.md -->
<!-- version: 1.0.0 -->
<!-- guid: a7b8c9d0-e1f2-3456-abcd-789012345ef6 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: CTX-1 — Thread Context Through AudiobookUpdateService

**TODO ID:** CTX-1
**Audience:** burndown bot
**Branch:** `fix/ctx-audiobook-update-service`
**PR title:** `fix(server): thread context through AudiobookUpdateService`

---

## What This Task Does

Replaces `context.Background()` calls in `AudiobookUpdateService` (or equivalent)
with the request context passed down from the HTTP handler. This ensures that
cancelled HTTP requests also cancel their in-flight DB and HTTP sub-calls.

---

## What NOT to Do

- **Do NOT remove** context from functions that already accept it.
- **Do NOT use** `context.TODO()` as a replacement — use the actual request context.
- **Do NOT change** the public API of the service without updating all callers.
- **Do NOT change** functions that are legitimately background tasks with no
  request context (e.g., background sync goroutines that run on a schedule).

---

## Read First

1. Find `AudiobookUpdateService` (or analogous update service):

```bash
grep -rn 'AudiobookUpdateService\|UpdateAudiobook\|context\.Background' \
  internal/server/ | grep -v '_test.go' | head -30
```

2. Read the service file fully. Identify every `context.Background()` call. For
   each one, determine: is this function reachable from an HTTP handler? If yes,
   it should use the request context.
3. Find the handler(s) that call this service to understand what context they
   receive.

---

## Steps

### Step 1 — List all context.Background() in the service

```bash
grep -n 'context\.Background()' internal/server/audiobook_service.go \
  internal/server/server.go 2>/dev/null | head -30
```

### Step 2 — Thread context from handler to service

For each service method that uses `context.Background()` and is called from a
handler:

1. Add `ctx context.Context` as the first parameter (if not already present):
   ```go
   // Before:
   func (s *Server) UpdateAudiobookMetadata(id string) error {
   
   // After:
   func (s *Server) UpdateAudiobookMetadata(ctx context.Context, id string) error {
   ```

2. Replace `context.Background()` with `ctx` inside the method.

3. Update the handler call site:
   ```go
   // Before:
   err := s.UpdateAudiobookMetadata(bookID)
   
   // After:
   err := s.UpdateAudiobookMetadata(c.Request.Context(), bookID)
   ```

### Step 3 — Handle legitimate background contexts

If a service method is called from a background goroutine (not a handler), leave
it using a purpose-annotated context:

```go
// Acceptable:
ctx := context.WithValue(context.Background(), ctxKeySource, "background-sync")
s.UpdateAudiobookMetadata(ctx, id)
```

Or use a base context stored on the server struct (if available).

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/server/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/ctx-audiobook-update-service
git add internal/server/
git commit -m "fix(server): thread context through AudiobookUpdateService

Replaces context.Background() in AudiobookUpdateService methods with
the request context from the HTTP handler. Cancelled requests now
propagate cancellation to in-flight DB and HTTP calls.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/ctx-audiobook-update-service
gh pr create \
  --title "fix(server): thread context through AudiobookUpdateService" \
  --body "Propagates request context through audiobook update service. Enables cancellation of in-flight calls. Context fix CTX-1."
```

---

## Checklist

- [ ] All `context.Background()` calls in request-path methods replaced with `ctx`
- [ ] Method signatures updated to accept `context.Context` as first param
- [ ] Handler call sites pass `c.Request.Context()`
- [ ] Background goroutine callers use annotated context (not changed to request ctx)
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] PR opened with correct branch and title
