// file: internal/plugins/itunes/path_reconcile.go
// version: 1.0.0
// guid: d0e1f2a3-b4c5-6d7e-8f9a-0b1c2d3e4f5a
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

// pathReconcileRequest is the parameter type for itunes.path-reconcile.
type pathReconcileRequest struct {
	// No specific parameters needed; service uses defaults
}

func (p *Plugin) pathReconcileDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "itunes.path-reconcile",
		Plugin:          "itunes",
		DisplayName:     "iTunes Path Reconciliation",
		Description:     "Reconcile iTunes file paths with organized library",
		DefaultPriority: sdk.PriorityNormal,
		Phases: []sdk.Phase{
			{Name: "load_tracks"},
			{Name: "match_paths"},
			{Name: "write_results"},
		},
		ResumePolicy: sdk.ResumeRestart,
		Cancellable:  true,
		Timeout:      2 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
		},
		Run: p.pathReconcileRun,
	}
}

func (p *Plugin) pathReconcileRun(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	if p.svc == nil || !p.svc.Enabled() {
		return errors.New("iTunes service not available or disabled")
	}

	// Create a logger wrapper that implements logger.Logger and delegates to the SDK reporter
	logWrapper := NewLoggerWrapper(reporter)
	logWrapper.Info("Starting iTunes path reconciliation")

	// Wrap the SDK reporter to provide a compatible progress interface
	progressReporter := &progressAdapter{reporter: reporter}

	// Call the Paths service's Reconcile method
	// Using empty operation ID since the reporter should handle checkpoints
	err := p.svc.Paths.Reconcile(ctx, "", progressReporter)
	if err != nil {
		logWrapper.Error("iTunes path reconciliation failed: %v", err)
		return fmt.Errorf("path reconciliation: %w", err)
	}

	logWrapper.Info("iTunes path reconciliation completed successfully")
	return nil
}
