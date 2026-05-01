<!-- file: docs/superpowers/bot-tasks/2026-04-30-sec-2-auth-default.md -->
<!-- version: 1.0.0 -->
<!-- guid: d8e9f0a1-b2c3-4567-defa-890123456bcd -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: SEC-2 — Emit Startup Warning When Auth is Disabled

**TODO ID:** SEC-2
**Audience:** burndown bot
**Branch:** `fix/auth-enabled-default`
**PR title:** `fix(security): emit startup warning when auth is disabled`

---

## What This Task Does

Adds a prominent log warning at server startup when `config.AppConfig.EnableAuth`
is `false`. This does NOT change the default value — it only makes the insecure
default loud so operators notice it.

---

## What NOT to Do

- **Do NOT change** the default value of `enable_auth` in the config.
- **Do NOT add** authentication middleware — that is a larger separate change.
- **Do NOT add** the warning to every request — only at startup.
- **Do NOT use** `fmt.Printf` — use `log.Printf` or the project's structured logger.

---

## Read First

1. `internal/server/server.go:2262–2278` — find the server startup code. This is
   where the warning must be inserted. Look for `http.ListenAndServe`,
   `server.Start`, or `gin.Run` calls.
2. `internal/config/config.go` — find the `EnableAuth` field (may be named
   `EnableAuth bool`, `enable_auth`, or similar). Confirm the struct field name
   and how config is accessed at startup.
3. Check if the project uses a structured logger (e.g., `internal/logger/`). If it
   does, use that instead of `log.Printf`.

---

## Steps

### Step 1 — Find the startup location

```bash
grep -n 'ListenAndServe\|\.Run(\|\.Serve(\|Starting server\|server started' \
  internal/server/server.go | head -20
```

Find the code block that runs just before the server starts accepting connections.

### Step 2 — Add the warning

Add the following code just before the server start call (adapt field name to match
what you found in config.go):

```go
if !config.AppConfig.EnableAuth {
    log.Printf("⚠️  WARNING: Authentication is DISABLED (enable_auth: false). " +
        "All API endpoints are publicly accessible without credentials. " +
        "Set enable_auth: true in config.yaml for production deployments.")
}
```

If the project uses a structured logger, use:
```go
logger.Warn("authentication is disabled",
    "enable_auth", false,
    "note", "set enable_auth: true in config.yaml for production use")
```

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go build ./...
go vet ./...
```

If you can start the server locally with `enable_auth: false`, verify the warning
appears in the log output on startup.

### Step 4 — Commit and open PR

```bash
git checkout -b fix/auth-enabled-default
git add internal/server/server.go
git commit -m "fix(security): emit startup warning when auth is disabled

Adds a prominent WARNING log at server startup when enable_auth is false.
This makes the insecure default obvious in logs without changing any
default values or existing behaviour.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/auth-enabled-default
gh pr create \
  --title "fix(security): emit startup warning when auth is disabled" \
  --body "Adds a startup log warning when enable_auth is false. No default value changes. Security visibility fix S-2."
```

---

## Checklist

- [ ] Warning log added at server startup
- [ ] Warning only fires when `EnableAuth` is false
- [ ] Warning uses `log.Printf` (or structured logger), NOT `fmt.Printf`
- [ ] Default value of `enable_auth` is NOT changed
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean
- [ ] PR opened with correct branch and title
