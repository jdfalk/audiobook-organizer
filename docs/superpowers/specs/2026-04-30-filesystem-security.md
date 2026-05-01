<!-- file: docs/superpowers/specs/2026-04-30-filesystem-security.md -->
<!-- version: 1.1.0 -->
<!-- guid: 1c2d3e4f-5a6b-7c8d-9e0f-a1b2c3d4e5f6 -->
<!-- last-edited: 2026-04-30 -->

# Filesystem Browse Security

**Status:** Draft — awaiting implementation
**Scope:** `internal/server/filesystem_service.go`, `internal/server/middleware/ratelimit.go`, `internal/config/config.go`
**Related specs:** none

---

## Problem

Four related security gaps were identified in the 2026-04-30 codebase audit:

**S-1 — Unrestricted directory browsing:**
`BrowseDirectory` (`internal/server/filesystem_service.go:35–79`) resolves the
user-supplied `path` parameter to an absolute path and lists it with no allowlist
check. A caller can browse `/etc/passwd`, `/root`, or any path on the filesystem.

**S-2 — Auth disabled by default:**
`enable_auth` defaults to `false`. New deployments — including Docker quick-starts —
expose every API endpoint publicly. No warning is emitted at startup.

**S-3 — Rate limiter disabled by default:**
`api_rate_limit_per_minute` defaults to 0, meaning the rate limiter is entirely
disabled. A misconfigured deployment has no protection against API abuse.

**S-5 / N-6 — O(N) rate limiter cleanup:**
The rate limiter in `internal/server/middleware/ratelimit.go` iterates over every
IP entry under a write lock on every request to evict expired entries. Under load this
becomes a bottleneck; a single IP with a high request rate forces an O(N) scan
proportional to the number of unique IPs ever seen.

---

## Core Rule / Goal

> **The filesystem browser must never expose paths outside configured import paths
> or the library root. New deployments must loudly warn when running insecure defaults.**

---

## Approach

### SEC-1 — BrowseDirectory allowlist

After resolving the user path to an absolute path, check it against an allowlist
built from two layers:

**Layer 1 — Hardcoded safe prefixes** (work out-of-the-box, no config required):

| Prefix | Rationale |
|--------|-----------|
| `/home/` | Linux user home directories |
| `/media/` | Removable drives, USB mounts |
| `/mnt/` | NFS, CIFS, network mounts |
| `/audiobooks` | Docker library root convention |
| `/data` | Docker data volume convention |

**Layer 2 — Config-derived paths** (always allowed regardless of prefix):
- `config.RootDir` — the configured library root
- Each entry in `config.ImportPaths` — user-configured import locations

Return HTTP 403 if the resolved path is not under any prefix in either layer.
Use a path-separator-aware prefix check to prevent `/etc` from matching `/etc-evil`.

This design means:
- Docker containers using `/audiobooks` or `/data` work with zero config.
- Home users with `/home/user/audiobooks` work with zero config.
- Anything outside those prefixes (e.g. `/etc`, `/root`, `/var`, `/proc`) is denied
  unless explicitly added to `ImportPaths`.

### SEC-2 — Auth disabled warning

Emit a prominent `log.Printf` warning to stderr at server startup when `EnableAuth`
is false. Do not change the default yet — visibility first.

### SEC-3 — Non-zero rate limit default

Change the default value of `api_rate_limit_per_minute` from 0 to 120. Treat 0 as
"use default" rather than "disabled". Add a config comment explaining the default.

### SEC-4 — Background cleanup goroutine

Remove the inline O(N) cleanup loop from the per-request hot path in the rate limiter.
Replace with a background goroutine (started in the constructor) that ticks every
5 minutes and evicts entries where `now - lastSeen > idleTTL`. The per-request path
only touches the single entry for the requesting IP.

---

## Acceptance Criteria

- [ ] `GET /api/v1/filesystem/browse?path=/etc` returns HTTP 403.
- [ ] `GET /api/v1/filesystem/browse?path=<valid-import-path>` returns 200 as before.
- [ ] Server logs a `⚠️ WARNING: Authentication is DISABLED` message when `enable_auth` is false.
- [ ] Default `api_rate_limit_per_minute` is 120 in new deployments.
- [ ] Rate limiter per-request path does not iterate over all IPs.
- [ ] `go vet ./...` clean; `go test ./internal/server/...` passes.

---

## Related Bot-Tasks

- [`2026-04-30-sec-1-browse-allowlist.md`](../bot-tasks/2026-04-30-sec-1-browse-allowlist.md) — SEC-1
- [`2026-04-30-sec-2-auth-default.md`](../bot-tasks/2026-04-30-sec-2-auth-default.md) — SEC-2
- [`2026-04-30-sec-3-rate-limit-default.md`](../bot-tasks/2026-04-30-sec-3-rate-limit-default.md) — SEC-3
- [`2026-04-30-sec-4-ratelimit-cleanup.md`](../bot-tasks/2026-04-30-sec-4-ratelimit-cleanup.md) — SEC-4
