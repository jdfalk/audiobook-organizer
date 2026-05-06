// file: internal/operations/registry/registry_test.go
// version: 1.0.0
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5a
// last-edited: 2026-05-06

package registry_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/operations/registry"
)

// makeValidDef returns a valid OperationDef for use in tests.
func makeValidDef(id string) registry.OperationDef {
	return registry.OperationDef{
		ID:              id,
		Plugin:          "test",
		DisplayName:     "Test Op",
		Description:     "For testing",
		Run:             func(_ context.Context, _ json.RawMessage, _ registry.Reporter) error { return nil },
		DefaultPriority: registry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		ResumePolicy:    registry.ResumeDrop,
		ConcurrencyKey:  "",
	}
}

func newTestRegistry(t *testing.T) (*registry.Registry, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	r := registry.New(store, slog.Default(), 4)
	return r, store
}

// --- RegisterOp tests ---

func TestRegisterOp_RejectsNilRun(t *testing.T) {
	r, _ := newTestRegistry(t)
	def := makeValidDef("test.nil-run")
	def.Run = nil
	if err := r.RegisterOp(def); err == nil {
		t.Fatal("expected error for nil Run, got nil")
	}
}

func TestRegisterOp_RejectsDuplicateID(t *testing.T) {
	r, _ := newTestRegistry(t)
	def := makeValidDef("test.dup")
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := r.RegisterOp(def); err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

func TestRegisterOp_RejectsResumeUnspecified(t *testing.T) {
	r, _ := newTestRegistry(t)
	def := makeValidDef("test.unspecified")
	def.ResumePolicy = registry.ResumeUnspecified
	if err := r.RegisterOp(def); err == nil {
		t.Fatal("expected error for ResumeUnspecified, got nil")
	}
}

func TestRegisterOp_RejectsEmptyID(t *testing.T) {
	r, _ := newTestRegistry(t)
	def := makeValidDef("")
	if err := r.RegisterOp(def); err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}
}

func TestRegisterOp_AcceptsValidDef(t *testing.T) {
	r, _ := newTestRegistry(t)
	def := makeValidDef("test.valid")
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	defs := r.ActiveDefs()
	if len(defs) != 1 || defs[0].ID != "test.valid" {
		t.Errorf("expected 1 def with id test.valid, got %v", defs)
	}
}

// --- EnqueueOp tests ---

func TestEnqueueOp_ErrorForUnknownDef(t *testing.T) {
	r, _ := newTestRegistry(t)
	_, err := r.EnqueueOp(context.Background(), "unknown.def", nil)
	if err == nil {
		t.Fatal("expected error for unknown defID, got nil")
	}
}

func TestEnqueueOp_SucceedsForRegisteredDef(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.enqueue")
	_ = r.RegisterOp(def)

	opID, err := r.EnqueueOp(context.Background(), "test.enqueue", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opID == "" {
		t.Fatal("expected non-empty opID")
	}
	if store.statusOf(opID) != "queued" {
		t.Errorf("expected status queued, got %s", store.statusOf(opID))
	}
}

func TestEnqueueOp_WithPriorityOption(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.priority")
	_ = r.RegisterOp(def)

	opID, err := r.EnqueueOp(context.Background(), "test.priority", nil,
		registry.WithPriority(registry.PriorityHigh))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	row, _ := store.GetOperationV2(opID)
	if row == nil {
		t.Fatal("op row not found")
	}
	if row.Priority != int(registry.PriorityHigh) {
		t.Errorf("expected priority %d, got %d", registry.PriorityHigh, row.Priority)
	}
}

// --- Cancel tests ---

func TestCancel_QueuedOpSetsCanceled(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.cancel-queued")
	_ = r.RegisterOp(def)

	opID, _ := r.EnqueueOp(context.Background(), "test.cancel-queued", nil)
	if err := r.Cancel(opID); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if store.statusOf(opID) != "canceled" {
		t.Errorf("expected status canceled, got %s", store.statusOf(opID))
	}
}

func TestCancel_RunningOpCancelsContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	r, _ := newTestRegistry(t)

	started := make(chan struct{})
	canceled := make(chan struct{})

	def := makeValidDef("test.cancel-running")
	def.Run = func(runCtx context.Context, _ json.RawMessage, rep registry.Reporter) error {
		close(started)
		<-runCtx.Done()
		close(canceled)
		return runCtx.Err()
	}
	_ = r.RegisterOp(def)
	r.Start(ctx)

	opID, _ := r.EnqueueOp(ctx, "test.cancel-running", nil)

	// Wait for run to start.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("op did not start within 5s")
	}

	// Cancel the running op.
	if err := r.Cancel(opID); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	// Verify context was canceled inside the run.
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("op context was not canceled within 5s")
	}
}

// --- ActiveDefs tests ---

func TestActiveDefs_ReturnsAllRegistered(t *testing.T) {
	r, _ := newTestRegistry(t)
	_ = r.RegisterOp(makeValidDef("test.a"))
	_ = r.RegisterOp(makeValidDef("test.b"))
	_ = r.RegisterOp(makeValidDef("test.c"))
	defs := r.ActiveDefs()
	if len(defs) != 3 {
		t.Errorf("expected 3 defs, got %d", len(defs))
	}
}
