// file: internal/logger/operation_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package logger

import (
	"sync"
	"testing"
)

type mockOpStore struct {
	logs     []string
	changes  []interface{}
	progress []string
	mu       sync.Mutex
}

func (m *mockOpStore) AddOperationLog(opID, level, message string, details *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, level+":"+message)
	return nil
}

func (m *mockOpStore) CreateOperationChange(change interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, change)
	return nil
}

func (m *mockOpStore) UpdateOperationProgress(id string, current, total int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = append(m.progress, message)
	return nil
}

type mockHub struct {
	logsSent     int
	progressSent int
	mu           sync.Mutex
}

func (m *mockHub) SendOperationLog(opID, level, message string, details *string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logsSent++
}

func (m *mockHub) SendOperationProgress(opID string, current, total int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progressSent++
}

func TestOperationLogger_LevelFiltering(t *testing.T) {
	store := &mockOpStore{}
	hub := &mockHub{}
	log := ForOperation("op1", store, hub)

	log.Debug("debug msg") // below default minDBLevel (INFO)
	log.Info("info msg")   // at minDBLevel
	log.Warn("warn msg")   // above minDBLevel

	if len(store.logs) != 2 {
		t.Errorf("expected 2 DB logs (info+warn), got %d: %v", len(store.logs), store.logs)
	}
	if hub.logsSent != 2 {
		t.Errorf("expected 2 hub sends, got %d", hub.logsSent)
	}
}

func TestOperationLogger_DebugWhenVerbose(t *testing.T) {
	store := &mockOpStore{}
	log := ForOperation("op1", store, nil)
	log.SetMinDBLevel(LevelDebug)

	log.Debug("debug msg")
	log.Trace("trace msg") // still below DEBUG

	if len(store.logs) != 1 {
		t.Errorf("expected 1 DB log (debug), got %d", len(store.logs))
	}
}

func TestOperationLogger_RecordChange(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	log.RecordChange(Change{ChangeType: "book_create", Summary: "Created book A"})
	log.RecordChange(Change{ChangeType: "book_create", Summary: "Created book B"})
	log.RecordChange(Change{ChangeType: "book_update", Summary: "Updated book C"})

	counters := log.ChangeCounters()
	if counters["book_create"] != 2 {
		t.Errorf("book_create = %d, want 2", counters["book_create"])
	}
	if counters["book_update"] != 1 {
		t.Errorf("book_update = %d, want 1", counters["book_update"])
	}
}

func TestOperationLogger_IsCanceled(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	if log.IsCanceled() {
		t.Error("should not be canceled initially")
	}
	log.SetCanceled()
	if !log.IsCanceled() {
		t.Error("should be canceled after SetCanceled()")
	}
}

func TestOperationLogger_With(t *testing.T) {
	log := ForOperation("op1", nil, nil)
	child := log.With("scanner")

	// Child should share canceled state
	log.SetCanceled()
	if !child.IsCanceled() {
		t.Error("child should see parent's canceled state")
	}
}

func TestOperationLogger_Progress(t *testing.T) {
	store := &mockOpStore{}
	hub := &mockHub{}
	log := ForOperation("op1", store, hub)

	log.UpdateProgress(5, 100, "scanning")

	if len(store.progress) != 1 {
		t.Errorf("expected 1 progress update, got %d", len(store.progress))
	}
	if hub.progressSent != 1 {
		t.Errorf("expected 1 hub progress, got %d", hub.progressSent)
	}
}

func TestOperationLogger_BackwardCompatLog(t *testing.T) {
	store := &mockOpStore{}
	log := ForOperation("op1", store, nil)

	_ = log.Log("info", "backward compat message", nil)

	if len(store.logs) != 1 {
		t.Errorf("expected 1 DB log, got %d", len(store.logs))
	}
}
