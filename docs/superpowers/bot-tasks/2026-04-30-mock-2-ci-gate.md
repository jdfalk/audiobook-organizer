<!-- file: docs/superpowers/bot-tasks/2026-04-30-mock-2-ci-gate.md -->
<!-- version: 1.0.0 -->
<!-- guid: d2e3f4a5-b6c7-8901-defa-234567890bcd -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: MOCK-2 — Add Mock Freshness Check and staticcheck to make ci

**TODO ID:** MOCK-2
**Audience:** burndown bot
**Branch:** `chore/mock-ci-gate`
**PR title:** `chore(ci): add mock freshness check and staticcheck to make ci`

**Prerequisite:** MOCK-1 must be merged first (mock must be fresh before gating on it).

---

## What This Task Does

Adds two new steps to `make ci` in the `Makefile`:

1. **Mock freshness gate** — re-runs `go generate`, then diffs against the committed
   mock. `make ci` fails if they differ.
2. **staticcheck** — runs `staticcheck ./...` after `go vet`. Documents install
   instructions in the Makefile comment.

---

## What NOT to Do

- **Do NOT modify GitHub Actions workflow files** — the gate lives in `make ci` only.
- **Do NOT change** test logic or any source files.
- **Do NOT add new Go source files** — this is purely a Makefile change.
- **Do NOT remove** existing `make ci` steps.

---

## Read First

1. `Makefile` — read the full `ci` target to understand what it currently runs and
   where to insert the new steps.
2. `docs/MOCKERY_GUIDE.md` — find the exact `go generate` command or `//go:generate`
   directive used to regenerate the mock.

---

## Steps

### Step 1 — Read the current ci target

```bash
grep -A 50 '^ci:' /Users/jdfalk/.worktrees/audiobook-eval/Makefile
```

Understand the existing steps. Note where `go vet` appears — the new steps go after
`go vet` and before the test step (or at the end, after tests, if there is no obvious
insertion point).

### Step 2 — Add the mock freshness check

Find the `go generate` command from `docs/MOCKERY_GUIDE.md`. Add a new target and
incorporate it into `ci`. The pattern to add (adapt indentation/tabs to match the
Makefile style):

```makefile
.PHONY: check-mock-fresh
check-mock-fresh:
	@echo "==> Checking mock freshness..."
	go generate ./internal/database/...
	git diff --exit-code internal/database/mocks/ || \
		(echo "ERROR: MockStore is stale. Run 'make generate' and commit the result." && exit 1)
	@echo "==> Mock is fresh."
```

Add `check-mock-fresh` as a dependency of `ci` (or call it inline in the `ci` target).

### Step 3 — Add staticcheck

Add a new target and incorporate it into `ci`:

```makefile
.PHONY: staticcheck
staticcheck:
	@echo "==> Running staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)..."
	staticcheck ./...
	@echo "==> staticcheck passed."
```

Add `staticcheck` as a step in `ci` after `go vet ./...`.

### Step 4 — Verify the gate works

Test that introducing a mock drift causes failure:

```bash
# Temporarily add a fake method to the Store interface
echo "" >> internal/database/store.go
echo "// TestMethod is a temporary method for CI gate testing" >> internal/database/store.go
echo "TestMethod() error" >> internal/database/store.go

# Now run just the freshness check
make check-mock-fresh
# Expect: non-zero exit (mock is stale)

# Undo the change
git checkout internal/database/store.go
```

### Step 5 — Commit and open PR

```bash
git checkout -b chore/mock-ci-gate
git add Makefile
git commit -m "chore(ci): add mock freshness check and staticcheck to make ci

Adds two new gates to 'make ci':
1. check-mock-fresh: re-runs go generate and diffs the mock; fails if stale.
2. staticcheck: runs honnef.co/go/tools staticcheck after go vet.

Install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin chore/mock-ci-gate
gh pr create \
  --title "chore(ci): add mock freshness check and staticcheck to make ci" \
  --body "Adds mock freshness gate (go generate + git diff) and staticcheck to make ci. Prevents stale mocks from silently breaking the test suite."
```

---

## Checklist

- [ ] `Makefile` `ci` target now calls mock freshness check
- [ ] `Makefile` `ci` target now calls staticcheck
- [ ] Mock freshness check fails when mock is manually drifted (verified)
- [ ] `make ci` passes on the clean repo
- [ ] Only `Makefile` is changed in the commit
- [ ] PR opened with correct branch and title
