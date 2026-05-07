// file: internal/plugins/itunes/path_repair.go
// version: 1.0.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// pathRepairRequest is the parameter type for itunes.path-repair.
type pathRepairRequest struct {
	Apply bool `json:"apply,omitempty"` // If false, runs in dry-run mode
}

func (p *Plugin) pathRepairDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "itunes.path-repair",
		Plugin:          "itunes",
		DisplayName:     "iTunes Path Repair",
		Description:     "Repair iTunes file paths (destructive — requires confirmation)",
		DefaultPriority: sdk.PriorityNormal,
		Phases: []sdk.Phase{
			{Name: "scan_files"},
			{Name: "match_paths"},
			{Name: "apply_changes"},
		},
		ResumePolicy: sdk.ResumeAsk, // REQUIRED: destructive operation, user must confirm
		Cancellable:  true,
		Timeout:      6 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
		},
		Run: p.pathRepairRun,
	}
}

func (p *Plugin) pathRepairRun(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	var req pathRepairRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Default to dry-run for safety
		req = pathRepairRequest{Apply: false}
	}

	if p.svc == nil || !p.svc.Enabled() {
		return errors.New("iTunes service not available or disabled")
	}

	// Create a logger wrapper that implements logger.Logger and delegates to the SDK reporter
	logWrapper := NewLoggerWrapper(reporter)
	logWrapper.Info("Starting iTunes path repair (apply=%v)", req.Apply)

	// Wrap the SDK reporter to provide a compatible progress interface
	progressReporter := &progressAdapter{reporter: reporter}

	// Call the Repair service's Repair method
	// Using empty operation ID since the reporter should handle checkpoints
	err := p.svc.Repair.Repair(ctx, "", req.Apply, progressReporter)
	if err != nil {
		logWrapper.Error("iTunes path repair failed: %v", err)
		return fmt.Errorf("path repair: %w", err)
	}

	mode := "dry-run"
	if req.Apply {
		mode = "applied"
	}
	logWrapper.Info("iTunes path repair completed successfully (%s)", mode)
	return nil
}
