// file: internal/operations/registry/worker.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e
// last-edited: 2026-05-06

package registry

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"time"
)

// ErrSubprocessNotImplemented is returned when a Run with Isolate=true is
// dispatched. Subprocess execution lands in UOS-03.
var ErrSubprocessNotImplemented = errors.New("subprocess runner not yet wired (UOS-03)")

// runHandle tracks a single in-flight operation.
type runHandle struct {
	id             string
	defID          string
	plugin         string
	concurrencyKey string
	resumePolicy   ResumePolicy
	cancel         context.CancelFunc
	abandoned      bool
}

// queuedRun is the payload the dispatcher sends to a worker goroutine.
type queuedRun struct {
	opID         string
	defID        string
	params       json.RawMessage
	priority     Priority
	concurrKey   string
	plugin       string
	resumePolicy ResumePolicy
}

// startWorker is a long-running goroutine that reads from r.nextRun and
// executes each run in sequence.
func (r *Registry) startWorker(ctx context.Context, slot int) {
	r.logger.Info("registry: worker started", "slot", slot)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("registry: worker stopping", "slot", slot)
			return
		case qr := <-r.nextRun:
			r.executeRun(ctx, qr)
		}
	}
}

// executeRun runs a single queued operation.
func (r *Registry) executeRun(parentCtx context.Context, qr *queuedRun) {
	r.mu.RLock()
	def, ok := r.defs[qr.defID]
	r.mu.RUnlock()
	if !ok {
		r.logger.Warn("registry: worker got run for unknown def; skipping", "def_id", qr.defID)
		return
	}

	// Build per-run context with cancel and optional timeout.
	timeout := def.Timeout
	if timeout == 0 {
		if def.Isolate {
			timeout = 6 * time.Hour
		} else {
			timeout = 120 * time.Minute
		}
	}
	runCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	// Register the handle.
	h := &runHandle{
		id:             qr.opID,
		defID:          qr.defID,
		plugin:         qr.plugin,
		concurrencyKey: qr.concurrKey,
		resumePolicy:   qr.resumePolicy,
		cancel:         cancel,
	}
	r.mu.Lock()
	r.running[qr.opID] = h
	r.mu.Unlock()
	defer r.releaseRunHandle(qr.opID)

	// Mark running in DB.
	now := time.Now().UTC()
	if err := r.store.UpdateOperationV2Status(qr.opID, "running", &now, nil, nil); err != nil {
		r.logger.Warn("registry: failed to mark op running", "op_id", qr.opID, "error", err)
	}

	r.logger.Info("registry: starting run", "op_id", qr.opID, "def_id", qr.defID)

	// Subprocess path: not implemented in UOS-02.
	if def.Isolate {
		errMsg := ErrSubprocessNotImplemented.Error()
		completed := time.Now().UTC()
		_ = r.store.UpdateOperationV2Status(qr.opID, "failed", nil, &completed, &errMsg)
		r.logger.Warn("registry: isolate=true not yet supported", "op_id", qr.opID)
		return
	}

	// In-process path: call Run with panic recovery.
	reporter := newStubReporter(runCtx, qr.opID)
	runErr := r.safeRun(runCtx, def, qr.params, reporter)

	// Determine terminal status.
	var finalStatus string
	var errMsg *string
	completedAt := time.Now().UTC()

	switch {
	case runCtx.Err() == context.Canceled || runCtx.Err() == context.DeadlineExceeded:
		finalStatus = "canceled"
	case runErr != nil:
		finalStatus = "failed"
		msg := runErr.Error()
		errMsg = &msg
	default:
		finalStatus = "completed"
	}

	if err := r.store.UpdateOperationV2Status(qr.opID, finalStatus, nil, &completedAt, errMsg); err != nil {
		r.logger.Warn("registry: failed to update op terminal status", "op_id", qr.opID, "error", err)
	}

	r.logger.Info("registry: run finished", "op_id", qr.opID, "status", finalStatus)
}

// safeRun calls def.Run with panic recovery, returning any panic as an error.
func (r *Registry) safeRun(ctx context.Context, def OperationDef, params json.RawMessage, rep Reporter) (runErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			runErr = fmt.Errorf("operation panicked: %v", rec)
			r.logger.Error("registry: op panicked", "def_id", def.ID, "panic", rec)
		}
	}()
	return def.Run(ctx, params, rep)
}
