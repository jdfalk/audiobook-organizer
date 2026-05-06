// file: pkg/plugin/sdk/errors.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5678-2345-6789abcdef01
// last-edited: 2026-05-06

package sdk

import "errors"

var (
	// ErrCanceled is returned when an operation is canceled by the user or system.
	ErrCanceled = errors.New("operation canceled")
	// ErrQuiesced is returned when a plugin is quiesced for disable.
	ErrQuiesced = errors.New("plugin quiesced for disable")
	// ErrPluginCapabilityMissing is returned when an operation tries to use a
	// capability that was not declared in OperationDef.Capabilities.
	ErrPluginCapabilityMissing = errors.New("plugin capability not declared")
)
