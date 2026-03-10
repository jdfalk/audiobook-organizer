// file: internal/logger/logger.go
// version: 1.0.0
// guid: f47ac10b-58cc-4372-a567-0e02b2c3d479

package logger

import (
	"fmt"
	"log"
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
