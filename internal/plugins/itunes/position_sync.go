// file: internal/plugins/itunes/position_sync.go
// version: 1.0.0
// guid: f2a3b4c5-d6e7-8f9a-0b1c-2d3e4f5a6b7c
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// positionSyncRequest is the parameter type for itunes.position-sync.
type positionSyncRequest struct {
	// No parameters needed; service handles positions internally
}

func (p *Plugin) positionSyncDef() sdk.OperationDef {
	schedule := "*/15 * * * *" // Every 15 minutes
	return sdk.OperationDef{
		ID:              "itunes.position-sync",
		Plugin:          "itunes",
		DisplayName:     "iTunes Position Sync",
		Description:     "Synchronize reading positions between iTunes and app",
		DefaultPriority: sdk.PriorityNormal,
		ResumePolicy:    sdk.ResumeRequeue,
		Schedule:        &schedule,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
		},
		Run: p.positionSyncRun,
	}
}

func (p *Plugin) positionSyncRun(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	if p.svc == nil || !p.svc.Enabled() {
		return errors.New("iTunes service not available or disabled")
	}

	// Create a logger wrapper that implements logger.Logger and delegates to the SDK reporter
	logWrapper := NewLoggerWrapper(reporter)
	logWrapper.Info("Starting iTunes position sync")

	// Call the Positions service's Sync method
	pulled, pushed := p.svc.Positions.Sync()

	// Format message for logging and progress
	msg := fmt.Sprintf("iTunes position sync: pulled %d, pushed %d", pulled, pushed)
	// Note: Use explicit format string to satisfy logger interface requirements
	logWrapper.Info("iTunes position sync completed: %d pulled, %d pushed", pulled, pushed)

	// Update progress to indicate completion
	if err := reporter.UpdateProgress(1, 1, msg); err != nil {
		logWrapper.Warn("failed to update progress: %v", err)
	}

	return nil
}
