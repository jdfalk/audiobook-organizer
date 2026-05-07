// file: internal/plugins/itunes/adapter.go
// version: 1.0.0
// guid: c0d1e2f3-a4b5-6c7d-8e9f-0a1b2c3d4e5f
// last-edited: 2026-05-07

package itunes

import (
	"fmt"
	"log/slog"

	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// progressAdapter converts sdk.Reporter to operations.ProgressReporter.
// It wraps the SDK reporter and implements the operations.ProgressReporter interface
// so that service methods that expect ProgressReporter can work with SDK reporters.
type progressAdapter struct {
	reporter sdk.Reporter
}

func (pa *progressAdapter) UpdateProgress(current, total int, message string) error {
	return pa.reporter.UpdateProgress(current, total, message)
}

func (pa *progressAdapter) Log(level, message string, details *string) error {
	switch level {
	case "error":
		pa.reporter.Logger().Error(message)
	case "warn":
		pa.reporter.Logger().Warn(message)
	case "info":
		pa.reporter.Logger().Info(message)
	case "debug":
		pa.reporter.Logger().Debug(message)
	default:
		pa.reporter.Logger().Info(message)
	}
	return nil
}

func (pa *progressAdapter) IsCanceled() bool {
	return pa.reporter.IsCanceled()
}

// loggerWrapper wraps an SDK reporter and implements logger.Logger.
// It delegates logging to the SDK reporter's slog.Logger and forwards progress updates
// to the reporter so that service methods can use the standard logger.Logger interface.
type loggerWrapper struct {
	reporter sdk.Reporter
	slog     *slog.Logger
}

// NewLoggerWrapper creates a logger that wraps an SDK reporter.
func NewLoggerWrapper(reporter sdk.Reporter) logger.Logger {
	return &loggerWrapper{
		reporter: reporter,
		slog:     reporter.Logger(),
	}
}

func (lw *loggerWrapper) Trace(msg string, args ...any) {
	// Trace is converted to Debug for slog
	lw.slog.Debug(fmt.Sprintf(msg, args...))
}

func (lw *loggerWrapper) Debug(msg string, args ...any) {
	lw.slog.Debug(fmt.Sprintf(msg, args...))
}

func (lw *loggerWrapper) Info(msg string, args ...any) {
	lw.slog.Info(fmt.Sprintf(msg, args...))
}

func (lw *loggerWrapper) Warn(msg string, args ...any) {
	lw.slog.Warn(fmt.Sprintf(msg, args...))
}

func (lw *loggerWrapper) Error(msg string, args ...any) {
	lw.slog.Error(fmt.Sprintf(msg, args...))
}

func (lw *loggerWrapper) UpdateProgress(current, total int, message string) {
	// Delegate to the reporter
	_ = lw.reporter.UpdateProgress(current, total, message)
}

func (lw *loggerWrapper) RecordChange(change logger.Change) {
	// No-op for SDK reporters (they don't track changes)
}

func (lw *loggerWrapper) ChangeCounters() map[string]int {
	// No-op for SDK reporters
	return make(map[string]int)
}

func (lw *loggerWrapper) IsCanceled() bool {
	// Delegate to the reporter
	return lw.reporter.IsCanceled()
}

func (lw *loggerWrapper) With(subsystem string) logger.Logger {
	// For now, just return self — the subsystem context could be added to future logs
	// This is a simplified implementation since we delegate to slog
	return lw
}
