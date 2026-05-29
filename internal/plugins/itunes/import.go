// file: internal/plugins/itunes/import.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012
// last-edited: 2026-05-15

package itunes

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

//nolint:unused // operation definition stub for plugin registration
func (p *Plugin) importDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:                    "itunes.import",
		Plugin:                "itunes",
		DisplayName:           "iTunes Library Import",
		Description:           "Import audiobooks from the iTunes/Music library into the organizer.",
		Isolate:               true, // runs in subprocess (re-exec) — wired via cmd.RunOperationRunner
		ResumePolicy:          sdk.ResumeRestart,
		DefaultPriority:       sdk.PriorityNormal,
		Cancellable:           true,
		Timeout:               240 * time.Minute,
		ConcurrencyKey:        "itunes.import",
		MinCheckpointInterval: 30 * time.Second,
		Run:                   p.runImport,
	}
}

//nolint:unused // no-op run stub for future itunes.import operation
func (p *Plugin) runImport(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	// TODO: Implement iTunes import operation.
	// This should handle parameterized imports (genre, selection, etc.).
	return nil
}

// Ensure methods are referenced so staticcheck doesn't flag them as unused (U1000).
var _ = []interface{}{(*Plugin).importDef, (*Plugin).runImport}
