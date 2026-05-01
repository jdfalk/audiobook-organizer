<!-- file: docs/superpowers/bot-tasks/2026-04-30-mock-1-regenerate.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d2e3f4-a5b6-7890-cdef-123456789abc -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: MOCK-1 — Regenerate MockStore

**TODO ID:** MOCK-1
**Audience:** burndown bot
**Branch:** `fix/mock-regen`
**PR title:** `fix(database): regenerate MockStore to match current Store interface`

---

## What This Task Does

Regenerates `internal/database/mocks/mock_store.go` using mockery so it implements
all methods currently defined on the `Store` interface. After this task, `go vet ./...`
must produce zero errors.

---

## What NOT to Do

- **Do NOT manually edit** `internal/database/mocks/mock_store.go` — always use
  the mockery generator.
- **Do NOT change** `internal/database/store.go` or any interface file — only the
  mock is being updated.
- **Do NOT remove** any existing test that uses `MockStore` — they should compile
  and pass after regeneration.

---

## Read First

Before running any commands, read these files:

1. `docs/MOCKERY_GUIDE.md` — the canonical command to run mockery for this project.
   Use the exact command documented there.
2. `internal/database/store.go` — the `Store` interface (understand what methods
   currently exist, to verify after generation).
3. `Makefile` — look for a `generate` target (e.g., `make generate`) that wraps
   the mockery invocation.

---

## Steps

### Step 1 — Verify current failure

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
go vet ./... 2>&1 | head -40
```

Note the packages that fail. Confirm that failures are related to `MockStore` not
implementing the interface.

### Step 2 — Regenerate the mock

```bash
# Option A: if a make target exists
make generate

# Option B: if no make target, use the command from docs/MOCKERY_GUIDE.md
# (read that file first to find the exact command)
```

### Step 3 — Verify the fix

```bash
go vet ./...
```

Must output nothing (zero errors).

```bash
go build ./...
```

Must succeed.

### Step 4 — Confirm mock satisfies interface

```bash
go test ./internal/database/... -run TestMock -v 2>&1 | head -30
```

If no such test exists, at minimum verify:

```bash
go build ./internal/database/mocks/...
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/mock-regen
git add internal/database/mocks/mock_store.go
git commit -m "fix(database): regenerate MockStore to match current Store interface

MockStore was missing methods added after the last mockery run. Re-ran
make generate to synchronize the mock with the Store interface.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/mock-regen
gh pr create \
  --title "fix(database): regenerate MockStore to match current Store interface" \
  --body "Regenerates MockStore via mockery. Fixes go vet ./... failures across 9+ packages. No logic changes — mock only."
```

---

## Checklist

- [ ] `docs/MOCKERY_GUIDE.md` read
- [ ] `make generate` (or equivalent mockery command) run successfully
- [ ] `go vet ./...` produces zero errors
- [ ] `go build ./...` succeeds
- [ ] Only `internal/database/mocks/mock_store.go` is changed
- [ ] PR opened with correct branch and title
