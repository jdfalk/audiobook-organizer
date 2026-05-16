// file: internal/operations/registry/dispatcher.go
// version: 2.0.0
// guid: a7b8c9d0-e1f2-3a4b-5c6d-7e8f9a0b1c2d
// last-edited: 2026-05-06

package registry

import (
	"context"
	"encoding/json"
	"time"
)

// runDispatcher is the central dispatch loop. It ticks every 100ms or
// on a signal, walks queued ops in priority DESC / queued_at ASC order,
// and dispatches eligible ones to the worker pool.
func (r *Registry) runDispatcher(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("registry: dispatcher stopping")
			return
		case <-ticker.C:
			r.dispatchCycle(ctx)
		case <-r.dispatch:
			r.dispatchCycle(ctx)
		}
	}
}

// dispatchCycle walks all queued ops and sends eligible ones to nextRun.
func (r *Registry) dispatchCycle(ctx context.Context) {
	queued, err := r.store.ListQueuedOperationsV2()
	if err != nil {
		r.logger.Warn("registry: list queued ops failed", "error", err)
		return
	}

	for _, row := range queued {
		if ctx.Err() != nil {
			return
		}

		// Gate 1: def must be registered.
		r.mu.RLock()
		def, ok := r.defs[row.DefID]
		r.mu.RUnlock()
		if !ok {
			// Unknown def — skip; may appear during rolling restarts.
			continue
		}

		r.mu.RLock()
		// Gate 2: plugin max_concurrent.
		maxC := r.pluginMax[def.Plugin]
		currentRunning := r.pluginRunning[def.Plugin]
		r.mu.RUnlock()
		if maxC > 0 && currentRunning >= maxC {
			continue
		}

		// Gate 2b: abandoned goroutine cap.
		if r.abandoned.isBlocked(def.Plugin) {
			r.logger.Warn("registry: plugin blocked due to abandoned goroutines; skipping dispatch",
				"plugin", def.Plugin, "abandoned", r.abandoned.countFor(def.Plugin))
			continue
		}

		// Gate 3: ConcurrencyKey already running?
		if def.ConcurrencyKey != "" {
			r.mu.RLock()
			holder, held := r.concurrencyKeys[def.ConcurrencyKey]
			r.mu.RUnlock()
			if held && holder != row.ID {
				continue
			}
		}

		// Gate 4: DependsOn — all listed op defs must NOT be currently running.
		if blocked := r.checkDependsOn(def.DependsOn); blocked {
			continue
		}

		// All gates passed — claim and dispatch.
		r.mu.Lock()
		// Re-check under write lock to avoid TOCTOU.
		maxC = r.pluginMax[def.Plugin]
		currentRunning = r.pluginRunning[def.Plugin]
		if maxC > 0 && currentRunning >= maxC {
			r.mu.Unlock()
			continue
		}
		if def.ConcurrencyKey != "" {
			if holder, held := r.concurrencyKeys[def.ConcurrencyKey]; held && holder != row.ID {
				r.mu.Unlock()
				continue
			}
			r.concurrencyKeys[def.ConcurrencyKey] = row.ID
		}
		r.pluginRunning[def.Plugin]++
		r.mu.Unlock()

		qr := &queuedRun{
			opID:         row.ID,
			defID:        row.DefID,
			params:       json.RawMessage(row.Params),
			priority:     Priority(row.Priority),
			concurrKey:   def.ConcurrencyKey,
			plugin:       def.Plugin,
			resumePolicy: def.ResumePolicy,
		}

		select {
		case r.nextRun <- qr:
			r.logger.Info("registry: dispatched op", "op_id", row.ID, "def_id", row.DefID)
		default:
			// Worker channel is full; undo accounting and try next cycle.
			r.mu.Lock()
			r.pluginRunning[def.Plugin]--
			if def.ConcurrencyKey != "" {
				if holder := r.concurrencyKeys[def.ConcurrencyKey]; holder == row.ID {
					delete(r.concurrencyKeys, def.ConcurrencyKey)
				}
			}
			r.mu.Unlock()
		}
	}
}

// checkDependsOn returns true if any op in depDefIDs is currently running.
func (r *Registry) checkDependsOn(depDefIDs []string) bool {
	if len(depDefIDs) == 0 {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, h := range r.running {
		for _, depID := range depDefIDs {
			if h.defID == depID {
				return true
			}
		}
	}
	return false
}
