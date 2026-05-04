<!-- file: docs/superpowers/bot-tasks/2026-04-30-sec-4-ratelimit-cleanup.md -->
<!-- version: 1.0.0 -->
<!-- guid: f0a1b2c3-d4e5-6789-fabc-012345678def -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SEC-4 — Clean Up Duplicated o1 Rate-Limit Middleware

**TODO ID:** SEC-4
**Audience:** burndown bot
**Branch:** `fix/ratelimit-o1-cleanup`
**PR title:** `fix(server): remove duplicate o1 rate-limit middleware registration`

---

## What This Task Does

Finds and removes duplicate rate-limit middleware registrations introduced by the
o1 rewrite. The rate-limit middleware is registered once globally and once per
router group, causing double-counting. This task removes the duplicate.

---

## What NOT to Do

- **Do NOT disable** rate limiting entirely.
- **Do NOT change** the rate-limit config values (`RPS`, `Burst`, etc.).
- **Do NOT remove** both registrations — keep the router-group registration and
  remove the duplicate global one (or vice versa — determine which is authoritative
  by reading the code).
- **Do NOT change** how `EnableRateLimit` is checked.

---

## Read First

1. `internal/server/server.go:2262–2278` — the section where middleware is
   registered. Search for multiple uses of the rate-limit middleware.
2. Search the whole file for the rate-limiter registration:

```bash
grep -n 'RateLimit\|rateLimiter\|rate_limit\|limiter' \
  internal/server/server.go | head -30
```

3. Identify the two registration sites. Understand which one is redundant (i.e.,
   which applies to a superset of routes and thus covers the other).

---

## Steps

### Step 1 — Find the duplicate

Run:
```bash
grep -n 'RateLimit\|rateLimiter\|RateLimiter' internal/server/server.go
```

Identify all lines. There should be two `.Use(...)` calls applying the same
rate-limit middleware. Note their line numbers.

### Step 2 — Determine which to remove

- If one applies to the entire engine (`r.Use(...)`) and one to a group
  (`apiV1.Use(...)`), the global one is the duplicate (every request hits the group
  middleware AND the global middleware — double counting).
- Remove the global registration. Keep the group registration.
- If both are on groups, check if one group is a sub-group of the other. Remove the
  inner one.

### Step 3 — Remove the duplicate line

Delete the duplicate `.Use(rateLimitMiddleware)` call (one line). Do not touch
surrounding code.

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
go test ./internal/server/... -v 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/ratelimit-o1-cleanup
git add internal/server/server.go
git commit -m "fix(server): remove duplicate o1 rate-limit middleware registration

The o1 rewrite registered the rate-limit middleware both globally and
on the API v1 group, causing every request to be counted twice against
the rate limit. Remove the redundant global registration.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/ratelimit-o1-cleanup
gh pr create \
  --title "fix(server): remove duplicate o1 rate-limit middleware registration" \
  --body "Removes duplicate rate-limit middleware added by the o1 rewrite. Requests were being counted twice. Single-line removal. Security cleanup S-4."
```

---

## Checklist

- [ ] Exactly one registration of the rate-limit middleware remains
- [ ] The remaining registration covers the same routes as before
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] `go vet ./...` clean
- [ ] PR opened with correct branch and title
