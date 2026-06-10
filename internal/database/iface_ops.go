// file: internal/database/iface_ops.go
// version: 1.2.0
// guid: b93b0da0-8afb-46fb-983e-c43f238ea67c

package database

import "time"

// OperationStore covers the full operation-tracking surface:
// Operation + logs + state + results + changes + summary + retention.
type OperationStore interface {
	// Operation CRUD
	CreateOperation(id, opType string, folderPath *string) (*Operation, error)
	GetOperationByID(id string) (*Operation, error)
	GetRecentOperations(limit int) ([]Operation, error)
	ListOperations(limit, offset int) ([]Operation, int, error)
	UpdateOperationStatus(id, status string, progress, total int, message string) error
	UpdateOperationError(id, errorMessage string) error
	UpdateOperationResultData(id string, resultData string) error

	// State persistence (resumable operations)
	SaveOperationState(opID string, state []byte) error
	GetOperationState(opID string) ([]byte, error)
	SaveOperationParams(opID string, params []byte) error
	GetOperationParams(opID string) ([]byte, error)
	DeleteOperationState(opID string) error
	GetInterruptedOperations() ([]Operation, error)

	// Change tracking (undo/rollback)
	CreateOperationChange(change *OperationChange) error
	GetOperationChanges(operationID string) ([]*OperationChange, error)
	GetBookChanges(bookID string) ([]*OperationChange, error)
	RevertOperationChanges(operationID string) error

	// Logs
	AddOperationLog(operationID, level, message string, details *string) error
	GetOperationLogs(operationID string) ([]OperationLog, error)

	// Summary logs (persistent across restarts)
	SaveOperationSummaryLog(op *OperationSummaryLog) error
	GetOperationSummaryLog(id string) (*OperationSummaryLog, error)
	ListOperationSummaryLogs(limit, offset int) ([]OperationSummaryLog, error)

	// Per-book result rows
	CreateOperationResult(result *OperationResult) error
	GetOperationResults(operationID string) ([]OperationResult, error)
	// GetOperationResultsPage returns one page of results plus the total count for the operation.
	// limit=0 means no cap (returns everything from offset onward).
	GetOperationResultsPage(operationID string, limit, offset int) ([]OperationResult, int, error)
	GetRecentCompletedOperations(limit int) ([]Operation, error)

	// Retention
	PruneOperationLogs(olderThan time.Time) (int, error)
	PruneOperationChanges(olderThan time.Time) (int, error)
	DeleteOperationsByStatus(statuses []string) (int, error)

	// DeleteOperationWithLogs removes the operation record (operation:<id>) and all
	// associated log lines (operationlog:<id>:*) in a single atomic batch.
	// This is the correct deletion primitive for the retention sweep — deleting the
	// operation key alone would orphan its log lines in PebbleDB indefinitely.
	DeleteOperationWithLogs(id string) error
}
