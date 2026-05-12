// file: internal/serviceregistry/lifecycle.go
// version: 1.0.0

package serviceregistry

import "context"

// PostIniter is implemented by services that need cross-wiring after all
// Build() calls complete. PostInit runs in resolved dep order. Within
// PostInit, Get[T] is unrestricted — any built service may be retrieved.
type PostIniter interface {
	PostInit(ctx context.Context, c *Container) error
}

// Starter is implemented by services that run background goroutines or
// otherwise need an explicit start signal. Start runs in resolved dep
// order; on error, the container halts and calls Stop on already-started
// services in reverse.
type Starter interface {
	Start(ctx context.Context) error
}

// Stopper is implemented by services that hold resources requiring
// explicit release. Stop runs in REVERSE resolved order. Errors are
// logged but do not abort the sweep.
type Stopper interface {
	Stop(ctx context.Context) error
}
