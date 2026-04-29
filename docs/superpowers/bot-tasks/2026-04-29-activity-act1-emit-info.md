<!-- file: docs/superpowers/bot-tasks/2026-04-29-activity-act1-emit-info.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7e3a9f1c-4d2b-5087-b6e8-0c4f1a7d3e92 -->
<!-- last-edited: 2026-04-29 -->

# BOT TASK: ACT-1 — Add EmitInfo to series-normalize and dedup-refresh

**TODO ID:** ACT-1
**Audience:** burndown bot
**Branch:** `fix/activity-act1-emit-info-scheduler-ops`
**PR title:** `fix(activity): add EmitInfo summary lines to series-normalize and dedup-refresh`

---

## What This Task Does

Adds `activity.EmitInfo` summary lines to two scheduler tasks that currently finish
silently with no activity log entry visible in the UI:

- `series-normalize` (registered as `series_normalize` task, op type `"series-normalize"`)
- `dedup_refresh` (registered as `dedup_refresh` task, op type `"author-dedup-scan"`)

Both tasks currently use `ts.triggerOperation(...)` — a variant that does NOT pass
`opID` to the callback. To emit an activity entry you need the `opID`. You must
change both tasks from `triggerOperation` to `triggerOperationWithID`.

**There is no `bleve-index-rebuild` task in the scheduler.** The prompt mentioned it
as a third op, but after running `grep -n "bleve" internal/server/scheduler.go` the
result is empty — bleve is used elsewhere but there is no scheduled rebuild task.
Do NOT invent one. Only fix the two tasks listed above.

---

## What NOT to Do

- **Do NOT modify any file outside `internal/server/scheduler.go`** unless you
  also need to update the version header in a file you touch.
- **Do NOT add a bleve-index-rebuild task.** It does not exist.
- **Do NOT change the op type strings** (`"series-normalize"`, `"author-dedup-scan"`)
  — those are used by the frontend and must stay the same.
- **Do NOT remove the existing `progress.Log` calls** — keep them alongside the new
  `EmitInfo` calls.
- **Do NOT import any new packages.** `activity` is already imported in scheduler.go.

---

## Background: EmitInfo Signature

```go
// From internal/activity/api.go:
func EmitInfo(w *Writer, operationID, entryType, source, summary string, tags ...string)
```

- `w` = `ts.server.activityWriter`
- `operationID` = the `opID` string passed into the new `triggerOperationWithID` callback
- `entryType` = same as the op type string (e.g. `"series-normalize"`)
- `source` = same as the op type string (e.g. `"series-normalize"`)
- `summary` = human-readable message string (e.g. `"Series normalize complete: 3 series fixed"`)
- `tags` = `activity.TagsIf(count == 0, activity.NoOpTag)...`

Example of an existing correct EmitInfo call (from the `temp-file-cleanup` task in
the same file, approximately line 352):

```go
activity.EmitInfo(ts.server.activityWriter, opID, "temp-file-cleanup", "temp-file-cleanup", msg,
    activity.TagsIf(removed == 0, activity.NoOpTag)...)
```

---

## Verification: Run These Grep Commands First

Run both commands and read the output before making any changes. The output tells
you the exact line numbers so you know what to edit.

```bash
grep -n "series.normalize\|series_normalize" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/scheduler.go
```

Expected output includes lines similar to:
```
288:		Name:        "series_normalize",
292:			return ts.triggerOperation("series-normalize", func(ctx context.Context, progress operations.ProgressReporter) error {
302:				_, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)
303:				return err
```

```bash
grep -n "dedup.refresh\|dedup_refresh\|author-dedup-scan" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/scheduler.go
```

Expected output includes lines similar to:
```
191:	ts.registerTask(TaskDefinition{
192:		Name:        "dedup_refresh",
196:			return ts.triggerOperation("author-dedup-scan", func(ctx context.Context, progress operations.ProgressReporter) error {
224:			resultMsg := fmt.Sprintf("Dedup scan complete: %d duplicate groups found across %d authors", len(groups), total)
```

---

## Change 1 — series-normalize Task

### Find this exact block (approximately lines 291-304):

```go
			return ts.triggerOperation("series-normalize", func(ctx context.Context, progress operations.ProgressReporter) error {
				store := ts.server.Store()
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				enqueueWB := func(bookID string) {
					if ts.server.writeBackBatcher != nil {
						ts.server.writeBackBatcher.Enqueue(bookID)
					}
				}
				_, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)
				return err
			})
```

### Replace it with:

```go
			return ts.triggerOperationWithID("series-normalize", func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
				store := ts.server.Store()
				if store == nil {
					return fmt.Errorf("database not initialized")
				}
				enqueueWB := func(bookID string) {
					if ts.server.writeBackBatcher != nil {
						ts.server.writeBackBatcher.Enqueue(bookID)
					}
				}
				affected, err := executeSeriesNormalizeCore(ctx, store, enqueueWB)
				msg := fmt.Sprintf("Series normalize complete: %d series affected, %d books enqueued for write-back",
					len(affected), len(affected))
				_ = progress.Log("info", msg, nil)
				activity.EmitInfo(ts.server.activityWriter, opID, "series-normalize", "series-normalize", msg,
					activity.TagsIf(len(affected) == 0, activity.NoOpTag)...)
				return err
			})
```

**What changed:**
1. `triggerOperation` → `triggerOperationWithID`
2. Callback now receives `opID string` as third parameter
3. `_, err :=` changed to `affected, err :=` so we can use the returned slice length
4. Added `msg` variable, `progress.Log`, and `activity.EmitInfo` after the call

---

## Change 2 — dedup_refresh Task

### Find this exact block (approximately lines 224-228):

```go
				resultMsg := fmt.Sprintf("Dedup scan complete: %d duplicate groups found across %d authors", len(groups), total)
				_ = progress.Log("info", resultMsg, nil)
				_ = progress.UpdateProgress(100, 100, resultMsg)
				return nil
			})
```

This block is inside the `triggerOperation("author-dedup-scan", ...)` callback.

### Step 2a — Change `triggerOperation` to `triggerOperationWithID`

Find (approximately line 196):

```go
			return ts.triggerOperation("author-dedup-scan", func(ctx context.Context, progress operations.ProgressReporter) error {
```

Replace with:

```go
			return ts.triggerOperationWithID("author-dedup-scan", func(ctx context.Context, progress operations.ProgressReporter, opID string) error {
```

### Step 2b — Add EmitInfo after the existing resultMsg block

Find (approximately lines 224-227, now with `opID` available):

```go
				resultMsg := fmt.Sprintf("Dedup scan complete: %d duplicate groups found across %d authors", len(groups), total)
				_ = progress.Log("info", resultMsg, nil)
				_ = progress.UpdateProgress(100, 100, resultMsg)
				return nil
```

Replace with:

```go
				resultMsg := fmt.Sprintf("Dedup scan complete: %d duplicate groups found across %d authors", len(groups), total)
				_ = progress.Log("info", resultMsg, nil)
				_ = progress.UpdateProgress(100, 100, resultMsg)
				activity.EmitInfo(ts.server.activityWriter, opID, "author-dedup-scan", "author-dedup-scan", resultMsg,
					activity.TagsIf(len(groups) == 0, activity.NoOpTag)...)
				return nil
```

**What changed:**
1. `triggerOperation` → `triggerOperationWithID` (step 2a above)
2. Callback now receives `opID string` as third parameter (step 2a above)
3. Added `activity.EmitInfo` call after `progress.UpdateProgress` (step 2b above)

---

## Step 3 — Bump the Version Header

Open `internal/server/scheduler.go`. The first three lines look like:

```
// file: internal/server/scheduler.go
// version: 1.17.2
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

Increment the patch version: change `1.17.2` → `1.17.3`.

---

## Step 4 — Verify Build and Tests

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./...
go test ./internal/server/...
```

Both must pass with zero errors. The most likely failure is a type mismatch if
you forgot to change the callback signature from two parameters to three. If that
happens, re-read the diff above carefully.

---

## Step 5 — Commit and Open PR

```bash
git checkout -b fix/activity-act1-emit-info-scheduler-ops
git add internal/server/scheduler.go
git commit -m "fix(activity): add EmitInfo summary lines to series-normalize and dedup-refresh"
git push -u origin fix/activity-act1-emit-info-scheduler-ops
gh pr create \
  --title "fix(activity): add EmitInfo summary lines to series-normalize and dedup-refresh" \
  --body "Changes both tasks from triggerOperation to triggerOperationWithID so they get an opID, then emits an activity.EmitInfo summary line at completion. Adds no-op tag when zero items are affected."
```

---

## Checklist

- [ ] Grep commands run and output read before making any edits
- [ ] `series_normalize` task: `triggerOperation` → `triggerOperationWithID`, opID added, EmitInfo added
- [ ] `dedup_refresh` task: `triggerOperation` → `triggerOperationWithID`, opID added, EmitInfo added
- [ ] No bleve task invented or added
- [ ] Version header in `scheduler.go` bumped to `1.17.3`
- [ ] `go build ./...` passes
- [ ] `go test ./internal/server/...` passes
- [ ] PR opened with correct branch and title
