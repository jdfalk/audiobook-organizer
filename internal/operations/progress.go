// file: internal/operations/progress.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-012345678901
// last-edited: 2026-05-11

// Package operations provides shared types for async operation execution.
// This file holds ProgressReporter, OperationFunc, and LoggerFromReporter —
// extracted from the deleted queue.go during BridgeQueue elimination.

package operations

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/logger"
)

// OperationFunc is the signature for all async operation implementations.
type OperationFunc func(ctx context.Context, progress ProgressReporter) error

// ProgressReporter allows operations to report progress and check cancellation.
type ProgressReporter interface {
	UpdateProgress(current, total int, message string) error
	Log(level, message string, details *string) error
	IsCanceled() bool
}

// LoggerFromReporter returns a logger.Logger from a ProgressReporter.
// The v2 registry wraps reporters differently; this always returns a new
// standard logger now that the v1 queue's loggerProgressReporter is gone.
func LoggerFromReporter(_ ProgressReporter) logger.Logger {
	return logger.New("operation")
}
