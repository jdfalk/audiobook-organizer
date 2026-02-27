// file: internal/operations/queue.go
// version: 1.5.0
// guid: 7d6e5f4a-3c2b-1a09-8f7e-6d5c4b3a2190

package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
)

// Priority levels for operations
const (
	PriorityLow    = 0
	PriorityNormal = 1
	PriorityHigh   = 2
)

// OperationFunc represents an operation that can be executed
type OperationFunc func(ctx context.Context, progress ProgressReporter) error

// ProgressReporter allows operations to report their progress
type ProgressReporter interface {
	UpdateProgress(current, total int, message string) error
	Log(level, message string, details *string) error
	IsCanceled() bool
}

// QueuedOperation represents an operation in the queue
type QueuedOperation struct {
	ID       string
	Type     string
	Priority int
	Func     OperationFunc
	Context  context.Context
	Cancel   context.CancelFunc
}

// Queue defines the interface for operation queue management
type Queue interface {
	Enqueue(id, opType string, priority int, fn OperationFunc) error
	Cancel(id string) error
	ActiveOperations() []ActiveOperation
	Shutdown(timeout time.Duration) error
	SetStore(store database.Store)
}

// OperationQueue manages async operations with priority handling
type OperationQueue struct {
	mu         sync.RWMutex
	operations map[string]*QueuedOperation
	pending    chan *QueuedOperation
	workers    int
	store      database.Store
	timeout    time.Duration
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	listeners  map[string][]ProgressListener
}

// ProgressListener receives progress updates
type ProgressListener func(operationID string, progress OperationProgress)

// OperationProgress represents the current state of an operation
type OperationProgress struct {
	Current int
	Total   int
	Message string
}

// NewOperationQueue creates a new operation queue
func NewOperationQueue(store database.Store, workers int) *OperationQueue {
	if workers <= 0 {
		workers = 2 // Default to 2 workers
	}

	ctx, cancel := context.WithCancel(context.Background())

	q := &OperationQueue{
		operations: make(map[string]*QueuedOperation),
		pending:    make(chan *QueuedOperation, 100),
		workers:    workers,
		store:      store,
		timeout:    30 * time.Minute,
		ctx:        ctx,
		cancel:     cancel,
		listeners:  make(map[string][]ProgressListener),
	}

	// Start worker goroutines
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	return q
}

// EnqueueResume re-enqueues an interrupted operation without creating a new DB record.
// The operation record already exists; this just puts it back in the work queue.
func (q *OperationQueue) EnqueueResume(id, opType string, priority int, fn OperationFunc) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.operations[id]; exists {
		return fmt.Errorf("operation %s already exists", id)
	}

	ctx, cancel := context.WithCancel(q.ctx)
	op := &QueuedOperation{
		ID:       id,
		Type:     opType,
		Priority: priority,
		Func:     fn,
		Context:  ctx,
		Cancel:   cancel,
	}
	q.operations[id] = op

	if q.store != nil {
		_ = q.store.UpdateOperationStatus(id, "queued", 0, 0, "resuming interrupted operation")
	}

	select {
	case q.pending <- op:
		log.Printf("Resumed operation %s enqueued", id)
	default:
		log.Printf("Warning: pending queue full for resumed operation %s", id)
	}
	return nil
}

// Enqueue adds a new operation to the queue
func (q *OperationQueue) Enqueue(id, opType string, priority int, fn OperationFunc) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if operation already exists
	if _, exists := q.operations[id]; exists {
		return fmt.Errorf("operation %s already exists", id)
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(q.ctx)

	op := &QueuedOperation{
		ID:       id,
		Type:     opType,
		Priority: priority,
		Func:     fn,
		Context:  ctx,
		Cancel:   cancel,
	}

	q.operations[id] = op

	// Update database status to queued
	if q.store != nil {
		_ = q.store.UpdateOperationStatus(id, "queued", 0, 0, "operation queued")
	}

	// Add to pending channel (non-blocking)
	select {
	case q.pending <- op:
		log.Printf("Operation %s enqueued with priority %d", id, priority)
	default:
		// Channel full, operation will be picked up later
		log.Printf("Warning: pending queue full for operation %s", id)
	}

	return nil
}

// Cancel cancels an operation
func (q *OperationQueue) Cancel(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	op, exists := q.operations[id]
	if !exists {
		return fmt.Errorf("operation %s not found", id)
	}

	// Cancel the context
	op.Cancel()

	// Update database
	if q.store != nil {
		_ = q.store.UpdateOperationStatus(id, "canceled", 0, 0, "operation canceled by user")
	}

	log.Printf("Operation %s canceled", id)
	return nil
}

// GetStatus returns the current status of an operation
func (q *OperationQueue) GetStatus(id string) (*database.Operation, error) {
	if q.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return q.store.GetOperationByID(id)
}

// AddListener adds a progress listener for an operation
func (q *OperationQueue) AddListener(operationID string, listener ProgressListener) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.listeners[operationID] = append(q.listeners[operationID], listener)
}

// RemoveListeners removes all listeners for an operation
func (q *OperationQueue) RemoveListeners(operationID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.listeners, operationID)
}

// notifyListeners sends progress updates to all listeners
func (q *OperationQueue) notifyListeners(operationID string, progress OperationProgress) {
	q.mu.RLock()
	listeners := q.listeners[operationID]
	q.mu.RUnlock()

	for _, listener := range listeners {
		// Call listener in a goroutine to avoid blocking
		go listener(operationID, progress)
	}
}

// worker processes operations from the queue
func (q *OperationQueue) worker(id int) {
	defer q.wg.Done()

	log.Printf("Worker %d started", id)

	for {
		select {
		case <-q.ctx.Done():
			log.Printf("Worker %d stopped", id)
			return
		case op := <-q.pending:
			if op == nil {
				continue
			}

			log.Printf("Worker %d processing operation %s (type=%s)", id, op.ID, op.Type)

			// Metrics: mark start
			start := time.Now()
			metrics.IncOperationStarted(op.Type)

			// Update status to running
			if q.store != nil {
				if err := q.store.UpdateOperationStatus(op.ID, "running", 0, 0, "operation started"); err != nil {
					log.Printf("Worker %d: failed to update status to running for %s: %v", id, op.ID, err)
				}
			}

			// Create progress reporter
			reporter := &operationProgressReporter{
				operationID: op.ID,
				store:       q.store,
				queue:       q,
			}

			// Execute the operation with timeout protection and panic recovery.
			runCtx := op.Context
			cancelTimeout := func() {}
			if q.timeout > 0 {
				runCtx, cancelTimeout = context.WithTimeout(op.Context, q.timeout)
			}
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("operation panicked: %v", r)
						log.Printf("Worker %d: operation %s panicked: %v", id, op.ID, r)
					}
				}()
				err = op.Func(runCtx, reporter)
			}()
			cancelTimeout()
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				err = fmt.Errorf("operation timed out")
			}

			// Update final status
			if err != nil {
				if q.store != nil {
					_ = q.store.UpdateOperationError(op.ID, err.Error())
				}
				metrics.IncOperationFailed(op.Type)
				// Send real-time error status
				if hub := realtime.GetGlobalHub(); hub != nil {
					hub.SendOperationStatus(op.ID, "failed", map[string]interface{}{
						"error": err.Error(),
					})
				}
				log.Printf("Operation %s failed: %v", op.ID, err)
			} else if reporter.canceled {
				// Already marked as canceled
				metrics.IncOperationCanceled(op.Type)
				// Send real-time canceled status
				if hub := realtime.GetGlobalHub(); hub != nil {
					hub.SendOperationStatus(op.ID, "canceled", map[string]interface{}{
						"message": "operation canceled",
					})
				}
				log.Printf("Operation %s was canceled", op.ID)
			} else {
				if q.store != nil {
					_ = q.store.UpdateOperationStatus(op.ID, "completed", reporter.current, reporter.total, "operation completed")
				}
				metrics.IncOperationCompleted(op.Type)
				// Send real-time completed status
				if hub := realtime.GetGlobalHub(); hub != nil {
					hub.SendOperationStatus(op.ID, "completed", map[string]interface{}{
						"current": reporter.current,
						"total":   reporter.total,
						"message": "operation completed",
					})
				}
				log.Printf("Operation %s completed successfully", op.ID)
			}

			// Observe duration
			metrics.ObserveOperationDuration(op.Type, time.Since(start))

			// Persist operation summary log
			if q.store != nil {
				now := time.Now()
				summary := &database.OperationSummaryLog{
					ID:        op.ID,
					Type:      op.Type,
					CreatedAt: start,
					UpdatedAt: now,
				}
				if err != nil {
					errMsg := err.Error()
					summary.Status = "failed"
					summary.Error = &errMsg
					summary.CompletedAt = &now
				} else if reporter.canceled {
					summary.Status = "canceled"
					summary.CompletedAt = &now
				} else {
					summary.Status = "completed"
					summary.CompletedAt = &now
					if reporter.total > 0 {
						summary.Progress = float64(reporter.current) / float64(reporter.total) * 100
					} else {
						summary.Progress = 100
					}
				}
				if saveErr := q.store.SaveOperationSummaryLog(summary); saveErr != nil {
					log.Printf("Warning: failed to persist operation summary for %s: %v", op.ID, saveErr)
				}
			}

			// Clean up
			q.mu.Lock()
			delete(q.operations, op.ID)
			q.mu.Unlock()

			// Remove listeners
			q.RemoveListeners(op.ID)
		}
	}
}

// Shutdown gracefully shuts down the queue
func (q *OperationQueue) Shutdown(timeout time.Duration) error {
	log.Println("Shutting down operation queue...")

	// Mark all active operations as interrupted before canceling
	q.mu.RLock()
	for id, op := range q.operations {
		if q.store != nil {
			_ = q.store.UpdateOperationStatus(id, "interrupted", 0, 0, "server shutting down")
			// Update checkpoint status to interrupted
			if data, err := q.store.GetOperationState(id); err == nil && data != nil {
				var state OperationState
				if err := json.Unmarshal(data, &state); err == nil {
					state.Status = "interrupted"
					if updated, err := json.Marshal(state); err == nil {
						_ = q.store.SaveOperationState(id, updated)
					}
				}
			}
		}
		_ = op // suppress unused warning
	}
	q.mu.RUnlock()

	// Cancel all operations
	q.cancel()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Operation queue shut down gracefully")
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("shutdown timeout after %v", timeout)
	}
}

// operationProgressReporter implements ProgressReporter
type operationProgressReporter struct {
	operationID string
	store       database.Store
	queue       *OperationQueue
	current     int
	total       int
	canceled    bool
}

func (r *operationProgressReporter) UpdateProgress(current, total int, message string) error {
	r.current = current
	r.total = total

	// Update database
	if r.store != nil {
		if err := r.store.UpdateOperationStatus(r.operationID, "running", current, total, message); err != nil {
			return err
		}
	}

	// Notify listeners
	r.queue.notifyListeners(r.operationID, OperationProgress{
		Current: current,
		Total:   total,
		Message: message,
	})

	// Send real-time event
	if hub := realtime.GetGlobalHub(); hub != nil {
		hub.SendOperationProgress(r.operationID, current, total, message)
	}

	return nil
}

func (r *operationProgressReporter) Log(level, message string, details *string) error {
	if r.store != nil {
		if err := r.store.AddOperationLog(r.operationID, level, message, details); err != nil {
			return err
		}
	}

	// Send real-time log event
	if hub := realtime.GetGlobalHub(); hub != nil {
		hub.SendOperationLog(r.operationID, level, message, details)
	}

	return nil
}

func (r *operationProgressReporter) IsCanceled() bool {
	if r.canceled {
		return true
	}

	// Check database status
	if r.store != nil {
		op, err := r.store.GetOperationByID(r.operationID)
		if err == nil && op != nil && op.Status == "canceled" {
			r.canceled = true
			return true
		}
	}

	return false
}

// Global queue instance
var GlobalQueue Queue

// InitializeQueue initializes the global operation queue
func InitializeQueue(store database.Store, workers int) {
	if GlobalQueue != nil {
		log.Println("Warning: operation queue already initialized")
		return
	}
	GlobalQueue = NewOperationQueue(store, workers)
	log.Printf("Operation queue initialized with %d workers", workers)
}

// SetStore assigns a database store to an already-initialized queue if it doesn't have one yet.
// This enables early queue initialization (before database setup) while still allowing
// operation status persistence once the database becomes available.
func (q *OperationQueue) SetStore(store database.Store) {
	if q == nil || store == nil {
		return
	}
	if q.store != nil { // Do not overwrite an existing store
		return
	}
	q.store = store
	log.Println("Operation queue store attached")
}

// SetOperationTimeout sets the maximum operation execution duration.
// A zero or negative value disables timeout enforcement.
func (q *OperationQueue) SetOperationTimeout(timeout time.Duration) {
	if q == nil {
		return
	}
	q.timeout = timeout
}

// ActiveOperation represents lightweight info about an in-flight operation.
type ActiveOperation struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// ActiveOperations returns a snapshot of currently queued/running operations.
func (q *OperationQueue) ActiveOperations() []ActiveOperation {
	if q == nil {
		return []ActiveOperation{}
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	results := make([]ActiveOperation, 0, len(q.operations))
	for id, op := range q.operations {
		results = append(results, ActiveOperation{ID: id, Type: op.Type})
	}
	return results
}

// ShutdownQueue shuts down the global operation queue
func ShutdownQueue(timeout time.Duration) error {
	if GlobalQueue == nil {
		return nil
	}
	return GlobalQueue.Shutdown(timeout)
}

// SetGlobalOperationTimeout updates timeout for the initialized global queue, if available.
func SetGlobalOperationTimeout(timeout time.Duration) {
	if oq, ok := GlobalQueue.(*OperationQueue); ok {
		oq.SetOperationTimeout(timeout)
	}
}
