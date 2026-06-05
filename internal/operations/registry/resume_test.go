// file: internal/operations/registry/resume_test.go
// version: 1.1.0
// guid: 6f7a8b9c-0d1e-2345-f012-34567890abcd
// last-edited: 2026-05-06

package registry_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/operations/registry"
	"github.com/oklog/ulid/v2"
)

// insertRunningOp pre-loads the store with a running op so resumeAfterStartup
// can find it. Returns the op ID.
func insertRunningOp(store *fakeStore, defID, plugin string, priority int) string {
	opID := ulid.Make().String()
	store.InsertOperationV2(database.OperationV2Row{ //nolint:errcheck
		ID:       opID,
		DefID:    defID,
		Plugin:   plugin,
		TraceID:  ulid.Make().String(),
		SpanID:   ulid.Make().String(),
		Status:   "running",
		Priority: priority,
		Params:   "{}",
		QueuedAt: time.Now().UTC(),
	})
	return opID
}

// TestResume_DropLeavesInterruptedDropped verifies that ResumeDrop ops found
// in status=running at startup are marked interrupted_dropped.
func TestResume_DropLeavesInterruptedDropped(t *testing.T) {
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 30 * time.Second,
	})

	def := makeValidDef("test.resume-drop")
	def.ResumePolicy = registry.ResumeDrop
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error { return nil }
	_ = r.RegisterOp(def)

	opID := insertRunningOp(store, "test.resume-drop", "test", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	// resumeAfterStartup ran synchronously in Start; check immediately.
	time.Sleep(20 * time.Millisecond)
	if store.statusOf(opID) != "interrupted_dropped" {
		t.Errorf("expected interrupted_dropped, got %s", store.statusOf(opID))
	}
}

// TestResume_AskLeavesInterruptedAsk verifies that ResumeAsk ops are set to
// interrupted_ask.
func TestResume_AskLeavesInterruptedAsk(t *testing.T) {
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 30 * time.Second,
	})

	def := makeValidDef("test.resume-ask")
	def.ResumePolicy = registry.ResumeAsk
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error { return nil }
	_ = r.RegisterOp(def)

	opID := insertRunningOp(store, "test.resume-ask", "test", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	if store.statusOf(opID) != "interrupted_ask" {
		t.Errorf("expected interrupted_ask, got %s", store.statusOf(opID))
	}
}

// TestResume_RestartReDispatchesWithIncrementedResumeCount verifies that
// ResumeRestart ops are re-dispatched exactly once and resume_count is incremented.
func TestResume_RestartReDispatchesWithIncrementedResumeCount(t *testing.T) {
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 30 * time.Second,
	})

	var runCount atomic.Int32
	ran := make(chan struct{}, 1)
	def := makeValidDef("test.resume-restart")
	def.ResumePolicy = registry.ResumeRestart
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		runCount.Add(1)
		// Signal first run only; buffer=1 so extra sends are dropped.
		select {
		case ran <- struct{}{}:
		default:
		}
		return nil
	}
	_ = r.RegisterOp(def)

	opID := insertRunningOp(store, "test.resume-restart", "test", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	// resumeAfterStartup should dispatch the op; wait for it to run.
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("resume-restart op did not run within 5s")
	}

	// Give a brief window to detect a spurious second dispatch (double-dispatch bug).
	time.Sleep(100 * time.Millisecond)

	// Run must have been called exactly once — the resumed op, not a double-dispatch.
	if got := runCount.Load(); got != 1 {
		t.Errorf("expected Run called exactly 1 time, got %d (double-dispatch?)", got)
	}

	// resume_count should be 1 (was 0, incremented to 1 in resumeRestart).
	row, err := store.GetOperationV2(opID)
	if err != nil || row == nil {
		t.Fatal("op row not found")
	}
	if row.ResumeCount != 1 {
		t.Errorf("expected resume_count=1, got %d", row.ResumeCount)
	}
}

// TestResume_RequeueFreshRun verifies that ResumeRequeue ops create a new
// queued op (progress=0) and the original is marked interrupted_dropped.
func TestResume_RequeueFreshRun(t *testing.T) {
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 30 * time.Second,
	})

	ran := make(chan struct{}, 1)
	def := makeValidDef("test.resume-requeue")
	def.ResumePolicy = registry.ResumeRequeue
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error {
		select {
		case ran <- struct{}{}:
		default:
		}
		return nil
	}
	_ = r.RegisterOp(def)

	originalID := insertRunningOp(store, "test.resume-requeue", "test", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	// Wait for the new op (fresh run) to complete.
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("requeued op did not run within 5s")
	}

	// Original op should be interrupted_dropped.
	if store.statusOf(originalID) != "interrupted_dropped" {
		t.Errorf("original op: expected interrupted_dropped, got %s", store.statusOf(originalID))
	}
}

// TestResume_ReconcileScanAlwaysDropped verifies that even if reconcile_scan
// has a registered def with ResumeRestart, it is always force-dropped.
func TestResume_ReconcileScanAlwaysDropped(t *testing.T) {
	store := newFakeStore()
	r := registry.NewWithOptions(store, slog.Default(), 2, registry.Options{
		WatchdogInterval: 30 * time.Second,
	})

	// Register a def whose ID matches the hardcoded reconcile_scan constant.
	def := makeValidDef("reconcile_scan")
	def.ResumePolicy = registry.ResumeRestart // would normally restart, but must drop
	def.Run = func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error { return nil }
	_ = r.RegisterOp(def)

	opID := insertRunningOp(store, "reconcile_scan", "scanner", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)

	time.Sleep(20 * time.Millisecond)
	if store.statusOf(opID) != "interrupted_dropped" {
		t.Errorf("reconcile_scan: expected interrupted_dropped, got %s", store.statusOf(opID))
	}
}
