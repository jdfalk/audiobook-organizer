<!-- file: docs/superpowers/bot-tasks/2026-05-01-dead-1-remove-unused-code.md -->
<!-- version: 1.0.0 -->
<!-- guid: f5a6b7c8-d9e0-1f2a-3b4c-5d6e7f8a9b0c -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: DEAD-1 — Remove dead code flagged by staticcheck (U1000 / SA4006)

**TODO ID:** DEAD-1  
**Audience:** burndown bot  
**Branch:** `fix/dead-code-cleanup`  
**PR title:** `fix(cleanup): remove dead code flagged by staticcheck`

---

## What This Task Does

Removes or activates unused functions, variables, and constants flagged by
`staticcheck` with U1000 (unused code) and SA4006 (value assigned but never
used). Dead code increases binary size, confuses reviewers, and hides logic gaps.

---

## What NOT to Do

- **Do NOT** delete code that is exported and used by external packages — check
  all usages before deletion.
- **Do NOT** delete `nolint` comments without understanding why they were added.
- **Do NOT** change any logic — only remove or comment out the dead declarations.
- **Do NOT** touch test files unless the dead code is in a test file.

---

## Dead Code Items

### 1. `legacySaveConfigToDatabase_REMOVED` (U1000)
**File:** `internal/config/persistence.go:769`  
The function name explicitly says `_REMOVED` and has been dead since pre-v1.16.0.
Delete the entire function body and its doc comment (lines ~764–800+).

### 2. `bookTagKeyspace` (U1000)
**File:** `internal/database/pebble_store.go:6964`  
An unused `pebbleTagKeyspace` variable. Delete the declaration.

### 3. `bookSummarySelectColumnsQualified` (U1000)
**File:** `internal/database/sqlite_store.go:87`  
An unused `const` that was added alongside `GetBookSummaries` but never wired in.
Either wire it into `GetAllBookSummaries` query or delete it.
Check if `GetAllBookSummaries` could use this constant:
```bash
sed -n '1975,2010p' internal/database/sqlite_store.go
```
If the constant matches the query columns, use it. If not, delete it.

### 4. `linkAsVersion` (U1000)
**File:** `internal/itunes/service/importer.go:1065`  
An unused method on `*Importer`. Delete the method.

### 5. `counterValue` / `value` never used after assignment (SA4006)
**File:** `internal/database/pebble_store.go:236,238`  
Two variables assigned but never read:
```bash
sed -n '230,245p' internal/database/pebble_store.go
```
Either use them or remove the assignments.

### 6. `ok` variables never used (SA4006)
**File:** `internal/metadata/enhanced.go:187,191`  
```bash
sed -n '183,195p' internal/metadata/enhanced.go
```
Map lookup `ok` values are assigned but never checked. Either use them as guards
(`if !ok { continue }`) or use `_` for the ok return if checking is not needed.

---

## Steps

### Step 1 — Verify each item is still present

```bash
staticcheck ./... 2>&1 | grep -E 'U1000|SA4006' | grep -v '_test.go'
```

### Step 2 — Delete / fix each item

Work through items 1–6 above in order. For each deletion, verify no other file
references the deleted symbol:

```bash
grep -rn 'legacySaveConfigToDatabase_REMOVED' internal/ --include='*.go'
grep -rn 'bookTagKeyspace\b' internal/ --include='*.go'
grep -rn 'bookSummarySelectColumnsQualified\b' internal/ --include='*.go'
grep -rn 'linkAsVersion\b' internal/ --include='*.go'
```

### Step 3 — Re-run staticcheck to confirm no remaining U1000/SA4006

```bash
staticcheck ./... 2>&1 | grep -E 'U1000|SA4006' | grep -v '_test.go' | grep -v 'generate_test_itls'
```

(Test-file dead code is lower priority; focus on production code.)

### Step 4 — Build and test

```bash
go build ./...
go vet ./...
go test ./internal/config/... ./internal/database/... ./internal/itunes/... \
  ./internal/metadata/... -timeout 120s 2>&1 | grep -E 'FAIL|ok'
```

### Step 5 — Commit and open PR

```bash
git checkout -b fix/dead-code-cleanup
git add internal/
git commit -m "fix(cleanup): remove dead code flagged by staticcheck U1000/SA4006

Removes legacySaveConfigToDatabase_REMOVED, bookTagKeyspace (pebble),
bookSummarySelectColumnsQualified (sqlite), linkAsVersion (itunes),
and fixes two SA4006 unused-value assignments in pebble_store and
metadata/enhanced. Re-audit finding R-5 / DEAD-1.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/dead-code-cleanup
gh pr create \
  --title "fix(cleanup): remove dead code flagged by staticcheck U1000/SA4006" \
  --body "Removes unused functions, vars, and consts. Fixes staticcheck U1000/SA4006 findings. Re-audit finding R-5."
```

---

## Checklist

- [ ] `legacySaveConfigToDatabase_REMOVED` deleted
- [ ] `bookTagKeyspace` deleted or used
- [ ] `bookSummarySelectColumnsQualified` deleted or wired into query
- [ ] `linkAsVersion` deleted
- [ ] `counterValue` / `value` SA4006 fixed
- [ ] `ok` SA4006 in `enhanced.go` fixed
- [ ] `staticcheck ./...` shows no U1000/SA4006 in production code
- [ ] `go build ./...` clean
- [ ] Tests pass for affected packages
- [ ] PR opened with correct branch and title
