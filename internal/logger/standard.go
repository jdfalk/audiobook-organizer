// file: internal/logger/standard.go
// version: 1.1.0
// guid: 3f2504e0-4f89-11d3-9a0c-0305e82c3301

package logger

import (
	"fmt"
	"sync"
)

// ActivityLogWriter is an optional interface for writing to a system activity log.
type ActivityLogWriter interface {
	AddSystemActivityLog(source, level, message string) error
}

// ActivityEntryRecorder is a callback for dual-writing structured log entries
// to the unified activity log.
type ActivityEntryRecorder func(tier, typ, level, source, summary string)

// globalActivityRecorder is the package-level recorder set by the server.
var (
	globalActivityRecorderMu sync.RWMutex
	globalActivityRecorder   ActivityEntryRecorder
)

// SetGlobalActivityRecorder sets the package-level activity recorder.
// All new StandardLogger and OperationLogger instances will inherit it.
func SetGlobalActivityRecorder(r ActivityEntryRecorder) {
	globalActivityRecorderMu.Lock()
	defer globalActivityRecorderMu.Unlock()
	globalActivityRecorder = r
}

// getGlobalActivityRecorder returns the current global recorder (thread-safe).
func getGlobalActivityRecorder() ActivityEntryRecorder {
	globalActivityRecorderMu.RLock()
	defer globalActivityRecorderMu.RUnlock()
	return globalActivityRecorder
}

// StandardLogger logs to stdout only. Progress, changes, and cancellation are no-ops.
type StandardLogger struct {
	subsystem        string
	minStdout        Level
	activityWriter   ActivityLogWriter
	activityRecorder ActivityEntryRecorder
}

// SetActivityRecorder sets the activity entry recorder on this logger.
func (l *StandardLogger) SetActivityRecorder(r ActivityEntryRecorder) {
	l.activityRecorder = r
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
	// Dual-write to unified activity log (INFO+ only to avoid noise)
	recorder := l.activityRecorder
	if recorder == nil {
		recorder = getGlobalActivityRecorder()
	}
	if recorder != nil && level >= LevelInfo {
		formatted := msg
		if len(args) > 0 {
			formatted = fmt.Sprintf(msg, args...)
		}
		recorder("debug", "system", level.String(), l.subsystem, formatted)
	}
}

func (l *StandardLogger) Trace(msg string, args ...any) { l.log(LevelTrace, msg, args...) }
func (l *StandardLogger) Debug(msg string, args ...any) { l.log(LevelDebug, msg, args...) }
func (l *StandardLogger) Info(msg string, args ...any)  { l.log(LevelInfo, msg, args...) }
func (l *StandardLogger) Warn(msg string, args ...any)  { l.log(LevelWarn, msg, args...) }
func (l *StandardLogger) Error(msg string, args ...any) { l.log(LevelError, msg, args...) }

func (l *StandardLogger) UpdateProgress(current, total int, message string) {}
func (l *StandardLogger) RecordChange(change Change)                         {}
func (l *StandardLogger) ChangeCounters() map[string]int                     { return nil }
func (l *StandardLogger) IsCanceled() bool                                   { return false }

func (l *StandardLogger) With(subsystem string) Logger {
	prefix := subsystem
	if l.subsystem != "" {
		prefix = l.subsystem + "." + subsystem
	}
	return &StandardLogger{
		subsystem:        prefix,
		minStdout:        l.minStdout,
		activityWriter:   l.activityWriter,
		activityRecorder: l.activityRecorder,
	}
}
