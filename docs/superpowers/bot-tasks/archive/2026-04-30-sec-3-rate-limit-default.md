<!-- file: docs/superpowers/bot-tasks/2026-04-30-sec-3-rate-limit-default.md -->
<!-- version: 1.0.0 -->
<!-- guid: e9f0a1b2-c3d4-5678-efab-901234567cde -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SEC-3 — Emit Startup Warning When Rate Limiting is Disabled

**TODO ID:** SEC-3
**Audience:** burndown bot
**Branch:** `fix/rate-limit-default`
**PR title:** `fix(security): emit startup warning when rate limiting is disabled`

---

## What This Task Does

Adds a startup log warning when `config.AppConfig.EnableRateLimit` is `false`.
Analogous to SEC-2 — makes the insecure default visible without changing it.

---

## What NOT to Do

- **Do NOT change** the default value of `enable_rate_limit` in the config.
- **Do NOT add** or change rate limiting middleware — see SEC-4 for that.
- **Do NOT add** the warning to every request — only at startup.
- **Do NOT use** `fmt.Printf` — use `log.Printf` or the project's structured logger.

---

## Read First

1. `internal/server/server.go:2262–2278` — same startup location used in SEC-2.
2. `internal/config/config.go` — find the `EnableRateLimit` field (may be
   `enable_rate_limit bool` or similar). Confirm the struct field name.
3. Check that SEC-2 is merged first so the two warnings sit adjacent in the code.

---

## Steps

### Step 1 — Locate the startup block

Same location as SEC-2. Find the block added by SEC-2 and add the rate-limit
warning adjacent to it:

```go
if !config.AppConfig.EnableAuth {
    log.Printf("⚠️  WARNING: Authentication is DISABLED …")
}
if !config.AppConfig.EnableRateLimit {
    log.Printf("⚠️  WARNING: Rate limiting is DISABLED (enable_rate_limit: false). " +
        "The API is vulnerable to abuse. " +
        "Set enable_rate_limit: true in config.yaml for production deployments.")
}
```

Adapt field name to match what you found in config.go.

### Step 2 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
```

### Step 3 — Commit and open PR

```bash
git checkout -b fix/rate-limit-default
git add internal/server/server.go
git commit -m "fix(security): emit startup warning when rate limiting is disabled

Adds a startup log warning when enable_rate_limit is false, analogous
to the auth warning added in fix/auth-enabled-default. Does not change
defaults or middleware.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/rate-limit-default
gh pr create \
  --title "fix(security): emit startup warning when rate limiting is disabled" \
  --body "Startup warning when rate limiting is off. No default change. Depends on SEC-2 being merged. Security visibility fix S-3."
```

---

## Checklist

- [ ] Warning log added at server startup for rate limiting
- [ ] Warning only fires when `EnableRateLimit` is false
- [ ] Uses `log.Printf` (or structured logger), NOT `fmt.Printf`
- [ ] Default value of `enable_rate_limit` is NOT changed
- [ ] Warning is adjacent to the SEC-2 auth warning
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] PR opened with correct branch and title
