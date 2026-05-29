// file: internal/plugins/acoustid/scan.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-def0-123456789abc
// last-edited: 2026-05-06

package acoustid

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) scanDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:              "acoustid.scan",
		Plugin:          "acoustid",
		DisplayName:     "AcoustID fingerprint scan",
		Description:     "Runs AcoustID fingerprint-based dedup scan comparing acoustic fingerprints.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "acoustid.scan",
		Isolate:         true, // uses ffmpeg/fpcalc subprocess — runs in re-exec child
		Timeout:         6 * time.Hour,
		Capabilities: []sdk.Capability{
			sdk.CapLibraryRead,
			sdk.CapFilesRead,
			sdk.CapFilesExecute,
			sdk.CapSubprocessSpawn,
		},
		Run: p.runAcoustIDScan,
	}
}

func (p *Plugin) runAcoustIDScan(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if p.engine == nil {
		return fmt.Errorf("dedup engine not available")
	}

	_ = reporter.UpdateProgress(0, 100, "Starting AcoustID fingerprint scan...")

	var lastPct int
	scanErr := p.engine.AcoustIDScan(ctx, func(done, total int) {
		if total <= 0 {
			return
		}
		pct := 1 + (98 * done / total)
		if pct == lastPct {
			return
		}
		lastPct = pct
		_ = reporter.UpdateProgress(pct, 100, fmt.Sprintf("Scanning books: %d / %d", done, total))
	})
	if scanErr != nil {
		reporter.Logger().Error("AcoustIDScan error", "error", scanErr)
		return fmt.Errorf("acoustid scan: %w", scanErr)
	}

	pendingCount := 0
	if p.embeddingStore != nil {
		filter := database.CandidateFilter{EntityType: "book", Status: "pending", Layer: "acoustid", Limit: 1}
		if _, total, listErr := p.embeddingStore.ListCandidates(filter); listErr == nil {
			pendingCount = total
		}
	}
	_ = reporter.UpdateProgress(100, 100,
		fmt.Sprintf("AcoustID scan complete — %d pending candidate(s)", pendingCount))
	return nil
}
