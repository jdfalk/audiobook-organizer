<!-- file: docs/plans/performance-and-reliability.md -->
<!-- version: 1.1.0 -->
<!-- guid: b7c8d9e0-f1a2-3b4c-5d6e-7f8a9b0c1d2e -->
<!-- last-edited: 2026-01-31 -->

# Performance and Reliability

## Overview

Optimizations for scan speed, query responsiveness, and system resilience
against interruptions and external failures.

---

## Performance

### Parallel Scanning

- Goroutine pool respecting the `concurrent_scans` configuration setting
  (default: 4, configured via `viper` in `internal/config/config.go`)
- Coordinate progress reporting across workers

#### Current implementation (in `internal/scanner/scanner.go`)

The scanner already has two parallel stages. **Stage 1** (`ScanDirectoryParallel`)
walks the tree sequentially to collect directory paths, then fans out into a
goroutine-per-directory with a semaphore-based pool:

```go
// Current pattern in ScanDirectoryParallel (scanner.go ~line 112):

var mu sync.Mutex
var books []Book
var wg sync.WaitGroup
semaphore := make(chan struct{}, workers)  // workers = config.AppConfig.ConcurrentScans

for _, dir := range dirs {
    wg.Add(1)
    go func(scanDir string) {
        defer wg.Done()
        semaphore <- struct{}{}        // Acquire slot
        defer func() { <-semaphore }() // Release on exit

        entries, _ := os.ReadDir(scanDir)
        var localBooks []Book
        for _, entry := range entries {
            // ... extension matching ...
            localBooks = append(localBooks, Book{FilePath: path, Format: ext})
        }

        // Merge under lock — only taken when localBooks is non-empty
        if len(localBooks) > 0 {
            mu.Lock()
            books = append(books, localBooks...)
            mu.Unlock()
        }
    }(dir)
}
wg.Wait()
```

**Stage 2** (`ProcessBooksParallel`) processes metadata for each discovered
book in a second goroutine pool. Progress is serialized through a dedicated
channel so the progress bar and the caller's `progressFn` callback are never
called concurrently:

```go
// Current pattern in ProcessBooksParallel (scanner.go ~line 188):

// Single goroutine drains progress updates — no lock needed on the counter
progressCh := make(chan string, len(books))
go func() {
    processed := 0
    for path := range progressCh {
        processed++
        bar.Add(1)
        if progressFn != nil {
            progressFn(processed, total, path)
        }
    }
}()

// Worker pool — same semaphore pattern as Stage 1
semaphore := make(chan struct{}, workers)
errChan := make(chan error, len(books))

for i := range books {
    wg.Add(1)
    go func(idx int) {
        defer wg.Done()
        semaphore <- struct{}{}
        defer func() {
            <-semaphore
            progressCh <- books[idx].FilePath  // Signal completion
        }()

        // ... metadata extraction, AI fallback, series matching ...
        // ... saveBook (thread-safe: PebbleDB batch writes are atomic) ...
    }(i)
}
wg.Wait()
close(progressCh)  // Drain remaining progress updates
```

#### What to change for the goroutine pool upgrade

The current implementation is already correct and performant for most
workloads. The following enhancements are recommended for very large libraries
(50k+ files):

1. **Replace `sync.WaitGroup` + semaphore with `golang.org/x/sync/errgroup`**
   for Stage 2. `errgroup` propagates the first error automatically and
   integrates with context cancellation. The semaphore limit becomes
   `errgroup.SetLimit(workers)`:

```go
// Proposed upgrade for ProcessBooksParallel:

import "golang.org/x/sync/errgroup"

g, ctx := errgroup.WithContext(parentCtx)
g.SetLimit(workers) // Replaces the manual semaphore channel

for i := range books {
    idx := i
    g.Go(func() error {
        if ctx.Err() != nil {
            return ctx.Err() // Stop early if any prior worker errored
        }
        // ... same processing logic ...
        progressCh <- books[idx].FilePath
        return nil
    })
}

if err := g.Wait(); err != nil {
    close(progressCh)
    return err
}
close(progressCh)
```

2. **Batch the directory walk itself in Stage 1** for very deep trees.
   Currently `filepath.Walk` runs single-threaded before the fan-out begins.
   For libraries with thousands of nested directories, partition the top-level
   directories into `workers` buckets and walk each bucket concurrently:

```go
// Proposed: parallel directory walk for Stage 1

func partitionDirs(rootDir string, workers int) ([][]string, error) {
    // Walk only the first two levels to get top-level partitions
    var topLevel []string
    entries, err := os.ReadDir(rootDir)
    if err != nil { return nil, err }
    for _, e := range entries {
        if e.IsDir() {
            topLevel = append(topLevel, filepath.Join(rootDir, e.Name()))
        }
    }

    // Distribute top-level dirs round-robin across buckets
    buckets := make([][]string, workers)
    for i, dir := range topLevel {
        buckets[i%workers] = append(buckets[i%workers], dir)
    }
    return buckets, nil
}

// Then each worker walks its own bucket — no shared state needed during the walk.
```

3. **Progress reporting for Stage 1**: Currently Stage 1 has no progress
   callback. Add a counter that increments atomically as directories are
   processed, and expose it through the `progressFn` callback (or a separate
   `scanProgressFn`) so the UI can show walk progress before metadata
   processing begins.

### Debounced Library Size Recomputation

- Use inotify/fsnotify events instead of periodic full directory walk
- Trigger recomputation only when files actually change

### Caching Layer

- LRU cache for frequent book queries
- Cache key: filter parameters + page number
- Invalidation on write operations

#### Implementation

Use `github.com/hashicorp/golang-lru/v2` (typed, thread-safe LRU). The cache
sits between the HTTP handler and `database.GlobalStore`. Cache size is driven
by the existing `config.AppConfig.CacheSize` field (default 1000 items,
already defined in `internal/config/config.go`).

```go
// internal/cache/book_cache.go

package cache

import (
    "fmt"
    "sync"

    lru "github.com/hashicorp/golang-lru/v2"
    "github.com/jdfalk/audiobook-organizer/internal/database"
)

// CacheKey encodes all the parameters that produce a distinct result set.
// Two requests with identical CacheKeys will return the same data.
type CacheKey struct {
    Query  string // Search query (empty string = no filter)
    Limit  int
    Offset int
}

// String produces a deterministic string representation for use as a map key.
func (k CacheKey) String() string {
    return fmt.Sprintf("books:q=%s:l=%d:o=%d", k.Query, k.Limit, k.Offset)
}

// BookCache wraps an LRU cache of book query results.
type BookCache struct {
    mu    sync.RWMutex // Protects version counter only; LRU is internally thread-safe
    cache *lru.Cache[string, []database.Book]
    // version is bumped on any write. Cached entries older than the current
    // version are considered stale. This is a generation-based invalidation
    // strategy — simpler than tracking individual keys.
    version int64
    // versionCache maps cache keys to the version at which they were written.
    versionCache *lru.Cache[string, int64]
}

// NewBookCache creates a cache with the given capacity.
func NewBookCache(capacity int) (*BookCache, error) {
    c, err := lru.New[string, []database.Book](capacity)
    if err != nil {
        return nil, err
    }
    vc, err := lru.New[string, int64](capacity)
    if err != nil {
        return nil, err
    }
    return &BookCache{cache: c, version: 0, versionCache: vc}, nil
}

// Get returns cached results for the key, or nil if not present or stale.
func (bc *BookCache) Get(key CacheKey) []database.Book {
    bc.mu.RLock()
    curVersion := bc.version
    bc.mu.RUnlock()

    keyStr := key.String()

    // Check if entry exists and is current-version
    entryVersion, ok := bc.versionCache.Get(keyStr)
    if !ok || entryVersion < curVersion {
        return nil // Stale or missing
    }

    books, ok := bc.cache.Get(keyStr)
    if !ok {
        return nil
    }
    return books
}

// Put stores a result set in the cache at the current version.
func (bc *BookCache) Put(key CacheKey, books []database.Book) {
    bc.mu.RLock()
    curVersion := bc.version
    bc.mu.RUnlock()

    keyStr := key.String()
    bc.cache.Add(keyStr, books)
    bc.versionCache.Add(keyStr, curVersion)
}

// Invalidate bumps the version counter, making all existing entries stale.
// Call this after any write operation (CreateBook, UpdateBook, DeleteBook,
// organize, etc.).
func (bc *BookCache) Invalidate() {
    bc.mu.Lock()
    bc.version++
    bc.mu.Unlock()
}
```

#### Wiring into the book list handler

In `internal/server/server.go`, the handler that serves `GET /api/v1/books`
currently calls `database.GlobalStore.GetAllBooks(limit, offset)` or
`SearchBooks(query, limit, offset)` directly. Wrap those calls:

```go
// In the GET /api/v1/books handler:

cacheKey := cache.CacheKey{Query: query, Limit: limit, Offset: offset}
if cached := bookCache.Get(cacheKey); cached != nil {
    c.JSON(200, gin.H{"books": cached, "total": len(cached)})
    return
}

// Cache miss — fetch from store
var books []database.Book
var err error
if query != "" {
    books, err = database.GlobalStore.SearchBooks(query, limit, offset)
} else {
    books, err = database.GlobalStore.GetAllBooks(limit, offset)
}
if err != nil {
    c.JSON(500, gin.H{"error": err.Error()})
    return
}

bookCache.Put(cacheKey, books)
c.JSON(200, gin.H{"books": books, "total": len(books)})
```

#### Invalidation points

Call `bookCache.Invalidate()` from every handler that mutates book state:

- `POST /api/v1/books` (create)
- `PUT /api/v1/books/:id` (update)
- `DELETE /api/v1/books/:id` (delete / soft-delete)
- After any organize operation completes (in the `OperationFunc` completion
  path inside `internal/operations/queue.go`)
- After any scan operation completes (same pattern)

The `Invalidate` call is a single atomic increment — it costs nothing and
avoids the complexity of tracking which cache keys are affected by which
writes.

### Batch Metadata Fetch Pipeline

- Queue and coalesce external API calls (Open Library, etc.)
- Avoid redundant fetches for the same title/author

### Adaptive Worker Scaling

- Increase operation queue workers when backlog grows
- Shrink back to baseline when idle

### Memory Pressure Monitor

- Detect high memory usage
- Trigger GC hints and cache trimming automatically

---

## Reliability & Resilience

### Graceful Resume of Interrupted Scans

- Persist filesystem walker state at checkpoints
- On restart, resume from last checkpoint instead of rescanning from scratch

#### PebbleDB checkpoint design

Each scan operation writes a checkpoint after processing every N directories
(N = configurable, default 100). The checkpoint records which directories have
been fully scanned and how many books have been found so far. On restart (or
after a crash), the scan operation reads the latest checkpoint and skips
directories that were already processed.

**Key pattern:**

```
scan:checkpoint:<operation_id>   →  JSON-encoded ScanCheckpoint
```

This follows the project's key-prefix convention from `pebble_store.go`.
The `<operation_id>` ties the checkpoint to the specific scan operation
(the same ID used in `operation:<id>` keys), so multiple scans of different
paths never collide.

```go
// internal/scanner/checkpoint.go

package scanner

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/jdfalk/audiobook-organizer/internal/database"
)

// ScanCheckpoint records the state of a scan at a point in time.
type ScanCheckpoint struct {
    OperationID   string            `json:"operation_id"`
    RootDir       string            `json:"root_dir"`
    ScannedDirs   map[string]bool   `json:"scanned_dirs"`  // Set of fully-processed directory paths
    BooksFound    int               `json:"books_found"`
    LastUpdatedAt time.Time         `json:"last_updated_at"`
    // DirsRemaining is the ordered list of directories still to process.
    // Populated at checkpoint creation from the original dir list minus ScannedDirs.
    DirsRemaining []string          `json:"dirs_remaining"`
}

const checkpointInterval = 100 // Write checkpoint every N directories

// SaveCheckpoint persists the current scan state to PebbleDB.
func SaveCheckpoint(store database.Store, checkpoint *ScanCheckpoint) error {
    checkpoint.LastUpdatedAt = time.Now()
    data, err := json.Marshal(checkpoint)
    if err != nil {
        return err
    }
    key := []byte(fmt.Sprintf("scan:checkpoint:%s", checkpoint.OperationID))
    // PebbleStore.Set is used directly (same pattern as persistShadow in download/)
    if ps, ok := store.(*database.PebbleStore); ok {
        return ps.Set(key, data)
    }
    return fmt.Errorf("checkpoint storage requires PebbleDB")
}

// LoadCheckpoint loads the latest checkpoint for an operation, or nil if none.
func LoadCheckpoint(store database.Store, operationID string) (*ScanCheckpoint, error) {
    key := []byte(fmt.Sprintf("scan:checkpoint:%s", operationID))
    if ps, ok := store.(*database.PebbleStore); ok {
        value, closer, err := ps.Get(key)
        if err != nil {
            return nil, nil // Not found = no checkpoint
        }
        defer closer.Close()
        var cp ScanCheckpoint
        if err := json.Unmarshal(value, &cp); err != nil {
            return nil, err
        }
        return &cp, nil
    }
    return nil, fmt.Errorf("checkpoint storage requires PebbleDB")
}

// ClearCheckpoint removes the checkpoint after a scan completes successfully.
func ClearCheckpoint(store database.Store, operationID string) error {
    key := []byte(fmt.Sprintf("scan:checkpoint:%s", operationID))
    if ps, ok := store.(*database.PebbleStore); ok {
        return ps.Delete(key)
    }
    return nil
}
```

#### Integration into ScanDirectoryParallel

```go
// Modified Stage 1 loop in ScanDirectoryParallel:

// After collecting all dirs via filepath.Walk, check for a checkpoint:
checkpoint, _ := LoadCheckpoint(database.GlobalStore, operationID)

var dirsToScan []string
if checkpoint != nil {
    // Resume: skip already-scanned directories
    dirsToScan = checkpoint.DirsRemaining
    log.Printf("[INFO] Resuming scan from checkpoint: %d dirs remaining", len(dirsToScan))
} else {
    dirsToScan = dirs // Full scan
}

// ... existing parallel scan loop over dirsToScan ...

// Inside the loop, after each directory is processed, increment a counter.
// Every checkpointInterval directories, write a checkpoint:
processed := atomic.AddInt64(&processedCount, 1)
if processed % checkpointInterval == 0 {
    cp := &ScanCheckpoint{
        OperationID: operationID,
        RootDir:     rootDir,
        ScannedDirs: currentScannedDirs(), // snapshot of what's been done
        BooksFound:  len(books),           // current count (under mu)
    }
    _ = SaveCheckpoint(database.GlobalStore, cp)
}

// After the full scan completes successfully, clear the checkpoint:
ClearCheckpoint(database.GlobalStore, operationID)
```

#### Resume on server restart

When the server starts and `ScanOnStartup` is true, before launching a new
scan, check for orphaned checkpoints (scans that were interrupted):

```go
// In server startup, after queue initialization:

// Look for any scan:checkpoint:* keys in PebbleDB
orphanedCheckpoints := findOrphanedScanCheckpoints(database.GlobalStore)
for _, cp := range orphanedCheckpoints {
    // Re-enqueue a scan operation with the same ID so it picks up
    // the checkpoint and resumes from where it left off.
    operations.GlobalQueue.Enqueue(cp.OperationID, "scan", operations.PriorityNormal,
        func(ctx context.Context, progress operations.ProgressReporter) error {
            return scanner.ScanDirectoryParallel(cp.RootDir, config.AppConfig.ConcurrentScans)
        })
}
```

### Operation Retry Policy

- Automatic retry for transient failures (network timeouts on metadata
  retrieval)
- Configurable retry count and backoff

### Circuit Breaker for External Metadata

- Stop calling external sources (Open Library, etc.) after repeated failures
- Back off and retry after a cooldown period
- Prevents cascading failures from a single slow or down provider

#### State machine

The circuit breaker has three states:

| State | Behavior | Transitions |
|---|---|---|
| **Closed** (default) | Requests pass through normally. Failure counter increments on each error. | Opens when `failures >= threshold` |
| **Open** | All requests fail immediately with `ErrCircuitOpen`. No network calls made. | Transitions to Half-Open after `cooldown` elapses |
| **Half-Open** | One probe request is allowed through. All others fail immediately. | Closes if probe succeeds; reopens if probe fails |

#### Go implementation

```go
// internal/metadata/circuitbreaker.go

package metadata

import (
    "errors"
    "log"
    "sync"
    "time"
)

var ErrCircuitOpen = errors.New("circuit breaker open: external metadata source is unavailable")

type CircuitState int

const (
    StateClosed   CircuitState = iota // Normal operation
    StateOpen                         // Failing fast
    StateHalfOpen                     // Allowing one probe
)

// CircuitBreaker is safe for concurrent use.
type CircuitBreaker struct {
    mu           sync.Mutex
    state        CircuitState
    failures     int
    threshold    int           // Number of failures before opening (default: 5)
    cooldown     time.Duration // How long to stay open before allowing a probe (default: 30s)
    lastFailedAt time.Time     // When the last failure occurred (used to time cooldown)
    sourceName   string        // Human-readable label for logging (e.g. "openlibrary")
}

// NewCircuitBreaker creates a breaker with the given threshold and cooldown.
func NewCircuitBreaker(sourceName string, threshold int, cooldown time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:      StateClosed,
        threshold:  threshold,
        cooldown:   cooldown,
        sourceName: sourceName,
    }
}

// AllowRequest returns nil if the caller should proceed with the network
// call. Returns ErrCircuitOpen if the circuit is open and the cooldown
// has not elapsed.
func (cb *CircuitBreaker) AllowRequest() error {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case StateClosed:
        return nil // Allow

    case StateOpen:
        if time.Since(cb.lastFailedAt) > cb.cooldown {
            // Cooldown elapsed — transition to half-open, allow one probe
            cb.state = StateHalfOpen
            return nil
        }
        return ErrCircuitOpen

    case StateHalfOpen:
        // Already in half-open; another request arrived before the probe
        // completed. Reject it.
        return ErrCircuitOpen
    }

    return nil
}

// RecordSuccess resets the breaker to closed state. Call after a successful
// external call.
func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.state = StateClosed
    cb.failures = 0
}

// RecordFailure increments the failure counter and opens the circuit if
// the threshold is reached. Call after a failed external call.
func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.failures++
    cb.lastFailedAt = time.Now()

    // If we were in half-open (probe failed), go back to open
    if cb.state == StateHalfOpen {
        cb.state = StateOpen
        log.Printf("[WARN] circuit breaker: %s probe failed, reopened", cb.sourceName)
        return
    }

    if cb.failures >= cb.threshold {
        cb.state = StateOpen
        log.Printf("[WARN] circuit breaker: %s opened after %d consecutive failures", cb.sourceName, cb.failures)
    }
}

// State returns the current state (for monitoring/metrics).
func (cb *CircuitBreaker) State() CircuitState {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    return cb.state
}
```

#### Usage in the metadata fetch pipeline

Each external metadata source (Open Library, Goodreads, etc.) gets its own
`CircuitBreaker` instance, created at server startup. The fetch function
wraps every outbound HTTP call:

```go
// In the metadata fetcher for a given source:

var openLibraryCB = NewCircuitBreaker("openlibrary", 5, 30*time.Second)

func FetchFromOpenLibrary(ctx context.Context, title, author string) (*Metadata, error) {
    if err := openLibraryCB.AllowRequest(); err != nil {
        return nil, err // Fast fail — no network call made
    }

    result, err := httpCallToOpenLibrary(ctx, title, author)
    if err != nil {
        openLibraryCB.RecordFailure()
        return nil, err
    }

    openLibraryCB.RecordSuccess()
    return result, nil
}
```

The circuit breaker state can be exposed via the `/api/v1/system/status`
endpoint so operators can see which external sources are currently tripped.

### Transactional Organize Rollback Journal

- Record every file move/copy during organize
- On failure, replay journal in reverse to restore previous state

#### Data structure

Each file operation during an organize batch is recorded as a `JournalEntry`.
The journal is persisted to PebbleDB under the operation's ID so it survives
a crash mid-organize.

**PebbleDB key pattern:**

```
organize:journal:<operation_id>:<sequence>   →  JSON-encoded JournalEntry
```

The `<sequence>` is a monotonically increasing integer (formatted as zero-
padded decimal for lexicographic ordering: `0001`, `0002`, ...). This means
a PebbleDB prefix scan over `organize:journal:<operation_id>:` returns
entries in insertion order — no sorting needed.

```go
// internal/organizer/journal.go

package organizer

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "sync/atomic"
    "time"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/fileops"
)

// JournalEntry records one file operation for rollback purposes.
type JournalEntry struct {
    Sequence    int       `json:"seq"`
    OperationID string    `json:"operation_id"`
    Action      string    `json:"action"`       // "copy", "hardlink", "reflink", "symlink", "mkdir"
    SrcPath     string    `json:"src_path"`
    DstPath     string    `json:"dst_path"`
    SrcHash     string    `json:"src_hash"`     // SHA256 of source before the operation
    CreatedAt   time.Time `json:"created_at"`
    Rolled      bool      `json:"rolled"`       // true after this entry has been rolled back
}

// OrganizeJournal tracks file operations for a single organize batch.
type OrganizeJournal struct {
    operationID string
    store       database.Store
    seq         int64 // atomic counter
}

// NewOrganizeJournal creates a journal for the given operation.
func NewOrganizeJournal(operationID string, store database.Store) *OrganizeJournal {
    return &OrganizeJournal{operationID: operationID, store: store}
}

// Record appends a journal entry to PebbleDB.
func (j *OrganizeJournal) Record(action, srcPath, dstPath string) error {
    seq := int(atomic.AddInt64(&j.seq, 1))

    // Compute source hash for verification during rollback
    srcHash, _ := fileops.ComputeFileHash(srcPath) // Non-fatal if it fails

    entry := JournalEntry{
        Sequence:    seq,
        OperationID: j.operationID,
        Action:      action,
        SrcPath:     srcPath,
        DstPath:     dstPath,
        SrcHash:     srcHash,
        CreatedAt:   time.Now(),
    }

    data, err := json.Marshal(entry)
    if err != nil {
        return err
    }

    // Key: organize:journal:<operation_id>:<zero-padded seq>
    key := []byte(fmt.Sprintf("organize:journal:%s:%06d", j.operationID, seq))
    if ps, ok := j.store.(*database.PebbleStore); ok {
        return ps.Set(key, data)
    }
    return fmt.Errorf("journal requires PebbleDB")
}

// Rollback replays the journal in reverse order. For each entry:
//   - "copy", "hardlink", "reflink", "symlink": remove DstPath
//   - "mkdir": remove DstPath if empty
// After successful rollback of each entry, the entry is marked as rolled.
func (j *OrganizeJournal) Rollback() error {
    entries, err := j.loadEntries()
    if err != nil {
        return fmt.Errorf("rollback: load entries: %w", err)
    }

    // Replay in reverse order (highest seq first)
    for i := len(entries) - 1; i >= 0; i-- {
        entry := entries[i]
        if entry.Rolled {
            continue // Already rolled back (e.g. from a previous partial rollback)
        }

        switch entry.Action {
        case "copy", "hardlink", "reflink", "symlink":
            if err := os.Remove(entry.DstPath); err != nil && !os.IsNotExist(err) {
                log.Printf("[WARN] rollback: failed to remove %s: %v", entry.DstPath, err)
                // Continue — best-effort rollback
            } else {
                log.Printf("[INFO] rollback: removed %s (was %s of %s)", entry.DstPath, entry.Action, entry.SrcPath)
            }

        case "mkdir":
            // Only remove if empty
            if err := os.Remove(entry.DstPath); err != nil {
                log.Printf("[DEBUG] rollback: skipping non-empty dir %s", entry.DstPath)
            }
        }

        // Mark as rolled
        entry.Rolled = true
        j.persistEntry(&entry)
    }

    return nil
}

// Clear removes all journal entries for this operation after a successful
// organize (no rollback needed).
func (j *OrganizeJournal) Clear() error {
    // Delete all keys matching organize:journal:<operation_id>:*
    // Uses PebbleDB iterator with prefix bounds (same pattern as GetOperationLogs)
    ...
}

func (j *OrganizeJournal) loadEntries() ([]JournalEntry, error) {
    // Iterate over prefix organize:journal:<operation_id>:
    // Entries come back in lexicographic key order = insertion order
    ...
}

func (j *OrganizeJournal) persistEntry(entry *JournalEntry) {
    data, _ := json.Marshal(entry)
    key := []byte(fmt.Sprintf("organize:journal:%s:%06d", j.operationID, entry.Sequence))
    if ps, ok := j.store.(*database.PebbleStore); ok {
        _ = ps.Set(key, data)
    }
}
```

#### Integration into OrganizeBook

In `internal/organizer/organizer.go`, the `OrganizeBook` method receives a
`*OrganizeJournal` (or the caller creates one per batch). Every file
operation records itself before executing:

```go
// In the organize batch loop (called from the operation queue worker):

journal := NewOrganizeJournal(operationID, database.GlobalStore)

for _, book := range booksToOrganize {
    targetPath, err := org.OrganizeBook(book, journal) // journal passed in
    if err != nil {
        // Organize failed partway through — roll back everything done so far
        if rbErr := journal.Rollback(); rbErr != nil {
            log.Printf("[ERROR] rollback failed: %v", rbErr)
        }
        return err
    }
}

// All books organized successfully — clear the journal
journal.Clear()
```

Inside `OrganizeBook`, before each file operation, record:

```go
// Before the copy/link:
journal.Record("hardlink", book.FilePath, targetPath)
// Then perform the operation
err = os.Link(book.FilePath, targetPath)
```

### Startup Self-Diagnostic

- On startup, verify: paths are writable, database schema is current, config
  is sane
- Report issues before accepting traffic

#### Checks and implementation

The diagnostic runs as the very first step in `server.Run()`, before the
HTTP router is started and before the operation queue accepts work. If any
check fails at severity `fatal`, the server refuses to start and prints a
clear error message. Checks at severity `warn` are logged but do not block.

```go
// internal/server/diagnostics.go

package server

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "runtime"

    "github.com/jdfalk/audiobook-organizer/internal/config"
    "github.com/jdfalk/audiobook-organizer/internal/database"
)

// DiagnosticSeverity classifies a check result.
type DiagnosticSeverity int

const (
    SeverityOK   DiagnosticSeverity = iota
    SeverityWarn                    // Logged but server continues
    SeverityFatal                   // Server refuses to start
)

// DiagnosticResult is one check's outcome.
type DiagnosticResult struct {
    Check    string             `json:"check"`
    Severity DiagnosticSeverity `json:"severity"`
    Message  string             `json:"message"`
}

// RunStartupDiagnostics executes all pre-flight checks and returns the results.
// The caller (server.Run) inspects the slice for any Fatal results.
func RunStartupDiagnostics() []DiagnosticResult {
    var results []DiagnosticResult

    // --- Check 1: RootDir exists and is writable ---
    results = append(results, checkPathWritable("root_dir", config.AppConfig.RootDir))

    // --- Check 2: DatabasePath parent directory is writable ---
    dbDir := filepath.Dir(config.AppConfig.DatabasePath)
    results = append(results, checkPathWritable("database_dir", dbDir))

    // --- Check 3: PlaylistDir (if configured) exists and is writable ---
    if config.AppConfig.PlaylistDir != "" {
        results = append(results, checkPathWritable("playlist_dir", config.AppConfig.PlaylistDir))
    }

    // --- Check 4: Database schema is current ---
    results = append(results, checkDatabaseSchema())

    // --- Check 5: Config sanity ---
    results = append(results, checkConfigSanity()...)

    // --- Check 6: concurrent_scans is positive ---
    if config.AppConfig.ConcurrentScans < 1 {
        results = append(results, DiagnosticResult{
            Check:    "concurrent_scans",
            Severity: SeverityWarn,
            Message:  fmt.Sprintf("concurrent_scans is %d; must be >= 1. Defaulting to 1.", config.AppConfig.ConcurrentScans),
        })
        config.AppConfig.ConcurrentScans = 1
    }

    return results
}

// checkPathWritable verifies a directory exists and is writable by attempting
// to create (and immediately remove) a temp file inside it.
func checkPathWritable(label, dirPath string) DiagnosticResult {
    if dirPath == "" {
        return DiagnosticResult{
            Check: label, Severity: SeverityWarn,
            Message: fmt.Sprintf("%s is not configured", label),
        }
    }

    // Ensure directory exists
    if err := os.MkdirAll(dirPath, 0755); err != nil {
        return DiagnosticResult{
            Check: label, Severity: SeverityFatal,
            Message: fmt.Sprintf("%s directory %s cannot be created: %v", label, dirPath, err),
        }
    }

    // Probe writability
    probe := filepath.Join(dirPath, ".audiobook-organizer-startup-probe")
    f, err := os.Create(probe)
    if err != nil {
        return DiagnosticResult{
            Check: label, Severity: SeverityFatal,
            Message: fmt.Sprintf("%s directory %s is not writable: %v", label, dirPath, err),
        }
    }
    f.Close()
    os.Remove(probe)

    return DiagnosticResult{Check: label, Severity: SeverityOK, Message: "OK"}
}

// checkDatabaseSchema verifies that migrations have run and the schema is current.
// This is a lightweight check — it reads the schema version from PebbleDB
// (or SQLite) and compares it to the expected version in migrations.go.
func checkDatabaseSchema() DiagnosticResult {
    if database.GlobalStore == nil {
        return DiagnosticResult{
            Check: "database_schema", Severity: SeverityFatal,
            Message: "database store not initialized before diagnostics ran",
        }
    }

    // RunMigrations is idempotent — calling it here is both a check and a
    // self-heal. If migrations fail, that's fatal.
    if err := database.RunMigrations(database.GlobalStore); err != nil {
        return DiagnosticResult{
            Check: "database_schema", Severity: SeverityFatal,
            Message: fmt.Sprintf("database schema migration failed: %v", err),
        }
    }

    return DiagnosticResult{Check: "database_schema", Severity: SeverityOK, Message: "schema current"}
}

// checkConfigSanity validates configuration field values that are not
// path-related (paths are checked separately above).
func checkConfigSanity() []DiagnosticResult {
    var results []DiagnosticResult

    // Organization strategy must be a known value
    validStrategies := map[string]bool{"auto": true, "copy": true, "hardlink": true, "reflink": true, "symlink": true}
    if !validStrategies[config.AppConfig.OrganizationStrategy] {
        results = append(results, DiagnosticResult{
            Check: "organization_strategy", Severity: SeverityFatal,
            Message: fmt.Sprintf("unknown organization_strategy: %q (valid: auto, copy, hardlink, reflink, symlink)",
                config.AppConfig.OrganizationStrategy),
        })
    }

    // Database type must be supported
    validDBTypes := map[string]bool{"pebble": true, "sqlite": true, "": true}
    if !validDBTypes[config.AppConfig.DatabaseType] {
        results = append(results, DiagnosticResult{
            Check: "database_type", Severity: SeverityFatal,
            Message: fmt.Sprintf("unsupported database_type: %q", config.AppConfig.DatabaseType),
        })
    }

    // AI parsing enabled but no API key
    if config.AppConfig.EnableAIParsing && config.AppConfig.OpenAIAPIKey == "" {
        results = append(results, DiagnosticResult{
            Check: "ai_parsing", Severity: SeverityWarn,
            Message: "enable_ai_parsing is true but openai_api_key is empty. AI fallback will be disabled at runtime.",
        })
    }

    return results
}
```

#### Hook point in server startup

In `internal/server/server.go`, in the `Run()` method, after
`database.InitializeStore` and before starting the HTTP listener:

```go
// Run startup diagnostics
diagnostics := RunStartupDiagnostics()
hasFatal := false
for _, d := range diagnostics {
    switch d.Severity {
    case SeverityFatal:
        log.Printf("[FATAL] startup diagnostic [%s]: %s", d.Check, d.Message)
        hasFatal = true
    case SeverityWarn:
        log.Printf("[WARN]  startup diagnostic [%s]: %s", d.Check, d.Message)
    case SeverityOK:
        log.Printf("[INFO]  startup diagnostic [%s]: %s", d.Check, d.Message)
    }
}
if hasFatal {
    log.Fatal("Startup diagnostics failed. See above for details.")
    // os.Exit(1) is called implicitly by log.Fatal
}
```

The diagnostic results are also stored in memory and served via
`GET /api/v1/system/status` so they are visible in the dashboard even after
the server has been running for a while.

---

## Operation Queue Reliability

### Operation Dependency Graph

- Organize waits for scan completion on the same folder before starting
- Prevents conflicts from concurrent scan + organize on overlapping paths

### Priority Aging

- Long-waiting normal-priority ops receive a temporary priority boost
- Prevents starvation under sustained high-priority load

### Pause / Resume Queue

- Operator can pause the work queue (e.g., during maintenance)
- Queued and in-progress ops resume cleanly after unpause

---

## Dependencies

- Parallel scanning depends on the scanner being stateless per-file (currently
  true)
- Rollback journal depends on safe file operations (already implemented)
- Circuit breaker depends on metadata client being abstracted behind an
  interface

## References

- Scanner: `internal/scanner/scanner.go` (parallel scan + process with semaphore pools)
- Safe file operations: `internal/fileops/safe_operations.go` (copy-first with SHA256 verification)
- File hashing: `internal/fileops/hash.go`
- Operation queue: `internal/operations/queue.go` (async queue with priority, progress reporting, real-time events)
- PebbleDB key patterns: `internal/database/pebble_store.go`
- Config structure: `internal/config/config.go`
- Organizer: `internal/organizer/organizer.go` (reflink → hardlink → copy chain)
