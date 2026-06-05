// file: internal/maintenance/job.go
// version: 1.3.0
// guid: 11111111-1111-1111-1111-111111111111
// last-edited: 2026-04-28

package maintenance

import (
	"context"

	"github.com/falkcorp/audiobook-organizer/internal/database"
)

type contextKey string

const opIDKey contextKey = "maintenance_op_id"

// WithOperationID returns a context carrying the given operation ID.
func WithOperationID(ctx context.Context, opID string) context.Context {
	return context.WithValue(ctx, opIDKey, opID)
}

// OperationIDFromCtx returns the operation ID stored in the context, or "".
func OperationIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(opIDKey).(string)
	return v
}

// ProgressReporter is the minimal interface jobs use to report progress.
type ProgressReporter interface {
	SetTotal(n int)
	Increment()
	Log(level, message string, details *string)
}

// WriteBackEnqueuer is the narrow interface jobs use for iTunes write-back.
// Satisfied by *itunesservice.WriteBackBatcher.
type WriteBackEnqueuer interface {
	Enqueue(bookID string)
	EnqueueRemove(pid string)
}

// EnqueuerInjectable is implemented by jobs that need the write-back enqueuer.
type EnqueuerInjectable interface {
	InjectEnqueuer(e WriteBackEnqueuer)
}

// PermissionAware is optionally implemented by jobs that require a non-default
// permission. The dispatcher uses this to enforce per-job access control.
// Jobs that do not implement this interface default to the settings.manage permission.
type PermissionAware interface {
	Permission() string
}

// MaintenanceJob is the interface that every maintenance job must satisfy.
type MaintenanceJob interface {
	// ID returns the kebab-case identifier used in route paths and operation types.
	ID() string
	// Name returns the human-readable display name shown in the UI.
	Name() string
	// Description returns a one-sentence description of what the job does.
	Description() string
	// Category groups related jobs in the UI (e.g. "library", "files", "itunes", "dedup", "cleanup").
	Category() string
	// DefaultParams returns a struct with default parameter values (used by the frontend).
	DefaultParams() any
	// CanResume reports whether the job supports checkpoint-based resume after restart.
	CanResume() bool
	// Run executes the job. startFrom is the checkpoint index for resumable jobs (0 = fresh start).
	Run(ctx context.Context, store database.Store, reporter ProgressReporter, dryRun bool) error
}

var store database.Store

func InjectStore(s database.Store) { store = s }
func GetStore() database.Store     { return store }
