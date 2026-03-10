<!-- file: docs/superpowers/specs/2026-03-10-central-logger-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: b3c4d5e6-f7a8-9012-3456-789abcdef012 -->

# Central Logger Package Design

## Problem

The codebase has two disconnected logging channels:

1. **`log.Printf()`** in library code (scanner, metadata, organizer) — goes to stdout only, lost on restart
2. **`progress.Log()`** in service code — goes to operation DB + real-time WebSocket hub

When a user requests "all log levels" for an operation, they only see the service-level INFO logs. The thousands of DEBUG/TRACE lines from the actual work (tag extraction, file moves, scoring) are invisible in the UI because they never reach the database.

Additionally, 4 background goroutines run completely outside the operations system with no visibility at all: OpenLibrary downloads, updater checks, iTunes sync scheduler, and the stale operation reaper.

## Solution

A unified `internal/logger` package that replaces both channels with a single `Logger` interface. Every logger instance routes to stdout, and optionally to the operation log DB and real-time hub when bound to an operation.

## Logger Interface

```go
package logger

type Level int

const (
    LevelTrace Level = iota
    LevelDebug
    LevelInfo
    LevelWarn
    LevelError
)

// Logger is the central interface for logging, progress, and change tracking.
type Logger interface {
    Trace(msg string, args ...any)
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)

    // Progress reporting (operations only; no-op on StandardLogger)
    UpdateProgress(current, total int, message string)

    // Change tracking
    RecordChange(change Change)

    // Operation awareness
    IsCanceled() bool

    // Create child logger with subsystem prefix
    With(subsystem string) Logger
}

// Change represents a tracked change during an operation.
type Change struct {
    BookID     string
    ChangeType string // "book_create", "book_update", "file_move", "metadata_update"
    Field      string // optional: specific field name
    OldValue   string // optional
    NewValue   string // optional
    Summary    string // human-readable
}
```

## Implementations

### StandardLogger

For code outside operations: housekeeping goroutines, startup, CLI commands.

- All log calls go to stdout
- `UpdateProgress`, `RecordChange`, `IsCanceled` are no-ops
- Optionally writes to `system_activity_log` table for INFO+ messages

```go
log := logger.New("scheduler")
log.Info("starting scheduled task tick")  // → stdout + system_activity_log
log.Debug("checking interval for task X") // → stdout only
```

### OperationLogger

For code inside operations: scan, organize, metadata fetch, etc.

- All log calls go to stdout (if >= `minStdout` level)
- INFO+ log calls also go to operation_logs DB table and real-time hub (if >= `minDBLevel`)
- Progress updates go to operations table + real-time hub
- Changes go to operation_changes table + in-memory counters

```go
log := logger.ForOperation(operationID, store, hub)
scanLog := log.With("scanner")
scanLog.Debug("extracting tags for %s", path)  // → stdout only (default)
scanLog.Info("scanned 150 books")               // → stdout + DB + real-time
scanLog.RecordChange(Change{ChangeType: "book_create", Summary: "Created 'Storm Front'"})
```

### OperationLogger Fields

```go
type OperationLogger struct {
    operationID string
    subsystem   string
    store       database.Store
    hub         *realtime.Hub
    minDBLevel  Level            // default: LevelInfo
    minStdout   Level            // default: LevelDebug
    canceled    *atomic.Bool
    changes     []Change
    counters    map[string]int   // {"book_create": 150, "book_update": 43}
    mu          sync.Mutex       // protects changes/counters
}
```

## Level Filtering

| Level | Stdout | DB + Real-time (default) | DB + Real-time (verbose) |
|-------|--------|--------------------------|--------------------------|
| TRACE | Yes    | No                       | Yes (when configured)    |
| DEBUG | Yes    | No                       | Yes (when configured)    |
| INFO  | Yes    | Yes                      | Yes                      |
| WARN  | Yes    | Yes                      | Yes                      |
| ERROR | Yes    | Yes                      | Yes                      |

Default: `minStdout = DEBUG`, `minDBLevel = INFO`.

Users can set `minDBLevel` to DEBUG or TRACE for troubleshooting via config or per-operation override.

## Data Flow

### Log call
```
scanLog.Debug("extracting tags for %s", path)
    ├──→ stdout (if >= minStdout)
    ├──→ operation_logs table (if >= minDBLevel)
    └──→ real-time hub (if >= minDBLevel)
```

### Change
```
scanLog.RecordChange(Change{ChangeType: "book_create", ...})
    ├──→ operation_changes table (always persisted)
    └──→ In-memory counter incremented
```

### Progress
```
scanLog.UpdateProgress(150, 8298, "Scanning folder: Matt Dinniman")
    ├──→ operations table (progress fields)
    └──→ real-time hub (WebSocket push)
```

## Function Signature Changes

Library code accepts `logger.Logger` instead of using the global `log` package:

| Package | Current | New |
|---------|---------|-----|
| `scanner.ScanAudiobooks` | `(folders, books, ...)` | Add `logger.Logger` param |
| `metadata.ExtractMetadataFromFile` | `(path)` | Add `logger.Logger` param |
| `organizer.Organizer.OrganizeBook` | `(book)` | Add `logger.Logger` param |
| `mediainfo.Extract` | `(path)` | Add `logger.Logger` param |

Service layer creates the logger at the operation boundary and passes it down. Library code never imports `operations`, `database`, or `realtime` — it only knows `logger.Logger`.

## Backward Compatibility

`OperationLogger` implements the existing `operations.ProgressReporter` interface, so existing code works unchanged during migration:

```go
func (l *OperationLogger) Log(level, message string, details *string) error { ... }
func (l *OperationLogger) UpdateProgress(current, total int, message string) error { ... }
func (l *OperationLogger) IsCanceled() bool { ... }
```

## Non-Operation Goroutines

| Goroutine | Current | After |
|-----------|---------|-------|
| OpenLibrary download | Raw goroutine, no logging | Wrap in operation via Enqueue |
| Updater check | `log.Printf` only | `StandardLogger` + system_activity_log |
| iTunes sync scheduler | `log.Printf`, enqueues op | `StandardLogger` + system_activity_log |
| Stale reaper | `log.Printf` only | `StandardLogger` + system_activity_log |

## Database Changes

### New migration (31): system_activity_log table

```sql
CREATE TABLE IF NOT EXISTS system_activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_system_activity_source ON system_activity_log(source);
CREATE INDEX idx_system_activity_created ON system_activity_log(created_at);
```

PebbleDB equivalent: prefix key `syslog:{timestamp}:{source}` with JSON value.

### New Store interface methods

```go
// System activity log
AddSystemActivityLog(source, level, message string) error
GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error)

// Retention pruning
PruneOperationLogs(olderThan time.Time) (int, error)
PruneOperationChanges(olderThan time.Time) (int, error)
PruneSystemActivityLogs(olderThan time.Time) (int, error)
```

Both SQLiteStore and PebbleStore implement these.

### New config field

```go
LogRetentionDays int // default 90, 0 = keep forever
```

### New API endpoint

```
GET /api/v1/system/activity-log?source=scheduler&limit=50
```

## Log Retention

Scheduled task `purge_old_logs` runs weekly:

```
DELETE FROM operation_logs WHERE created_at < NOW() - retention_days
DELETE FROM operation_changes WHERE created_at < NOW() - retention_days
DELETE FROM system_activity_log WHERE created_at < NOW() - retention_days
```

Operation rows themselves are kept (so history shows "scan ran on March 10, completed") but their detailed logs/changes are pruned. A `logs_pruned` flag on the operation row indicates logs have been cleaned up.

## Rollout Phases

### Phase 1: Create package + wire into operations
- Build `internal/logger/` with Logger interface, StandardLogger, OperationLogger
- OperationLogger implements both Logger and ProgressReporter interfaces
- Queue worker creates OperationLogger, existing code works unchanged

### Phase 2: Migrate service layer
- Each service function gets `logger.Logger` param instead of `operations.ProgressReporter`
- Services pass logger down to library calls
- One service at a time, starting with scan (biggest pain point)

### Phase 3: Migrate library layer
- Replace `log.Printf("[DEBUG]...")` with `log.Debug(...)` in scanner, metadata, organizer, mediainfo
- Add `logger.Logger` parameter to public functions
- Each package independently

### Phase 4: Non-operation goroutines + retention
- OpenLibrary download → wrap in operation
- Housekeeping goroutines → StandardLogger + system_activity_log
- Add migration 31 (system_activity_log table)
- Add `purge_old_logs` scheduled task
- Add `log_retention_days` config field

### Phase 5: Cleanup
- Remove remaining `log.Printf` calls in operation-aware code
- Remove old ProgressReporter interface or keep as type alias
- Add system activity log API endpoint + frontend display
