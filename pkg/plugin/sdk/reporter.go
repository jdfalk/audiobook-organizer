// file: pkg/plugin/sdk/reporter.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012
// last-edited: 2026-05-06

package sdk

import "github.com/jdfalk/audiobook-organizer/internal/operations/registry"

// Reporter is the per-run API surface an in-flight operation uses to emit
// progress, logs, and checkpoints.
type Reporter = registry.Reporter
