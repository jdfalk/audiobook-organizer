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

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
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

	_ = reporter.UpdateProgress(0, 100, "Purging stale candidates...")
	if deleted, err := p.engine.PurgeStaleCandidates(ctx); err != nil {
		reporter.Logger().Error("purge stale candidates error", "error", err)
	} else if deleted > 0 {
		reporter.Logger().Info("purged stale candidates before scan", "count", deleted)
	}

	// FullScan reports progress via a callback (done, total). Translate
	// that into ProgressReporter updates, reserving 5% at the start for
	// the purge pass and 5% at the end for the "complete" line so the
	// bar actually moves all the way to 100.
	var lastPct int
	fullScanErr := p.engine.FullScan(ctx, func(done, total int) {
		if total <= 0 {
			return
		}
		pct := 5 + (90 * done / total)
		if pct == lastPct {
			return
		}
		lastPct = pct
		_ = reporter.UpdateProgress(pct, 100, fmt.Sprintf("Scanning books: %d / %d", done, total))
	})
	if fullScanErr != nil {
		reporter.Logger().Error("FullScan error", "error", fullScanErr)
		return fmt.Errorf("dedup scan: %w", fullScanErr)
	}

	// Fetch final candidate counts for the completion message.
	pendingCount := 0
	if p.embeddingStore != nil {
		filter := database.CandidateFilter{EntityType: "book", Status: "pending", Limit: 1}
		if _, total, listErr := p.embeddingStore.ListCandidates(filter); listErr == nil {
			pendingCount = total
		}
	}
	_ = reporter.UpdateProgress(100, 100,
		fmt.Sprintf("Dedup scan complete — %d pending candidates", pendingCount))
	return nil
}
