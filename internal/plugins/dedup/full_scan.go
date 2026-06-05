// file: internal/plugins/dedup/full_scan.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-06

package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) fullScanDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "dedup.full-scan",
		Plugin:          "dedup",
		DisplayName:     "Full dedup scan",
		Description:     "Runs a full embedding-based dedup scan, purging stale candidates first.",
		ResumePolicy:    sdk.ResumeRequeue,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "dedup.full-scan",
		Cancellable:     true,
		Isolate:         false,
		Timeout:         120 * time.Minute,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapLibraryWrite,
			sdk.CapNetworkOpenAI,
		},
		Run: p.runFullScan,
	}
}

func (p *Plugin) runFullScan(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return fmt.Errorf("dedup engine not available")
	}

	// Progress is created lazily once FullScan reports its total. Until then
	// we emit a Start frame on a zero-N progress so the bar isn't 0/0.
	startProg := sdk.NewProgress(reporter, 0)
	startProg.Start("Purging stale candidates...")
	if deleted, err := p.engine.PurgeStaleCandidates(ctx); err != nil {
		reporter.Logger().Error("purge stale candidates error", "error", err)
	} else if deleted > 0 {
		reporter.Logger().Info("purged stale candidates before scan", "count", deleted)
	}

	// FullScan reports progress via a callback (done, total). Build the
	// real Progress on the first callback once we know N.
	var prog *sdk.Progress
	fullScanErr := p.engine.FullScan(ctx, func(done, total int) {
		if total <= 0 {
			return
		}
		if prog == nil {
			prog = sdk.NewProgress(reporter, total)
			prog.Start(fmt.Sprintf("Scanning books: 0 / %d", total))
		}
		prog.StepN(done, fmt.Sprintf("Scanning books: %d / %d", done, total))
	})
	if fullScanErr != nil {
		reporter.Logger().Error("FullScan error", "error", fullScanErr)
		return fmt.Errorf("dedup scan: %w", fullScanErr)
	}

	if prog == nil {
		prog = sdk.NewProgress(reporter, 0)
		prog.Start("Scanning books: 0 / 0")
	}

	// Fetch final candidate counts for the completion message.
	pendingCount := 0
	if p.embeddingStore != nil {
		filter := database.CandidateFilter{EntityType: "book", Status: "pending", Limit: 1}
		if _, total, listErr := p.embeddingStore.ListCandidates(filter); listErr == nil {
			pendingCount = total
		}
	}
	prog.Finalize("writing results...")
	prog.Done(fmt.Sprintf("Dedup scan complete — %d pending candidates", pendingCount))
	return nil
}
