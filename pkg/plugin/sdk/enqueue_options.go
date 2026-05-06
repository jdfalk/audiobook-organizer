// file: pkg/plugin/sdk/enqueue_options.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4567-1234-56789abcdef0
// last-edited: 2026-05-06

package sdk

import "github.com/jdfalk/audiobook-organizer/internal/operations/registry"

// EnqueueOption is the function-option pattern for EnqueueOp.
type EnqueueOption = registry.EnqueueOption

// Enqueue option constructors.
var (
	// WithParent sets the parent run ID for trigger lineage.
	WithParent = registry.WithParent
	// WithActor sets the user ID of the actor triggering the run.
	WithActor = registry.WithActor
	// WithPriority overrides the OperationDef's DefaultPriority for this run.
	WithPriority = registry.WithPriority
)
