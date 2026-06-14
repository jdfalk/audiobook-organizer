// file: internal/operations/registry/registry_test.go
// version: 1.2.0
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5a
// last-edited: 2026-06-13

package registry_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/operations/registry"
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
	r := registry.New(store, slog.Default(), 4, nil)
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

// --- op.created event tests ---

// recordingBus captures every Publish call so the test can assert on event
// names. Implements registry.Bus.
type recordingBus struct {
	events []recordedEvent
}

type recordedEvent struct {
	name    string
	payload any
}

func (b *recordingBus) Publish(_ context.Context, name string, payload any) error {
	b.events = append(b.events, recordedEvent{name: name, payload: payload})
	return nil
}

func TestEnqueueOp_PublishesOpCreated(t *testing.T) {
	store := newFakeStore()
	bus := &recordingBus{}
	r := registry.New(store, slog.Default(), 4, nil)
	r.SetBus(bus)
	_ = r.RegisterOp(makeValidDef("test.opcreated"))

	opID, err := r.EnqueueOp(context.Background(), "test.opcreated", nil)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var found bool
	for _, ev := range bus.events {
		if ev.name != "op.created" {
			continue
		}
		p, ok := ev.payload.(map[string]any)
		if !ok {
			t.Fatalf("op.created payload not a map: %T", ev.payload)
		}
		if p["op_id"] != opID {
			t.Errorf("op.created op_id: got %v want %v", p["op_id"], opID)
		}
		if p["resumed"] != false {
			t.Errorf("op.created resumed: got %v want false for fresh enqueue", p["resumed"])
		}
		found = true
	}
	if !found {
		t.Fatalf("expected an op.created event, got events: %+v", bus.events)
	}
}

// --- Task 4: enqueue parking tests ---

// TestEnqueueOp_ParksWhenRequirementUnmet asserts that an op whose def has
// Requires is parked as "waiting_deps" when the requirement is not satisfied.
// The fakeStore's GetDepRev/GetOpCompletion stubs return 0/false, so any
// op_completed requirement is always unmet.
func TestEnqueueOp_ParksWhenRequirementUnmet(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.park-unmet")
	def.Requires = []registry.Requirement{
		{Kind: registry.ReqOpCompleted, OpType: "test.prereq"},
	}
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("RegisterOp: %v", err)
	}

	params := map[string]string{"book_id": "b1"}
	opID, err := r.EnqueueOp(context.Background(), "test.park-unmet", params)
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}

	row, err := store.GetOperationV2(opID)
	if err != nil || row == nil {
		t.Fatalf("GetOperationV2: %v", err)
	}
	if row.Status != "waiting_deps" {
		t.Errorf("expected status=waiting_deps, got %q", row.Status)
	}
	if row.SubjectType != "book" {
		t.Errorf("expected SubjectType=book, got %q", row.SubjectType)
	}
	if row.SubjectID != "b1" {
		t.Errorf("expected SubjectID=b1, got %q", row.SubjectID)
	}
	if row.Requirements == "" {
		t.Error("expected Requirements JSON to be persisted")
	}
}

// TestEnqueueOp_NoRequirements_StillQueued asserts that an op with no
// requirements goes straight to "queued" (additive guarantee: no new branches).
func TestEnqueueOp_NoRequirements_StillQueued(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.no-reqs")
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("RegisterOp: %v", err)
	}

	opID, err := r.EnqueueOp(context.Background(), "test.no-reqs", nil)
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}
	if store.statusOf(opID) != "queued" {
		t.Errorf("expected status=queued for op without requirements, got %q", store.statusOf(opID))
	}
}

// TestEnqueueOp_CycleInDef_RejectsRegistration asserts that RegisterOp
// rejects an OperationDef that would form a requirement cycle once a
// second def creating the cycle is registered.
func TestEnqueueOp_CycleInDef_RejectsRegistration(t *testing.T) {
	r, _ := newTestRegistry(t)

	defA := makeValidDef("test.cycle-a")
	defA.Requires = []registry.Requirement{{Kind: registry.ReqOpCompleted, OpType: "test.cycle-b"}}
	defB := makeValidDef("test.cycle-b")
	defB.Requires = []registry.Requirement{{Kind: registry.ReqOpCompleted, OpType: "test.cycle-a"}}

	// First registration is fine (no cycle yet with only one node).
	if err := r.RegisterOp(defA); err != nil {
		t.Fatalf("RegisterOp(defA): %v", err)
	}
	// Second registration closes the cycle → must be rejected.
	if err := r.RegisterOp(defB); err == nil {
		t.Fatal("expected RegisterOp to reject a def that creates a requirement cycle")
	}
}

// TestEnqueueOp_WithRequiresOption_Parks verifies that per-enqueue requirements
// added via WithRequires also cause parking when unmet.
func TestEnqueueOp_WithRequiresOption_Parks(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.park-via-option")
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("RegisterOp: %v", err)
	}

	params := map[string]string{"book_id": "b2"}
	opID, err := r.EnqueueOp(context.Background(), "test.park-via-option", params,
		registry.WithRequires(registry.Requirement{Kind: registry.ReqOpCompleted, OpType: "test.other"}))
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}
	if store.statusOf(opID) != "waiting_deps" {
		t.Errorf("expected status=waiting_deps, got %q", store.statusOf(opID))
	}
}

// TestEnqueueOp_NoBookID_EmptySubject verifies that an op with requirements
// but no book_id in params still enqueues (parks) with an empty subject —
// the system degrades gracefully rather than returning an error.
func TestEnqueueOp_NoBookID_EmptySubject(t *testing.T) {
	r, store := newTestRegistry(t)
	def := makeValidDef("test.park-no-bookid")
	def.Requires = []registry.Requirement{
		{Kind: registry.ReqOpCompleted, OpType: "test.prereq"},
	}
	if err := r.RegisterOp(def); err != nil {
		t.Fatalf("RegisterOp: %v", err)
	}

	// No book_id in params.
	opID, err := r.EnqueueOp(context.Background(), "test.park-no-bookid", map[string]string{})
	if err != nil {
		t.Fatalf("EnqueueOp: %v", err)
	}
	row, _ := store.GetOperationV2(opID)
	if row == nil {
		t.Fatal("op row not found")
	}
	// With no subject, requirements cannot be evaluated → park conservatively.
	if row.Status != "waiting_deps" {
		t.Errorf("expected status=waiting_deps for op with requirements but no subject, got %q", row.Status)
	}
}
