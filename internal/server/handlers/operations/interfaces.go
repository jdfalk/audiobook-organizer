// file: internal/server/handlers/operations/interfaces.go
// version: 1.0.0
// guid: 37502068-5061-401b-841e-0b191567f0bf
// last-edited: 2026-06-03

// Narrow dependency interfaces for the operations domain handlers (scan /
// organize / optimize / transcode triggers, operation status / logs / result /
// changes / revert, stale-op management, DB optimize, tasks, and the
// maintenance window). Each interface lists only what the handlers actually
// call so package operations stays decoupled from the concrete
// scheduler / registry / pipeline / store implementations and never imports
// package server (which would create an import cycle).

package operations

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/scheduler"
)

// OperationsStore is the narrow database.Store subset the operations handlers
// require. The concrete database.Store implementations satisfy it.
//
// It embeds the composed sub-interfaces (rather than enumerating their methods)
// for two reasons: (1) the optimizeDatabase / sweepTombstones / auditFileConsistency
// handlers pass the store opaquely to sweep.SweepTombstones / AuditFileConsistency,
// which demand a database.BookStore — structural satisfaction requires the full
// BookStore method set, not a hand-list; and (2) setInternalFlag's SetSetting plus
// the config.SaveConfigToDatabase calls (in updateTaskConfig /
// updateMaintenanceWindowConfig) require the full database.SettingsStore.
// GetOperationV2 / GetOpLogsV2 (from database.OpsV2Store) are listed individually
// because only those two of OpsV2Store's ~10 methods are used.
type OperationsStore interface {
	database.OperationStore // op CRUD, status, logs, changes, delete-by-status
	database.BookStore      // GetAllBooks + sweep/audit BookStore satisfaction
	database.AuthorStore    // optimizeDatabase compound-author split
	database.NarratorStore  // optimizeDatabase compound-narrator split
	database.SettingsStore  // setInternalFlag + config.SaveConfigToDatabase

	// v2 registry reads (subset of database.OpsV2Store). getOperationStatus
	// reads GetOperationV2; getOperationLogs reads GetOpLogsV2.
	GetOperationV2(id string) (*database.OperationV2Row, error)
	GetOpLogsV2(opID string, limit int) ([]database.OpLogV2Row, error)
}

// OperationsRegistry is the narrow operations-registry subset the operations
// handlers require: EnqueueOp (scan / organize / optimize / transcode starters)
// and Cancel (cancelOperation v2 path). The variadic opts param on EnqueueOp is
// preserved so the concrete *opsregistry.Registry satisfies the interface.
type OperationsRegistry interface {
	EnqueueOp(ctx context.Context, defID string, params any, opts ...opsregistry.EnqueueOption) (string, error)
	Cancel(opID string) error
}

// Scheduler is the narrow *scheduler.TaskScheduler subset used by the task and
// maintenance-window handlers.
type Scheduler interface {
	ListTasks() []scheduler.TaskInfo
	RunTaskManual(name string) (*database.Operation, error)
	RunMaintenanceWindow(ctx context.Context) error
	IsMaintenanceRunning() bool
	GetLastMaintenanceRunDate() string
}

// ScanCanceler is the narrow *aiscan.PipelineManager subset used by
// cancelOperation to cancel an in-flight AI scan by scan ID.
type ScanCanceler interface {
	CancelScan(scanID int) error
}

// AIScanLister is the narrow *database.AIScanStore subset used by
// cancelOperation to find the AI scan whose OperationID matches the op being
// canceled.
type AIScanLister interface {
	ListScans() ([]database.Scan, error)
}
