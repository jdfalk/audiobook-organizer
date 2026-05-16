// file: internal/plugins/itunes/sync.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901
// last-edited: 2026-05-07

package itunes

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) syncDef() sdk.OperationDef {
	sched := "*/30 * * * *"
	return sdk.OperationDef{
		ID:                    "itunes.sync",
		Plugin:                "itunes",
		DisplayName:           "iTunes Library Sync",
		Description:           "Sync audiobook metadata with the iTunes/Music library.",
		Schedule:              &sched,
		Isolate:               false,
		ResumePolicy:          sdk.ResumeRestart,
		DefaultPriority:       sdk.PriorityNormal,
		Cancellable:           false,
		Timeout:               120 * time.Minute,
		ConcurrencyKey:        "itunes.sync",
		MinCheckpointInterval: 30 * time.Second,
		Run:                   p.runSync,
	}
}

func (p *Plugin) runSync(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	// TODO: Implement iTunes sync operation.
	// This should call p.svc.Importer.Sync with appropriate path mappings.
	return nil
}
