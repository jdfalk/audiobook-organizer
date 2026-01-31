<!-- file: docs/plans/observability-and-monitoring.md -->
<!-- version: 1.1.0 -->
<!-- guid: a6b7c8d9-e0f1-2a3b-4c5d-6e7f8a9b0c1d -->
<!-- last-edited: 2026-01-31 -->

# Observability and Monitoring

## Overview

Tools for understanding system behavior: log persistence, live metrics,
operation health, and error tracking. Some items are P2 (log persistence, log
UX, SSE heartbeats); the rest are longer-term backlog.

---

## P2 — Medium Priority

### Persist Operation Logs

- Retain historical log tail per operation after completion
- Add `/api/v1/operations/:id/logs?tail=` endpoint
- Implement system-wide log retention policy

#### Current state

Operation logs are already persisted to PebbleDB. The `Store` interface
defines `AddOperationLog` and `GetOperationLogs` (in `internal/database/store.go`).
The `operationProgressReporter` in `internal/operations/queue.go` calls
`store.AddOperationLog` on every `Log()` invocation, and also broadcasts the
log entry in real-time via `realtime.GlobalHub.SendOperationLog`.

The PebbleDB key pattern (from `internal/database/pebble_store.go`) is:

```
operationlog:<operation_id>:<timestamp_nanos>:<seq>   →  JSON OperationLog
```

This key design gives two properties for free:
1. A prefix scan over `operationlog:<operation_id>:` returns all logs for
   that operation.
2. Logs are in chronological order (timestamp is the second key segment),
   with the sequential counter breaking ties for sub-nanosecond batches.

The `OperationLog` struct (from `store.go`):

```go
type OperationLog struct {
    ID          int       `json:"id"`
    OperationID string    `json:"operation_id"`
    Level       string    `json:"level"`     // "info", "warn", "error"
    Message     string    `json:"message"`
    Details     *string   `json:"details,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
}
```

#### What is missing: the API endpoint

The store layer already has the data. What needs to be added is the HTTP
handler that exposes it. Add a route in `internal/server/server.go` inside
the `api` group (alongside the existing `/api/v1/operations/:id` route):

```go
// In the route registration block of server.go:

api.GET("/operations/:id/logs", s.getOperationLogs)
```

Handler implementation:

```go
// internal/server/server.go — handler

func (s *Server) getOperationLogs(c *gin.Context) {
    operationID := c.Param("id")
    if operationID == "" {
        c.JSON(400, gin.H{"error": "operation ID required"})
        return
    }

    // Verify the operation exists
    op, err := database.GlobalStore.GetOperationByID(operationID)
    if err != nil || op == nil {
        c.JSON(404, gin.H{"error": "operation not found"})
        return
    }

    // Load all logs for this operation
    logs, err := database.GlobalStore.GetOperationLogs(operationID)
    if err != nil {
        c.JSON(500, gin.H{"error": fmt.Sprintf("failed to load logs: %v", err)})
        return
    }

    // Optional tail parameter: return only the last N entries
    tail := 0
    if tailStr := c.Query("tail"); tailStr != "" {
        if t, err := strconv.Atoi(tailStr); err == nil && t > 0 {
            tail = t
        }
    }
    if tail > 0 && tail < len(logs) {
        logs = logs[len(logs)-tail:]
    }

    c.JSON(200, gin.H{
        "operation_id": operationID,
        "logs":         logs,
        "total":        len(logs),
    })
}
```

#### Log retention policy

A background goroutine (or a periodic check in the heartbeat loop) scans
for operations older than a configurable retention window (default: 7 days)
and deletes their log entries. This prevents unbounded PebbleDB growth for
long-running servers.

```go
// Retention cleanup — runs in the heartbeat goroutine or a dedicated ticker

func cleanupOldOperationLogs(store database.Store, retentionDays int) {
    cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
    operations, _ := store.GetRecentOperations(10000) // Get all operations

    for _, op := range operations {
        if op.CompletedAt != nil && op.CompletedAt.Before(cutoff) {
            // Delete all operationlog:<op.ID>:* keys
            // Uses the same PebbleDB prefix-iteration pattern as GetOperationLogs
            deleteOperationLogs(store, op.ID)
        }
    }
}
```

### Log View UX

- Auto-scroll when following live tail
- Level-based coloring (info / warn / error)
- Collapsible verbose details
- Memory usage guard for large log volumes

### SSE System Status Heartbeats

- Push `system.status` diff events every 5 seconds
- Live memory and library metrics without polling
- Reduces Dashboard API call frequency

#### Current state — already implemented

The heartbeat is already wired in `internal/server/server.go` (line ~580).
A goroutine runs a `time.NewTicker(5 * time.Second)` and, on each tick,
gathers lightweight metrics and pushes them through the SSE hub:

```go
// Current heartbeat loop (server.go ~line 584):

ticker := time.NewTicker(5 * time.Second)
go func() {
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            if realtime.GlobalHub != nil {
                var alloc runtime.MemStats
                runtime.ReadMemStats(&alloc)

                bookCount, _ := database.GlobalStore.CountBooks()
                folders, _ := database.GlobalStore.GetAllImportPaths()

                // Also updates Prometheus gauges at the same time
                metrics.SetBooks(bookCount)
                metrics.SetFolders(len(folders))
                metrics.SetMemoryAlloc(alloc.Alloc)
                metrics.SetGoroutines(runtime.NumGoroutine())

                realtime.GlobalHub.SendSystemStatus(map[string]interface{}{
                    "books":        bookCount,
                    "folders":      len(folders),
                    "memory_alloc": alloc.Alloc,
                    "goroutines":   runtime.NumGoroutine(),
                    "timestamp":    time.Now().Unix(),
                })
            }
        case <-quit:
            return
        }
    }
}()
```

#### JSON shape of a `system.status` event on the wire

The SSE client receives this exact payload (the `Event` struct from
`internal/realtime/events.go` serialized to JSON, then wrapped in the
`data: ...\n\n` SSE framing):

```json
{
  "type": "system.status",
  "id": "",
  "timestamp": "2026-01-31T12:00:05Z",
  "data": {
    "books": 1247,
    "folders": 3,
    "memory_alloc": 48234560,
    "goroutines": 42,
    "timestamp": 1738324805
  }
}
```

| Field | Type | Description |
|---|---|---|
| `type` | string | Always `"system.status"` for these events |
| `id` | string | Empty — system-wide events have no operation ID, so they are broadcast to all connected SSE clients regardless of subscription filters |
| `timestamp` | string | RFC3339 timestamp when the event was created (from `Event.Timestamp`) |
| `data.books` | int | Current number of non-deleted books in the library |
| `data.folders` | int | Number of configured import paths |
| `data.memory_alloc` | int | `runtime.MemStats.Alloc` in bytes (current heap allocation) |
| `data.goroutines` | int | `runtime.NumGoroutine()` |
| `data.timestamp` | int | Unix epoch seconds (redundant with the outer timestamp but convenient for clients that prefer numeric comparison) |

#### What to add: enriched status fields

Future iterations should add these fields to the `data` map without changing
the event structure:

```go
// Additions to the heartbeat data map:

"active_operations": len(operations.GlobalQueue.ActiveOperations()),
"sse_clients":       realtime.GlobalHub.GetClientCount(),
"db_type":           config.AppConfig.DatabaseType,
"circuit_breakers": map[string]string{
    // One entry per external metadata source, value = "closed"/"open"/"half_open"
},
```

#### Per-connection heartbeats (already implemented)

In addition to the system-wide status events, each individual SSE connection
also receives a lightweight `heartbeat` ping every 5 seconds (from
`internal/realtime/events.go` `HandleSSE`, line ~283). This keeps Safari
from closing idle connections and has a different shape:

```json
{
  "type": "heartbeat",
  "timestamp": "2026-01-31T12:00:05Z"
}
```

This per-connection heartbeat is distinct from the `system.status` event.
The per-connection heartbeat is a keep-alive; the system.status event is a
data push.

---

## Backlog — Structured Metrics & Health

### Metrics Endpoint

- Prometheus-compatible `/metrics` endpoint
- Operation duration histograms
- Scan and organize counters

#### Current state — already implemented

The metrics package (`internal/metrics/metrics.go`) uses
`github.com/prometheus/client_golang/prometheus` and is already registered
and served. The `/metrics` endpoint is wired in `internal/server/server.go`
(line ~680):

```go
// Route registration (server.go):
s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

Registration is idempotent (guarded by `sync.Once`):

```go
// internal/metrics/metrics.go:
func Register() {
    registerOnce.Do(func() {
        prometheus.MustRegister(
            operationStarted, operationCompleted, operationFailed,
            operationCanceled, operationDuration,
            booksGauge, foldersGauge, memoryAllocGauge, goroutinesGauge,
        )
    })
}
```

#### Metrics currently exposed

| Metric name | Type | Labels | Description |
|---|---|---|---|
| `audiobook_organizer_operations_started_total` | Counter | `type` | Total operations started, by type (scan, organize, etc.) |
| `audiobook_organizer_operations_completed_total` | Counter | `type` | Total operations completed successfully |
| `audiobook_organizer_operations_failed_total` | Counter | `type` | Total operations that failed |
| `audiobook_organizer_operations_canceled_total` | Counter | `type` | Total operations canceled by user |
| `audiobook_organizer_operation_duration_seconds` | Histogram | `type` | Duration distribution. Buckets: exponential from 50ms, factor 1.6, 10 buckets (covers up to ~30 minutes) |
| `audiobook_organizer_books_total` | Gauge | (none) | Current book count in library |
| `audiobook_organizer_import_paths_total` | Gauge | (none) | Number of enabled import paths |
| `audiobook_organizer_process_memory_alloc_bytes` | Gauge | (none) | Current Go heap allocation |
| `audiobook_organizer_process_goroutines` | Gauge | (none) | Current goroutine count |

The gauges are updated every 5 seconds in the heartbeat loop (same goroutine
that pushes SSE system.status events).

The counter and histogram increments happen inside `internal/operations/queue.go`
in the worker function, wrapping each operation execution:

```go
// queue.go worker — metrics calls (already in place):
start := time.Now()
metrics.IncOperationStarted(op.Type)
// ... execute operation ...
if err != nil {
    metrics.IncOperationFailed(op.Type)
} else {
    metrics.IncOperationCompleted(op.Type)
}
metrics.ObserveOperationDuration(op.Type, time.Since(start))
```

#### Metrics to add

The following metrics should be added to `internal/metrics/metrics.go` to
round out the observability surface:

```go
// Add to metrics.go:

var (
    // Scan-specific counters
    scanFilesScanned = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "audiobook_organizer",
        Name:      "scan_files_scanned_total",
        Help:      "Total number of files examined during all scans",
    })
    scanBooksDiscovered = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "audiobook_organizer",
        Name:      "scan_books_discovered_total",
        Help:      "Total number of audiobook files discovered across all scans",
    })

    // Organize-specific counters
    organizeFilesProcessed = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "audiobook_organizer",
        Name:      "organize_files_processed_total",
        Help:      "Total number of files organized (moved/linked/copied)",
    })
    organizeByStrategy = prometheus.NewCounterVec(prometheus.CounterOpts{
        Namespace: "audiobook_organizer",
        Name:      "organize_strategy_total",
        Help:      "Files organized by strategy used",
    }, []string{"strategy"}) // Labels: "copy", "hardlink", "reflink", "symlink"

    // SSE client tracking
    sseClientsConnected = prometheus.NewGauge(prometheus.GaugeOpts{
        Namespace: "audiobook_organizer",
        Name:      "sse_clients_connected",
        Help:      "Number of currently connected SSE clients",
    })

    // Circuit breaker state
    circuitBreakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
        Namespace: "audiobook_organizer",
        Name:      "circuit_breaker_state",
        Help:      "Circuit breaker state per source (0=closed, 1=open, 2=half_open)",
    }, []string{"source"})
)

// Export functions:
func IncScanFilesScanned()                      { scanFilesScanned.Inc() }
func IncScanBooksDiscovered()                   { scanBooksDiscovered.Inc() }
func IncOrganizeFilesProcessed()                { organizeFilesProcessed.Inc() }
func IncOrganizeByStrategy(strategy string)     { organizeByStrategy.WithLabelValues(strategy).Inc() }
func SetSSEClients(n int)                       { sseClientsConnected.Set(float64(n)) }
func SetCircuitBreakerState(source string, state int) { circuitBreakerState.WithLabelValues(source).Set(float64(state)) }
```

Register them in the `Register()` function alongside the existing metrics.

### Per-Operation Timing

- Wall time, file count, and throughput summary stored after each operation
  completes

### Slow Operation Detector

- Warn if a scan or organize exceeds a configurable time threshold

### Library Growth Trends

- Daily book count snapshot table for trend visualization

### File Integrity Checker

- Periodic checksum verification against stored hashes
- Surface mismatches for investigation

#### Design

The integrity checker runs as a background operation enqueued through the
normal operation queue (`internal/operations/queue.go`). It iterates over all
books in the database, recomputes their file hash on disk, and compares it to
the stored hash(es) (`FileHash`, `OriginalFileHash`, `OrganizedFileHash`).
Mismatches are surfaced through three channels: a log entry on the operation,
an SSE event, and a persisted mismatch record in PebbleDB.

The hash computation reuses `internal/scanner/scanner.go`'s `ComputeFileHash`,
which already implements the chunked strategy for large files (first 10 MB +
last 10 MB + size for files over 100 MB; full hash for smaller files).

#### PebbleDB key pattern for mismatch records

```
integrity:mismatch:<book_id>   →  JSON IntegrityMismatch
```

```go
// internal/integrity/checker.go

package integrity

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/operations"
    "github.com/jdfalk/audiobook-organizer/internal/realtime"
    "github.com/jdfalk/audiobook-organizer/internal/scanner"
)

// IntegrityMismatch records a single hash mismatch found during verification.
type IntegrityMismatch struct {
    BookID       string    `json:"book_id"`
    BookTitle    string    `json:"book_title"`
    FilePath     string    `json:"file_path"`
    StoredHash   string    `json:"stored_hash"`   // The hash stored in the DB at check time
    ComputedHash string    `json:"computed_hash"` // The hash computed from the file on disk
    DetectedAt   time.Time `json:"detected_at"`
    HashType     string    `json:"hash_type"` // "file_hash", "original_file_hash", or "organized_file_hash"
}

// RunIntegrityCheck is an OperationFunc suitable for enqueuing via the
// operation queue. It verifies every book's on-disk hash against stored hashes.
func RunIntegrityCheck(ctx context.Context, progress operations.ProgressReporter) error {
    if database.GlobalStore == nil {
        return fmt.Errorf("database not initialized")
    }

    // Load all books (paginated to avoid memory spike on huge libraries)
    const pageSize = 500
    offset := 0
    totalChecked := 0
    mismatches := 0

    for {
        books, err := database.GlobalStore.GetAllBooks(pageSize, offset)
        if err != nil {
            return fmt.Errorf("failed to load books at offset %d: %w", offset, err)
        }
        if len(books) == 0 {
            break // Done
        }

        for _, book := range books {
            // Check context cancellation (operation may be canceled by user)
            if ctx.Err() != nil {
                return ctx.Err()
            }

            mismatch := checkBook(&book)
            if mismatch != nil {
                mismatches++
                persistMismatch(mismatch)
                logMismatch(progress, mismatch)
                broadcastMismatch(mismatch)
            }

            totalChecked++
            // Report progress every 50 books
            if totalChecked%50 == 0 {
                _ = progress.UpdateProgress(totalChecked, 0, // total unknown until full scan
                    fmt.Sprintf("checked %d files, %d mismatches found", totalChecked, mismatches))
            }
        }

        offset += pageSize
    }

    // Final progress update
    _ = progress.UpdateProgress(totalChecked, totalChecked,
        fmt.Sprintf("integrity check complete: %d files checked, %d mismatches", totalChecked, mismatches))

    return nil
}

// checkBook computes the file hash and compares it to all stored hash fields.
// Returns the first mismatch found, or nil if everything matches.
func checkBook(book *database.Book) *IntegrityMismatch {
    // Compute current hash from disk
    computedHash, err := scanner.ComputeFileHash(book.FilePath)
    if err != nil {
        log.Printf("[WARN] integrity: could not hash %s: %v", book.FilePath, err)
        return nil // File may not exist (deleted externally) — not a mismatch per se
    }

    // Compare against FileHash (the primary hash)
    if book.FileHash != nil && *book.FileHash != "" && *book.FileHash != computedHash {
        return &IntegrityMismatch{
            BookID: book.ID, BookTitle: book.Title, FilePath: book.FilePath,
            StoredHash: *book.FileHash, ComputedHash: computedHash,
            DetectedAt: time.Now(), HashType: "file_hash",
        }
    }

    // Compare against OrganizedFileHash (set after organize)
    if book.OrganizedFileHash != nil && *book.OrganizedFileHash != "" && *book.OrganizedFileHash != computedHash {
        return &IntegrityMismatch{
            BookID: book.ID, BookTitle: book.Title, FilePath: book.FilePath,
            StoredHash: *book.OrganizedFileHash, ComputedHash: computedHash,
            DetectedAt: time.Now(), HashType: "organized_file_hash",
        }
    }

    return nil // All hashes match (or no stored hash to compare against)
}

// persistMismatch writes the mismatch record to PebbleDB.
func persistMismatch(m *IntegrityMismatch) {
    data, _ := json.Marshal(m)
    key := []byte(fmt.Sprintf("integrity:mismatch:%s", m.BookID))
    if ps, ok := database.GlobalStore.(*database.PebbleStore); ok {
        _ = ps.Set(key, data)
    }
}

// logMismatch writes a log entry on the running operation.
func logMismatch(progress operations.ProgressReporter, m *IntegrityMismatch) {
    details := fmt.Sprintf("stored=%s computed=%s type=%s", m.StoredHash, m.ComputedHash, m.HashType)
    _ = progress.Log("warn",
        fmt.Sprintf("integrity mismatch: %s at %s", m.BookTitle, m.FilePath),
        &details)
}

// broadcastMismatch pushes a real-time SSE event so the dashboard can
// highlight the mismatch immediately.
func broadcastMismatch(m *IntegrityMismatch) {
    if realtime.GlobalHub == nil {
        return
    }
    realtime.GlobalHub.Broadcast(&realtime.Event{
        Type: "integrity.mismatch",
        Data: map[string]interface{}{
            "book_id":       m.BookID,
            "book_title":    m.BookTitle,
            "file_path":     m.FilePath,
            "stored_hash":   m.StoredHash,
            "computed_hash": m.ComputedHash,
            "hash_type":     m.HashType,
            "detected_at":   m.DetectedAt,
        },
    })
}
```

#### Triggering the check

The integrity check is enqueued as a normal operation through the operation
queue. It can be triggered manually via API or on a schedule:

```go
// Manual trigger via API — add a handler:
// POST /api/v1/integrity/check

func (s *Server) startIntegrityCheck(c *gin.Context) {
    opID := ulid.MustNew(ulid.Timestamp(time.Now()), ulid.DefaultEntropy()).String()
    database.GlobalStore.CreateOperation(opID, "integrity_check", nil)

    operations.GlobalQueue.Enqueue(opID, "integrity_check", operations.PriorityLow,
        integrity.RunIntegrityCheck)

    c.JSON(202, gin.H{"operation_id": opID, "message": "integrity check enqueued"})
}

// Scheduled trigger — add to the heartbeat goroutine or a separate ticker.
// Example: run once per day at a configurable time.
```

#### Retrieving mismatches

A `GET /api/v1/integrity/mismatches` endpoint scans PebbleDB for all
`integrity:mismatch:*` keys and returns them as a JSON array. This lets the
dashboard display a persistent list of files that need attention, even if the
user was not connected when the mismatch was first detected.

### Background Health Check Pings

- SSE pings reporting database latency classification

### Error Aggregation Dashboard

- Top recurring errors with counts across all operations

---

## Real-Time & Streaming Enhancements

- Upgrade SSE hub to optional WebSocket mode for bidirectional
  cancel/resubscribe
- Client subscription refinement (subscribe to multiple ops, filter event types)
- Replay last N events on connect for quick state hydration
- Rate-controlled progress events (coalesce rapid updates to reduce client load)

---

## Operation Queue Observability

- Real-time worker utilization stats
- Priority aging (long-waiting normal-priority ops get temporary priority boost)
- Pause / resume queue functionality

---

## Dependencies

- Log persistence is a prerequisite for the metrics and health dashboarding work
- SSE heartbeats depend on the existing SSE infrastructure (already in place)

## References

- SSE implementation: `internal/realtime/events.go` (EventHub, HandleSSE, per-client heartbeats)
- SSE route + system heartbeat: `internal/server/server.go` (route registration at `/api/events`, heartbeat goroutine)
- Operation queue: `internal/operations/queue.go` (worker loop, progress reporter, real-time event emission)
- Operation log persistence: `internal/database/pebble_store.go` (`AddOperationLog`, `GetOperationLogs`, key pattern `operationlog:<id>:<ts>:<seq>`)
- Store interface (log methods): `internal/database/store.go`
- Prometheus metrics: `internal/metrics/metrics.go` (registration, counter/histogram helpers)
- File hashing: `internal/fileops/hash.go`, `internal/scanner/scanner.go` (`ComputeFileHash` with chunked strategy)
- Book hash fields: `internal/database/store.go` (`Book` struct: `FileHash`, `OriginalFileHash`, `OrganizedFileHash`)
