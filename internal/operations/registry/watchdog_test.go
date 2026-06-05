// file: internal/operations/registry/watchdog_test.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0123-def0-1234567890ab
// last-edited: 2026-05-06

package registry_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/operations/registry"
)

// TestWatchdog_StuckOpGetStrike verifies that an op with stale last_progress_at
// gets a "stuck" strike and its context is canceled.
func TestWatchdog_StuckOpGetsStrike(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use 50ms watchdog interval for fast test.
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 50 * time.Millisecond,
	})

	canceled := make(chan struct{})
	def := makeValidDef("test.wdog-stuck")
	def.ResumePolicy = registry.ResumeDrop
	def.ProgressTimeout = 100 * time.Millisecond
	def.Run = func(runCtx context.Context, _ json.RawMessage, _ registry.Reporter) error {
		<-runCtx.Done()
		close(canceled)
		return runCtx.Err()
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	opID, _ := r.EnqueueOp(ctx, "test.wdog-stuck", nil)

	// Wait for the op to start running.
	awaitStatus(t, store, opID, "running", 3*time.Second)

	// Backdate last_progress_at to be stale (older than ProgressTimeout).
	stale := time.Now().UTC().Add(-200 * time.Millisecond)
	store.setLastProgressAt(opID, &stale)

	// Wait for watchdog to fire and cancel the op.
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("stuck op was not canceled within 3s")
	}

	// Give the worker time to write terminal status.
	awaitStatus(t, store, opID, "canceled", 3*time.Second)

	// Verify a "stuck" strike was written.
	if n := len(store.strikesOfKind(opID, "stuck")); n == 0 {
		t.Error("expected at least 1 stuck strike, got 0")
	}
}

// TestWatchdog_UncheckpointedOpGetsStrike verifies that a ResumeRestart op
// that hasn't checkpointed in ≥5 minutes accumulates an uncheckpointed strike.
func TestWatchdog_UncheckpointedOpGetsStrike(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 50 * time.Millisecond,
	})

	started := make(chan struct{})
	def := makeValidDef("test.wdog-uncheckpointed")
	def.ResumePolicy = registry.ResumeRestart
	def.MinCheckpointInterval = 100 * time.Millisecond
	def.Run = func(runCtx context.Context, _ json.RawMessage, _ registry.Reporter) error {
		close(started)
		<-runCtx.Done()
		return runCtx.Err()
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	opID, _ := r.EnqueueOp(ctx, "test.wdog-uncheckpointed", nil)

	<-started

	// Backdate started_at to trigger the uncheckpointed window (≥5 min in real
	// code; the watchdog uses defaultMinCheckpointTimeout which we override via
	// a very stale started_at).
	stale := time.Now().UTC().Add(-6 * time.Minute)
	store.setStartedAt(opID, &stale)
	// Also set the status to "running" explicitly (may already be, but ensure).
	_ = store.UpdateOperationV2Status(opID, "running", nil, nil, nil)

	// Wait up to 500ms for the watchdog to fire and write a strike.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if n := len(store.strikesOfKind(opID, "uncheckpointed")); n > 0 {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("expected at least 1 uncheckpointed strike within 500ms, got 0")
}

// TestWatchdog_InfiniteRestartForceDrop verifies that an op with resume_count≥3
// and no high_water_progress advancement is force-dropped.
func TestWatchdog_InfiniteRestartForceDrop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 50 * time.Millisecond,
	})

	def := makeValidDef("test.wdog-infinite-restart")
	def.ResumePolicy = registry.ResumeRestart
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		return nil
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	// Enqueue an op and pre-set resume_count=3 (no high_water_progress).
	opID, _ := r.EnqueueOp(ctx, "test.wdog-infinite-restart", nil)
	// Wait for it to be picked up (queued), then set resume_count before dispatch.
	// We need to set it before the worker calls checkInfiniteRestart.
	// Insert directly with resume_count=3.
	store.setResumeCount(opID, 3)

	// Wait for the op to be force-dropped (executeRun calls checkInfiniteRestart).
	awaitStatus(t, store, opID, "interrupted_dropped", 5*time.Second)

	// Verify infinite_restart strike was written.
	if n := len(store.strikesOfKind(opID, "infinite_restart")); n == 0 {
		t.Error("expected at least 1 infinite_restart strike, got 0")
	}
}
