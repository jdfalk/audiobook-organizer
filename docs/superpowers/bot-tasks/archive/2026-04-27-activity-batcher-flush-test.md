<!-- file: docs/superpowers/bot-tasks/2026-04-27-activity-batcher-flush-test.md -->
<!-- version: 1.0.0 -->
<!-- guid: 24c6e79d-1378-6286-0394-67894603deef -->

# BOT TASK: ACT-BATCH-FU-1 — LogBatch context-cancel flush test

**TODO ID:** ACT-BATCH-FU-1
**Companion human design:** [`docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md`](../specs/2026-04-27-activity-batcher-followups-design.md)

## Branch

```
test/activity-batcher-cancel-flush
```

## Files

- **Read:** `internal/activity/batcher.go` (find with `find internal/activity -name 'batch*.go'`)
- **Read:** existing batcher tests (`*batcher*_test.go`)
- **Edit or create:** `internal/activity/batcher_test.go` (use existing if present)

## What this test proves

The contract: when a batcher's parent context is cancelled, any partially-accumulated batches (entries that haven't hit the 15s window or 200-item cap yet) flush to the underlying activity store before the batcher exits.

Without this guarantee, the last 15 seconds of an operation's activity is invisible after Ctrl-C / SIGTERM.

## Test skeleton

```go
func TestActivityBatcher_FlushesPendingOnContextCancel(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    fakeStore := newFakeActivityStore(t) // search the package for the existing fake; do not invent one
    b := activity.NewBatcher(ctx, fakeStore /* + any required deps */)

    // Push 3 entries that share a batch key (so they accumulate, don't flush yet).
    for i := 0; i < 3; i++ {
        b.LogBatch(ctx, activity.BatchEntry{
            Type:        "embedded-tag-load",
            Source:      "tag-scanner",
            OperationID: "op-test",
            Item:        activity.BatchItem{Name: fmt.Sprintf("file-%d.m4b", i), Count: 1},
        })
    }

    // Confirm nothing has been flushed to the store yet (window is 15s, only 3 items).
    require.Equal(t, 0, fakeStore.RecordCallCount(), "batcher should be holding 3 entries")

    // Cancel — must flush.
    cancel()

    // Wait briefly for the batcher's drain goroutine to flush. Use a deterministic
    // wait — either a sync primitive the batcher exposes, or a short polling loop with
    // a deadline. Do NOT use bare time.Sleep without a deadline check.
    waitForFlush(t, fakeStore, 1*time.Second)

    require.GreaterOrEqual(t, fakeStore.RecordCallCount(), 1, "pending batch must flush on cancel")

    // Verify the flushed entry contains all 3 items merged.
    last := fakeStore.LastEntry()
    require.Equal(t, 3, len(last.Items))
}
```

Adjust the import path, type names, and helper functions to match what the package actually exports. Read the file first.

## Helper search

The `fakeStore` and `waitForFlush` helpers may already exist in the test file. Look first:

```
grep -n "fakeActivityStore\|fakeStore\|waitFor" internal/activity/*_test.go
```

If they don't exist, build minimal versions. Keep them in the same test file — don't pollute the package.

## Verify

```
go test -run TestActivityBatcher_FlushesPendingOnContextCancel -v ./internal/activity/
go test -race ./internal/activity/  # cancel-flush is goroutine-heavy; race detector matters
make ci
```

The `-race` flag is non-negotiable here. A flush-on-cancel test that races between the test goroutine and the batcher's drain goroutine is exactly the kind of bug this test exists to catch.

## Commit

```
test(activity): batcher flushes pending entries on context cancel (ACT-BATCH-FU-1)

Proves the contract that the 15s/200-item batcher flushes its partial
accumulator when its parent context cancels. Adds <fakeStore helpers if
new>. Runs clean with -race.

Spec: docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md
```

## Definition of done

- [ ] Test passes
- [ ] Test passes with `-race`
- [ ] CHANGELOG prepended
- [ ] TODO.md `ACT-BATCH-FU-1` flipped to `[x]`

## When to STOP

NEEDS_REVIEW if:

- The batcher API exposes no way to push entries from outside the package (everything is private). The test would have to be in `internal/activity` package itself, not `_test`. That's fine — flag the choice in the commit message.
- The batcher's "pending" state isn't observable without a synthetic clock. If the test can't be deterministic without a clock injection that doesn't exist today, surface as a structural gap; don't add `time.Sleep` and call it a day.
