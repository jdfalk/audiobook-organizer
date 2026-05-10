// file: internal/operations/bridge.go
// version: 1.1.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b
// last-edited: 2026-05-10

package operations

import (
	"context"
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// BridgeQueue wraps an OperationQueue and dual-writes run lifecycle events
// to operations_v2 so legacy ops appear in the v2 timeline (UOS-14).
// All functional behaviour is delegated to the inner queue.
type BridgeQueue struct {
	inner   *OperationQueue
	v2Store database.OpsV2Store
}

// NewBridgeQueue creates a BridgeQueue. v2Store must not be nil.
func NewBridgeQueue(inner *OperationQueue, v2Store database.OpsV2Store) *BridgeQueue {
	return &BridgeQueue{inner: inner, v2Store: v2Store}
}

// Enqueue inserts a queued row in operations_v2, wraps fn to update the row
// as the op transitions to running/completed/failed, then delegates to inner.
func (b *BridgeQueue) Enqueue(id, opType string, priority int, fn OperationFunc) error {
	now := time.Now().UTC()
	row := database.OperationV2Row{
		ID:       id,
		DefID:    "bridge." + opType,
		Plugin:   opType,
		TraceID:  id,
		SpanID:   id,
		Status:   "queued",
		Priority: priority,
		Params:   "{}",
		QueuedAt: now,
	}
	if err := b.v2Store.InsertOperationV2(row); err != nil {
		log.Printf("[WARN] bridge: insert operations_v2 for %s/%s: %v", opType, id, err)
	}
	return b.inner.Enqueue(id, opType, priority, b.wrapFn(id, fn))
}

// EnqueueResume re-enqueues an already-existing DB record. The v2 row may not
// exist for ops that pre-date the bridge; status updates are best-effort.
func (b *BridgeQueue) EnqueueResume(id, opType string, priority int, fn OperationFunc) error {
	return b.inner.EnqueueResume(id, opType, priority, b.wrapFn(id, fn))
}

func (b *BridgeQueue) Cancel(id string) error { return b.inner.Cancel(id) }

func (b *BridgeQueue) ActiveOperations() []ActiveOperation { return b.inner.ActiveOperations() }

func (b *BridgeQueue) Shutdown(timeout time.Duration) error { return b.inner.Shutdown(timeout) }

func (b *BridgeQueue) SetStore(store database.OperationStore) { b.inner.SetStore(store) }

func (b *BridgeQueue) SetOperationTimeout(d time.Duration) { b.inner.SetOperationTimeout(d) }

func (b *BridgeQueue) SetActivityLogger(l ActivityLogger) { b.inner.SetActivityLogger(l) }

// wrapFn returns an OperationFunc that updates the operations_v2 row around fn.
// The ProgressReporter passed to fn is wrapped so that UpdateProgress calls are
// dual-written to the v2 store, making progress visible in the timeline.
func (b *BridgeQueue) wrapFn(id string, fn OperationFunc) OperationFunc {
	return func(ctx context.Context, progress ProgressReporter) error {
		startedAt := time.Now().UTC()
		_ = b.v2Store.UpdateOperationV2Status(id, "running", &startedAt, nil, nil)

		wrapped := &bridgeProgressReporter{inner: progress, id: id, store: b.v2Store}
		err := fn(ctx, wrapped)

		completedAt := time.Now().UTC()
		if err != nil {
			msg := err.Error()
			_ = b.v2Store.UpdateOperationV2Status(id, "failed", nil, &completedAt, &msg)
		} else {
			_ = b.v2Store.UpdateOperationV2Status(id, "completed", nil, &completedAt, nil)
		}
		return err
	}
}

// bridgeProgressReporter wraps a ProgressReporter and syncs progress updates to
// the v2 store so the timeline shows real progress for bridged operations.
type bridgeProgressReporter struct {
	inner ProgressReporter
	id    string
	store database.OpsV2Store
}

func (r *bridgeProgressReporter) UpdateProgress(current, total int, message string) error {
	_ = r.store.UpdateOpProgressV2(r.id, current, total, message)
	return r.inner.UpdateProgress(current, total, message)
}

func (r *bridgeProgressReporter) Log(level, message string, details *string) error {
	return r.inner.Log(level, message, details)
}

func (r *bridgeProgressReporter) IsCanceled() bool {
	return r.inner.IsCanceled()
}
