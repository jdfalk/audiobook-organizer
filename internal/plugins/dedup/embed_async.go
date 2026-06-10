// file: internal/plugins/dedup/embed_async.go
// version: 2.0.0
// guid: b1c2d3e4-f5a6-7890-bcde-f01234567890
// last-edited: 2026-06-10

// T018: embed_async.go is a thin wrapper that delegates to runEmbedScanMode
// (in embed_scan.go) with async=true.
//
// Deprecated (T018): Both op IDs remain registered for one release so that
// existing callers that trigger "dedup.embed-async" by ID continue to work.
// New callers should use dedup.embed-scan with {"async": true}.

package dedup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) embedAsyncDef() sdk.OperationDef {
	sched := "0 3 * * *"
	return sdk.OperationDef{
		ID:              "dedup.embed-async",
		Plugin:          "dedup",
		DisplayName:     "Embed books async (batch API) [deprecated — use embed-scan with async:true]",
		Description:     "Deprecated: delegates to dedup.embed-scan with async=true. Submits all un-embedded books to the OpenAI Batch API. Results arrive within 24 hours and are ingested automatically.",
		ResumePolicy:    sdk.ResumeRequeue,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "dedup.embed-async",
		Cancellable:     false,
		Isolate:         false,
		Timeout:         10 * time.Minute,
		Schedule:        &sched, // nightly at 03:00 server time
		Run:             p.runEmbedAsync,
	}
}

// runEmbedAsync delegates to runEmbedScanMode(async=true).
// Keeping this as a named method (not an inline closure) preserves the
// method expression stored in OperationDef.Run so sdk.Registry can
// identify and invoke it correctly.
func (p *Plugin) runEmbedAsync(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	return p.runEmbedScanMode(ctx, true, reporter)
}
