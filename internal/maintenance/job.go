// file: internal/maintenance/job.go
// version: 1.0.0
// guid: 11111111-1111-1111-1111-111111111111
// last-edited: 2026-05-03

package maintenance

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
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

// MaintenanceJob is the interface that every maintenance job must satisfy.
type MaintenanceJob interface {
	ID() string
	Description() string
	CanResume() bool
	Run(ctx context.Context, store database.Store, reporter ProgressReporter, dryRun bool) error
}

var store database.Store

func InjectStore(s database.Store) { store = s }
func GetStore() database.Store    { return store }
