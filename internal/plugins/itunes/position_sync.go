// file: internal/plugins/itunes/position_sync.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-fghi-456789012345
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) positionSyncDef() sdk.OperationDef {
	sched := "*/10 * * * *"
	return sdk.OperationDef{
		ID:               "itunes.position-sync",
		Plugin:           "itunes",
		DisplayName:      "iTunes Position Sync",
		Description:      "Sync reading positions between iTunes bookmarks and the app.",
		Schedule:         &sched,
		Isolate:          false,
		ResumePolicy:     sdk.ResumeRequeue,
		DefaultPriority:  sdk.PriorityNormal,
		Cancellable:      false,
		Timeout:          30 * time.Minute,
		ConcurrencyKey:   "itunes.position-sync",
		MinCheckpointInterval: 0, // Use default
		Run:              p.runPositionSync,
	}
}

func (p *Plugin) runPositionSync(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	// TODO: Implement iTunes position sync operation.
	// This should call p.svc.Positions.Sync().
	return nil
}
