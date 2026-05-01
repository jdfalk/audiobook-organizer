<!-- file: docs/superpowers/specs/2026-04-30-context-propagation.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2b3c4d5e-6f7a-8b9c-0d1e-2f3a4b5c6d7e -->
<!-- last-edited: 2026-04-30 -->

# Context Propagation — Replace context.Background() in HTTP Handlers

**Status:** Draft — awaiting implementation
**Scope:** `internal/server/audiobook_update_service.go`, `internal/server/openlibrary_service.go`, `internal/server/filesystem_handlers.go`
**Related specs:** [`2026-04-30-db-hygiene.md`](./2026-04-30-db-hygiene.md)

---

## Problem

30+ calls to `context.Background()` exist inside HTTP handler code across at least
three files. When a Gin handler uses `context.Background()` for DB or network calls:

- The call continues even after the HTTP client disconnects.
- The call continues past the server's per-request timeout.
- Database connections are held longer than necessary.
- Downstream HTTP calls (e.g., Open Library API) cannot be cancelled.

This is a correctness issue: cancelled requests silently consume resources.

---

## Core Rule / Goal

> **HTTP handler code must use `c.Request.Context()` (or the propagated ctx parameter)
> for all DB and network calls. `context.Background()` is only valid in constructors,
> background goroutines, and non-handler init paths.**

---

## Approach

For each affected file:

1. Identify every `context.Background()` call in handler or handler-called functions.
2. If the function has access to a `*gin.Context c`, replace with `c.Request.Context()`.
3. If the function does not have gin context but is called from a handler, add a
   `ctx context.Context` parameter and thread it through.
4. If the function is called from BOTH handler and non-handler code, add the `ctx`
   parameter and pass `context.Background()` from non-handler call sites with a
   `// non-handler: no request context available` comment.

**Do NOT** replace `context.Background()` in:
- Goroutines launched by the server at startup
- `init()` functions
- Background polling loops (these should use a long-lived context from server startup)

---

## Files in Scope

| Task | File | Approx. instances |
|------|------|-------------------|
| CTX-1 | `internal/server/audiobook_update_service.go` | 15+ |
| CTX-2 | `internal/server/openlibrary_service.go:181` | 1–3 |
| CTX-3 | `internal/server/filesystem_handlers.go:165` | 1–3 |

---

## Acceptance Criteria

- [ ] `grep -n 'context\.Background()' internal/server/audiobook_update_service.go` returns 0 handler-path occurrences.
- [ ] `grep -n 'context\.Background()' internal/server/openlibrary_service.go` returns 0 handler-path occurrences.
- [ ] `grep -n 'context\.Background()' internal/server/filesystem_handlers.go` returns 0 handler-path occurrences.
- [ ] `go build ./...` is clean.
- [ ] `go vet ./...` is clean.
- [ ] Any remaining `context.Background()` in these files has a comment explaining why it is legitimate.

---

## Related Bot-Tasks

- [`2026-04-30-ctx-1-audiobook-update.md`](../bot-tasks/2026-04-30-ctx-1-audiobook-update.md) — CTX-1
- [`2026-04-30-ctx-2-openlibrary.md`](../bot-tasks/2026-04-30-ctx-2-openlibrary.md) — CTX-2
- [`2026-04-30-ctx-3-filesystem-handlers.md`](../bot-tasks/2026-04-30-ctx-3-filesystem-handlers.md) — CTX-3
