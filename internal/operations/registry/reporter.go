// file: internal/operations/registry/reporter.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b
// last-edited: 2026-05-06

package registry

// Reporter is the per-run handle a plugin's Run function uses to emit
// progress, logs, and checkpoints. The full DB-write implementation
// lands in UOS-03/04. This file defines only the interface and a
// minimal stub sufficient for the worker loop.

import (
	"context"
	"log/slog"
	"sync"
)

// Reporter is the per-run API surface for an in-flight operation.
// Real DB writes are implemented in UOS-03; this stub buffers to memory.
type Reporter interface {
	UpdateProgress(current, total int, message string) error
	Log(level slog.Level, message string, attrs ...slog.Attr) error
	Logger() *slog.Logger
	Checkpoint(state any) error
	IsCanceled() bool
	RunPhase(ctx context.Context, name string, fn func(context.Context, Reporter) error) error
	Trigger(ctx context.Context, eventName string, payload any) error
}

// stubReporter is the UOS-02 stand-in. It buffers log lines in memory
// and does nothing with checkpoints. Real persistence is UOS-03/04.
type stubReporter struct {
	opID   string
	logs   []string
	mu     sync.Mutex
	logger *slog.Logger
	ctx    context.Context
}

// newStubReporter creates a stub Reporter bound to the given operation id.
func newStubReporter(ctx context.Context, opID string) Reporter {
	return &stubReporter{
		opID:   opID,
		logger: slog.Default().With("op_id", opID),
		ctx:    ctx,
	}
}

func (r *stubReporter) UpdateProgress(current, total int, message string) error {
	r.mu.Lock()
	r.logs = append(r.logs, message)
	r.mu.Unlock()
	return nil
}

func (r *stubReporter) Log(level slog.Level, message string, attrs ...slog.Attr) error {
	r.mu.Lock()
	r.logs = append(r.logs, message)
	r.mu.Unlock()
	return nil
}

func (r *stubReporter) Logger() *slog.Logger {
	return r.logger
}

func (r *stubReporter) Checkpoint(_ any) error {
	// No-op in stub; real implementation persists to op_state_v2 (UOS-03).
	return nil
}

func (r *stubReporter) IsCanceled() bool {
	select {
	case <-r.ctx.Done():
		return true
	default:
		return false
	}
}

func (r *stubReporter) RunPhase(ctx context.Context, name string, fn func(context.Context, Reporter) error) error {
	// Phase tracking (skip completed phases on resume) lands in UOS-03.
	_ = name
	return fn(ctx, r)
}

func (r *stubReporter) Trigger(_ context.Context, _ string, _ any) error {
	// Event bus wiring lands in UOS-05.
	return nil
}
