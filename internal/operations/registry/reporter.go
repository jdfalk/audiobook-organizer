// file: internal/operations/registry/reporter.go
// version: 1.2.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b
// last-edited: 2026-05-12

package registry

// Reporter is the per-run handle a plugin's Run function uses to emit
// progress, logs, and checkpoints. The real implementation is reporterDB
// in reporter_db.go (UOS-03+). This file is interface-only — the
// transitional UOS-02 stub (stubReporter / newStubReporter) was removed
// after UOS-03 made it unused.

import (
	"context"
	"log/slog"
)

// Reporter is the per-run API surface for an in-flight operation.
type Reporter interface {
	UpdateProgress(current, total int, message string) error
	Log(level slog.Level, message string, attrs ...slog.Attr) error
	Logger() *slog.Logger
	Checkpoint(state any) error
	IsCanceled() bool
	RunPhase(ctx context.Context, name string, fn func(context.Context, Reporter) error) error
	Trigger(ctx context.Context, eventName string, payload any) error
	// SetCurrentItem sets the ephemeral "currently working on" label. It is
	// purely in-memory (no DB write) and fans out via SSE as op.current_item.
	// Pass an empty string to clear the label. Safe to call once per loop
	// iteration without measurable cost.
	SetCurrentItem(label string)
}
