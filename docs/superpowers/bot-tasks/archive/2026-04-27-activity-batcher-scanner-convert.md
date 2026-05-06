<!-- file: docs/superpowers/bot-tasks/2026-04-27-activity-batcher-scanner-convert.md -->
<!-- version: 1.0.0 -->
<!-- guid: 35d7f8ae-2489-7397-1405-789a567 0eff0 -->

# BOT TASK: ACT-BATCH-FU-2 — Convert scanner per-file logs to LogBatch

**TODO ID:** ACT-BATCH-FU-2
**Companion human design:** [`docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md`](../specs/2026-04-27-activity-batcher-followups-design.md)

## Branch

```
refactor/scanner-use-logbatch
```

## Files

- **Read:** `internal/activity/batcher.go` (LogBatch API contract)
- **Read:** `internal/scanner/scanner.go` (find per-file log lines)
- **Edit:** `internal/scanner/scanner.go`
- **Edit:** `internal/scanner/scanner_test.go` (add a test that LogBatch is invoked, not log.Printf)

## What this changes

PR #482 already removed the per-file DEBUG `log.Printf` flood. Some `INFO`-level per-file logs may still go to the activity log via the regular Writer (vs the new structured-batch API). This task converts those to `LogBatch` calls so they collapse server-side into one batched activity entry per scan-window.

## Step 1 — Identify candidates

```
grep -n "log.Printf\|s.activity.Log\|logf\|writer.Log" internal/scanner/scanner.go
```

For each hit, classify:

- **Per-file** (inside a loop over files / books / segments) → **convert to LogBatch**.
- **One-shot** (scan started, scan finished, error summary) → **leave as-is** — these aren't batchable noise; they're load-bearing markers.

If the file has fewer than 3 per-file log calls, this task is a no-op. Mark it complete with a "no conversion needed" CHANGELOG note and flip the TODO.

## Step 2 — Convert

For each per-file log:

```go
// Before:
log.Printf("[scan] processed %s: %d tags", file.Path, tagCount)

// After:
s.activityWriter.LogBatch(ctx, activity.BatchEntry{
    Type:        "scan-file-processed",
    Source:      "scanner",
    OperationID: opID, // whatever the surrounding op ID is in scope
    Item: activity.BatchItem{
        Name:   filepath.Base(file.Path),
        Count:  1,
        Detail: fmt.Sprintf("%d tags", tagCount),
    },
})
```

The exact `BatchEntry` field names must match what the package exports. Read `internal/activity/batcher.go` for the canonical struct.

## Step 3 — Test

Add to `scanner_test.go`:

```go
func TestScanner_PerFileLogsUseBatcher(t *testing.T) {
    fakeWriter := newFakeActivityWriter(t)
    s := newTestScanner(t, fakeWriter)
    s.Scan(ctx, threeTestFiles)

    require.GreaterOrEqual(t, fakeWriter.LogBatchCallCount(), 3, "per-file logs must use LogBatch")
    require.Equal(t, 0, fakeWriter.PrintfFallbackCount(), "no log.Printf flood")
}
```

If `fakeActivityWriter` doesn't exist in the scanner test surface, look across packages (`internal/activity`) for an existing fake before building one.

## Step 4 — Verify

```
go vet ./...
go test -race ./internal/scanner/
make ci
```

Manual verification (skip if not feasible): run a small scan against testdata, confirm activity log shows ONE batched entry per file-type-grouping rather than N individual entries.

## Step 5 — Commit

```
refactor(scanner): per-file logs use LogBatch (ACT-BATCH-FU-2)

Converts <N> per-file log calls inside the scan loop to the structured
LogBatch API. One-shot scan markers (start/finish/summary) preserved.
First real consumer of the structured-batch API shipped in PR #481.

Spec: docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md
```

## Definition of done

- [ ] No per-file `log.Printf` or `activityWriter.Log` calls remain inside scan loops
- [ ] Test asserts LogBatch is called per file
- [ ] `make ci` green (with `-race`)
- [ ] CHANGELOG prepended
- [ ] TODO.md `ACT-BATCH-FU-2` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- The scanner has zero per-file logs (the PR #482 cleanup may have removed them all). Then this task is unnecessary — close it `[x]` with the no-op explanation.
- Scanner emits per-file logs through a Logger interface that has no `LogBatch` method (would mean the API hasn't reached the scanner's logger). Surface the wiring gap; don't add a separate writer.
