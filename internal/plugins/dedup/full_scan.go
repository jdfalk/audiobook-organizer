// file: internal/plugins/dedup/full_scan.go
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-06-10

// T018: full_scan.go enforces phase ordering for the full dedup scan:
//
//  1. Hygiene: purge stale candidates (always).
//  2. Index (embedding+exact): FullScan via engine.
//  3. LSH candidates: CollectLSHAcoustID — only runs when the LSH index
//     exists (lsh_index_v1_done flag). When absent, a log line explains
//     how to enable it.
//
// The LSH gate in runFullScan is an op-level assertion complementing the
// collector-level gate already present in CollectLSHAcoustID. The op-level
// check lets the reporter surface a user-visible skip message in the
// operation log so operators know the LSH phase was skipped and why.

package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// LSHFlagStore is the narrow interface used by runFullScan to assert whether
// the LSH index has been built before emitting the LSH-phase log line.
// *database.PebbleStore satisfies this interface. Other store implementations
// (SQLite, mocks) that do not carry an LSH index should return false.
type LSHFlagStore interface {
	IsLSHIndexBuilt() bool
}

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

	// Phase 1 — Hygiene: purge stale candidates before the scan so the
	// index phase starts with a clean candidate table.
	startProg := sdk.NewProgress(reporter, 0)
	startProg.Start("Purging stale candidates...")
	if deleted, err := p.engine.PurgeStaleCandidates(ctx); err != nil {
		reporter.Logger().Error("purge stale candidates error", "error", err)
	} else if deleted > 0 {
		reporter.Logger().Info("purged stale candidates before scan", "count", deleted)
	}

	// Phase 2 — Index: run exact + embedding collectors for every primary book.
	// FullScan reports progress via a callback (done, total). Build the real
	// Progress on the first callback once we know N.
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

	// Phase 3 — LSH candidates: assert that the LSH index has been built
	// before the engine's CollectLSHAcoustID collector runs. The collector
	// already self-gates (it calls IsLSHIndexBuilt() internally), but we
	// surface the skip reason here so it appears in the operation log and
	// is visible to operators.
	if flagStore, ok := p.store.(LSHFlagStore); ok {
		if !flagStore.IsLSHIndexBuilt() {
			reporter.Logger().Info(
				"full-scan: LSH phase skipped — index not yet built",
				"hint", "run dedup.lsh-index-build to enable sub-linear AcoustID matching",
			)
		}
		// If built, no-op: the engine's FullScan already invoked the LSH
		// collector via runUnifiedScoringForBook for each book.
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
