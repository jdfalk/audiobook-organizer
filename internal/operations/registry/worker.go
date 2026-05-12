// file: internal/operations/registry/worker.go
// version: 2.2.0
// guid: b8c9d0e1-f2a3-4b5c-6d7e-8f9a0b1c2d3e
// last-edited: 2026-05-08

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrSubprocessNotImplemented is returned when a Run with Isolate=true is
// dispatched. Subprocess execution lands in UOS-03.
var ErrSubprocessNotImplemented = errors.New("subprocess runner not yet wired (UOS-03)")

// abandonGrace is the time a ctx-canceled goroutine has to return before
// it is classified as abandoned and the worker slot is freed.
const abandonGrace = 5 * time.Second

// runHandle tracks a single in-flight operation.
type runHandle struct {
	id             string
	defID          string
	plugin         string
	concurrencyKey string
	resumePolicy   ResumePolicy
	cancel         context.CancelFunc
	abandoned      bool
	currentItem    string
	currentItemMu  sync.Mutex
}

func (h *runHandle) setCurrentItem(label string) {
	h.currentItemMu.Lock()
	h.currentItem = label
	h.currentItemMu.Unlock()
}

func (h *runHandle) getCurrentItem() string {
	h.currentItemMu.Lock()
	defer h.currentItemMu.Unlock()
	return h.currentItem
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
// executes each run in sequence. When the worker is notified its run was
// abandoned (ctx-canceled goroutine didn't return within abandonGrace), it
// exits so the replacement worker (spawned by executeRun) becomes the sole
// occupant of that conceptual slot.
func (r *Registry) startWorker(ctx context.Context, slot int) {
	r.logger.Info("registry: worker started", "slot", slot)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("registry: worker stopping", "slot", slot)
			return
		case qr := <-r.nextRun:
			abandoned := r.executeRun(ctx, qr)
			if abandoned {
				// The run goroutine is still alive but classified as abandoned.
				// A replacement worker was already spawned by executeRun.
				// This worker exits so we don't grow the pool.
				r.logger.Info("registry: worker exiting after abandoning run", "slot", slot, "op_id", qr.opID)
				return
			}
		}
	}
}

// executeRun runs a single queued operation. It returns true if the run was
// classified as abandoned (ctx-canceled but goroutine didn't drain within
// abandonGrace), in which case the caller (startWorker) should exit so the
// replacement worker owns that slot.
func (r *Registry) executeRun(parentCtx context.Context, qr *queuedRun) (wasAbandoned bool) {
	r.mu.RLock()
	def, ok := r.defs[qr.defID]
	r.mu.RUnlock()
	if !ok {
		r.logger.Warn("registry: worker got run for unknown def; skipping", "def_id", qr.defID)
		return false
	}

	// Check infinite-restart strike: if resume_count >= 3 and high_water hasn't
	// advanced, force ResumeDrop behavior.
	if qr.resumePolicy == ResumeRestart {
		if forced := r.checkInfiniteRestart(qr, def); forced {
			return false
		}
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

	// Mark running in DB.
	now := time.Now().UTC()
	if err := r.store.UpdateOperationV2Status(qr.opID, "running", &now, nil, nil); err != nil {
		r.logger.Warn("registry: failed to mark op running", "op_id", qr.opID, "error", err)
	}

	r.logger.Info("registry: starting run", "op_id", qr.opID, "def_id", qr.defID)

	// Build reporter (DB-backed). Pass a setter so SetCurrentItem updates
	// the runHandle's in-memory currentItem without a DB write.
	setItemFn := func(label string) { h.setCurrentItem(label) }
	reporter := newDBReporter(runCtx, qr.opID, qr.defID, qr.plugin,
		"", "", // traceID / spanID loaded from DB row in future; empty for now
		r.store, r.bus, r.logger, setItemFn)

	// Subprocess path (Isolate=true): re-exec self.
	if def.Isolate {
		runErr := runSubprocess(runCtx, def, qr.opID, qr.params, reporter)
		r.releaseRunHandle(qr.opID)
		var finalStatus string
		var errMsg *string
		completedAt := time.Now().UTC()
		if runCtx.Err() != nil {
			finalStatus = "canceled"
		} else if runErr != nil {
			finalStatus = "failed"
			msg := runErr.Error()
			errMsg = &msg
		} else {
			finalStatus = "completed"
		}
		if err := r.store.UpdateOperationV2Status(qr.opID, finalStatus, nil, &completedAt, errMsg); err != nil {
			r.logger.Warn("registry: failed to update subprocess op terminal status", "op_id", qr.opID, "error", err)
		}
		r.logger.Info("registry: subprocess run finished", "op_id", qr.opID, "status", finalStatus)
		return false
	}

	// In-process path: run in a separate goroutine so we can detect abandonment.
	done := make(chan error, 1)
	go func() {
		done <- r.safeRun(runCtx, def, qr.params, reporter)
	}()

	// Wait for the run to finish or the context to be canceled.
	var runErr error
	var ctxCanceled bool

	select {
	case runErr = <-done:
		// Normal completion or run-level error; check if ctx was already done.
		if runCtx.Err() != nil {
			ctxCanceled = true
		}
	case <-runCtx.Done():
		ctxCanceled = true
		// Give the goroutine abandonGrace to return cleanly.
		select {
		case runErr = <-done:
			// Goroutine returned within grace — not abandoned, but ctx was canceled.
		case <-time.After(abandonGrace):
			// Goroutine is stuck. Classify as abandoned.
			r.releaseRunHandle(qr.opID)
			r.abandoned.increment(qr.plugin)
			r.logger.Warn("registry: op goroutine abandoned; spawning replacement worker",
				"op_id", qr.opID, "plugin", qr.plugin)
			// Spawn a replacement so the pool doesn't shrink.
			go r.startWorker(parentCtx, -1)
			// Monitor the goroutine; when it returns, decrement abandoned count.
			go func() {
				<-done
				r.abandoned.decrement(qr.plugin)
				r.logger.Info("registry: abandoned goroutine returned",
					"op_id", qr.opID, "plugin", qr.plugin)
			}()
			return true // signal caller to exit
		}
	}

	// We have a result. Release the handle and write terminal status.
	r.releaseRunHandle(qr.opID)

	var finalStatus string
	var errMsg *string
	completedAt := time.Now().UTC()

	switch {
	case ctxCanceled:
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
	return false
}

// checkInfiniteRestart checks whether an op should be force-dropped due to
// repeated restarts without progress. Returns true if the op was force-dropped
// (terminal status written, handle released).
func (r *Registry) checkInfiniteRestart(qr *queuedRun, def OperationDef) bool {
	row, err := r.store.GetOperationV2(qr.opID)
	if err != nil || row == nil {
		return false
	}
	if row.ResumeCount < 3 {
		return false
	}
	// Check if high_water_progress advanced. We use 0 as the baseline when
	// no prior state is tracked; if it's still 0 after 3 restarts, force drop.
	// (The reporter writes high_water_progress in UOS-03; for now we use
	// whatever value is in the DB.)
	if row.HighWaterProgress > 0 {
		return false
	}

	// Write infinite_restart strike.
	r.writeStrike(qr.opID, def.ID, def.Plugin, "infinite_restart",
		fmt.Sprintf("resume_count=%d high_water_progress=%d; forcing drop", row.ResumeCount, row.HighWaterProgress))

	// Mark interrupted_dropped.
	completed := time.Now().UTC()
	msg := "force-dropped: infinite restart without progress"
	_ = r.store.UpdateOperationV2Status(qr.opID, "interrupted_dropped", nil, &completed, &msg)
	r.logger.Warn("registry: force-dropping op due to infinite restart",
		"op_id", qr.opID, "def_id", qr.defID, "resume_count", row.ResumeCount)
	return true
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
