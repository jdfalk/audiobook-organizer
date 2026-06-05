// file: internal/plugins/dedup/embed_async.go
// version: 1.0.0
// guid: b1c2d3e4-f5a6-7890-bcde-f01234567890
// last-edited: 2026-05-17

package dedup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) embedAsyncDef() sdk.OperationDef {
	sched := "0 3 * * *"
	return sdk.OperationDef{
		ID:              "dedup.embed-async",
		Plugin:          "dedup",
		DisplayName:     "Embed books async (batch API)",
		Description:     "Submits all un-embedded books to the OpenAI Batch API. Results arrive within 24 hours and are ingested automatically.",
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

func (p *Plugin) runEmbedAsync(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return errors.New("dedup engine not available (embedding may be disabled or API key not configured)")
	}

	collectProg := sdk.NewProgress(reporter, 0)
	collectProg.Start("Collecting un-embedded books...")

	batchID, count, err := p.engine.EmbedBooksAsync(ctx)
	if err != nil {
		return fmt.Errorf("submit embedding batch: %w", err)
	}
	if count == 0 {
		collectProg.Done("All books already embedded — nothing to submit")
		return nil
	}

	prog := sdk.NewProgress(reporter, count)
	prog.Start(fmt.Sprintf("Submitting %d books to batch API...", count))
	prog.StepN(count, fmt.Sprintf("Submitted %d / %d books", count, count))
	prog.Finalize("writing results...")
	prog.Done(fmt.Sprintf("Submitted %d books to OpenAI Batch API (batch_id=%s). Results will be ingested automatically within 24h.", count, batchID))
	return nil
}
