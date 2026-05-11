// file: internal/server/scheduler_triggers.go
// version: 1.1.0
// guid: 03dbb3f6-0076-484c-b55f-5f59c649402d
// last-edited: 2026-05-11
// NOTE: triggerOperation and triggerOperationWithID are now dead code — all
// TriggerFns in scheduler_tasks.go have been migrated to the hybrid UOS v2
// pattern (create v1 op record + opRegistry.EnqueueOp). These helpers are
// retained for the transition period and will be removed once v1 is fully retired.

package server

import (
	"context"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// triggerOperation is a helper that creates a DB operation and enqueues it.
func (ts *TaskScheduler) triggerOperation(opType, source string, fn func(context.Context, operations.ProgressReporter) error) (*database.Operation, error) {
	store := ts.server.Store()
	if store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if ts.server.queue == nil {
		return nil, fmt.Errorf("operation queue not initialized")
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, opType, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create operation: %w", err)
	}

	srcFn := func(ctx context.Context, progress operations.ProgressReporter) error {
		return fn(operations.WithTriggerSource(ctx, source), progress)
	}
	if err := ts.server.queue.Enqueue(op.ID, opType, operations.PriorityNormal, srcFn); err != nil {
		return nil, fmt.Errorf("failed to enqueue operation: %w", err)
	}

	return op, nil
}

// triggerOperationWithID is like triggerOperation but passes the operation ID to the function.
func (ts *TaskScheduler) triggerOperationWithID(opType, source string, fn func(context.Context, operations.ProgressReporter, string) error) (*database.Operation, error) {
	store := ts.server.Store()
	if store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if ts.server.queue == nil {
		return nil, fmt.Errorf("operation queue not initialized")
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, opType, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create operation: %w", err)
	}

	wrappedFn := func(ctx context.Context, progress operations.ProgressReporter) error {
		return fn(operations.WithTriggerSource(ctx, source), progress, op.ID)
	}

	if err := ts.server.queue.Enqueue(op.ID, opType, operations.PriorityNormal, wrappedFn); err != nil {
		return nil, fmt.Errorf("failed to enqueue operation: %w", err)
	}

	return op, nil
}

