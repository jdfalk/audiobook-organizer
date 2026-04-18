# Eliminate Remaining Package Globals

**Date:** 2026-04-17
**Status:** Approved
**Companion spec:** `2026-04-15-replace-globalstore-with-di-design.md` (completed),
store interface segregation (in progress, separate instance)

## Problem

The DI migration (backlog 4.4) moved `database.GlobalStore` to constructor
injection. Twelve package-level globals remain as service locators and callback
hooks. They block unit testing because tests must set/restore globals (preventing
`t.Parallel()`), hide dependency graphs, and create implicit coupling between
packages.

## Scope

### In scope — 12 globals

| Global | Package | Type | Category |
|--------|---------|------|----------|
| `ActivityRecorder` | operations | `func(ActivityEntry)` | callback hook |
| `ScanActivityRecorder` | scanner | `func(bookID, title string)` | callback hook |
| `DedupOnImportHook` | scanner | `func(bookID string)` | callback hook |
| `itunesActivityRecorder` | server | `func(ActivityEntry)` | callback hook |
| `GlobalQueue` | operations | `Queue` interface | singleton service |
| `GlobalHub` | realtime | `*EventHub` | singleton service |
| `GlobalWriteBackBatcher` | server | `*WriteBackBatcher` | singleton service |
| `GlobalFileIOPool` | server | `*FileIOPool` | singleton service |
| `globalServer` | server | `*Server` | back-reference |
| `GlobalScanner` | scanner | `Scanner` interface | test-only fallback |
| `GlobalMetadataExtractor` | metadata | `MetadataExtractor` | test-only fallback |
| `DB` | database | `*sql.DB` | legacy |

### Out of scope

| Global | Reason |
|--------|--------|
| `config.AppConfig` | 1,186+ read sites; app config is a different category |
| `database.encryptionKey` | Encryption state, not service wiring |
| `database.globalStore` | Handled by store interface segregation effort |

## Design

### Phase 1: Callback hooks → Interface injection

**Targets:** `ActivityRecorder`, `ScanActivityRecorder`, `DedupOnImportHook`,
`itunesActivityRecorder`

These are function variables set by the Server during init so that lower-level
packages (operations, scanner) can call "up" without importing the server
package. Replace with small interfaces accepted at construction time.

#### operations package

```go
// operations/activity.go (new file)
type ActivityLogger interface {
    RecordActivity(entry database.ActivityEntry)
}
```

`NewOperationQueue` gains an `ActivityLogger` parameter:

```go
// Before
func NewOperationQueue(store database.Store, workers int) *operationQueue

// After (final signature — includes hub from Phase 2)
func NewOperationQueue(store database.Store, workers int, logger ActivityLogger, hub *realtime.EventHub) *operationQueue
```

The queue calls `logger.RecordActivity(entry)` where it currently calls
`ActivityRecorder(entry)`. The `ActivityRecorder` package var is deleted.

If `logger` is nil, activity recording is silently skipped (preserving current
nil-check behavior for tests that don't care about activity).

#### scanner package

```go
// scanner/hooks.go (new file)
type ScanHooks interface {
    OnBookScanned(bookID, title string)
    OnImportDedup(bookID string)
}
```

Scanner functions that currently read `ScanActivityRecorder` and
`DedupOnImportHook` accept `ScanHooks` as a parameter instead. Both hook
globals are deleted.

If `hooks` is nil, callbacks are skipped (same nil-check behavior as today).

#### server package (itunesActivityRecorder)

`itunesActivityRecorder` is internal to the server package, so it becomes a
field on the `Server` struct:

```go
type Server struct {
    // ...
    itunesActivityFn func(entry database.ActivityEntry)
}
```

Set during `NewServer()` as a closure over `server.activityService.Record`.

### Phase 2: Singleton services → Server struct fields

**Targets:** `GlobalQueue`, `GlobalHub`, `GlobalWriteBackBatcher`,
`GlobalFileIOPool`

Each becomes a field on the `Server` struct. Handler methods already have
`s *Server` as receiver, so access changes from `operations.GlobalQueue` to
`s.queue`.

```go
type Server struct {
    // ... existing fields ...
    queue            operations.Queue
    hub              *realtime.EventHub
    writeBackBatcher *WriteBackBatcher
    fileIOPool       *FileIOPool
}
```

#### Construction changes in NewServer()

```go
// Before
operations.InitializeQueue(store, workers)
realtime.InitializeEventHub()
InitFileIOPool()

// After
server.hub = realtime.NewEventHub()
server.queue = operations.NewOperationQueue(store, workers, server.activityService)
server.fileIOPool = NewFileIOPool(4)
server.writeBackBatcher = NewWriteBackBatcher(store, server.hub)
```

#### Callsite migration

All handler methods change from global access to field access:

```go
// Before (in handler methods)
operations.GlobalQueue.Enqueue(op)
realtime.GetGlobalHub().Broadcast(event)
GlobalFileIOPool.Submit(job)

// After
s.queue.Enqueue(op)
s.hub.Broadcast(event)
s.fileIOPool.Submit(job)
```

Estimated callsite counts:
- `GlobalQueue`: ~60 sites (all in server package handler methods)
- `GlobalHub` / `GetGlobalHub()`: ~12 sites (server package + operations)
- `GlobalFileIOPool` / `GetGlobalFileIOPool()`: ~5 sites (server package)
- `GlobalWriteBackBatcher`: ~3 sites (server package)

For `GlobalHub` references in the `operations` package (outside server): pass
the hub to `NewOperationQueue` so it can broadcast operation status events
directly.

#### Init function removal

Delete `InitializeQueue()`, `InitializeEventHub()`, `InitFileIOPool()`, and
`InitWriteBackBatcher()`. Construction moves to `NewServer()`.

### Phase 3: Back-references → Specific callbacks

**Targets:** `globalServer`, `GlobalScanner`, `GlobalMetadataExtractor`

#### globalServer

Used in exactly 3 places for 2 purposes:

1. **iTunes import dedup** (`itunes.go:1076, 2316`): Calls
   `globalServer.fireDedupOnImport(bookID)`. Replace with the `DedupOnImportHook`
   callback — but since we're deleting that hook too, the iTunes import functions
   receive a `dedupHook func(bookID string)` parameter, wired in `NewServer()`
   as a closure: `func(id string) { server.fireDedupOnImport(id) }`.

2. **File IO pool recovery** (`file_io_pool.go:214`): Calls
   `globalServer.metadataFetchService` for crash recovery. `NewFileIOPool`
   accepts an optional `recoveryFn func(bookID string)` parameter, wired in
   `NewServer()` as a closure over the server's metadata service.

After both callsites are replaced, `globalServer` is deleted.

#### GlobalScanner and GlobalMetadataExtractor

Both use a nil-fallback pattern: if nil, use the default concrete
implementation. They're only set in tests for mocking.

**Change:** Delete the globals. Test files that currently set them pass the mock
via constructor parameter or function argument instead. The 2-3 affected test
files update their setup to pass mocks explicitly.

### Phase 4: Legacy cleanup

**Target:** `database.DB` (`*sql.DB`)

Check whether any production code references `database.DB` directly:
- If only `SQLiteStore` uses it internally, move it to a private `db` field on
  `SQLiteStore` and delete the package-level var.
- If nothing uses it, delete it entirely.

## Migration strategy

Each phase is a separate PR to keep diffs reviewable:

| PR | Phase | Scope | Risk |
|----|-------|-------|------|
| 1 | Callback hooks | 4 globals, ~15 callsites | Low — behavioral no-op |
| 2 | Singleton services | 4 globals, ~80 callsites | Medium — large mechanical diff |
| 3 | Back-references | 3 globals, ~5 callsites | Low |
| 4 | Legacy cleanup | 1 global | Low |

All PRs must pass existing tests. No behavioral changes — purely structural.

## Testing impact

After this work:
- Services can be unit-tested with mock dependencies (no global setup/teardown)
- Tests can use `t.Parallel()` since there are no shared mutable globals
- Mock injection is explicit in constructors, visible in test setup
- The `testutil.SetupIntegrationTest()` helper simplifies (no global assignment)

## Coordination with store interface segregation

The other instance is splitting `database.Store` into ~25 sub-interfaces. These
efforts are independent:

- **This spec** changes how dependencies are passed (global → constructor)
- **That spec** changes what types are passed (`Store` → `BookReader`, etc.)

The only shared touch-point is `NewServer()` in `server.go`, where both efforts
modify constructor calls. Merge conflicts will be trivial (different lines).

## Non-goals

- Decomposing the Server struct into smaller handler groups
- Eliminating `config.AppConfig`
- Refactoring the operations queue interface
- Adding new abstractions beyond what's needed for injection
