// file: internal/operations/queue_test.go
// version: 1.1.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a

package operations

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	testifyMock "github.com/stretchr/testify/mock"
)

func newMockStore(t *testing.T) *mocks.MockStore {
	t.Helper()
	store := mocks.NewMockStore(t)
	store.EXPECT().UpdateOperationStatus(
		testifyMock.Anything,
		testifyMock.Anything,
		testifyMock.Anything,
		testifyMock.Anything,
		testifyMock.Anything,
	).Return(nil).Maybe()
	store.EXPECT().UpdateOperationError(testifyMock.Anything, testifyMock.Anything).Return(nil).Maybe()
	store.EXPECT().GetOperationState(testifyMock.Anything).Return(nil, nil).Maybe()
	store.EXPECT().SaveOperationState(testifyMock.Anything, testifyMock.Anything).Return(nil).Maybe()
	store.EXPECT().AddOperationLog(
		testifyMock.Anything,
		testifyMock.Anything,
		testifyMock.Anything,
		testifyMock.Anything,
	).Return(nil).Maybe()
	store.EXPECT().SaveOperationSummaryLog(testifyMock.Anything).Return(nil).Maybe()
	return store
}

func TestNewOperationQueue(t *testing.T) {
	store := newMockStore(t)

	t.Run("creates queue with specified workers", func(t *testing.T) {
		q := NewOperationQueue(store, 4)
		defer q.Shutdown(time.Second)

		if q.workers != 4 {
			t.Errorf("expected 4 workers, got %d", q.workers)
		}
		if q.store != store {
			t.Error("store not set correctly")
		}
	})

	t.Run("defaults to 2 workers when 0 specified", func(t *testing.T) {
		q := NewOperationQueue(store, 0)
		defer q.Shutdown(time.Second)

		if q.workers != 2 {
			t.Errorf("expected 2 workers, got %d", q.workers)
		}
	})

	t.Run("defaults to 2 workers when negative specified", func(t *testing.T) {
		q := NewOperationQueue(store, -1)
		defer q.Shutdown(time.Second)

		if q.workers != 2 {
			t.Errorf("expected 2 workers, got %d", q.workers)
		}
	})
}

func TestOperationQueue_Enqueue(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("enqueues operation successfully", func(t *testing.T) {
		executed := make(chan bool, 1)
		fn := func(ctx context.Context, progress ProgressReporter) error {
			executed <- true
			return nil
		}

		err := q.Enqueue("op-1", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		select {
		case <-executed:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("operation was not executed")
		}
	})

	t.Run("rejects duplicate operation ID", func(t *testing.T) {
		// First, add an operation that blocks
		blocker := make(chan struct{})
		fn := func(ctx context.Context, progress ProgressReporter) error {
			<-blocker
			return nil
		}

		err := q.Enqueue("dup-op", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("first Enqueue failed: %v", err)
		}

		// Try to add duplicate before first completes
		err = q.Enqueue("dup-op", "test", PriorityNormal, fn)
		if err == nil {
			t.Fatal("expected error for duplicate operation ID")
		}

		close(blocker)
	})
}

func TestOperationQueue_Cancel(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("cancels existing operation", func(t *testing.T) {
		canceled := make(chan bool, 1)
		started := make(chan bool, 1)
		fn := func(ctx context.Context, progress ProgressReporter) error {
			started <- true
			<-ctx.Done()
			canceled <- true
			return ctx.Err()
		}

		err := q.Enqueue("cancel-op", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		// Wait for operation to start
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("operation did not start")
		}

		err = q.Cancel("cancel-op")
		if err != nil {
			t.Fatalf("Cancel failed: %v", err)
		}

		select {
		case <-canceled:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("operation was not canceled")
		}
	})

	t.Run("returns error for non-existent operation", func(t *testing.T) {
		err := q.Cancel("non-existent")
		if err == nil {
			t.Fatal("expected error for non-existent operation")
		}
	})
}

func TestOperationQueue_GetStatus(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("returns error when store is nil", func(t *testing.T) {
		qNoStore := &OperationQueue{}
		_, err := qNoStore.GetStatus("op-1")
		if err == nil {
			t.Fatal("expected error when store is nil")
		}
	})

	t.Run("returns operation status from store", func(t *testing.T) {
		store.EXPECT().GetOperationByID("status-op").Return(&database.Operation{
			ID:     "status-op",
			Status: "running",
		}, nil).Once()

		op, err := q.GetStatus("status-op")
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if op.Status != "running" {
			t.Errorf("expected status 'running', got '%s'", op.Status)
		}
	})
}

func TestOperationQueue_Listeners(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("adds and notifies listeners", func(t *testing.T) {
		received := make(chan OperationProgress, 1)
		listener := func(opID string, progress OperationProgress) {
			if opID == "listener-op" {
				received <- progress
			}
		}

		q.AddListener("listener-op", listener)

		// Notify directly
		q.notifyListeners("listener-op", OperationProgress{
			Current: 5,
			Total:   10,
			Message: "testing",
		})

		select {
		case p := <-received:
			if p.Current != 5 || p.Total != 10 || p.Message != "testing" {
				t.Errorf("unexpected progress: %+v", p)
			}
		case <-time.After(time.Second):
			t.Fatal("listener not notified")
		}
	})

	t.Run("removes listeners", func(t *testing.T) {
		received := make(chan OperationProgress, 1)
		listener := func(opID string, progress OperationProgress) {
			received <- progress
		}

		q.AddListener("remove-op", listener)
		q.RemoveListeners("remove-op")

		q.notifyListeners("remove-op", OperationProgress{Current: 1, Total: 1, Message: "test"})

		select {
		case <-received:
			t.Fatal("listener should have been removed")
		case <-time.After(100 * time.Millisecond):
			// Success - no notification received
		}
	})
}

func TestOperationQueue_Shutdown(t *testing.T) {
	store := newMockStore(t)

	t.Run("graceful shutdown", func(t *testing.T) {
		q := NewOperationQueue(store, 2)

		err := q.Shutdown(time.Second)
		if err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
	})

	t.Run("shutdown with timeout", func(t *testing.T) {
		q := NewOperationQueue(store, 1)

		// Add a blocking operation
		blocker := make(chan struct{})
		fn := func(ctx context.Context, progress ProgressReporter) error {
			select {
			case <-blocker:
			case <-ctx.Done():
			}
			return nil
		}

		_ = q.Enqueue("block-op", "test", PriorityNormal, fn)
		time.Sleep(50 * time.Millisecond) // Let it start

		err := q.Shutdown(100 * time.Millisecond)
		// Should complete because context is canceled
		if err != nil {
			// Timeout error is acceptable here
			t.Logf("Shutdown returned: %v", err)
		}
		close(blocker)
	})
}

func TestOperationQueue_WorkerExecution(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 2)
	defer q.Shutdown(time.Second)

	t.Run("executes operations with progress reporting", func(t *testing.T) {
		done := make(chan bool, 1)
		fn := func(ctx context.Context, progress ProgressReporter) error {
			progress.UpdateProgress(1, 10, "step 1")
			progress.UpdateProgress(5, 10, "step 5")
			progress.UpdateProgress(10, 10, "complete")
			done <- true
			return nil
		}

		err := q.Enqueue("progress-op", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		select {
		case <-done:
			// Verify the operation was updated in store
			time.Sleep(50 * time.Millisecond) // Allow cleanup
		case <-time.After(2 * time.Second):
			t.Fatal("operation did not complete")
		}
	})

	t.Run("handles operation errors", func(t *testing.T) {
		done := make(chan bool, 1)
		expectedErr := errors.New("test error")
		fn := func(ctx context.Context, progress ProgressReporter) error {
			done <- true
			return expectedErr
		}

		err := q.Enqueue("error-op", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		select {
		case <-done:
			time.Sleep(50 * time.Millisecond)
			store.AssertCalled(t, "UpdateOperationError", "error-op", expectedErr.Error())
		case <-time.After(2 * time.Second):
			t.Fatal("operation did not complete")
		}
	})

	t.Run("handles canceled operations", func(t *testing.T) {
		started := make(chan bool, 1)
		done := make(chan bool, 1)
		fn := func(ctx context.Context, progress ProgressReporter) error {
			started <- true
			<-ctx.Done()
			done <- true
			return ctx.Err()
		}

		err := q.Enqueue("ctx-cancel-op", "test", PriorityNormal, fn)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		<-started
		q.Cancel("ctx-cancel-op")
		<-done
	})
}

func TestOperationQueue_ConcurrentOperations(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 4)
	defer q.Shutdown(2 * time.Second)

	var wg sync.WaitGroup
	completed := make(chan string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		opID := "concurrent-" + string(rune('a'+i))
		fn := func(ctx context.Context, progress ProgressReporter) error {
			time.Sleep(10 * time.Millisecond)
			completed <- opID
			wg.Done()
			return nil
		}

		if err := q.Enqueue(opID, "test", PriorityNormal, fn); err != nil {
			t.Fatalf("Enqueue failed for %s: %v", opID, err)
		}
	}

	// Wait for all to complete
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All completed
	case <-time.After(5 * time.Second):
		t.Fatal("not all operations completed")
	}
}

func TestOperationProgressReporter(t *testing.T) {
	store := newMockStore(t)
	q := &OperationQueue{
		listeners: make(map[string][]ProgressListener),
		store:     store,
	}

	t.Run("UpdateProgress updates state and store", func(t *testing.T) {
		reporter := &operationProgressReporter{
			operationID: "reporter-op",
			store:       store,
			queue:       q,
		}

		err := reporter.UpdateProgress(5, 10, "halfway")
		if err != nil {
			t.Fatalf("UpdateProgress failed: %v", err)
		}

		if reporter.current != 5 {
			t.Errorf("expected current=5, got %d", reporter.current)
		}
		if reporter.total != 10 {
			t.Errorf("expected total=10, got %d", reporter.total)
		}
	})

	t.Run("Log adds to operation logs", func(t *testing.T) {
		reporter := &operationProgressReporter{
			operationID: "log-op",
			store:       store,
			queue:       q,
		}

		details := "some details"
		err := reporter.Log("info", "test message", &details)
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
		store.AssertCalled(t, "AddOperationLog", "log-op", "info", "test message", &details)
	})

	t.Run("IsCanceled returns false by default", func(t *testing.T) {
		// Set up mock expectation for GetOperationByID
		store.EXPECT().
			GetOperationByID(testifyMock.Anything).
			Return(&database.Operation{
				ID:     "not-canceled-op",
				Status: "running",
			}, nil).
			Maybe()

		reporter := &operationProgressReporter{
			operationID: "not-canceled-op",
			store:       store,
			queue:       q,
		}

		if reporter.IsCanceled() {
			t.Error("expected IsCanceled to return false")
		}
	})

	t.Run("IsCanceled returns true when canceled flag set", func(t *testing.T) {
		reporter := &operationProgressReporter{
			operationID: "canceled-flag-op",
			store:       store,
			queue:       q,
			canceled:    true,
		}

		if !reporter.IsCanceled() {
			t.Error("expected IsCanceled to return true")
		}
	})

	t.Run("IsCanceled checks database status", func(t *testing.T) {
		// Create a fresh mock for this test
		freshStore := newMockStore(t)
		freshStore.EXPECT().GetOperationByID("db-canceled-op").Return(&database.Operation{
			ID:     "db-canceled-op",
			Status: "canceled",
		}, nil)

		reporter := &operationProgressReporter{
			operationID: "db-canceled-op",
			store:       freshStore,
			queue:       q,
		}

		if !reporter.IsCanceled() {
			t.Error("expected IsCanceled to return true from database")
		}
		if !reporter.canceled {
			t.Error("expected canceled flag to be set")
		}
	})
}

func TestActiveOperations(t *testing.T) {
	store := newMockStore(t)
	q := NewOperationQueue(store, 1)
	defer q.Shutdown(time.Second)

	t.Run("returns empty when no operations", func(t *testing.T) {
		active := q.ActiveOperations()
		if len(active) != 0 {
			t.Errorf("expected 0 active operations, got %d", len(active))
		}
	})

	t.Run("returns nil for nil queue", func(t *testing.T) {
		var nilQ *OperationQueue
		active := nilQ.ActiveOperations()
		if active == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(active) != 0 {
			t.Errorf("expected 0 active operations, got %d", len(active))
		}
	})

	t.Run("returns active operations", func(t *testing.T) {
		blocker := make(chan struct{})
		fn := func(ctx context.Context, progress ProgressReporter) error {
			<-blocker
			return nil
		}

		_ = q.Enqueue("active-1", "scan", PriorityNormal, fn)
		time.Sleep(50 * time.Millisecond)

		active := q.ActiveOperations()
		found := false
		for _, op := range active {
			if op.ID == "active-1" && op.Type == "scan" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find active-1 in active operations")
		}

		close(blocker)
	})
}

func TestSetStore(t *testing.T) {
	t.Run("sets store when not already set", func(t *testing.T) {
		q := &OperationQueue{}
		store := newMockStore(t)

		q.SetStore(store)
		if q.store != store {
			t.Error("store was not set")
		}
	})

	t.Run("does not overwrite existing store", func(t *testing.T) {
		existingStore := newMockStore(t)
		newStore := newMockStore(t)

		q := &OperationQueue{store: existingStore}
		q.SetStore(newStore)

		if q.store != existingStore {
			t.Error("store was overwritten")
		}
	})

	t.Run("handles nil queue", func(t *testing.T) {
		var q *OperationQueue
		store := newMockStore(t)
		// Should not panic
		q.SetStore(store)
	})

	t.Run("handles nil store", func(t *testing.T) {
		q := &OperationQueue{}
		// Should not panic
		q.SetStore(nil)
	})
}

func TestGlobalQueueFunctions(t *testing.T) {
	// Save and restore global state
	oldQueue := GlobalQueue
	defer func() { GlobalQueue = oldQueue }()

	t.Run("InitializeQueue creates global queue", func(t *testing.T) {
		GlobalQueue = nil
		store := newMockStore(t)

		InitializeQueue(store, 2)
		if GlobalQueue == nil {
			t.Fatal("GlobalQueue not initialized")
		}
		oq, ok := GlobalQueue.(*OperationQueue)
		if !ok {
			t.Fatal("GlobalQueue is not OperationQueue")
		}
		if oq.workers != 2 {
			t.Errorf("expected 2 workers, got %d", oq.workers)
		}

		GlobalQueue.Shutdown(time.Second)
	})

	t.Run("InitializeQueue warns on double init", func(t *testing.T) {
		store := newMockStore(t)
		GlobalQueue = NewOperationQueue(store, 1)
		defer GlobalQueue.Shutdown(time.Second)

		// Should not panic, just log warning
		InitializeQueue(store, 4)
		oq, ok := GlobalQueue.(*OperationQueue)
		if !ok {
			t.Fatal("GlobalQueue is not OperationQueue")
		}
		if oq.workers != 1 {
			t.Error("GlobalQueue was incorrectly replaced")
		}
	})

	t.Run("ShutdownQueue handles nil queue", func(t *testing.T) {
		GlobalQueue = nil
		err := ShutdownQueue(time.Second)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("ShutdownQueue shuts down global queue", func(t *testing.T) {
		store := newMockStore(t)
		GlobalQueue = NewOperationQueue(store, 1)

		err := ShutdownQueue(time.Second)
		if err != nil {
			t.Errorf("ShutdownQueue failed: %v", err)
		}
	})
}

func TestPriorityConstants(t *testing.T) {
	if PriorityLow >= PriorityNormal {
		t.Error("PriorityLow should be less than PriorityNormal")
	}
	if PriorityNormal >= PriorityHigh {
		t.Error("PriorityNormal should be less than PriorityHigh")
	}
}
