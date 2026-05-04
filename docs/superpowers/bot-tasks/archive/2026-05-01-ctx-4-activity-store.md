<!-- file: docs/superpowers/bot-tasks/2026-05-01-ctx-4-activity-store.md -->
<!-- version: 1.0.0 -->
<!-- guid: a6b7c8d9-e0f1-2a3b-4c5d-6e7f8a9b0c1d -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: CTX-4 — Thread context through `activity_store.go` transactions

**TODO ID:** CTX-4  
**Audience:** burndown bot  
**Branch:** `fix/ctx-activity-store`  
**PR title:** `fix(database): thread context through ActivityStore transactions`

---

## What This Task Does

Replaces 8 `context.Background()` calls in `internal/database/activity_store.go`
with a passed-in `context.Context`. The same fix was applied to `sqlite_store.go`
by DB-2, but `activity_store.go` was missed. `Summarize` and `CompactByDay` are
called from HTTP handlers and should propagate the request context.

---

## What NOT to Do

- **Do NOT** add context to `Record(e ActivityEntry)` — it is called from
  fire-and-forget goroutines where no request context is available. Keep that one
  with `context.Background()` or a stored background context.
- **Do NOT** change the `ActivityStore` struct interface exported from
  `internal/activity/service.go` without updating all callers.
- **Do NOT** change the `Store` database interface in `store.go`.

---

## Read First

1. Identify all `context.Background()` calls:

```bash
grep -n 'context\.Background()' internal/database/activity_store.go
```

2. Read the `Summarize` and `CompactByDay` function signatures:

```bash
grep -n 'func.*Summarize\|func.*CompactByDay\|func.*Prune' internal/database/activity_store.go
```

3. Find all callers of `Summarize` and `CompactByDay`:

```bash
grep -rn '\.Summarize(\|\.CompactByDay(' --include='*.go' internal/ | grep -v '_test.go' | head -20
```

4. Check the `ActivityStore` interface if one exists:

```bash
grep -rn 'Summarize\|CompactByDay' internal/activity/ --include='*.go' | head -10
```

---

## Steps

### Step 1 — Add `ctx context.Context` to `Summarize`

```go
// Before:
func (s *ActivityStore) Summarize(olderThan time.Time, tier string) (int, error) {

// After:
func (s *ActivityStore) Summarize(ctx context.Context, olderThan time.Time, tier string) (int, error) {
```

Replace all `context.Background()` inside `Summarize` with `ctx`.

### Step 2 — Add `ctx context.Context` to `CompactByDay`

```go
// Before:
func (s *ActivityStore) CompactByDay(olderThan time.Time) (CompactResult, error) {

// After:
func (s *ActivityStore) CompactByDay(ctx context.Context, olderThan time.Time) (CompactResult, error) {
```

Replace all `context.Background()` inside `CompactByDay` with `ctx`.

### Step 3 — Update callers

For each caller found in Step 3 of Read First:
- If called from an HTTP handler: pass `c.Request.Context()`
- If called from a background goroutine or scheduler: pass a stored base context
  or `context.WithTimeout(context.Background(), 5*time.Minute)`

```bash
grep -rn '\.Summarize(\|\.CompactByDay(' --include='*.go' internal/ | grep -v '_test.go'
```

### Step 4 — Update any interface definitions

If `ActivityStore` implements an interface in `internal/activity/service.go` or
`internal/database/store.go`, update the interface signature too.

### Step 5 — Build and test

```bash
go build ./...
go test ./internal/database/... ./internal/activity/... -timeout 60s 2>&1 | grep -E 'FAIL|ok'
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/ctx-activity-store
git add internal/database/activity_store.go internal/activity/ internal/server/
git commit -m "fix(database): thread context through ActivityStore transactions

Replaces context.Background() in Summarize() and CompactByDay() with
a caller-supplied context.Context. Enables cancellation of long-running
compaction transactions when the caller's context is done. Re-audit
finding R-6 / CTX-4. Mirrors the DB-2 fix applied to sqlite_store.go.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/ctx-activity-store
gh pr create \
  --title "fix(database): thread context through ActivityStore transactions" \
  --body "Adds context propagation to ActivityStore.Summarize and CompactByDay. Re-audit finding R-6."
```

---

## Checklist

- [ ] `Summarize` accepts `ctx context.Context` as first param
- [ ] `CompactByDay` accepts `ctx context.Context` as first param
- [ ] No `context.Background()` in `Summarize` or `CompactByDay`
- [ ] All callers updated (HTTP handlers pass request ctx; goroutines pass timeout ctx)
- [ ] Any interface definition updated to match new signature
- [ ] `go build ./...` clean
- [ ] Tests pass
- [ ] PR opened with correct branch and title
