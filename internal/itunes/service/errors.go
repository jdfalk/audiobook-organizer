// file: internal/itunes/service/errors.go
// version: 1.0.0
// guid: c34d7365-ba73-4d8f-87a2-9bd2259ba2a0

package itunesservice

import "errors"

// ErrITunesDisabled is returned by methods called on a Service
// constructed with NewDisabled. Callers should surface this as a 503
// Service Unavailable at the HTTP layer.
var ErrITunesDisabled = errors.New("iTunes integration is disabled")

// ErrNotImplemented is a placeholder returned from sub-component method
// stubs until they're filled in during PR 2. Should never appear on
// main after PR 2 merges.
var ErrNotImplemented = errors.New("iTunes service method not yet implemented")
