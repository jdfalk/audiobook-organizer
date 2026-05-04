<!-- file: docs/superpowers/bot-tasks/2026-04-30-srv-2-sse-heartbeat.md -->
<!-- version: 1.0.0 -->
<!-- guid: f8a9b0c1-d2e3-4567-fabc-890123456de7 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SRV-2 — Add SSE Heartbeat to Prevent Proxy Timeouts

**TODO ID:** SRV-2
**Audience:** burndown bot
**Branch:** `fix/sse-heartbeat`
**PR title:** `fix(server): add SSE heartbeat to prevent proxy timeouts`

---

## What This Task Does

Adds a periodic heartbeat (comment line) to the SSE event stream so that proxies
and load balancers do not close idle connections. The heartbeat sends a `: ping`
comment every 15–30 seconds.

---

## What NOT to Do

- **Do NOT add** data events as heartbeats — use SSE comment lines (`: ping\n\n`).
- **Do NOT change** the existing event types or payload structure.
- **Do NOT use** a goroutine leak — ensure the heartbeat goroutine exits when the
  client disconnects.
- **Do NOT add** heartbeat to non-SSE endpoints.

---

## Read First

1. Find the SSE handler:

```bash
grep -rn 'text/event-stream\|SSE\|event-stream\|flusher\|Flusher' \
  internal/server/ | head -20
```

2. Read the SSE handler fully. Understand:
   - How events are sent (channel? direct write?)
   - How the handler detects client disconnect (context cancellation? `CloseNotify`?)
   - Where the goroutine loop is

---

## Steps

### Step 1 — Understand the current SSE loop

The handler likely looks like:

```go
func (s *Server) SSEHandler(c *gin.Context) {
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    flusher, ok := c.Writer.(http.Flusher)
    if !ok { ... }

    for {
        select {
        case event := <-s.eventChan:
            fmt.Fprintf(c.Writer, "data: %s\n\n", event)
            flusher.Flush()
        case <-c.Request.Context().Done():
            return
        }
    }
}
```

### Step 2 — Add heartbeat ticker

Add a `time.NewTicker` to the select loop:

```go
heartbeat := time.NewTicker(25 * time.Second)
defer heartbeat.Stop()

for {
    select {
    case event := <-s.eventChan:
        fmt.Fprintf(c.Writer, "data: %s\n\n", event)
        flusher.Flush()
    case <-heartbeat.C:
        // SSE comment line — keeps connection alive, ignored by clients
        fmt.Fprintf(c.Writer, ": ping\n\n")
        flusher.Flush()
    case <-c.Request.Context().Done():
        return
    }
}
```

The `heartbeat.Stop()` in `defer` ensures the ticker goroutine is cleaned up when
the handler returns (client disconnect).

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/server/... -v 2>&1 | tail -20
```

### Step 4 — Commit and open PR

```bash
git checkout -b fix/sse-heartbeat
git add internal/server/
git commit -m "fix(server): add SSE heartbeat to prevent proxy timeouts

Sends a comment line (': ping') every 25 seconds to keep SSE
connections alive through proxies and load balancers. Uses
time.NewTicker with defer Stop to avoid goroutine leaks.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/sse-heartbeat
gh pr create \
  --title "fix(server): add SSE heartbeat to prevent proxy timeouts" \
  --body "Adds 25-second SSE comment heartbeat. Prevents proxy/LB timeouts on idle streams. No event format change. Server optimization SRV-2."
```

---

## Checklist

- [ ] `time.NewTicker(25 * time.Second)` added to SSE handler
- [ ] `defer heartbeat.Stop()` added to prevent ticker goroutine leak
- [ ] Heartbeat sends `: ping\n\n` (SSE comment, not data event)
- [ ] Handler still exits cleanly on client disconnect (`ctx.Done()`)
- [ ] Existing event format unchanged
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] PR opened with correct branch and title
