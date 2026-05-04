<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-1-server-response-helpers.md -->
<!-- version: 1.0.0 -->
<!-- guid: b1c2d3e4-f5a6-7890-bcde-f12345678901 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: STRUCT-1 — Add HTTP response helpers to `internal/server`

**TODO ID:** STRUCT-1
**Audience:** burndown bot
**Branch:** `refactor/struct-1-server-response-helpers`
**PR title:** `refactor(server): add shared HTTP response helper functions`

---

## What This Task Does

`internal/server/error_handler.go` **already exists** with 14 response helpers
(`RespondWithError`, `RespondWithBadRequest`, `RespondWithNotFound`,
`RespondWithInternalError`, `RespondWithSuccess`, `RespondWithList`,
`RespondWithCreated`, `RespondWithOK`, `RespondWithNoContent`, etc.).

Despite these helpers existing, **287 handlers still call `c.JSON(http.Status...)` directly**.
This task replaces the three highest-frequency direct-call patterns with the existing
helpers — no new code, just adoption.

**Evidence of the problem:**
- `c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})` — **36×**
- `c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})` — **14×**
- `c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})` — **11×**

---

## What NOT to Do

- **Do NOT** create a new response file — use the existing `error_handler.go` helpers.
- **Do NOT** change handler logic, only the response call.
- **Do NOT** change test files.

---

## Step-by-step

### Step 1 — Read the existing helpers

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
cat internal/server/error_handler.go | head -80
```

Note the exact function signatures (`RespondWithError`, `RespondWithBadRequest`,
`RespondWithInternalError`, `RespondWithNotFound`, `RespondWithSuccess`).

### Step 2 — Find the 36 "database not initialized" direct calls

```bash
grep -rn '"database not initialized"' internal/server/ | grep -v '_test' | grep 'c\.JSON'
```

For each hit, replace:
```go
// BEFORE:
c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})

// AFTER:
RespondWithInternalError(c, errors.New("database not initialized"))
```

Or if the existing helper signature differs, use whatever matches. Add `"errors"` import
if not already present.

### Step 3 — Find the 14 BadRequest direct calls

```bash
grep -rn 'c\.JSON(http\.StatusBadRequest, gin\.H{"error": err\.Error' internal/server/ | grep -v '_test'
```

For each hit, replace:
```go
// BEFORE:
c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

// AFTER:
RespondWithBadRequest(c, err)
```

### Step 4 — Find the 11 InternalServerError direct calls

```bash
grep -rn 'c\.JSON(http\.StatusInternalServerError, gin\.H{"error": err\.Error' internal/server/ | grep -v '_test'
```

For each hit, replace with `RespondWithInternalError(c, err)`.

### Step 5 — Build and test

```bash
go build ./internal/server/...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

Both must pass.

### Step 6 — Bump version headers on all changed files

Every changed file gets a patch bump on `<!-- version: X.Y.Z -->`.

### Step 7 — Commit and open PR

```bash
git checkout -b refactor/struct-1-server-response-helpers
git add internal/server/
git commit -m "refactor(server): adopt existing RespondWith* helpers for common patterns

Replaces 61 direct c.JSON() calls (36 db-not-initialized, 14 bad-request,
11 internal-error) with the existing error_handler.go helpers. Reduces
boilerplate and centralises error message wording. Structure audit STRUCT-1.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-1-server-response-helpers
gh pr create \
  --title "refactor(server): adopt existing RespondWith* helpers for common patterns" \
  --body "Replaces 61 direct c.JSON calls with existing error_handler.go helpers. Structure audit STRUCT-1."
```

---

## Checklist

- [ ] `internal/server/response.go` created with all 8 functions + `ErrDBNotInitialized`
- [ ] `go build ./internal/server/...` clean
- [ ] Version header present on new file
- [ ] PR opened on branch `refactor/struct-1-server-response-helpers`
