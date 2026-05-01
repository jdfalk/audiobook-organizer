<!-- file: docs/superpowers/bot-tasks/2026-05-01-test-2-fix-database-test-coverage.md -->
<!-- version: 1.0.0 -->
<!-- guid: c2d3e4f5-a6b7-8c9d-0e1f-2a3b4c5d6e7f -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: TEST-2 — Fix `TestStoreAdditionalCoverageSQLite` failure in database package

**TODO ID:** TEST-2  
**Audience:** burndown bot  
**Branch:** `fix/test-sqlite-coverage`  
**PR title:** `fix(database): fix TestStoreAdditionalCoverageSQLite after recent migrations`

---

## What This Task Does

Diagnoses and fixes the `TestStoreAdditionalCoverageSQLite` failure in
`internal/database`. The test suite reports `FAIL github.com/jdfalk/audiobook-organizer/internal/database`
after 177 seconds. This blocks coverage reporting for the entire database layer.

---

## What NOT to Do

- **Do NOT** skip or remove the failing test.
- **Do NOT** change production database logic to pass a test — fix the test.
- **Do NOT** change the schema in migrations that have already been applied.

---

## Read First

1. Get the full failure output:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/database/ -run 'TestStoreAdditionalCoverageSQLite' -v -timeout 240s 2>&1 | tail -50
```

2. Find the test file:

```bash
grep -rn 'TestStoreAdditionalCoverageSQLite' internal/database/ | head -5
```

3. Read the failing test function and identify the assertion or setup that fails.

4. Check recent migrations that may require schema changes in the test fixture:

```bash
git --no-pager log --oneline internal/database/migrations.go | head -15
```

---

## Steps

### Step 1 — Reproduce and read the error

Run the test with verbose output and capture the full error:

```bash
go test ./internal/database/ -run 'TestStoreAdditionalCoverageSQLite' -v \
  -timeout 240s 2>&1 | grep -A 20 'FAIL\|Error\|panic'
```

### Step 2 — Common fix patterns

**Case A: Missing migration in test DB setup**
If the test sets up a schema manually (not via `RunMigrations`), check if recent
migrations added tables or columns that the test doesn't create. Fix by calling
`RunMigrations(db)` in test setup instead of manual schema.

**Case B: Wrong expected row count / data shape**
If recent schema changes altered what `GetAllBooks` or related queries return,
update the fixture data or assertions to match.

**Case C: Timeout from slow test**
If the test is simply slow, add `t.Parallel()` where safe or reduce the fixture
dataset size.

### Step 3 — Fix and verify

```bash
go test ./internal/database/ -run 'TestStoreAdditionalCoverageSQLite' -v -timeout 240s 2>&1 | tail -10
```

### Step 4 — Run full database test suite

```bash
go test ./internal/database/... -timeout 240s -count=1 2>&1 | grep -E 'FAIL|ok'
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/test-sqlite-coverage
git add internal/database/
git commit -m "fix(database): fix TestStoreAdditionalCoverageSQLite after recent migrations

Updates test fixtures or assertions to account for schema changes
introduced since the last test run. Fixes FAIL in internal/database
package. Re-audit finding TEST-2.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/test-sqlite-coverage
gh pr create \
  --title "fix(database): fix TestStoreAdditionalCoverageSQLite after recent migrations" \
  --body "Fixes the failing database test suite. Re-audit finding TEST-2."
```

---

## Checklist

- [ ] `TestStoreAdditionalCoverageSQLite` passes
- [ ] Full `internal/database/...` suite passes
- [ ] `go build ./...` clean
- [ ] PR opened with correct branch and title
