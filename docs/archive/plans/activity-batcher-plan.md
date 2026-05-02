# Activity Batcher Implementation Plan
<!-- version: 1.0.0 | 2026-04-27 -->

## Problem

The Activity page accumulates thousands of individual rows during any bulk
operation (tag scan, embedded-tag load, metadata apply). Each `log.Printf`
call becomes its own `activity_log` row. A moderate library scan can emit
2,000–5,000 rows in minutes, causing:

- 30-page activity log with no useful signal
- Memory pressure in the frontend (building DOM for thousands of rows)
- `CompactByDay` only helps retroactively — doesn't stop the real-time flood

## Goal

For high-volume repetitive events (same type + source + operation), collapse
N rows into 1 batched row with a grouped message like:

```
Loaded embedded tags from 47 files
  1. Harry Potter and the Chamber of Secrets.m4b — 1 book processed
  2. Dune.m4b — 1 book processed
  …
```

Batch window: **15 seconds**. UI shows "~15s ago" on batch entries.
Non-batchable entries (one-off events, errors, audit entries) are unchanged.

---

## Architecture

```
log.Printf / LogBatch()
        │
        ▼
  ┌─────────────────────────────────────┐
  │  activity.Writer (existing)         │
  │                                     │
  │  sendEntry() ──► Is batchable? ─Yes─► ActivityBatcher
  │                       │                      │
  │                      No                      │ 15s timer (per BatchKey)
  │                       │                      │ or 500-item cap
  │                       ▼                      ▼
  │              channel (10k deep)     Flush → channel
  │                       │
  │              drain() goroutine
  │              (100 entries / 500ms)
  │                       │
  └───────────────────────┼─────────────────────┘
                          ▼
                   ActivityStore.Record()
                   (SQLite activity_log)
```

**BatchKey** = `(type, source, operation_id)`. Entries with the same key
within a 15-second window are merged into one `ActivityEntry` with
`details.batched = true` and a list of sub-items.

**No DB schema change.** The existing `details JSON` column holds the
batched payload. No migration needed.

---

## Data Model (BatchEntry details JSON)

```json
{
  "batched": true,
  "batch_key": "embedded-tag-load:tag-scanner:op-01KQ64DYPNC6BABHGV...",
  "items": [
    {"name": "Harry Potter and the Chamber of Secrets.m4b", "count": 1, "detail": "80 tags"},
    {"name": "Dune.m4b", "count": 1, "detail": "49 tags"},
    {"name": "...", "count": 42}
  ],
  "window_start": "2026-04-27T03:15:00Z",
  "window_end":   "2026-04-27T03:15:15Z",
  "original_count": 47,
  "truncated": false
}
```

Max items stored per batch: **200** (rest counted only). Summary line:
`"Loaded embedded tags from 47 files"` (generated at flush time).

---

## Batchable Entry Detection

An entry is batchable if ALL of the following are true:
1. `OperationID` is non-empty (only batch within operations)
2. `Tier == "debug"` (audit/change entries are always individual)
3. `Type` is in the registered batchable type list (see Task 1)

Registered batchable types (initial set):
- `embedded-tag-load`
- `tag-scan`
- `metadata-apply`
- `path-repair`
- `isbn-enrich`

Callers emit structured batch entries via `LogBatch()` (Task 3) which
bypasses log-line parsing entirely. The log-line parser path is unchanged
and still routes unrecognized lines as non-batchable.

---

## Task Breakdown (Haiku-executable, idempotent)

Each task is independent enough to run in parallel with a separate Haiku
sub-agent. Wire-up order: **Tasks 1, 5, 6** first (independent), then
**Tasks 2, 3, 4** after Task 1 lands.

---

### Task 1 — `internal/activity/batcher.go` (new file)

**Scope:** New file only. No changes to any existing file.

**File to create:** `internal/activity/batcher.go`

**Also create:** `internal/activity/batcher_test.go`

**Implement:**

```go
package activity

import (
    "encoding/json"
    "sync"
    "time"

    "github.com/jdfalk/audiobook-organizer/internal/database"
)

// BatchItem is one element within a batched ActivityEntry.
type BatchItem struct {
    Name   string `json:"name"`   // human-readable subject (filename, book title)
    Count  int    `json:"count"`  // items processed for this subject
    Detail string `json:"detail,omitempty"` // extra info (e.g. "80 tags")
}

// BatchKey identifies a group of related entries to merge.
type BatchKey struct {
    Type        string
    Source      string
    OperationID string
}

// BatchDetails is stored in ActivityEntry.Details["batched"] entries.
type BatchDetails struct {
    Batched       bool        `json:"batched"`
    BatchKeyStr   string      `json:"batch_key"`
    Items         []BatchItem `json:"items"`
    WindowStart   time.Time   `json:"window_start"`
    WindowEnd     time.Time   `json:"window_end"`
    OriginalCount int         `json:"original_count"`
    Truncated     bool        `json:"truncated,omitempty"`
}

const (
    maxBatchItems    = 200           // items stored; rest counted only
    batchWindow      = 15 * time.Second
    batchAbsoluteCap = 60 * time.Second
)

type pendingBatch struct {
    key         BatchKey
    items       []BatchItem
    firstSeen   time.Time
    lastSeen    time.Time
    overflowCnt int    // items beyond maxBatchItems
    timer       *time.Timer
}

// ActivityBatcher accumulates batchable entries for up to 15s,
// then emits one merged ActivityEntry per BatchKey.
type ActivityBatcher struct {
    mu      sync.Mutex
    pending map[BatchKey]*pendingBatch
    out     chan<- database.ActivityEntry // writes go here when flushed
    done    chan struct{}
    wg      sync.WaitGroup
}

// NewActivityBatcher creates a batcher that writes flushed entries to out.
func NewActivityBatcher(out chan<- database.ActivityEntry) *ActivityBatcher {
    b := &ActivityBatcher{
        pending: make(map[BatchKey]*pendingBatch),
        out:     out,
        done:    make(chan struct{}),
    }
    return b
}

// Submit adds item to the pending batch for key.
// If this is the first item, it starts the 15s flush timer.
// If accumulated items reach 500, it triggers an early flush.
func (b *ActivityBatcher) Submit(key BatchKey, item BatchItem) {
    b.mu.Lock()
    defer b.mu.Unlock()

    pb, exists := b.pending[key]
    if !exists {
        pb = &pendingBatch{
            key:       key,
            firstSeen: time.Now(),
        }
        b.pending[key] = pb
        pb.timer = time.AfterFunc(batchWindow, func() { b.flushKey(key) })
    }
    pb.lastSeen = time.Now()

    if len(pb.items) < maxBatchItems {
        pb.items = append(pb.items, item)
    } else {
        pb.overflowCnt++
    }

    // Early flush if we've accumulated 500+ items or hit absolute cap
    total := len(pb.items) + pb.overflowCnt
    if total >= 500 || time.Since(pb.firstSeen) >= batchAbsoluteCap {
        pb.timer.Stop()
        go b.flushKey(key)
    }
}

// FlushAll flushes every pending batch immediately. Safe to call concurrently.
func (b *ActivityBatcher) FlushAll() {
    b.mu.Lock()
    keys := make([]BatchKey, 0, len(b.pending))
    for k := range b.pending {
        keys = append(keys, k)
    }
    b.mu.Unlock()
    for _, k := range keys {
        b.flushKey(k)
    }
}

// flushKey snapshot-and-clears the pending batch for key and emits one entry.
func (b *ActivityBatcher) flushKey(key BatchKey) {
    b.mu.Lock()
    pb, ok := b.pending[key]
    if !ok {
        b.mu.Unlock()
        return
    }
    pb.timer.Stop()
    delete(b.pending, key)
    b.mu.Unlock()

    if len(pb.items)+pb.overflowCnt == 0 {
        return
    }

    details := BatchDetails{
        Batched:       true,
        BatchKeyStr:   key.Type + ":" + key.Source + ":" + key.OperationID,
        Items:         pb.items,
        WindowStart:   pb.firstSeen,
        WindowEnd:     pb.lastSeen,
        OriginalCount: len(pb.items) + pb.overflowCnt,
        Truncated:     pb.overflowCnt > 0,
    }

    raw, _ := json.Marshal(details)
    detailMap := map[string]any{}
    json.Unmarshal(raw, &detailMap) //nolint:errcheck

    noun := pluralize(key.Type)
    summary := fmt.Sprintf("Batched %d %s", details.OriginalCount, noun)

    entry := database.ActivityEntry{
        Tier:        "debug",
        Type:        key.Type,
        Level:       "info",
        Source:      key.Source,
        OperationID: key.OperationID,
        Summary:     summary,
        Details:     detailMap,
    }

    select {
    case b.out <- entry:
    case <-b.done:
    }
}

// Close flushes all pending batches then shuts down the batcher.
func (b *ActivityBatcher) Close() {
    b.FlushAll()
    close(b.done)
}

// pluralize returns a human-readable noun for a batch type.
func pluralize(batchType string) string {
    switch batchType {
    case "embedded-tag-load":
        return "embedded tag loads"
    case "tag-scan":
        return "tag scans"
    case "metadata-apply":
        return "metadata applies"
    case "path-repair":
        return "path repairs"
    case "isbn-enrich":
        return "ISBN enrichments"
    default:
        return batchType + " operations"
    }
}
```

**Tests to write** (`batcher_test.go`):
- Submit 3 items → FlushAll → verify one entry with 3 items in details
- Submit 501 items to same key → verify early flush fires before 15s
- Submit to 2 different keys → FlushAll → verify 2 entries emitted
- Timer-triggered flush: submit 1 item, sleep 16s, verify entry emitted
- Close with pending items → verify all flushed, no panic
- Concurrent Submit from 10 goroutines → no data race (run with `-race`)

**File header:** include `// file:`, `// version: 1.0.0`, `// guid: <new uuid>`

---

### Task 2 — Wire Batcher into `internal/activity/writer.go`

**Scope:** Modify existing `internal/activity/writer.go` only.
Depends on Task 1 (imports `ActivityBatcher`, `BatchKey`, `BatchItem`).

**Changes:**

1. Add `batcher *ActivityBatcher` field to `Writer` struct (after `closed`).

2. In `NewWriter()`, after creating `w`, initialize batcher:
   ```go
   w.batcher = NewActivityBatcher(w.ch)
   ```

3. In `sendEntry()`, after parsing the log line, check if batchable:
   ```go
   if isBatchable(entry) {
       w.batcher.Submit(BatchKey{
           Type:        entry.Type,
           Source:      entry.Source,
           OperationID: entry.OperationID,
       }, BatchItem{Name: entry.Summary})
       return
   }
   ```
   Where `isBatchable` checks:
   ```go
   func isBatchable(e database.ActivityEntry) bool {
       if e.OperationID == "" || e.Tier != "debug" {
           return false
       }
       switch e.Type {
       case "embedded-tag-load", "tag-scan", "metadata-apply",
            "path-repair", "isbn-enrich":
           return true
       }
       return false
   }
   ```
   **Note:** Log-line-parsed entries never have OperationID set (that field
   comes only from structured `LogBatch()` calls). So this guard naturally
   means log-line-parsed entries are never batched — they still go to the
   channel as before. `isBatchable` will only trigger for structured calls.

4. In `Flush()`, add before draining channel:
   ```go
   w.batcher.FlushAll()
   ```

5. In `Stop()`, before closing `done`:
   ```go
   w.batcher.Close()
   ```

6. Bump file version to `1.1.0`.

**Tests:** Update `writer_test.go` to verify:
- Structured `LogBatch()` call routes through batcher (not directly to channel)
- `Flush()` calls `FlushAll()` on batcher
- `Stop()` drains batcher before channel

---

### Task 3 — `LogBatch()` structured API in `internal/activity/api.go` (new file)

**Scope:** New file only. No changes to existing files.

**File to create:** `internal/activity/api.go`

**Purpose:** Callers that know they emit batch-friendly events can call
`LogBatch()` directly, bypassing log-line parsing. This is the primary
path for high-volume structured events.

```go
package activity

import "github.com/jdfalk/audiobook-organizer/internal/database"

// batchableTypes is the registry of type strings that route through the batcher.
var batchableTypes = map[string]bool{
    "embedded-tag-load": true,
    "tag-scan":          true,
    "metadata-apply":    true,
    "path-repair":       true,
    "isbn-enrich":       true,
}

// LogBatch submits a BatchItem directly to the batcher inside w.
// If w is nil or the type is not registered as batchable, the call is a no-op.
// operationID must be non-empty for batching to occur; otherwise the item
// falls through to a regular debug entry.
func LogBatch(w *Writer, operationID, batchType, source string, item BatchItem) {
    if w == nil {
        return
    }
    if operationID == "" || !batchableTypes[batchType] {
        // Fallback: emit as a plain debug entry
        w.ch <- database.ActivityEntry{
            Tier:        "debug",
            Type:        batchType,
            Level:       "info",
            Source:      source,
            OperationID: operationID,
            Summary:     item.Name,
        }
        return
    }
    w.batcher.Submit(BatchKey{
        Type:        batchType,
        Source:      source,
        OperationID: operationID,
    }, item)
}

// FlushOperation immediately flushes all pending batches for operationID.
// Call this when an operation completes so its partial batches are emitted
// before the operation's completion event is written.
func FlushOperation(w *Writer, operationID string) {
    if w == nil {
        return
    }
    w.batcher.mu.Lock()
    keys := []BatchKey{}
    for k := range w.batcher.pending {
        if k.OperationID == operationID {
            keys = append(keys, k)
        }
    }
    w.batcher.mu.Unlock()
    for _, k := range keys {
        w.batcher.flushKey(k)
    }
}
```

**Tests:** `api_test.go`
- `LogBatch` with empty operationID → falls through to plain entry
- `LogBatch` with non-batchable type → falls through to plain entry
- `LogBatch` with valid operationID + batchable type → routed to batcher
- `FlushOperation` → only flushes keys matching operationID

---

### Task 4 — High-Volume Caller Updates

**Scope:** Find and update callers that emit per-book debug log lines during
bulk operations. Add `activity.LogBatch()` calls alongside existing
`log.Printf` calls (don't remove `log.Printf` — it still goes to stdout).

**Search targets:**
```bash
grep -rn "embedded tag\|LoadEmbedded\|tag.*scan\|scanTags\|ExtractMetadata" \
    internal/ --include="*.go" -l
```

**Pattern to apply:** For each file where per-book progress is logged in a
loop with an `operationID` in scope:

```go
// existing code (keep):
log.Printf("[debug] tag-scanner: loaded embedded tags from %s", path)

// add after:
activity.LogBatch(activityWriter, operationID, "embedded-tag-load", "tag-scanner",
    activity.BatchItem{
        Name:   filepath.Base(path),
        Count:  1,
        Detail: fmt.Sprintf("%d tags", tagCount),
    })
```

**Expected callers to update** (verify by searching — don't guess):
- `internal/metadata/` — embedded tag extraction
- `internal/library/` or `internal/scanner/` — library scan progress
- `internal/itunes/service/` — iTunes tag scan (if applicable)
- `internal/ops/` — any op that logs per-book progress in a loop

**Important:** `activityWriter` must be threaded through to callers. If it
isn't currently available, add it as a parameter or via a context value.
Don't create a global. Look for how `ActivityStore` is currently threaded
through to find the pattern.

**Tests:** None required for this task (behavior is tested via Task 1 tests).

---

### Task 5 — Frontend: Batch Entry Rendering

**Scope:** Modify `web/src/pages/ActivityLog.tsx` and create
`web/src/components/activity/BatchActivityEntry.tsx` (new file).

**Goal:** When `entry.details?.batched === true`, render a grouped,
expandable entry instead of a flat row.

**New component `BatchActivityEntry.tsx`:**

```tsx
interface BatchActivityEntryProps {
  entry: ActivityEntry;
}

// Reads entry.details as BatchDetails, renders:
// [▶] Batched 47 embedded tag loads  (2026-04-27 03:15 · ~15s delay)
//     When expanded:
//     1. Harry Potter and the Chamber of Secrets.m4b — 1 book, 80 tags
//     2. Dune.m4b — 1 book, 49 tags
//     … 45 more
```

**Type to add** (`web/src/types/activity.ts` or inline):
```ts
interface BatchDetails {
  batched: true;
  batch_key: string;
  items: Array<{ name: string; count: number; detail?: string }>;
  window_start: string;
  window_end: string;
  original_count: number;
  truncated?: boolean;
}
```

**Changes to `ActivityLog.tsx`:**
1. In the entry render loop, check `entry.details?.batched === true`
2. If true, render `<BatchActivityEntry entry={entry} />`
3. Otherwise, render existing flat row (unchanged)
4. Add `(~15s delay)` tooltip on batch entry timestamps (title attribute)

**Staleness label:** Show on batch entries only:
```tsx
<span title="Batch entries may be up to 15 seconds old" style={{opacity:0.6}}>
  (~15s delay)
</span>
```

**Collapse state:** Use `useState<Set<number>>` for expanded entry IDs.
Default: collapsed.

**Tests:** Add to `web/src/pages/ActivityLog.test.tsx` (or create if absent):
- Renders flat entry for non-batched
- Renders `BatchActivityEntry` for `details.batched === true`
- Expand/collapse toggles item list

---

### Task 6 — Frontend: Performance Hardening

**Scope:** Modify `web/src/pages/ActivityLog.tsx` only.
Fully independent — no dependency on Tasks 1–5.

**Changes:**

1. **Default exclude "debug" tier** on initial load:
   - Change `excludedTiers` initial state from `[]` to `['debug']`
   - Add a UI note: "Debug entries hidden by default. Toggle to show."
   - Persist this default to localStorage so it survives refresh.

2. **Smarter auto-refresh intervals:**
   - Active operations present: keep 3s polling (unchanged)
   - No active operations: increase idle poll from 5s → **60s**
   - Add "Last updated: Xs ago" indicator below the activity feed

3. **Reduce default page size:**
   - Change default `limit` from 50 → **25**
   - This halves the initial DOM node count without losing functionality

4. **Memoize filter construction:**
   - The `fetchActivity` call reconstructs URLSearchParams on every render
   - Wrap the params construction in `useMemo` keyed on filter state
   - This avoids unnecessary re-fetches from referential inequality

5. **Add "Load more" cap:**
   - If total result count > 1000, show a warning banner:
     "Showing most recent 1000 entries. Use filters or compact old entries
     to reduce log size."

**No new dependencies** (no react-window, no virtualization — that's a
bigger change and the page-size reduction + debug exclusion already fixes
the reported issue).

---

## Wire-Up Sequence

```
Wave 1 (parallel — no inter-dependencies):
  Task 1 (batcher.go)
  Task 5 (frontend batch rendering)
  Task 6 (frontend perf)

Wave 2 (after Task 1 merges):
  Task 2 (wire writer)    ─┐
  Task 3 (api.go)         ─┼─ parallel
  Task 4 (caller updates)  ─┘
```

---

## Rollback Strategy

- Tasks 1–3: Pure additions (new files + non-breaking modifications to
  `writer.go`). Roll back by reverting writer.go changes. The batcher
  is opt-in; existing log-line paths are unchanged.
- Task 4: Adding `LogBatch()` calls alongside existing `log.Printf` calls.
  Removing them restores previous behavior exactly.
- Tasks 5–6: Frontend only. Revert the component changes and the default
  excludedTiers state.

---

## Test Strategy

| Task | Test file | Key cases |
|------|-----------|-----------|
| 1 | `batcher_test.go` | merge, early-flush, timer, race |
| 2 | `writer_test.go` | routing, flush order, stop order |
| 3 | `api_test.go` | fallthrough cases, FlushOperation scoping |
| 4 | n/a | manual verification via activity page |
| 5 | `ActivityLog.test.tsx` | batched vs plain rendering |
| 6 | `ActivityLog.test.tsx` | default tier exclusion, page size |

Run all backend tests: `make test`
Run all frontend tests: `make test-all`

---

## Acceptance Criteria

1. A 1,000-book tag scan emits ≤ 10 activity rows visible on the UI
   (one batch row per ~15s window per operation, not 1,000 individual rows)
2. Batch entries expand to show the full item list (up to 200 shown,
   remainder counted)
3. Activity page default view excludes "debug" tier; user can toggle
4. No regression in audit/change tier entries (always written individually)
5. `make test` and `make test-all` pass with `-race`
