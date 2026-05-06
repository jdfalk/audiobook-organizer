// file: internal/operations/registry/teststore_test.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5c6d-7e8f-9a0b1c2d3e4f
// last-edited: 2026-05-06

package registry_test

// fakeStore is a minimal in-memory implementation of database.OpsV2Store,
// which is all the registry depends on. No SQLite required.

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// fakeStore implements database.OpsV2Store in memory.
type fakeStore struct {
	mu   sync.Mutex
	defs map[string]database.OpDefinitionV2Row
	ops  map[string]database.OperationV2Row
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		defs: make(map[string]database.OpDefinitionV2Row),
		ops:  make(map[string]database.OperationV2Row),
	}
}

// Compile-time assertion: fakeStore must implement OpsV2Store.
var _ database.OpsV2Store = (*fakeStore)(nil)

func (f *fakeStore) UpsertOpDefinitionV2(row database.OpDefinitionV2Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defs[row.ID] = row
	return nil
}

func (f *fakeStore) DeleteOrphanOpDefsV2(keepIDs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	keep := make(map[string]bool, len(keepIDs))
	for _, id := range keepIDs {
		keep[id] = true
	}
	for id := range f.defs {
		if !keep[id] {
			delete(f.defs, id)
		}
	}
	return nil
}

func (f *fakeStore) InsertOperationV2(row database.OperationV2Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.ops[row.ID]; exists {
		return fmt.Errorf("duplicate op id %s", row.ID)
	}
	f.ops[row.ID] = row
	return nil
}

func (f *fakeStore) ListQueuedOperationsV2() ([]database.OperationV2Row, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []database.OperationV2Row
	for _, op := range f.ops {
		if op.Status == "queued" {
			result = append(result, op)
		}
	}
	// Sort: priority DESC, queued_at ASC
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority > result[j].Priority
		}
		return result[i].QueuedAt.Before(result[j].QueuedAt)
	})
	return result, nil
}

func (f *fakeStore) GetOperationV2(id string) (*database.OperationV2Row, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok {
		return nil, fmt.Errorf("op %s not found", id)
	}
	return &op, nil
}

func (f *fakeStore) UpdateOperationV2Status(id, status string, startedAt, completedAt *time.Time, errMsg *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok {
		return nil // best-effort
	}
	op.Status = status
	if startedAt != nil {
		op.StartedAt = startedAt
	}
	if completedAt != nil {
		op.CompletedAt = completedAt
	}
	if errMsg != nil {
		op.ErrorMessage = errMsg
	}
	f.ops[id] = op
	return nil
}

func (f *fakeStore) SetOperationV2StatusIfQueued(id, newStatus string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	op, ok := f.ops[id]
	if !ok {
		return false, nil
	}
	if op.Status != "queued" {
		return false, nil
	}
	op.Status = newStatus
	f.ops[id] = op
	return true, nil
}

func (f *fakeStore) CountRunningByPluginV2(plugin string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, op := range f.ops {
		if op.Plugin == plugin && op.Status == "running" {
			n++
		}
	}
	return n, nil
}

// statusOf is a helper for tests to read an op's current status.
func (f *fakeStore) statusOf(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if op, ok := f.ops[id]; ok {
		return op.Status
	}
	return ""
}

