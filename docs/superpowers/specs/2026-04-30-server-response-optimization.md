<!-- file: docs/superpowers/specs/2026-04-30-server-response-optimization.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f -->
<!-- last-edited: 2026-04-30 -->

# Server Response Optimization — Gzip & SSE Heartbeat

**Status:** Draft — awaiting implementation
**Scope:** `internal/server/server.go`, `internal/realtime/`
**Related specs:** none

---

## Problem

**N-4 — No gzip compression:**
The server sends all API responses uncompressed. A list of 1,000 audiobooks as JSON
can be 500 KB–1 MB. Adding gzip middleware typically reduces payload size by 60–80%,
cutting bandwidth and reducing time-to-first-byte on slow connections.

**N-9 — Hung SSE clients hold connections forever:**
`server.go:3316–3318` explicitly sets `WriteTimeout: 0` on the HTTP server to
prevent SSE connections from timing out. But there is no heartbeat mechanism to
detect when a client has disconnected without a clean close (e.g., network partition,
NAT timeout). Hung connections accumulate indefinitely, eventually exhausting file
descriptors.

---

## Core Rule / Goal

> **Compress API responses where beneficial. Detect and clean up hung SSE clients
> via periodic heartbeat pings.**

---

## Approach

### SRV-1 — Gzip compression middleware

Add `github.com/gin-contrib/gzip` middleware to the Gin router:

```go
r.Use(gzip.Gzip(gzip.DefaultCompression))
```

Register it after CORS middleware but before route registration. SSE routes must be
excluded because they use chunked streaming, which is incompatible with gzip buffering.
Use `gzip.ExcludePaths([]string{"/api/v1/events", "/api/v1/sse"})` or register SSE
routes before attaching gzip to the group.

### SRV-2 — SSE heartbeat

In the SSE handler/broadcaster, start a goroutine per client connection that sends
a heartbeat comment every 30 seconds:

```
: heartbeat\n\n
```

SSE comment lines (starting with `:`) are ignored by all SSE clients per the spec.
If the write fails (broken pipe), cancel the client's context so the connection is
cleaned up. This does not require changing `WriteTimeout` on the HTTP server.

---

## What Does NOT Change

- SSE event format — the heartbeat is a comment line, invisible to client handlers.
- Any existing route — gzip is transparent to callers.
- `WriteTimeout: 0` — this remains because heartbeat failure drives cleanup instead.

---

## Acceptance Criteria

- [ ] `curl -H "Accept-Encoding: gzip" .../api/v1/audiobooks` response is gzip-encoded.
- [ ] SSE endpoints are NOT gzip-encoded (chunked streaming works normally).
- [ ] `go build ./...` is clean.
- [ ] Server does not crash or deadlock when a heartbeat write fails.
- [ ] `go mod tidy` succeeds; `gin-contrib/gzip` appears in `go.mod`.

---

## Related Bot-Tasks

- [`2026-04-30-srv-1-gzip.md`](../bot-tasks/2026-04-30-srv-1-gzip.md) — SRV-1
- [`2026-04-30-srv-2-sse-heartbeat.md`](../bot-tasks/2026-04-30-srv-2-sse-heartbeat.md) — SRV-2
