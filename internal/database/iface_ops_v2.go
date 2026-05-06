// file: internal/database/iface_ops_v2.go
// version: 2.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-06

package database

import "time"

// OpDefinitionV2Row is the DB representation of a registered OperationDef.
type OpDefinitionV2Row struct {
	ID             string
	Plugin         string
	DisplayName    string
	Description    string
	Capabilities   string // JSON array
	Permissions    string // JSON array
	Cancellable    bool
	Isolate        bool
	ResumePolicy   string
	ScheduleCron   *string
	Triggers       string // JSON array
	DependsOn      string // JSON array
	Phases         string // JSON array
	TimeoutSeconds int
	RegisteredAt   time.Time
}

// OperationV2Row is a queued/running/terminal row from operations_v2.
type OperationV2Row struct {
	ID               string
	DefID            string
	Plugin           string
	ParentID         *string
	ActorUserID      *string
	TraceID          string
	SpanID           string
	ParentSpanID     *string
	Status           string
	Priority         int
	ProgressCurrent  int
	ProgressTotal    int
	ProgressMessage  string
	CurrentPhase     *string
	Params           string
	ErrorMessage     *string
	ResultData       *string
	QueuedAt         time.Time
	StartedAt        *time.Time
	CompletedAt      *time.Time
	LastProgressAt   *time.Time
	LastCheckpointAt *time.Time
	HighWaterProgress int
	ResumeCount      int
}

// OpStrikeV2Row is a single row in op_strikes_v2.
type OpStrikeV2Row struct {
	DefID       string
	OperationID string
	Kind        string // "uncheckpointed" | "stuck" | "infinite_restart"
	Details     string // JSON object with plugin, message, etc.
	OccurredAt  time.Time
}

// OpStateV2Row is a single row in op_state_v2.
type OpStateV2Row struct {
	OperationID   string
	Phase         *string
	StateBlob     []byte
	SchemaVersion int
	WrittenAt     time.Time
}

// OpLogV2Row is a single log line written to op_logs_v2.
type OpLogV2Row struct {
	OperationID string
	Level       string // "debug", "info", "warn", "error"
	Message     string
	Attrs       string // JSON object
	CreatedAt   time.Time
}

// OpErrorV2Row is a persistent error record written to op_errors_v2.
type OpErrorV2Row struct {
	OperationID string
	Plugin      string
	DefID       string
	Message     string
	Attrs       string // JSON object
	OccurredAt  time.Time
}

// OpsV2Store covers the UOS v2 schema surface used by the registry.
// Only implemented by SQLiteStore; PebbleStore returns ErrNotSupported.
type OpsV2Store interface {
	// UpsertOpDefinitionV2 inserts or replaces a definition row.
	UpsertOpDefinitionV2(row OpDefinitionV2Row) error

	// DeleteOrphanOpDefsV2 removes rows not in the keepIDs set.
	DeleteOrphanOpDefsV2(keepIDs []string) error

	// InsertOperationV2 inserts a new queued run.
	InsertOperationV2(row OperationV2Row) error

	// ListQueuedOperationsV2 returns queued ops ordered by priority DESC, queued_at ASC.
	ListQueuedOperationsV2() ([]OperationV2Row, error)

	// ListActiveOperationsV2 returns ops with status 'queued' or 'running'.
	ListActiveOperationsV2() ([]OperationV2Row, error)

	// GetOperationV2 returns a single run by id.
	GetOperationV2(id string) (*OperationV2Row, error)

	// UpdateOperationV2Status sets the status (and optional timestamps).
	// startedAt / completedAt are set when non-nil.
	UpdateOperationV2Status(id, status string, startedAt, completedAt *time.Time, errMsg *string) error

	// SetOperationV2StatusIfQueued atomically sets status=canceled only if status was queued.
	// Returns true if the row was updated.
	SetOperationV2StatusIfQueued(id, newStatus string) (bool, error)

	// CountRunningByPluginV2 returns the number of running ops for a plugin.
	CountRunningByPluginV2(plugin string) (int, error)

	// IncrementResumeCountV2 atomically increments resume_count for the given op.
	IncrementResumeCountV2(id string) error

	// InsertOpStrikeV2 appends a row to op_strikes_v2.
	InsertOpStrikeV2(row OpStrikeV2Row) error

	// GetOpStateV2 returns the state blob for an op, or nil if none.
	GetOpStateV2(opID string) (*OpStateV2Row, error)

	// DeleteOpStateV2 removes the state blob for an op (used by ResumeRequeue).
	DeleteOpStateV2(opID string) error

	// UpdateOpProgressV2 updates the progress columns and last_progress_at.
	UpdateOpProgressV2(id string, current, total int, message string) error

	// UpdateOpPhaseV2 sets (or clears) current_phase on an operation.
	UpdateOpPhaseV2(id string, phase *string) error

	// UpdateOpCheckpointV2 sets last_checkpoint_at and updates high_water_progress
	// to MAX(old, newHWM).
	UpdateOpCheckpointV2(id string, newHWM int) error

	// AppendOpLogsV2 bulk-inserts log rows into op_logs_v2.
	AppendOpLogsV2(rows []OpLogV2Row) error

	// InsertOpErrorV2 inserts a single row into op_errors_v2.
	InsertOpErrorV2(row OpErrorV2Row) error

	// UpsertOpStateV2 inserts or replaces a checkpoint row in op_state_v2.
	UpsertOpStateV2(row OpStateV2Row) error
}
