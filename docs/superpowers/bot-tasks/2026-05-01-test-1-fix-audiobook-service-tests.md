<!-- file: docs/superpowers/bot-tasks/2026-05-01-test-1-fix-audiobook-service-tests.md -->
<!-- version: 1.0.0 -->
<!-- guid: b1c2d3e4-f5a6-7b8c-9d0e-1f2a3b4c5d6e -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: TEST-1 — Fix `audiobook_service_unit_test.go` after GetAllBookSummaries migration

**TODO ID:** TEST-1  
**Audience:** burndown bot  
**Branch:** `fix/test-audiobook-service-summaries`  
**PR title:** `fix(server): update audiobook_service_unit_test.go for GetAllBookSummaries`

---

## What This Task Does

Fixes 11+ failing unit tests in `internal/server/audiobook_service_unit_test.go` that
broke when `audiobook_service.go` was updated (PROJ-1/PROJ-2) to call
`GetAllBookSummaries` instead of `GetAllBooks`. The tests still set up
`Mock.On("GetAllBooks")` but the production code now calls `GetAllBookSummaries`.

---

## What NOT to Do

- **Do NOT** revert the `GetAllBookSummaries` change in production code.
- **Do NOT** remove test cases — fix them.
- **Do NOT** add `Mock.On("GetAllBooks")` back; replace it with `GetAllBookSummaries`.
- **Do NOT** change the `Store` interface or mock generation.

---

## Read First

1. Run the failing tests to get the full error list:

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./internal/server/ -run 'TestAudiobookService' -v -timeout 30s 2>&1 | head -60
go test ./internal/server/ -run 'TestGetAudiobooks' -v -timeout 30s 2>&1 | head -60
go test ./internal/server/ -run 'TestServerSide' -v -timeout 30s 2>&1 | head -60
```

2. Read the current implementation:

```bash
grep -n 'GetAllBookSummaries\|GetAllBooks' internal/server/audiobook_service.go | head -20
```

3. Read the test file:

```bash
cat internal/server/audiobook_service_unit_test.go
```

---

## Steps

### Step 1 — Identify all failing tests and their mock setups

For each failing test, find the `MockOn("GetAllBooks", …)` call and replace with
`MockOn("GetAllBookSummaries", …)`.

The `BookSummary` type is in `internal/database/store.go`. Use:
```go
mock.On("GetAllBookSummaries", limitArg, offsetArg).Return([]database.BookSummary{…}, nil)
```

### Step 2 — Fix `GetUserBookState` mock expectations

Several tests also fail with `GetUserBookState(string,string)` unexpected calls.
Add appropriate stubs:
```go
mock.On("GetUserBookState", mock.Anything, mock.Anything).Return((*database.UserBookState)(nil), nil)
```

### Step 3 — Update return value shapes

`BookSummary` has fewer fields than `Book`. Adjust any assertions that check fields
only present on `Book` but not `BookSummary`. Check what `GetAudiobooks` returns to
the caller and adjust test assertions accordingly.

### Step 4 — Run all server tests

```bash
go test ./internal/server/... -timeout 60s -count=1 2>&1 | grep -E 'FAIL|ok|PASS'
```

All tests should pass. If `TestServerSideSorting` or `TestServerSideFieldFiltering`
still fail, read their error output and fix the specific mock expectations.

### Step 5 — Verify no regressions elsewhere

```bash
go build ./...
go vet ./...
go test ./internal/... -timeout 120s -count=1 2>&1 | grep -E 'FAIL|ok' | grep -v mocks
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/test-audiobook-service-summaries
git add internal/server/
git commit -m "fix(server): update audiobook_service_unit_test.go for GetAllBookSummaries

Unit tests were still mocking GetAllBooks after audiobook_service.go was
updated by PROJ-1/PROJ-2 to call GetAllBookSummaries. Also adds missing
GetUserBookState stubs. Fixes 11+ failing tests in the server package.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/test-audiobook-service-summaries
gh pr create \
  --title "fix(server): update audiobook_service_unit_test.go for GetAllBookSummaries" \
  --body "Fixes 11+ failing unit tests caused by PROJ-1/PROJ-2 migration from GetAllBooks to GetAllBookSummaries. Re-audit finding TEST-1."
```

---

## Checklist

- [ ] All `TestAudiobookService_*` tests pass
- [ ] All `TestGetAudiobooks_*` tests pass
- [ ] `TestServerSideSorting` passes
- [ ] `TestServerSideFieldFiltering` passes
- [ ] `go build ./...` clean
- [ ] PR opened with correct branch and title
