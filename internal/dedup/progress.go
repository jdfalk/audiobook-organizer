// file: internal/dedup/progress.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

// Package dedup: ProgressReporter is a minimal reporting interface used by the
// extracted dedup operation functions. The server's registryProgressAdapter
// satisfies this interface automatically.
package dedup

// ProgressReporter is implemented by anything that can relay progress updates,
// log messages, and cancellation signals to the caller.
type ProgressReporter interface {
	// UpdateProgress reports current position within a [0, total] range.
	UpdateProgress(current, total int, message string) error
	// Log emits a log message at the given level ("info", "warn", "error", "debug").
	// details is an optional extra string (may be nil).
	Log(level, message string, details *string) error
	// IsCanceled reports whether the caller has requested cancellation.
	IsCanceled() bool
}
