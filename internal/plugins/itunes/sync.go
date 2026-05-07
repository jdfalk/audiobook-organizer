// file: internal/plugins/itunes/sync.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-5c6d-8e9f-0a1b2c3d4e5f
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

// syncRequest is the parameter type for itunes.sync.
type syncRequest struct {
	LibraryPath  string                     `json:"library_path,omitempty"`
	PathMappings []itunesservice.PathMapping `json:"path_mappings,omitempty"`
	Force        bool                       `json:"force,omitempty"`
}

func (p *Plugin) syncDef() sdk.OperationDef {
	schedule := "0 4 * * *" // 4 AM daily
	return sdk.OperationDef{
		ID:              "itunes.sync",
		Plugin:          "itunes",
		DisplayName:     "iTunes Library Sync",
		Description:     "Synchronize changes with iTunes/Music library",
		DefaultPriority: sdk.PriorityNormal,
		ResumePolicy:    sdk.ResumeRequeue,
		Schedule:        &schedule,
		Timeout:         2 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
		},
		Run: p.syncRun,
	}
}

func (p *Plugin) syncRun(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	var req syncRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// If params are empty or invalid, use defaults
		req = syncRequest{
			LibraryPath:  "",
			PathMappings: []itunesservice.PathMapping{},
			Force:        false,
		}
	}

	if p.svc == nil || !p.svc.Enabled() {
		return errors.New("iTunes service not available or disabled")
	}

	// Create a logger wrapper that implements logger.Logger and delegates to the SDK reporter
	logWrapper := NewLoggerWrapper(reporter)
	logWrapper.Info("Starting iTunes sync")

	// Create the iTunes import request for sync
	svcReq := itunesservice.ImportRequest{
		LibraryPath:      req.LibraryPath,
		ImportMode:       "import", // sync mode
		PreserveLocation: false,
		ImportPlaylists:  false,
		SkipDuplicates:   false,
		FetchMetadata:    false,
		PathMappings:     req.PathMappings,
	}

	// Call the iTunes service's Execute method
	err := p.svc.Importer.Execute(ctx, "", svcReq, logWrapper)
	if err != nil {
		logWrapper.Error("iTunes sync failed: %v", err)
		return fmt.Errorf("iTunes sync: %w", err)
	}

	logWrapper.Info("iTunes sync completed successfully")
	return nil
}
