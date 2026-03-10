<!-- file: docs/superpowers/plans/2026-03-10-central-logger.md -->
<!-- version: 1.0.0 -->
<!-- guid: c4d5e6f7-a8b9-0123-4567-89abcdef0123 -->

# Central Logger Package Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the split `log.Printf` / `progress.Log()` logging with a unified `Logger` interface that routes to stdout, operation DB, and real-time hub through a single call.

**Architecture:** New `internal/logger` package with two implementations — `StandardLogger` (stdout + optional system activity log) and `OperationLogger` (stdout + DB + real-time). Library code accepts `logger.Logger` instead of using the global `log` package. Service layer creates the logger at the operation boundary and passes it down. `OperationLogger` also implements the existing `ProgressReporter` interface for backward compatibility during migration.

**Tech Stack:** Go standard library, existing SQLite/PebbleDB stores, existing realtime hub

**Spec:** `docs/superpowers/specs/2026-03-10-central-logger-design.md`

---

## File Structure

### New files to create:
- `internal/logger/logger.go` — Logger interface, Level type, Change struct, constructor functions
- `internal/logger/standard.go` — StandardLogger implementation (stdout + system activity log)
- `internal/logger/operation.go` — OperationLogger implementation (stdout + DB + real-time)
- `internal/logger/retention.go` — Pruning logic
- `internal/logger/logger_test.go` — Unit tests for StandardLogger
- `internal/logger/operation_test.go` — Unit tests for OperationLogger

### Files to modify:
- `internal/database/store.go` — Add new Store interface methods
- `internal/database/sqlite_store.go` — Implement new methods + migration 31
- `internal/database/pebble_store.go` — Implement new methods
- `internal/database/mock_store.go` — Add mock functions for new methods
- `internal/database/mocks/mock_store.go` — Add testify mock methods
- `internal/database/migrations.go` — Add migration 31
- `internal/config/config.go` — Add LogRetentionDays field
- `internal/operations/queue.go` — Create OperationLogger in worker instead of operationProgressReporter
- `internal/server/scheduler.go` — Add purge_old_logs task
- `internal/server/server.go` — Add system activity log endpoint
- `internal/server/scan_service.go` — Accept logger.Logger, pass to scanner
- `internal/server/organize_service.go` — Accept logger.Logger, pass to organizer
- `internal/scanner/scanner.go` — Accept logger.Logger, replace log.Printf
- `internal/metadata/metadata.go` — Accept logger.Logger, replace log.Printf
- `internal/organizer/organizer.go` — Accept logger.Logger, replace log.Printf
- `internal/mediainfo/mediainfo.go` — Accept logger.Logger, replace log.Printf

---

## Chunk 1: Logger Package Core

### Task 1: Logger Interface and Types

**Files:**
- Create: `internal/logger/logger.go`
- Test: `internal/logger/logger_test.go`

- [ ] **Step 1: Create the logger package with interface and types**

```go
// file: internal/logger/logger.go
// version: 1.0.0
// guid: <generate>

package logger

import (
	"fmt"
	"log"
	"sync"
)

// Level represents a log severity level.
type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the level name for display.
func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string to a Level.
func ParseLevel(s string) Level {
	switch s {
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
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

	// Get accumulated change counters (e.g., {"book_create": 150})
	ChangeCounters() map[string]int

	// Operation awareness
	IsCanceled() bool

	// Create child logger with subsystem prefix
	With(subsystem string) Logger
}

// logToStdout formats and prints a log line to stdout.
func logToStdout(subsystem string, level Level, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)
	if subsystem != "" {
		log.Printf("[%s] %s: %s", level.String(), subsystem, formatted)
	} else {
		log.Printf("[%s] %s", level.String(), formatted)
	}
}
```

- [ ] **Step 2: Write test for Level parsing**

```go
// file: internal/logger/logger_test.go
// version: 1.0.0
// guid: <generate>

package logger

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"trace", LevelTrace},
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"garbage", LevelInfo}, // default
		{"", LevelInfo},
	}
	for _, tc := range tests {
		if got := ParseLevel(tc.input); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestLevelString(t *testing.T) {
	if LevelDebug.String() != "debug" {
		t.Errorf("LevelDebug.String() = %q, want 'debug'", LevelDebug.String())
	}
}
```

- [ ] **Step 3: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/logger/logger.go internal/logger/logger_test.go
git commit -m "feat(logger): add Logger interface, Level type, and Change struct"
```

---

### Task 2: StandardLogger Implementation

**Files:**
- Create: `internal/logger/standard.go`
- Modify: `internal/logger/logger_test.go`

- [ ] **Step 1: Implement StandardLogger**

```go
// file: internal/logger/standard.go
// version: 1.0.0
// guid: <generate>

package logger

// ActivityLogWriter is an optional interface for writing to a system activity log.
// When provided to StandardLogger, INFO+ messages are also written to persistent storage.
type ActivityLogWriter interface {
	AddSystemActivityLog(source, level, message string) error
}

// StandardLogger logs to stdout only. Progress, changes, and cancellation are no-ops.
// Used for housekeeping goroutines, startup code, and CLI commands.
type StandardLogger struct {
	subsystem      string
	minStdout      Level
	activityWriter ActivityLogWriter // optional; nil = stdout only
}

// New creates a StandardLogger for the given subsystem.
func New(subsystem string) *StandardLogger {
	return &StandardLogger{
		subsystem: subsystem,
		minStdout: LevelDebug,
	}
}

// NewWithActivityLog creates a StandardLogger that also writes INFO+ to the system activity log.
func NewWithActivityLog(subsystem string, writer ActivityLogWriter) *StandardLogger {
	return &StandardLogger{
		subsystem:      subsystem,
		minStdout:      LevelDebug,
		activityWriter: writer,
	}
}

func (l *StandardLogger) log(level Level, msg string, args ...any) {
	if level >= l.minStdout {
		logToStdout(l.subsystem, level, msg, args...)
	}
	if l.activityWriter != nil && level >= LevelInfo {
		formatted := msg
		if len(args) > 0 {
			formatted = fmt.Sprintf(msg, args...)
		}
		_ = l.activityWriter.AddSystemActivityLog(l.subsystem, level.String(), formatted)
	}
}

func (l *StandardLogger) Trace(msg string, args ...any) { l.log(LevelTrace, msg, args...) }
func (l *StandardLogger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args...) }
func (l *StandardLogger) Info(msg string, args ...any)  { l.log(LevelInfo, msg, args...) }
func (l *StandardLogger) Warn(msg string, args ...any)  { l.log(LevelWarn, msg, args...) }
func (l *StandardLogger) Error(msg string, args ...any) { l.log(LevelError, msg, args...) }

func (l *StandardLogger) UpdateProgress(current, total int, message string) {} // no-op
func (l *StandardLogger) RecordChange(change Change)                         {} // no-op
func (l *StandardLogger) ChangeCounters() map[string]int                     { return nil }
func (l *StandardLogger) IsCanceled() bool                                   { return false }

func (l *StandardLogger) With(subsystem string) Logger {
	prefix := subsystem
	if l.subsystem != "" {
		prefix = l.subsystem + "." + subsystem
	}
	return &StandardLogger{
		subsystem:      prefix,
		minStdout:      l.minStdout,
		activityWriter: l.activityWriter,
	}
}
```

- [ ] **Step 2: Add StandardLogger tests**

Add to `internal/logger/logger_test.go`:

```go
func TestStandardLogger_With(t *testing.T) {
	log := New("parent")
	child := log.With("child")
	// Child should be a StandardLogger with combined subsystem
	sl, ok := child.(*StandardLogger)
	if !ok {
		t.Fatal("expected *StandardLogger from With()")
	}
	if sl.subsystem != "parent.child" {
		t.Errorf("subsystem = %q, want 'parent.child'", sl.subsystem)
	}
}

func TestStandardLogger_NoOps(t *testing.T) {
	log := New("test")
	// These should not panic
	log.UpdateProgress(1, 10, "msg")
	log.RecordChange(Change{ChangeType: "test"})
	if log.IsCanceled() {
		t.Error("StandardLogger.IsCanceled() should return false")
	}
	if log.ChangeCounters() != nil {
		t.Error("StandardLogger.ChangeCounters() should return nil")
	}
}

func TestStandardLogger_ActivityWriter(t *testing.T) {
	var captured []string
	writer := &mockActivityWriter{
		addFunc: func(source, level, message string) error {
			captured = append(captured, level+":"+message)
			return nil
		},
	}
	log := NewWithActivityLog("test", writer)
	log.Debug("should not be captured")  // below INFO
	log.Info("should be captured")
	log.Warn("also captured")

	if len(captured) != 2 {
		t.Errorf("expected 2 captured, got %d", len(captured))
	}
}

type mockActivityWriter struct {
	addFunc func(source, level, message string) error
}

func (m *mockActivityWriter) AddSystemActivityLog(source, level, message string) error {
	if m.addFunc != nil {
		return m.addFunc(source, level, message)
	}
	return nil
}
```

- [ ] **Step 3: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/logger/standard.go internal/logger/logger_test.go
git commit -m "feat(logger): add StandardLogger implementation"
```

---

### Task 3: OperationLogger Implementation

**Files:**
- Create: `internal/logger/operation.go`
- Create: `internal/logger/operation_test.go`

- [ ] **Step 1: Implement OperationLogger**

```go
// file: internal/logger/operation.go
// version: 1.0.0
// guid: <generate>

package logger

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// OperationStore is the subset of database.Store needed by OperationLogger.
// Keeps the logger package decoupled from the full Store interface.
type OperationStore interface {
	AddOperationLog(operationID, level, message string, details *string) error
	CreateOperationChange(change interface{}) error
	UpdateOperationProgress(id string, current, total int, message string) error
}

// RealtimeHub is the subset of the real-time hub needed by OperationLogger.
type RealtimeHub interface {
	SendOperationLog(operationID, level, message string, details *string)
	SendOperationProgress(operationID string, current, total int, message string)
}

// OperationLogger logs to stdout + operation DB + real-time hub.
// It also implements the operations.ProgressReporter interface for backward compatibility.
type OperationLogger struct {
	operationID string
	subsystem   string
	store       OperationStore
	hub         RealtimeHub
	minDBLevel  Level
	minStdout   Level
	canceled    *atomic.Bool
	changes     []Change
	counters    map[string]int
	mu          sync.Mutex
}

// ForOperation creates an OperationLogger bound to a specific operation.
func ForOperation(operationID string, store OperationStore, hub RealtimeHub) *OperationLogger {
	return &OperationLogger{
		operationID: operationID,
		store:       store,
		hub:         hub,
		minDBLevel:  LevelInfo,
		minStdout:   LevelDebug,
		canceled:    &atomic.Bool{},
		counters:    make(map[string]int),
	}
}

// SetMinDBLevel sets the minimum level for DB/real-time logging.
func (l *OperationLogger) SetMinDBLevel(level Level) {
	l.minDBLevel = level
}

// SetCanceled marks the operation as canceled.
func (l *OperationLogger) SetCanceled() {
	l.canceled.Store(true)
}

func (l *OperationLogger) log(level Level, msg string, args ...any) {
	formatted := fmt.Sprintf(msg, args...)

	if level >= l.minStdout {
		logToStdout(l.subsystem, level, msg, args...)
	}

	if level >= l.minDBLevel {
		if l.store != nil {
			_ = l.store.AddOperationLog(l.operationID, level.String(), formatted, nil)
		}
		if l.hub != nil {
			l.hub.SendOperationLog(l.operationID, level.String(), formatted, nil)
		}
	}
}

func (l *OperationLogger) Trace(msg string, args ...any) { l.log(LevelTrace, msg, args...) }
func (l *OperationLogger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args...) }
func (l *OperationLogger) Info(msg string, args ...any)  { l.log(LevelInfo, msg, args...) }
func (l *OperationLogger) Warn(msg string, args ...any)  { l.log(LevelWarn, msg, args...) }
func (l *OperationLogger) Error(msg string, args ...any) { l.log(LevelError, msg, args...) }

func (l *OperationLogger) UpdateProgress(current, total int, message string) {
	if l.store != nil {
		_ = l.store.UpdateOperationProgress(l.operationID, current, total, message)
	}
	if l.hub != nil {
		l.hub.SendOperationProgress(l.operationID, current, total, message)
	}
}

func (l *OperationLogger) RecordChange(change Change) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changes = append(l.changes, change)
	l.counters[change.ChangeType]++
}

func (l *OperationLogger) ChangeCounters() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make(map[string]int, len(l.counters))
	for k, v := range l.counters {
		cp[k] = v
	}
	return cp
}

func (l *OperationLogger) Changes() []Change {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]Change, len(l.changes))
	copy(cp, l.changes)
	return cp
}

func (l *OperationLogger) IsCanceled() bool {
	return l.canceled.Load()
}

func (l *OperationLogger) With(subsystem string) Logger {
	prefix := subsystem
	if l.subsystem != "" {
		prefix = l.subsystem + "." + subsystem
	}
	return &OperationLogger{
		operationID: l.operationID,
		subsystem:   prefix,
		store:       l.store,
		hub:         l.hub,
		minDBLevel:  l.minDBLevel,
		minStdout:   l.minStdout,
		canceled:    l.canceled,   // shared
		changes:     l.changes,    // shared slice header (append-only via parent)
		counters:    l.counters,   // shared map
		mu:          sync.Mutex{}, // child gets own mutex but shares data via parent
	}
}

// --- Backward compatibility with operations.ProgressReporter ---

// Log implements operations.ProgressReporter.Log for backward compatibility.
func (l *OperationLogger) Log(level, message string, details *string) error {
	lvl := ParseLevel(level)
	if lvl >= l.minStdout {
		logToStdout(l.subsystem, lvl, "%s", message)
	}
	if lvl >= l.minDBLevel {
		if l.store != nil {
			_ = l.store.AddOperationLog(l.operationID, level, message, details)
		}
		if l.hub != nil {
			l.hub.SendOperationLog(l.operationID, level, message, details)
		}
	}
	return nil
}
```

**Important note about `With()` and shared state:** Child loggers created via `With()` share the parent's `canceled` flag and accumulate changes/counters in the parent's data structures. This is intentional — all subsystem loggers for one operation contribute to the same change counters. The mutex on each child protects concurrent writes. However, the child's `changes` slice shares the backing array via append semantics, so we should use the parent's mutex for writes. Let me revise:

Actually, the simpler approach: child loggers hold a pointer back to the parent's mutex and data. Let me adjust the `With()` to share the parent's mutex:

Replace the `With()` method and add a shared data struct:

```go
// sharedState holds data shared between a parent OperationLogger and its children.
type sharedState struct {
	mu       sync.Mutex
	changes  []Change
	counters map[string]int
	canceled *atomic.Bool
}
```

Update `OperationLogger` to use `*sharedState` and adjust `ForOperation()` and `With()` accordingly. The child logger references the same `*sharedState`.

- [ ] **Step 2: Write OperationLogger tests**

```go
// file: internal/logger/operation_test.go
// version: 1.0.0
// guid: <generate>

package logger

import (
	"sync"
	"testing"
)

type mockOpStore struct {
	logs     []string
	changes  []interface{}
	progress []string
	mu       sync.Mutex
}

func (m *mockOpStore) AddOperationLog(opID, level, message string, details *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, level+":"+message)
	return nil
}

func (m *mockOpStore) CreateOperationChange(change interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, change)
	return nil
}

func (m *mockOpStore) UpdateOperationProgress(id string, current, total int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = append(m.progress, message)
	return nil
}

type mockHub struct {
	logsSent     int
	progressSent int
	mu           sync.Mutex
}

func (m *mockHub) SendOperationLog(opID, level, message string, details *string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logsSent++
}

func (m *mockHub) SendOperationProgress(opID string, current, total int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progressSent++
}

func TestOperationLogger_LevelFiltering(t *testing.T) {
	store := &mockOpStore{}
	hub := &mockHub{}
	log := ForOperation("op1", store, hub)

	log.Debug("debug msg")  // below default minDBLevel (INFO)
	log.Info("info msg")    // at minDBLevel
	log.Warn("warn msg")    // above minDBLevel

	if len(store.logs) != 2 {
		t.Errorf("expected 2 DB logs (info+warn), got %d: %v", len(store.logs), store.logs)
	}
	if hub.logsSent != 2 {
		t.Errorf("expected 2 hub sends, got %d", hub.logsSent)
	}
}

func TestOperationLogger_DebugWhenVerbose(t *testing.T) {
	store := &mockOpStore{}
	log := ForOperation("op1", store, nil)
	log.SetMinDBLevel(LevelDebug)

	log.Debug("debug msg")
	log.Trace("trace msg") // still below DEBUG

	if len(store.logs) != 1 {
		t.Errorf("expected 1 DB log (debug), got %d", len(store.logs))
	}
}

func TestOperationLogger_RecordChange(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	log.RecordChange(Change{ChangeType: "book_create", Summary: "Created book A"})
	log.RecordChange(Change{ChangeType: "book_create", Summary: "Created book B"})
	log.RecordChange(Change{ChangeType: "book_update", Summary: "Updated book C"})

	counters := log.ChangeCounters()
	if counters["book_create"] != 2 {
		t.Errorf("book_create = %d, want 2", counters["book_create"])
	}
	if counters["book_update"] != 1 {
		t.Errorf("book_update = %d, want 1", counters["book_update"])
	}
}

func TestOperationLogger_IsCanceled(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	if log.IsCanceled() {
		t.Error("should not be canceled initially")
	}
	log.SetCanceled()
	if !log.IsCanceled() {
		t.Error("should be canceled after SetCanceled()")
	}
}

func TestOperationLogger_With(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	child := log.With("scanner")

	// Child should share canceled state
	log.SetCanceled()
	if !child.IsCanceled() {
		t.Error("child should see parent's canceled state")
	}
}

func TestOperationLogger_Progress(t *testing.T) {
	store := &mockOpStore{}
	hub := &mockHub{}
	log := ForOperation("op1", store, hub)

	log.UpdateProgress(5, 100, "scanning")

	if len(store.progress) != 1 {
		t.Errorf("expected 1 progress update, got %d", len(store.progress))
	}
	if hub.progressSent != 1 {
		t.Errorf("expected 1 hub progress, got %d", hub.progressSent)
	}
}

func TestOperationLogger_BackwardCompatLog(t *testing.T) {
	store := &mockOpStore{}
	log := ForOperation("op1", store, nil)

	// Call the ProgressReporter-compatible Log method
	_ = log.Log("info", "backward compat message", nil)

	if len(store.logs) != 1 {
		t.Errorf("expected 1 DB log, got %d", len(store.logs))
	}
}
```

- [ ] **Step 3: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/logger/operation.go internal/logger/operation_test.go
git commit -m "feat(logger): add OperationLogger implementation with DB + real-time routing"
```

---

## Chunk 2: Database Layer — Store Methods, Migration, Config

### Task 4: Add Store Interface Methods and Migration 31

**Files:**
- Modify: `internal/database/store.go:150-157` — Add new interface methods
- Modify: `internal/database/migrations.go:217-221` — Add migration 31
- Modify: `internal/database/sqlite_store.go` — Implement new methods
- Modify: `internal/database/pebble_store.go` — Implement new methods
- Modify: `internal/database/mock_store.go` — Add mock function fields
- Modify: `internal/config/config.go:130` — Add LogRetentionDays

- [ ] **Step 1: Add SystemActivityLog struct and new methods to store.go**

In `internal/database/store.go`, after the OperationChange struct (~line 453), add:

```go
// SystemActivityLog represents a log entry from a housekeeping goroutine.
type SystemActivityLog struct {
	ID        int       `json:"id"`
	Source    string    `json:"source"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
```

In the Store interface, add after GetOperationChanges (~line 157):

```go
// System activity log
AddSystemActivityLog(source, level, message string) error
GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error)

// Retention pruning
PruneOperationLogs(olderThan time.Time) (int, error)
PruneOperationChanges(olderThan time.Time) (int, error)
PruneSystemActivityLogs(olderThan time.Time) (int, error)
```

- [ ] **Step 2: Add migration 31 to migrations.go**

In `internal/database/migrations.go`, add to the migrations slice after migration 30:

```go
{
	Version:     31,
	Description: "Add system_activity_log table and logs_pruned flag",
	Up:          migration031Up,
	Down:        nil,
},
```

Add the migration function:

```go
func migration031Up(store Store) error {
	sqlStore, ok := store.(*SQLiteStore)
	if !ok {
		return nil // PebbleDB handles this via prefix keys
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS system_activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_activity_source ON system_activity_log(source)`,
		`CREATE INDEX IF NOT EXISTS idx_system_activity_created ON system_activity_log(created_at)`,
		`ALTER TABLE operations ADD COLUMN logs_pruned BOOLEAN DEFAULT 0`,
	}
	for _, stmt := range statements {
		if _, err := sqlStore.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migration 31: %w", err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 3: Implement SQLite methods in sqlite_store.go**

Add after the existing GetOperationChanges method:

```go
func (s *SQLiteStore) AddSystemActivityLog(source, level, message string) error {
	_, err := s.db.Exec(
		"INSERT INTO system_activity_log (source, level, message) VALUES (?, ?, ?)",
		source, level, message,
	)
	return err
}

func (s *SQLiteStore) GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error) {
	query := "SELECT id, source, level, message, created_at FROM system_activity_log"
	args := []interface{}{}
	if source != "" {
		query += " WHERE source = ?"
		args = append(args, source)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SystemActivityLog
	for rows.Next() {
		var l SystemActivityLog
		if err := rows.Scan(&l.ID, &l.Source, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *SQLiteStore) PruneOperationLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_logs WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) PruneOperationChanges(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM operation_changes WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) PruneSystemActivityLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM system_activity_log WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
```

- [ ] **Step 4: Implement PebbleDB methods in pebble_store.go**

Use PebbleDB's prefix-scan pattern. Key format: `syslog:{RFC3339Nano}:{source}` with JSON value.

```go
func (p *PebbleStore) AddSystemActivityLog(source, level, message string) error {
	key := fmt.Sprintf("syslog:%s:%s", time.Now().Format(time.RFC3339Nano), source)
	val := SystemActivityLog{
		Source:    source,
		Level:     level,
		Message:   message,
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return p.db.Set([]byte(key), data, pebble.Sync)
}

func (p *PebbleStore) GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error) {
	prefix := []byte("syslog:")
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: incrementPrefix(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var logs []SystemActivityLog
	// Collect all, then reverse (newest first) and filter
	for iter.Last(); iter.Valid(); iter.Prev() {
		var l SystemActivityLog
		if err := json.Unmarshal(iter.Value(), &l); err != nil {
			continue
		}
		if source != "" && l.Source != source {
			continue
		}
		logs = append(logs, l)
		if len(logs) >= limit {
			break
		}
	}
	return logs, nil
}

func (p *PebbleStore) PruneOperationLogs(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("oplog:", olderThan)
}

func (p *PebbleStore) PruneOperationChanges(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("opchange:", olderThan)
}

func (p *PebbleStore) PruneSystemActivityLogs(olderThan time.Time) (int, error) {
	return p.pruneByTimestampPrefix("syslog:", olderThan)
}

// pruneByTimestampPrefix deletes all keys with the given prefix whose
// embedded RFC3339 timestamp is before olderThan.
func (p *PebbleStore) pruneByTimestampPrefix(prefix string, olderThan time.Time) (int, error) {
	prefixBytes := []byte(prefix)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefixBytes,
		UpperBound: incrementPrefix(prefixBytes),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	deleted := 0
	batch := p.db.NewBatch()
	defer batch.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Extract timestamp from key: "prefix:2026-03-10T..."
		parts := strings.SplitN(strings.TrimPrefix(key, prefix), ":", 2)
		if len(parts) == 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			continue
		}
		if ts.Before(olderThan) {
			_ = batch.Delete(iter.Key(), nil)
			deleted++
		}
	}
	if deleted > 0 {
		return deleted, batch.Commit(pebble.Sync)
	}
	return 0, nil
}
```

- [ ] **Step 5: Add mock functions to mock_store.go**

In `internal/database/mock_store.go`, add to the MockStore struct:

```go
AddSystemActivityLogFunc    func(source, level, message string) error
GetSystemActivityLogsFunc   func(source string, limit int) ([]SystemActivityLog, error)
PruneOperationLogsFunc      func(olderThan time.Time) (int, error)
PruneOperationChangesFunc   func(olderThan time.Time) (int, error)
PruneSystemActivityLogsFunc func(olderThan time.Time) (int, error)
```

Add the corresponding method implementations that delegate to the function fields.

- [ ] **Step 6: Add LogRetentionDays to config**

In `internal/config/config.go`, after `OperationTimeoutMinutes` (~line 130), add:

```go
LogRetentionDays int `json:"log_retention_days"` // default 90, 0 = keep forever
```

In the defaults function, set: `LogRetentionDays: 90`

- [ ] **Step 7: Run tests to verify compilation**

Run: `GOEXPERIMENT=jsonv2 go build ./...`
Expected: compiles without errors

- [ ] **Step 8: Commit**

```bash
git add internal/database/ internal/config/config.go
git commit -m "feat(db): add system_activity_log table, retention methods, migration 31"
```

---

## Chunk 3: Wire Logger Into Operations Queue

### Task 5: Replace operationProgressReporter with OperationLogger

**Files:**
- Modify: `internal/operations/queue.go:269-274` — Create OperationLogger in worker
- Test: Run existing operation tests to verify backward compat

- [ ] **Step 1: Import logger package in queue.go**

Add `"github.com/jdfalk/audiobook-organizer/internal/logger"` to imports.

- [ ] **Step 2: Update worker to create OperationLogger**

At lines 269-274 in queue.go, replace the operationProgressReporter creation:

```go
// Before:
reporter := &operationProgressReporter{
	operationID: op.ID,
	store:       q.store,
	queue:       q,
}

// After:
reporter := logger.ForOperation(op.ID, &queueStoreAdapter{store: q.store, queue: q}, realtime.GetGlobalHub())
```

The `queueStoreAdapter` bridges the full `database.Store` to the `logger.OperationStore` interface:

```go
// queueStoreAdapter adapts the database.Store + queue to the logger.OperationStore interface.
type queueStoreAdapter struct {
	store database.Store
	queue *OperationQueue
}

func (a *queueStoreAdapter) AddOperationLog(operationID, level, message string, details *string) error {
	return a.store.AddOperationLog(operationID, level, message, details)
}

func (a *queueStoreAdapter) CreateOperationChange(change interface{}) error {
	if c, ok := change.(*database.OperationChange); ok {
		return a.store.CreateOperationChange(c)
	}
	return nil
}

func (a *queueStoreAdapter) UpdateOperationProgress(id string, current, total int, message string) error {
	return a.store.UpdateOperationProgress(id, current, total, message)
}
```

- [ ] **Step 3: Verify OperationLogger satisfies ProgressReporter**

The existing `OperationFunc` takes `ProgressReporter`. Since `OperationLogger` implements `Log()`, `UpdateProgress()` (returning error), and `IsCanceled()`, we need to ensure the signatures match. The `OperationLogger.Log()` method already returns `error`. The `UpdateProgress` on Logger interface returns nothing, but the backward-compat `UpdateProgress` on ProgressReporter returns `error`.

Add an adapter method to OperationLogger:

```go
// UpdateProgressCompat implements operations.ProgressReporter.UpdateProgress.
func (l *OperationLogger) UpdateProgressCompat(current, total int, message string) error {
	l.UpdateProgress(current, total, message)
	return nil
}
```

Or better: change the `OperationFunc` signature to accept `logger.Logger` directly. But for backward compat in this phase, we can have `OperationLogger` implement `ProgressReporter` explicitly. Add a compile-time check:

```go
var _ ProgressReporter = (*logger.OperationLogger)(nil)
```

If the interfaces don't align exactly, create a thin wrapper struct in queue.go that adapts OperationLogger to ProgressReporter.

- [ ] **Step 4: Run existing operation tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/operations/ -v`
Expected: PASS (all existing tests still work)

- [ ] **Step 5: Commit**

```bash
git add internal/operations/queue.go internal/logger/operation.go
git commit -m "feat(logger): wire OperationLogger into operations queue worker"
```

---

## Chunk 4: Migrate Service Layer (Scan + Organize)

### Task 6: Migrate ScanService to use Logger

**Files:**
- Modify: `internal/server/scan_service.go:44-56` — Change ProgressReporter param to Logger
- Modify: `internal/scanner/scanner.go:84-92` — Add Logger param to ScanDirectory/ScanDirectoryParallel
- Modify: `internal/scanner/scanner.go` — Replace all `log.Printf` with logger calls
- Modify: `internal/metadata/metadata.go:92` — Add Logger param to ExtractMetadata
- Modify: `internal/metadata/metadata.go` — Replace all `log.Printf` with logger calls

- [ ] **Step 1: Update ScanService to accept logger.Logger**

In `scan_service.go`, change `PerformScan` signature:

```go
// Before:
func (ss *ScanService) PerformScan(ctx context.Context, req *ScanRequest, progress operations.ProgressReporter) error

// After:
func (ss *ScanService) PerformScan(ctx context.Context, req *ScanRequest, log logger.Logger) error
```

Replace all `progress.Log("info", msg, nil)` calls with `log.Info(msg)`.
Replace all `progress.Log("warn", msg, nil)` calls with `log.Warn(msg)`.
Replace all `progress.Log("error", msg, details)` calls with `log.Error(msg)`.
Replace all `progress.UpdateProgress(...)` calls with `log.UpdateProgress(...)`.
Replace all `progress.IsCanceled()` calls with `log.IsCanceled()`.

- [ ] **Step 2: Update scanner.go public functions to accept logger.Logger**

```go
// Before:
func ScanDirectory(rootDir string) ([]Book, error)

// After:
func ScanDirectory(rootDir string, log logger.Logger) ([]Book, error)
```

Replace all `log.Printf("[DEBUG] scanner: ...")` with `log.Debug("...")`.
Replace all `log.Printf("[TRACE] scanner: ...")` with `log.Trace("...")`.
Replace all `log.Printf("[INFO] scanner: ...")` with `log.Info("...")`.

If a nil logger is passed (for backward compat), create a default:
```go
if log == nil {
	log = logger.New("scanner")
}
```

- [ ] **Step 3: Update metadata.go to accept logger.Logger**

```go
// Before:
func ExtractMetadata(filePath string) (Metadata, error)

// After:
func ExtractMetadata(filePath string, log logger.Logger) (Metadata, error)
```

Replace all `log.Printf("[DEBUG] metadata: ...")` with `log.Debug("...")`.
Replace all `log.Printf("[TRACE] metadata: ...")` with `log.Trace("...")`.

If nil logger, default to `logger.New("metadata")`.

- [ ] **Step 4: Update all callers of ScanDirectory and ExtractMetadata**

Search for all call sites and pass the logger through. Most callers are in scan_service.go which already has the logger.

- [ ] **Step 5: Add scan change tracking**

In scan_service.go, after a book is created or updated, add:

```go
log.RecordChange(logger.Change{
	BookID:     book.ID,
	ChangeType: "book_create",
	Summary:    fmt.Sprintf("Created '%s' by %s", book.Title, book.Author),
})
```

At the end of PerformScan, log the summary:

```go
counters := log.ChangeCounters()
log.Info("scan complete: %d created, %d updated, %d skipped",
	counters["book_create"], counters["book_update"], counters["book_skip"])
```

- [ ] **Step 6: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestScan -v`
Run: `GOEXPERIMENT=jsonv2 go test ./internal/scanner/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/server/scan_service.go internal/scanner/scanner.go internal/metadata/metadata.go
git commit -m "feat(logger): migrate scan service, scanner, and metadata to unified logger"
```

---

### Task 7: Migrate OrganizeService to use Logger

**Files:**
- Modify: `internal/server/organize_service.go:52-61` — Change ProgressReporter to Logger
- Modify: `internal/organizer/organizer.go:38-53` — Add Logger param

- [ ] **Step 1: Update OrganizeService to accept logger.Logger**

Same pattern as Task 6: replace `progress` param with `log logger.Logger`, update all log calls.

- [ ] **Step 2: Update organizer.go to accept logger.Logger**

Add logger to `NewOrganizer` or to `OrganizeBook` method. Replace `log.Printf` calls.

- [ ] **Step 3: Update mediainfo.go to accept logger.Logger**

```go
func Extract(filePath string, log logger.Logger) (*MediaInfo, error)
```

Replace `log.Printf` calls. Nil-check with default.

- [ ] **Step 4: Run tests**

Run: `GOEXPERIMENT=jsonv2 go test ./internal/server/ -run TestOrganize -v`
Run: `GOEXPERIMENT=jsonv2 go test ./internal/organizer/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/organize_service.go internal/organizer/organizer.go internal/mediainfo/mediainfo.go
git commit -m "feat(logger): migrate organize service, organizer, and mediainfo to unified logger"
```

---

## Chunk 5: Non-Operation Goroutines, Retention, and Remaining Services

### Task 8: Wrap OpenLibrary Download in Operation

**Files:**
- Modify: `internal/server/openlibrary_service.go` — Wrap startOLDownload in Enqueue

- [ ] **Step 1: Find startOLDownload and wrap in operation**

Currently it spawns a raw goroutine. Wrap the download work in `GlobalQueue.Enqueue()` so it gets progress, logging, and cancellation.

- [ ] **Step 2: Run tests, commit**

```bash
git commit -m "feat(logger): wrap OpenLibrary download in operations queue"
```

---

### Task 9: Add Log Retention Scheduled Task

**Files:**
- Create: `internal/logger/retention.go`
- Modify: `internal/server/scheduler.go` — Register purge_old_logs task

- [ ] **Step 1: Implement retention logic**

```go
// file: internal/logger/retention.go
// version: 1.0.0
// guid: <generate>

package logger

import (
	"fmt"
	"time"
)

// RetentionStore is the subset of the store needed for log pruning.
type RetentionStore interface {
	PruneOperationLogs(olderThan time.Time) (int, error)
	PruneOperationChanges(olderThan time.Time) (int, error)
	PruneSystemActivityLogs(olderThan time.Time) (int, error)
}

// PruneOldLogs deletes logs, changes, and activity entries older than retentionDays.
// Returns total records pruned.
func PruneOldLogs(store RetentionStore, retentionDays int, log Logger) (int, error) {
	if retentionDays <= 0 {
		log.Info("log retention disabled (0 days), skipping prune")
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	total := 0

	n, err := store.PruneOperationLogs(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune operation logs: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d operation log entries", n)
	}

	n, err = store.PruneOperationChanges(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune operation changes: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d operation change entries", n)
	}

	n, err = store.PruneSystemActivityLogs(cutoff)
	if err != nil {
		return total, fmt.Errorf("prune system activity logs: %w", err)
	}
	total += n
	if n > 0 {
		log.Info("pruned %d system activity log entries", n)
	}

	return total, nil
}
```

- [ ] **Step 2: Register purge_old_logs task in scheduler.go**

Add after the last task registration:

```go
s.registerTask(TaskDefinition{
	Name:        "purge_old_logs",
	Description: "Prune operation logs and system activity logs older than retention period",
	Category:    "maintenance",
	TriggerFn: func() (*database.Operation, error) {
		opID := ulid.Make().String()
		op, err := database.GlobalStore.CreateOperation(opID, "purge_old_logs", "")
		if err != nil {
			return nil, err
		}
		operations.GlobalQueue.Enqueue(opID, "purge_old_logs", 1, func(ctx context.Context, progress operations.ProgressReporter) error {
			log := logger.ForOperation(opID, /* adapter */, realtime.GetGlobalHub())
			_, err := logger.PruneOldLogs(database.GlobalStore, config.AppConfig.LogRetentionDays, log)
			return err
		})
		return op, nil
	},
	IsEnabled:   func() bool { return config.AppConfig.LogRetentionDays > 0 },
	GetInterval: func() time.Duration { return 7 * 24 * time.Hour }, // weekly
	RunOnStart:  func() bool { return false },
})
```

- [ ] **Step 3: Run tests, commit**

```bash
git add internal/logger/retention.go internal/server/scheduler.go
git commit -m "feat(logger): add log retention pruning scheduled task"
```

---

### Task 10: Migrate Remaining Services and Housekeeping Goroutines

**Files:**
- Modify: `internal/server/server.go` — Add system activity log endpoint, update housekeeping goroutines
- Modify: `internal/server/itunes.go` — Migrate to logger
- Modify: `internal/server/reconcile.go` — Migrate to logger
- Modify: `internal/server/metadata_fetch_service.go` — Migrate to logger

- [ ] **Step 1: Add system activity log API endpoint**

In server.go route registration, add:

```go
protected.GET("/system/activity-log", s.getSystemActivityLog)
```

Handler:
```go
func (s *Server) getSystemActivityLog(c *gin.Context) {
	source := c.Query("source")
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	logs, err := database.GlobalStore.GetSystemActivityLogs(source, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
}
```

- [ ] **Step 2: Update housekeeping goroutines to use StandardLogger**

Replace `log.Printf` in:
- `failStaleOperations()` → `logger.NewWithActivityLog("reaper", database.GlobalStore)`
- iTunes sync scheduler → `logger.NewWithActivityLog("scheduler", database.GlobalStore)`
- Updater checker → `logger.NewWithActivityLog("updater", database.GlobalStore)`

- [ ] **Step 3: Migrate remaining services**

For each service (itunes.go, reconcile.go, metadata_fetch_service.go):
- Change `progress operations.ProgressReporter` to `log logger.Logger`
- Replace `progress.Log(...)` with `log.Info(...)` etc.
- Replace `progress.UpdateProgress(...)` with `log.UpdateProgress(...)`

- [ ] **Step 4: Run full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(logger): migrate all services and housekeeping to unified logger"
```

---

### Task 11: Cleanup — Remove Old ProgressReporter Usage

**Files:**
- Modify: `internal/operations/queue.go` — Remove operationProgressReporter struct (if no longer used)
- Verify: No remaining `log.Printf("[DEBUG]` or `log.Printf("[TRACE]` in operation-aware code

- [ ] **Step 1: Search for remaining log.Printf in operation-aware code**

Run: `grep -rn 'log.Printf.*\[DEBUG\]\|log.Printf.*\[TRACE\]\|log.Printf.*\[INFO\]' internal/scanner/ internal/metadata/ internal/organizer/ internal/mediainfo/ internal/server/`

Expected: no matches (all replaced with logger calls)

- [ ] **Step 2: Remove operationProgressReporter if unused**

If all code now uses `OperationLogger`, remove the old struct from queue.go.
If some code still uses `ProgressReporter`, keep the interface as a type alias or adapter.

- [ ] **Step 3: Run full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "refactor(logger): remove legacy ProgressReporter, complete migration to unified logger"
```
