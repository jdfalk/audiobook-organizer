// file: internal/plugins/itunes/import.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4b5c-8d9e-0f1a2b3c4d5e
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

// importRequest is the parameter type for itunes.import.
type importRequest struct {
	LibraryPath      string                     `json:"library_path"`
	ImportMode       string                     `json:"import_mode"`
	PreserveLocation bool                       `json:"preserve_location"`
	ImportPlaylists  bool                       `json:"import_playlists"`
	SkipDuplicates   bool                       `json:"skip_duplicates"`
	FetchMetadata    bool                       `json:"fetch_metadata"`
	PathMappings     []itunesservice.PathMapping `json:"path_mappings,omitempty"`
}

func (p *Plugin) importDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "itunes.import",
		Plugin:          "itunes",
		DisplayName:     "iTunes Library Import",
		Description:     "Import audiobooks from an iTunes/Music library",
		DefaultPriority: sdk.PriorityNormal,
		Phases: []sdk.Phase{
			{Name: "parse_xml"},
			{Name: "match_books"},
			{Name: "import_tracks"},
			{Name: "post_process"},
		},
		ResumePolicy: sdk.ResumeRestart,
		Cancellable:  true,
		Timeout:       2 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapFilesRead,
		},
		Run: p.importRun,
	}
}

func (p *Plugin) importRun(ctx context.Context, params json.RawMessage, reporter sdk.Reporter) error {
	var req importRequest
	if err := json.Unmarshal(params, &req); err != nil {
		reporter.Logger().Error("failed to unmarshal import params", "error", err)
		return fmt.Errorf("unmarshal params: %w", err)
	}

	if p.svc == nil || !p.svc.Enabled() {
		return errors.New("iTunes service not available or disabled")
	}

	// Create a logger wrapper that implements logger.Logger and delegates to the SDK reporter
	logWrapper := NewLoggerWrapper(reporter)
	logWrapper.Info("Starting iTunes import from %s", req.LibraryPath)

	// Create the iTunes import request
	svcReq := itunesservice.ImportRequest{
		LibraryPath:      req.LibraryPath,
		ImportMode:       req.ImportMode,
		PreserveLocation: req.PreserveLocation,
		ImportPlaylists:  req.ImportPlaylists,
		SkipDuplicates:   req.SkipDuplicates,
		FetchMetadata:    req.FetchMetadata,
		PathMappings:     req.PathMappings,
	}

	// Call the iTunes service's Execute method
	// The service will handle all phases internally
	err := p.svc.Importer.Execute(ctx, "", svcReq, logWrapper)
	if err != nil {
		logWrapper.Error("iTunes import failed: %v", err)
		return err
	}

	logWrapper.Info("iTunes import completed successfully")
	return nil
}
