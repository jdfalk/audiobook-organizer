// file: internal/database/iface_ops_v2.go
// version: 2.0.0
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
	ID              string
	DefID           string
	Plugin          string
	ParentID        *string
	ActorUserID     *string
	TraceID         string
	SpanID          string
	ParentSpanID    *string
	Status          string
	Priority        int
	ProgressCurrent int
	ProgressTotal   int
	ProgressMessage string
	CurrentPhase    *string
	Params          string
	ErrorMessage    *string
	ResultData      *string
	QueuedAt        time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	LastProgressAt  *time.Time
	LastCheckpointAt *time.Time
	HighWaterProgress int
	ResumeCount     int
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
}
