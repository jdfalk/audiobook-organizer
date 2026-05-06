// file: pkg/plugin/sdk/registration.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-0123-456789abcdef
// last-edited: 2026-05-06

package sdk

import "context"

// Registry is the narrowed plugin-facing interface for registering and enqueuing operations.
// Implementations provide access to the global UOS-02 registry.
type Registry interface {
	// RegisterOp registers an OperationDef with the registry. Called during Plugin.Register.
	RegisterOp(def OperationDef) error
	// EnqueueOp enqueues a new operation run with optional metadata (parent, actor, priority).
	EnqueueOp(ctx context.Context, defID string, params any, opts ...EnqueueOption) (string, error)
}
