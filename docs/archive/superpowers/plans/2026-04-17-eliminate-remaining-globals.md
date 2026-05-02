# Eliminate Remaining Package Globals — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove 13 package-level global variables that act as service locators or callback hooks, replacing them with constructor injection so services can be unit-tested in isolation.

**Architecture:** Each global becomes either a struct field (singletons → Server fields), a constructor parameter (callback hooks → interfaces), or is deleted (test-only fallbacks). Four PRs, one per phase, each a behavioral no-op — only the wiring changes.

**Tech Stack:** Go 1.24, Mockery v3 (auto-generated mocks), Gin HTTP framework

**Spec:** `docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md`

---

## File Structure

### New files
- `internal/operations/activity.go` — `ActivityLogger` interface (3 lines)
- `internal/scanner/hooks.go` — `ScanHooks` interface (6 lines)
- `internal/organizer/hooks.go` — `OrganizeHooks` interface (4 lines)

### Modified files (by phase)

**Phase 1 (callback hooks):**
- `internal/operations/queue.go` — delete `ActivityRecorder` var, add `activityLogger` field to `OperationQueue`, update constructor + 4 callsites
- `internal/scanner/scanner.go` — delete `ScanActivityRecorder` + `DedupOnImportHook` vars, thread `ScanHooks` through package-level functions + 1 callsite
- `internal/organizer/organizer.go` — delete `OrganizeCollisionHook` var, add `hooks` field to `Organizer`, update constructor + 2 callsites
- `internal/server/itunes.go` — delete `itunesActivityRecorder` var, use `s.itunesActivityFn` field + 1 callsite
- `internal/server/server.go` — update `NewServer()` wiring for all 5 hooks

**Phase 2 (singleton services → Server fields):**
- `internal/server/server.go` — add `queue`, `hub`, `writeBackBatcher`, `fileIOPool` fields to Server struct; update `NewServer()` to construct inline; update `Shutdown()`
- `internal/operations/queue.go` — delete `GlobalQueue`, `InitializeQueue`; add `hub` field to `OperationQueue`
- `internal/realtime/events.go` — delete `GlobalHub`, `GetGlobalHub()`, `SetGlobalHub()`, `InitializeEventHub()`
- `internal/server/itunes_writeback_batcher.go` — delete `GlobalWriteBackBatcher`, `InitWriteBackBatcher()`
- `internal/server/file_io_pool.go` — delete `GlobalFileIOPool`, `globalFileIOPoolMu`, `GetGlobalFileIOPool()`, `SetGlobalFileIOPool()`, `InitFileIOPool()`
- ~30 handler files in `internal/server/` — mechanical `GlobalQueue` → `s.queue`, `GetGlobalHub()` → `s.hub`, etc.
- Test files — update `setupTestServer` and per-test setup

**Phase 3 (back-references + test-only globals):**
- `internal/server/file_io_pool.go` — delete `globalServer`, accept recovery callback in constructor
- `internal/server/itunes.go` — accept `dedupHook func(string)` parameter in import functions
- `internal/scanner/scanner.go` — delete `GlobalScanner`, make package-level functions accept optional `Scanner` or use default
- `internal/metadata/metadata.go` — delete `GlobalMetadataExtractor`, make `ExtractMetadata` accept optional extractor
- Test files that set these globals — pass mocks via parameter instead

**Phase 4 (legacy DB):**
- `internal/database/database.go` — delete `var DB *sql.DB` and `Initialize()`
- `internal/scanner/scanner.go` — replace `database.DB` usage with store interface calls
- `internal/playlist/playlist.go` — replace `database.DB` usage with store interface calls
- `internal/tagger/tagger.go` — replace `database.DB` usage with store interface calls

---

## Task 1: Define ActivityLogger interface + inject into OperationQueue

**Files:**
- Create: `internal/operations/activity.go`
- Modify: `internal/operations/queue.go:24,72-84,96-122,286-296,347-358,385-396,573-584`
- Modify: `internal/server/server.go:1019-1024`

- [ ] **Step 1: Create the ActivityLogger interface**

```go
// file: internal/operations/activity.go
package operations

import "github.com/jdfalk/audiobook-organizer/internal/database"

// ActivityLogger receives activity entries for audit logging.
// Implementations must be safe for concurrent use.
type ActivityLogger interface {
	RecordActivity(entry database.ActivityEntry)
}
```

- [ ] **Step 2: Add activityLogger field to OperationQueue and update constructor**

In `internal/operations/queue.go`, add the field to the struct and constructor:

```go
// In OperationQueue struct (line ~73):
type OperationQueue struct {
	// ... existing fields ...
	activityLogger ActivityLogger
}

// Update NewOperationQueue (line ~96):
func NewOperationQueue(store database.Store, workers int, activityLogger ActivityLogger) *OperationQueue {
	// ... existing body, plus:
	q := &OperationQueue{
		// ... existing fields ...
		activityLogger: activityLogger,
	}
	// ... rest unchanged ...
}
```

- [ ] **Step 3: Replace all ActivityRecorder reads with q.activityLogger**

Replace 4 callsites in `queue.go`. Each follows the same pattern:

```go
// Before (e.g., line 286):
if ActivityRecorder != nil {
    ActivityRecorder(database.ActivityEntry{...})
}

// After:
if q.activityLogger != nil {
    q.activityLogger.RecordActivity(database.ActivityEntry{...})
}
```

For the `queueStoreAdapter.CreateOperationChange` method (line ~573), the adapter needs access to the logger. Add it as a field:

```go
type queueStoreAdapter struct {
	store          database.Store
	activityLogger ActivityLogger
}
```

Update the adapter creation in the worker (line ~299):
```go
storeAdapter := &queueStoreAdapter{store: q.store, activityLogger: q.activityLogger}
```

And in `CreateOperationChange`:
```go
if a.activityLogger != nil {
    a.activityLogger.RecordActivity(database.ActivityEntry{...})
}
```

- [ ] **Step 4: Delete the ActivityRecorder global variable**

Remove line 24 from `queue.go`:
```go
// DELETE THIS:
var ActivityRecorder func(entry database.ActivityEntry)
```

- [ ] **Step 5: Update NewServer() wiring in server.go**

In `internal/server/server.go`, replace the global assignment (lines 1022-1024):

```go
// Before:
operations.ActivityRecorder = func(entry database.ActivityEntry) {
    _ = server.activityService.Record(entry)
}

// After: (handled by passing activityService to NewOperationQueue — see step 6)
```

The queue is currently created via `InitializeQueue` (called from `cmd/root.go`). After Phase 2 moves queue construction into `NewServer()`, the activity logger will be passed there. For now, create a bridge: make the server's activity service implement `ActivityLogger`:

```go
// In server.go, define a thin adapter:
type activityServiceLogger struct {
    svc *ActivityService
}

func (a *activityServiceLogger) RecordActivity(entry database.ActivityEntry) {
    _ = a.svc.Record(entry)
}
```

Then in `NewServer()`, after the queue is available, inject the logger:
```go
if server.activityService != nil {
    if oq, ok := operations.GlobalQueue.(*operations.OperationQueue); ok {
        oq.SetActivityLogger(&activityServiceLogger{svc: server.activityService})
    }
}
```

Add a `SetActivityLogger` method to `OperationQueue`:
```go
func (q *OperationQueue) SetActivityLogger(logger ActivityLogger) {
    q.activityLogger = logger
}
```

- [ ] **Step 6: Update InitializeQueue to pass nil logger**

In `queue.go`, update `InitializeQueue` (line ~714):
```go
func InitializeQueue(store database.Store, workers int) {
    if GlobalQueue != nil {
        log.Println("Warning: operation queue already initialized")
        return
    }
    GlobalQueue = NewOperationQueue(store, workers, nil) // logger wired later by server
    log.Printf("Operation queue initialized with %d workers", workers)
}
```

- [ ] **Step 7: Run tests**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && make test
```

Expected: All existing tests pass. The behavioral change is nil — activity recording still flows through the same code path.

- [ ] **Step 8: Commit**

```bash
git add internal/operations/activity.go internal/operations/queue.go internal/server/server.go
git commit -m "refactor: inject ActivityLogger into OperationQueue, delete global"
```

---

## Task 2: Define ScanHooks interface + inject into scanner functions

**Files:**
- Create: `internal/scanner/hooks.go`
- Modify: `internal/scanner/scanner.go:37,48,1553-1560`
- Modify: `internal/server/server.go:927,1044-1053`
- Modify: `internal/scanner/save_book_to_database_test.go:172-176`

- [ ] **Step 1: Create the ScanHooks interface**

```go
// file: internal/scanner/hooks.go
package scanner

// ScanHooks provides optional callbacks for scan-time side effects.
// All methods must be safe for concurrent use. A nil ScanHooks value
// means no hooks fire — callers must nil-check before calling.
type ScanHooks interface {
	OnBookScanned(bookID, title string)
	OnImportDedup(bookID string)
}
```

- [ ] **Step 2: Add a package-level scanHooks variable (temporarily)**

In `scanner.go`, replace the two separate globals with one:

```go
// Before (lines 37, 48):
var ScanActivityRecorder func(bookID, title string)
var DedupOnImportHook func(bookID string)

// After:
// scanHooks is the active hooks instance. Set by SetScanHooks().
var scanHooks ScanHooks

// SetScanHooks sets the scan hooks. This replaces the former
// ScanActivityRecorder and DedupOnImportHook package globals.
func SetScanHooks(hooks ScanHooks) {
    scanHooks = hooks
}
```

Note: We use a single unexported var + setter instead of a constructor because `scanner` package functions are called as package-level functions (`scanner.ScanDirectory(...)`), not via a struct. The exported setter is the injection point.

- [ ] **Step 3: Update the hook callsites in scanner.go**

At line ~1553:
```go
// Before:
if ScanActivityRecorder != nil {
    ScanActivityRecorder(dbBook.ID, dbBook.Title)
}
if DedupOnImportHook != nil {
    DedupOnImportHook(dbBook.ID)
}

// After:
if scanHooks != nil {
    scanHooks.OnBookScanned(dbBook.ID, dbBook.Title)
    scanHooks.OnImportDedup(dbBook.ID)
}
```

- [ ] **Step 4: Update server.go wiring**

In `NewServer()`, replace the two separate assignments (lines 927, 1044-1053) with one `SetScanHooks` call. Create an adapter struct in server.go:

```go
type serverScanHooks struct {
	activityService *ActivityService
	dedupFn         func(bookID string)
}

func (h *serverScanHooks) OnBookScanned(bookID, title string) {
	if h.activityService != nil {
		_ = h.activityService.Record(database.ActivityEntry{
			Tier:    "change",
			Type:    "scan",
			Level:   "info",
			Source:  "background",
			BookID:  bookID,
			Summary: fmt.Sprintf("Scan found: %s", title),
		})
	}
}

func (h *serverScanHooks) OnImportDedup(bookID string) {
	if h.dedupFn != nil {
		h.dedupFn(bookID)
	}
}
```

Wire it in `NewServer()`:
```go
// Replace lines 927 and 1044-1053 with:
scanner.SetScanHooks(&serverScanHooks{
    activityService: server.activityService,
    dedupFn:         server.fireDedupOnImport,
})
```

- [ ] **Step 5: Update test that sets DedupOnImportHook**

In `internal/scanner/save_book_to_database_test.go:172-176`, replace:

```go
// Before:
prevHook := DedupOnImportHook
DedupOnImportHook = func(bookID string) {
    hookCalls = append(hookCalls, bookID)
}
t.Cleanup(func() { DedupOnImportHook = prevHook })

// After:
type testHooks struct{ calls []string }
func (h *testHooks) OnBookScanned(_, _ string) {}
func (h *testHooks) OnImportDedup(bookID string) { h.calls = append(h.calls, bookID) }
th := &testHooks{}
prevHooks := scanHooks // unexported, so test must be in scanner package (it already is)
SetScanHooks(th)
t.Cleanup(func() { SetScanHooks(prevHooks) })
```

Then check `th.calls` instead of `hookCalls`.

- [ ] **Step 6: Run tests**

```bash
make test
```

- [ ] **Step 7: Commit**

```bash
git add internal/scanner/hooks.go internal/scanner/scanner.go internal/scanner/save_book_to_database_test.go internal/server/server.go
git commit -m "refactor: replace scanner globals with ScanHooks interface"
```

---

## Task 3: Inject OrganizeCollisionHook into Organizer struct

**Files:**
- Create: `internal/organizer/hooks.go`
- Modify: `internal/organizer/organizer.go:37-43,122-124,161-163`
- Modify: `internal/server/server.go:941-973`
- Modify: `internal/organizer/organizer_test.go:945-949`

- [ ] **Step 1: Create the OrganizeHooks interface**

```go
// file: internal/organizer/hooks.go
package organizer

// OrganizeHooks provides optional callbacks for organize-time side effects.
type OrganizeHooks interface {
	OnCollision(currentBookID, occupantPath string)
}
```

- [ ] **Step 2: Add hooks field to Organizer and update constructor**

Check the Organizer struct and its constructor. Add a `hooks` field:

```go
type Organizer struct {
    // ... existing fields ...
    hooks OrganizeHooks
}
```

Add a setter (the Organizer may be constructed before hooks are available):

```go
func (o *Organizer) SetHooks(hooks OrganizeHooks) {
    o.hooks = hooks
}
```

- [ ] **Step 3: Replace OrganizeCollisionHook callsites**

At lines 122-124 and 161-163 in `organizer.go`:

```go
// Before:
if OrganizeCollisionHook != nil {
    OrganizeCollisionHook(book.ID, existingBook.FilePath)
}

// After:
if o.hooks != nil {
    o.hooks.OnCollision(book.ID, existingBook.FilePath)
}
```

- [ ] **Step 4: Delete the global variable**

Remove from `organizer.go`:
```go
// DELETE:
var OrganizeCollisionHook func(currentBookID, occupantPath string)
```

- [ ] **Step 5: Update server.go wiring**

Replace lines 941-973 in `server.go`. Create an adapter and wire it:

```go
type serverOrganizeHooks struct {
    server *Server
}

func (h *serverOrganizeHooks) OnCollision(currentBookID, occupantPath string) {
    if h.server.embeddingStore == nil || h.server.store == nil {
        return
    }
    h.server.bgWG.Add(1)
    go func() {
        defer h.server.bgWG.Done()
        occupant, err := h.server.store.GetBookByFilePath(occupantPath)
        if err != nil {
            log.Printf("[WARN] organize-collision hook: lookup %s failed: %v", occupantPath, err)
            return
        }
        if occupant == nil || occupant.ID == currentBookID {
            return
        }
        sim := 1.0
        if err := h.server.embeddingStore.UpsertCandidate(database.DedupCandidate{
            EntityType: "book",
            EntityAID:  currentBookID,
            EntityBID:  occupant.ID,
            Layer:      "exact",
            Similarity: &sim,
            Status:     "pending",
        }); err != nil {
            log.Printf("[WARN] organize-collision hook: upsert candidate %s/%s failed: %v",
                currentBookID, occupant.ID, err)
        }
    }()
}
```

Wire it after the organizer is available in NewServer():
```go
server.organizeService.SetHooks(&serverOrganizeHooks{server: server})
```

Note: Check whether `organizeService` wraps an `Organizer` or is one. Adapt the wiring path accordingly.

- [ ] **Step 6: Update organizer test**

In `internal/organizer/organizer_test.go:945-949`:

```go
// Before:
prev := OrganizeCollisionHook
OrganizeCollisionHook = func(bookID, occupant string) {
    calls = append(calls, call{bookID, occupant})
}
t.Cleanup(func() { OrganizeCollisionHook = prev })

// After:
type testHooks struct{ calls []call }
func (h *testHooks) OnCollision(bookID, occupant string) {
    h.calls = append(h.calls, call{bookID, occupant})
}
th := &testHooks{}
org.SetHooks(th) // org is the Organizer instance in this test
t.Cleanup(func() { org.SetHooks(nil) })
```

Then check `th.calls`.

- [ ] **Step 7: Run tests**

```bash
make test
```

- [ ] **Step 8: Commit**

```bash
git add internal/organizer/hooks.go internal/organizer/organizer.go internal/organizer/organizer_test.go internal/server/server.go
git commit -m "refactor: inject OrganizeHooks into Organizer, delete global"
```

---

## Task 4: Move itunesActivityRecorder to Server field

**Files:**
- Modify: `internal/server/itunes.go:139,2248`
- Modify: `internal/server/server.go:669-730,1039-1041`

- [ ] **Step 1: Add field to Server struct**

In `server.go`, add to the Server struct (line ~729):

```go
type Server struct {
    // ... existing fields ...
    itunesActivityFn func(entry database.ActivityEntry)
}
```

- [ ] **Step 2: Wire in NewServer()**

Replace lines 1039-1041:

```go
// Before:
itunesActivityRecorder = func(entry database.ActivityEntry) {
    _ = server.activityService.Record(entry)
}

// After:
server.itunesActivityFn = func(entry database.ActivityEntry) {
    _ = server.activityService.Record(entry)
}
```

- [ ] **Step 3: Update the callsite in itunes.go**

Line 2248 is inside `executeITunesSync()` (line 2067) — a package-level function with signature `func executeITunesSync(ctx context.Context, store database.Store, log logger.Logger, libraryPath string, pathMappings []itunes.PathMapping) error`. It has no Server access.

Add an `activityFn` parameter:

```go
func executeITunesSync(ctx context.Context, store database.Store, log logger.Logger, libraryPath string, pathMappings []itunes.PathMapping, activityFn func(database.ActivityEntry)) error {
```

Then at line 2248:
```go
// Before:
if itunesActivityRecorder != nil {
    itunesActivityRecorder(database.ActivityEntry{...})
}

// After:
if activityFn != nil {
    activityFn(database.ActivityEntry{...})
}
```

Update the caller (likely `handleITunesSync` on Server, line 1973) to pass `s.itunesActivityFn`.

- [ ] **Step 4: Delete the global variable**

Remove from `itunes.go` line 139:
```go
// DELETE:
var itunesActivityRecorder func(entry database.ActivityEntry)
```

- [ ] **Step 5: Run tests**

```bash
make test
```

- [ ] **Step 6: Commit**

```bash
git add internal/server/itunes.go internal/server/server.go
git commit -m "refactor: move itunesActivityRecorder to Server field"
```

---

## Task 5: Commit Phase 1 PR

- [ ] **Step 1: Run full test suite**

```bash
make ci
```

- [ ] **Step 2: Create PR**

```bash
git push -u origin feat/eliminate-callback-globals
gh pr create --title "refactor: eliminate callback hook globals (Phase 1)" \
  --body "Phase 1 of globals elimination. Replaces 5 callback function globals with interface injection:
- operations.ActivityRecorder → ActivityLogger interface
- scanner.ScanActivityRecorder + DedupOnImportHook → ScanHooks interface
- organizer.OrganizeCollisionHook → OrganizeHooks interface
- server.itunesActivityRecorder → Server struct field

Spec: docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md"
```

---

## Task 6: Move GlobalQueue to Server field

**Files:**
- Modify: `internal/operations/queue.go:711-721` — delete `GlobalQueue`, `InitializeQueue`
- Modify: `internal/server/server.go:669-730` — add `queue` field
- Modify: ~30 handler files — `operations.GlobalQueue` → `s.queue`
- Modify: `internal/server/server.go:1019-1081` — construct queue inline
- Modify: `cmd/root.go` — remove `InitializeQueue` call
- Modify: test files — update setup helpers

- [ ] **Step 1: Add queue field to Server struct**

```go
type Server struct {
    // ... existing fields ...
    queue operations.Queue
}
```

- [ ] **Step 2: Construct queue in NewServer()**

Replace the external `InitializeQueue` call. In `NewServer()`, after the store is resolved:

```go
server.queue = operations.NewOperationQueue(resolvedStore, 2, nil)
```

The activity logger will be wired later in the same function (from Task 1).

- [ ] **Step 3: Remove InitializeQueue from cmd/root.go**

Find and remove the call to `operations.InitializeQueue(...)` in `cmd/root.go`. Also remove early queue initialization in `main.go` if present.

- [ ] **Step 4: Mechanical replacement — all handler files**

This is the largest single change (~55 callsites). The pattern is always:

```go
// Before:
if operations.GlobalQueue == nil {
    c.JSON(http.StatusServiceUnavailable, ...)
    return
}
operations.GlobalQueue.Enqueue(...)

// After:
if s.queue == nil {
    c.JSON(http.StatusServiceUnavailable, ...)
    return
}
s.queue.Enqueue(...)
```

Files to update (search for `operations.GlobalQueue` or `GlobalQueue`):
- `internal/server/operations_handlers.go`
- `internal/server/duplicates_handlers.go`
- `internal/server/dedup_handlers.go`
- `internal/server/metadata_handlers.go`
- `internal/server/entities_handlers.go`
- `internal/server/ai_handlers.go`
- `internal/server/metadata_batch_candidates.go`
- `internal/server/openlibrary_service.go`
- `internal/server/reconcile.go`
- `internal/server/filesystem_handlers.go`
- `internal/server/organize_service.go`
- `internal/server/diagnostics_handlers.go`
- `internal/server/scheduler.go`
- `internal/server/itunes.go`
- `internal/server/itunes_path_reconcile.go`
- `internal/server/server.go` (shutdown + other references)

- [ ] **Step 5: Delete GlobalQueue and InitializeQueue**

In `queue.go`, remove lines 711-721:
```go
// DELETE:
var GlobalQueue Queue

func InitializeQueue(store database.Store, workers int) {
    // ...
}
```

- [ ] **Step 6: Update test setup helpers**

In `internal/server/server_test.go`, `setupTestServer` and similar helpers that set `operations.GlobalQueue`:

```go
// Before:
operations.GlobalQueue = operations.NewOperationQueue(store, 2)

// After:
// The queue is now created inside NewServer, no external setup needed.
// If tests need a mock queue, set server.queue directly or pass via constructor.
```

Also update `internal/testutil/integration.go:54`.

- [ ] **Step 7: Run tests**

```bash
make test
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: move GlobalQueue to Server.queue field"
```

---

## Task 7: Move GlobalHub to Server field

**Files:**
- Modify: `internal/realtime/events.go:323-351` — delete globals and accessors
- Modify: `internal/server/server.go` — add `hub` field, construct in NewServer()
- Modify: `internal/operations/queue.go` — add `hub` field, accept in constructor
- Modify: handler files referencing `realtime.GetGlobalHub()`

- [ ] **Step 1: Add hub field to Server struct and OperationQueue**

In `server.go`:
```go
type Server struct {
    // ... existing fields ...
    hub *realtime.EventHub
}
```

In `queue.go`, add to OperationQueue:
```go
type OperationQueue struct {
    // ... existing fields ...
    hub *realtime.EventHub
}
```

Update `NewOperationQueue` to accept the hub:
```go
func NewOperationQueue(store database.Store, workers int, activityLogger ActivityLogger, hub *realtime.EventHub) *OperationQueue {
    // ... existing body, plus:
    q := &OperationQueue{
        // ... existing fields ...
        hub: hub,
    }
}
```

- [ ] **Step 2: Construct hub in NewServer()**

```go
server.hub = realtime.NewEventHub()
server.queue = operations.NewOperationQueue(resolvedStore, 2, nil, server.hub)
```

- [ ] **Step 3: Replace GetGlobalHub() in operations/queue.go**

Replace ~7 callsites in `queue.go`:

```go
// Before:
if hub := realtime.GetGlobalHub(); hub != nil {
    hub.SendOperationStatus(...)
}

// After:
if q.hub != nil {
    q.hub.SendOperationStatus(...)
}
```

- [ ] **Step 4: Replace GetGlobalHub() in server handler files**

Replace ~5 callsites in server.go, system_handlers.go:

```go
// Before:
if hub := realtime.GetGlobalHub(); hub != nil {
    hub.Broadcast(...)
}

// After:
if s.hub != nil {
    s.hub.Broadcast(...)
}
```

- [ ] **Step 5: Delete globals from realtime/events.go**

Remove `GlobalHub`, `GetGlobalHub()`, `SetGlobalHub()`, `InitializeEventHub()`. Keep `NewEventHub()` — that's the constructor we now call from `NewServer()`.

- [ ] **Step 6: Update test files**

Any test that calls `realtime.InitializeEventHub()` or `realtime.SetGlobalHub()` now either:
- Gets the hub from `server.hub` (if testing via server), or
- Creates its own `realtime.NewEventHub()` and passes it directly

- [ ] **Step 7: Run tests**

```bash
make test
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: move GlobalHub to Server.hub and OperationQueue.hub"
```

---

## Task 8: Move GlobalWriteBackBatcher + GlobalFileIOPool to Server fields

**Files:**
- Modify: `internal/server/server.go` — add fields, construct inline
- Modify: `internal/server/itunes_writeback_batcher.go` — delete global + init func
- Modify: `internal/server/file_io_pool.go` — delete global + init func + globalServer
- Modify: ~20 files with `GlobalWriteBackBatcher` references
- Modify: ~5 files with `GlobalFileIOPool` / `GetGlobalFileIOPool()` references

- [ ] **Step 1: Add fields to Server struct**

```go
type Server struct {
    // ... existing fields ...
    writeBackBatcher *WriteBackBatcher
    fileIOPool       *FileIOPool
}
```

- [ ] **Step 2: Construct inline in NewServer()**

Replace lines 1072-1078:

```go
// Before:
InitWriteBackBatcher()
globalServer = server
InitFileIOPool()

// After:
server.writeBackBatcher = NewWriteBackBatcher(5 * time.Second)
server.fileIOPool = NewFileIOPool(4)

// Register recovery handler with a closure instead of globalServer:
RegisterFileOpRecovery("apply_metadata", func(bookID string) {
    if server.metadataFetchService == nil {
        log.Printf("[WARN] no metadata service for apply_metadata recovery of book %s", bookID)
        return
    }
    // ... rest of recovery logic from current InitFileIOPool
})
```

- [ ] **Step 3: Mechanical replacement — GlobalWriteBackBatcher**

~58 callsites. Pattern:

```go
// Before:
if GlobalWriteBackBatcher != nil {
    GlobalWriteBackBatcher.Enqueue(bookID)
}

// After:
if s.writeBackBatcher != nil {
    s.writeBackBatcher.Enqueue(bookID)
}
```

For non-handler code that references `GlobalWriteBackBatcher` (e.g., services), pass the batcher via setter or constructor parameter.

- [ ] **Step 4: Mechanical replacement — GlobalFileIOPool**

~5 callsites. Pattern:

```go
// Before:
if pool := GetGlobalFileIOPool(); pool != nil {
    pool.SubmitTyped(...)
}

// After:
if s.fileIOPool != nil {
    s.fileIOPool.SubmitTyped(...)
}
```

- [ ] **Step 5: Delete globals and init functions**

From `itunes_writeback_batcher.go`:
```go
// DELETE:
var GlobalWriteBackBatcher *WriteBackBatcher
func InitWriteBackBatcher() { ... }
```

From `file_io_pool.go`:
```go
// DELETE:
var GlobalFileIOPool *FileIOPool
var globalFileIOPoolMu sync.Mutex
var globalServer *Server
func GetGlobalFileIOPool() *FileIOPool { ... }
func SetGlobalFileIOPool(p *FileIOPool) { ... }
func InitFileIOPool() { ... }
```

- [ ] **Step 6: Update Shutdown() in server.go**

The shutdown sequence references these globals. Update to use fields:

```go
// Before:
if GlobalWriteBackBatcher != nil {
    GlobalWriteBackBatcher.Stop()
}

// After:
if s.writeBackBatcher != nil {
    s.writeBackBatcher.Stop()
}
```

- [ ] **Step 7: Update test files**

Test setup helpers that set `GlobalWriteBackBatcher` or `SetGlobalFileIOPool()` now set `server.writeBackBatcher` and `server.fileIOPool` directly.

- [ ] **Step 8: Run tests**

```bash
make test
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: move WriteBackBatcher + FileIOPool to Server fields, delete globalServer"
```

---

## Task 9: Commit Phase 2 PR

- [ ] **Step 1: Run full test suite**

```bash
make ci
```

- [ ] **Step 2: Create PR**

```bash
gh pr create --title "refactor: move singleton services to Server fields (Phase 2)" \
  --body "Phase 2 of globals elimination. Moves 4 singleton globals to Server struct fields:
- operations.GlobalQueue → Server.queue
- realtime.GlobalHub → Server.hub + OperationQueue.hub
- GlobalWriteBackBatcher → Server.writeBackBatcher
- GlobalFileIOPool → Server.fileIOPool

Also deletes globalServer back-reference (replaced by closure in file IO pool recovery).

Spec: docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md"
```

---

## Task 10: Delete GlobalScanner + GlobalMetadataExtractor test-only globals

**Files:**
- Modify: `internal/scanner/scanner.go:55-65,142,151,271,280,1643`
- Modify: `internal/metadata/metadata.go:32,98`
- Modify: `internal/scanner/scanner_coverage_test.go:724-769`
- Modify: `internal/server/server_import_paths_and_blocklist_test.go:123-129`
- Modify: `internal/server/server_import_file_mocks_test.go:49-171`
- Modify: `internal/server/audiobook_service_tags_test.go:77-85`
- Modify: `internal/metadata/assemble_test.go:250-257`

- [ ] **Step 1: Refactor scanner package-level functions to accept optional Scanner**

The package-level functions like `ScanDirectory`, `ProcessBooks`, etc. currently check `GlobalScanner`. Add an optional parameter or use a different injection strategy.

Since these are called from many places, the cleanest approach is an unexported `var` with a setter (same pattern as ScanHooks):

```go
// In scanner.go, replace GlobalScanner:

// scanner is the active Scanner implementation. Defaults to nil,
// which means the concrete functions in this package are used.
var activeScanner Scanner

// SetScanner overrides the default scanner implementation.
// Used in tests to inject mocks.
func SetScanner(s Scanner) {
    activeScanner = s
}

// Update each function:
func ScanDirectory(rootDir string, scanLog logger.Logger) ([]Book, error) {
    if activeScanner != nil {
        return activeScanner.ScanDirectory(rootDir, scanLog)
    }
    return ScanDirectoryParallel(rootDir, 1, scanLog)
}
```

Delete the exported `GlobalScanner` variable and the `Scanner` interface export (keep the interface, just change the injection point).

- [ ] **Step 2: Refactor metadata package similarly**

In `metadata.go`:

```go
// Replace:
var GlobalMetadataExtractor MetadataExtractor

// With:
var activeExtractor MetadataExtractor

func SetMetadataExtractor(e MetadataExtractor) {
    activeExtractor = e
}

// Update ExtractMetadata:
func ExtractMetadata(filePath string) (*Metadata, error) {
    if activeExtractor != nil {
        return activeExtractor.ExtractMetadata(filePath)
    }
    // ... default implementation
}
```

- [ ] **Step 3: Update all test files**

Each test that sets `GlobalScanner` or `GlobalMetadataExtractor` now calls the setter:

```go
// Before:
scanner.GlobalScanner = mockScanner
t.Cleanup(func() { scanner.GlobalScanner = origScanner })

// After:
scanner.SetScanner(mockScanner)
t.Cleanup(func() { scanner.SetScanner(nil) })
```

Same pattern for metadata:
```go
metadata.SetMetadataExtractor(mockExtractor)
t.Cleanup(func() { metadata.SetMetadataExtractor(nil) })
```

- [ ] **Step 4: Run tests**

```bash
make test
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: replace GlobalScanner + GlobalMetadataExtractor with setter injection"
```

---

## Task 11: Remove legacy database.DB global

**Files:**
- Modify: `internal/database/database.go:15`
- Modify: `internal/scanner/scanner.go:1583-1610`
- Modify: `internal/playlist/playlist.go:38-236`
- Modify: `internal/tagger/tagger.go:19`
- Modify: related test files

- [ ] **Step 1: Audit database.DB usage**

Three production files use `database.DB`:
1. `scanner.go:1583-1610` — raw SQL for author/series/book insert (legacy save path)
2. `playlist/playlist.go:38-236` — raw SQL for playlist CRUD
3. `tagger/tagger.go:19` — raw SQL query for book metadata

All of these bypass the Store interface and use raw SQL directly. They should be migrated to use the Store interface methods instead.

- [ ] **Step 2: Replace database.DB in scanner.go**

The code at lines 1583-1610 does author/series/book lookups and inserts via raw SQL. Replace with Store method calls. This code path may be dead (superseded by `saveBookToDatabase` which uses the Store). Verify by checking if the function containing these lines is called anywhere. If dead, delete it.

- [ ] **Step 3: Replace database.DB in playlist/playlist.go**

Replace raw SQL queries with calls to the Store's playlist methods (`CreatePlaylist`, `GetPlaylistByID`, etc.). The playlist package needs to accept a Store parameter (constructor injection or function parameter).

- [ ] **Step 4: Replace database.DB in tagger/tagger.go**

Replace the raw SQL query with a Store method call. The tagger needs to accept a Store parameter.

- [ ] **Step 5: Delete database.DB and Initialize()**

From `database.go`:
```go
// DELETE:
var DB *sql.DB

func Initialize(path string) error {
    // ...
}
```

Also remove any `database.DB` assignment in `store.go` (line ~1209 sets it for backwards compatibility).

- [ ] **Step 6: Update test files**

Test files that set `database.DB` directly now use the Store interface instead, matching the production code changes.

- [ ] **Step 7: Run tests**

```bash
make test
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: remove legacy database.DB global, migrate to Store interface"
```

---

## Task 12: Phase 3+4 PR + final verification

- [ ] **Step 1: Run full CI**

```bash
make ci
```

- [ ] **Step 2: Create PR**

```bash
gh pr create --title "refactor: remove test-only globals + legacy database.DB (Phase 3+4)" \
  --body "Phase 3+4 of globals elimination:
- GlobalScanner → unexported var + SetScanner() setter
- GlobalMetadataExtractor → unexported var + SetMetadataExtractor() setter
- database.DB → migrated to Store interface calls

Spec: docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md"
```

- [ ] **Step 3: Verify all globals are gone**

```bash
cd internal
grep -rn 'var Global\|var global\|var ActivityRecorder\|var ScanActivityRecorder\|var DedupOnImportHook\|var itunesActivityRecorder\|var OrganizeCollisionHook\|var DB \*sql' --include='*.go' | grep -v '_test.go' | grep -v 'mocks/'
```

Expected: No matches (all globals eliminated from production code).

- [ ] **Step 4: Update spec status**

Change status in `docs/superpowers/specs/2026-04-17-eliminate-remaining-globals-design.md` from "Approved" to "Completed".
