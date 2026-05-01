<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-10-narrow-server-interfaces.md -->
<!-- version: 1.0.0 -->
<!-- guid: d0e1f2a3-b4c5-6789-defa-012345678901 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-10 — Narrow `*Server` dependencies with small interfaces

**Priority:** Low  
**Effort:** Medium  
**Branch:** `refactor/struct-10-narrow-server-interfaces`

---

## Why This Matters

Most handler functions are declared as `func (s *Server) handlerName(c *gin.Context)`,
giving every handler access to the entire 80+ field `Server` struct. This makes
handlers hard to test in isolation and obscures what each handler actually needs.

The codebase already has a good pattern in `internal/server/audiobook_service.go`
(lines 29–50) where a narrow composite interface is defined locally. We should apply
the same pattern to the 5–10 most complex handler groups.

**Evidence of current pattern:**
```bash
grep -c 'func (s \*Server)' internal/server/*.go
# Many hits across many files
```

**Evidence of existing narrow interface pattern:**
```bash
grep -n 'type.*interface' internal/server/audiobook_service.go
```

---

## What This Task Does

For the handler groups listed below, define a small local interface in the handler
file capturing only the `*Server` fields/methods that group actually uses. Change
the handler receiver from `*Server` to the new interface type.

This does NOT change behaviour — it only tightens the types.

---

## What NOT to Do

- **Do NOT** change any handler logic.
- **Do NOT** change any exported function names or HTTP routes.
- **Do NOT** change the `Server` struct itself.
- **Do NOT** change the `setupRoutes` call sites — `*Server` satisfies any interface
  it implements automatically.
- **Do NOT** change test files (unless they fail to compile after the interface change).

---

## Handler Groups to Narrow

### Group 1: `organize_handlers.go`

Handlers: `previewRename`, `applyRename`, and related (~lines 27–200).

Uses: only `s.Store()`.

Define at top of file:
```go
type organizeStore interface {
    Store() database.Store
}
```

Change receivers:
```go
// BEFORE:
func (s *Server) previewRename(c *gin.Context) {

// AFTER:
func (s organizeStore) previewRename(c *gin.Context) {
```

Do this for every handler in the file that uses only `s.Store()`.

### Group 2: `ai_jobs_handlers.go`

Handlers: `handleListAIJobs`, `handleGetAIJob`, and related (~lines 34–150).

Uses: only `s.Store()`.

Define:
```go
type aiJobsStore interface {
    Store() database.Store
}
```

### Group 3: `filesystem_handlers.go`

Handlers in this file use `s.Store()` and `s.cfg` (config).

Define:
```go
type filesystemDeps interface {
    Store() database.Store
    Config() *ServerConfig   // or whatever the field accessor is
}
```

### Group 4: `reading_handlers.go`

Uses: `s.Store()`, possibly `s.audiobookService`.

Define a narrow interface for those two.

### Group 5: `activity_handlers.go`

Uses: `s.Store()`.

Define `type activityHandlerStore interface { Store() database.Store }`.

---

## Steps

### Step 1 — Read the existing pattern

```bash
sed -n '25,55p' internal/server/audiobook_service.go
```

This shows the narrow composite interface pattern already in use. Your new
interfaces should follow the same style.

### Step 2 — Inventory each handler file

For each handler group above, run:
```bash
grep -n 's\.' internal/server/organize_handlers.go | grep -v '//' | sort -u
```

This shows every `s.Field` or `s.Method()` access. Build the interface from exactly
those accesses.

### Step 3 — Add the interface and change receivers (one file at a time)

For each file:
1. Add the `type XXXDeps interface { ... }` block at the top of the file (after imports).
2. Change all handler receivers in that file from `(s *Server)` to `(s XXXDeps)`.
3. Run `go build ./internal/server/...` — fix errors.
4. `*Server` satisfies the interface implicitly; no call sites need to change.

### Step 4 — Update unit tests if needed

```bash
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|---'
```

If tests fail because they passed `*Server` directly and now need an interface
value: no change is needed — `*Server` satisfies the interface automatically in Go.
If tests used mock structs, update the mock to implement the new interface.

### Step 5 — Final build + test

```bash
go build ./...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok'
```

### Step 6 — Commit and open PR

```bash
git checkout -b refactor/struct-10-narrow-server-interfaces
git add internal/server/
git commit -m "refactor(server): narrow *Server receivers with local handler interfaces

Defines small local interfaces for 5 handler groups (organize, ai_jobs,
filesystem, reading, activity) so each handler only depends on the
fields it actually uses. Follows the existing pattern in audiobook_service.go.
No behaviour changes. Structure audit STRUCT-10.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-10-narrow-server-interfaces
gh pr create \
  --title "refactor(server): narrow *Server receivers with local handler interfaces" \
  --body "Adds narrow local interfaces for 5 handler groups. No behaviour changes. Structure audit STRUCT-10."
```

---

## Checklist

- [ ] Narrow interface defined in each of the 5 handler files
- [ ] Handler receivers updated from `*Server` to the local interface type
- [ ] `*Server` still satisfies all interfaces (no explicit impl needed)
- [ ] `go build ./...` clean
- [ ] Tests pass
- [ ] PR opened on branch `refactor/struct-10-narrow-server-interfaces`
