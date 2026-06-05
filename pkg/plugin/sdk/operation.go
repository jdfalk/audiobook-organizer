// file: pkg/plugin/sdk/operation.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901
// last-edited: 2026-05-06

package sdk

import "github.com/falkcorp/audiobook-organizer/internal/operations/registry"

// Type aliases for operation definition and control types.
type OperationDef = registry.OperationDef
type ResumePolicy = registry.ResumePolicy
type Priority = registry.Priority
type ActorMode = registry.ActorMode
type Phase = registry.Phase

// Resume policy constants.
const (
	ResumeUnspecified = registry.ResumeUnspecified
	ResumeRestart     = registry.ResumeRestart
	ResumeRequeue     = registry.ResumeRequeue
	ResumeDrop        = registry.ResumeDrop
	ResumeAsk         = registry.ResumeAsk
)

// Priority level constants.
const (
	PriorityLow    = registry.PriorityLow
	PriorityNormal = registry.PriorityNormal
	PriorityHigh   = registry.PriorityHigh
)

// Actor mode constants.
const (
	ActorContext = registry.ActorContext
	ActorSystem  = registry.ActorSystem
)
