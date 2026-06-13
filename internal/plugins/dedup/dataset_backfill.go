// file: internal/plugins/dedup/dataset_backfill.go
// version: 1.0.2
// guid: 2d6f8a13-7c40-4e92-8b15-9a3e5c7d2f64
// last-edited: 2026-06-13

// Package dedup — op dedup.dataset-backfill (spec C4 backfill).
//
// Iterates all pending candidates, builds a LabeledExample for each, runs the
// deterministic catchers (Classify), and writes the labeled example to the
// dedup:label keyspace. With apply=true, any candidate a catcher labels
// "not_dup" is suppressed (status → "dismissed") so residual part-vs-whole /
// missing-file false positives leave the review queue.
//
// Dry-run by default: reports counts, writes nothing. The apply path is
// idempotent — UpsertLabeledExample overwrites and re-dismissing an already-
// dismissed candidate is a no-op, so re-running is safe (no done-flag needed).
//
// NOTE on suppression counts: in practice, the dominant residual class (stub /
// unscanned-file pairs with one side duration=0) is NOT caught by the
// duration-ratio or missing-file catchers when file records exist but the file
// is 0-second. The op labels and suppresses only what the catchers actually
// fire on — counters are always honest and may be lower than the total pending
// backlog. That is by design.
package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/dedup/dataset"
	"github.com/falkcorp/audiobook-organizer/pkg/plugin/sdk"
)

// datasetBackfillParams are the JSON parameters accepted by the op.
type datasetBackfillParams struct {
	// Apply, if true, writes labeled examples and suppresses not_dup candidates.
	// Default false (dry-run) — the op only reports counts, writes nothing.
	Apply bool `json:"apply"`
}

// datasetBackfillDef returns the OperationDef for dedup.dataset-backfill.
func (p *Plugin) datasetBackfillDef() sdk.OperationDef {
	return sdk.OperationDef{
		ID:          "dedup.dataset-backfill",
		Plugin:      "dedup",
		DisplayName: "Backfill dedup tuning dataset",
		Description: "Builds a labeled example per pending candidate, runs deterministic catchers, " +
			"and (apply=true) suppresses rule-labeled not_dup candidates (status → dismissed). " +
			"Dry-run by default.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityNormal,
		ConcurrencyKey:  "dedup.dataset-backfill",
		Cancellable:     true,
		Timeout:         60 * time.Minute,
		Capabilities:    []sdk.Capability{sdk.CapLibraryRead, sdk.CapLibraryWrite},
		Run:             p.runDatasetBackfill,
	}
}

// builderAdapter satisfies dataset.BuilderStore using the plugin's main store.
// dataset.BuilderStore requires GetBook(id string) and GetBookFiles(id string).
// database.Store exposes GetBookByID (not GetBook), so the adapter bridges the
// name mismatch while keeping the interface names canonical.
type builderAdapter struct{ store database.Store }

func (b builderAdapter) GetBook(id string) (*database.Book, error) {
	return b.store.GetBookByID(id)
}

func (b builderAdapter) GetBookFiles(id string) ([]database.BookFile, error) {
	return b.store.GetBookFiles(id)
}

// runDatasetBackfill implements the dedup.dataset-backfill op.
func (p *Plugin) runDatasetBackfill(ctx context.Context, rawParams json.RawMessage, reporter sdk.Reporter) error {
	if p.embeddingStore == nil {
		return fmt.Errorf("embedding store not available")
	}
	if p.store == nil {
		return fmt.Errorf("main store not available")
	}

	// --- Parse params ---
	var params datasetBackfillParams
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return fmt.Errorf("parse params: %w", err)
		}
	}

	reporter.Logger().Info("dataset-backfill start", "apply", params.Apply)

	// --- Load all pending candidates ---
	_ = reporter.UpdateProgress(0, 2, "Loading pending candidates…")
	filter := database.CandidateFilter{
		Status: "pending",
		Limit:  1_000_000,
	}
	cands, _, err := p.embeddingStore.ListCandidates(filter)
	if err != nil {
		return fmt.Errorf("list candidates: %w", err)
	}

	reporter.Logger().Info("dataset-backfill: candidates loaded", "count", len(cands))

	adapter := builderAdapter{store: p.store}

	var (
		examined   int
		labeled    int
		suppressed int
		notDup     int
		trueDup    int
		buildErrs  int
	)

	_ = reporter.UpdateProgress(1, 2, fmt.Sprintf("Processing %d candidates…", len(cands)))

	for i := range cands {
		if reporter.IsCanceled() {
			return context.Canceled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c := cands[i]
		examined++

		// Periodic progress update so the op doesn't appear frozen on large sets.
		if examined%1000 == 0 {
			_ = reporter.UpdateProgress(1, 2, fmt.Sprintf("Processed %d/%d candidates…", examined, len(cands)))
		}

		// Build feature vector for the candidate pair.
		ex, err := dataset.BuildExample(adapter, c)
		if err != nil {
			buildErrs++
			reporter.Logger().Warn("dataset-backfill: build error",
				"candidate_id", c.ID,
				"entity_a", c.EntityAID,
				"entity_b", c.EntityBID,
				"error", err)
			continue
		}

		// Run deterministic catchers.
		if label, reason, fires := dataset.Classify(ex); fires {
			ex.Label = label
			ex.LabelSource = "rule"
			ex.LabelReason = reason
			ex.DecidedAt = time.Now().UTC().Format(time.RFC3339)
			switch label {
			case "not_dup":
				notDup++
			case "true_dup":
				trueDup++
			}
		}
		// If no catcher fired, ex.Label remains "" — example is unlabeled, written
		// as-is so features are captured for future human/ML labeling.

		if params.Apply {
			// Write the labeled (or unlabeled) example to the store.
			if err := p.embeddingStore.UpsertLabeledExample(ex); err != nil {
				reporter.Logger().Error("dataset-backfill: upsert error",
					"candidate_id", c.ID, "error", err)
				// Continue — partial progress is better than aborting.
			} else {
				labeled++
			}

			// Suppress only catchers-confirmed not_dup candidates.
			if ex.Label == "not_dup" {
				if err := p.embeddingStore.UpdateCandidateStatus(c.ID, "dismissed"); err != nil {
					reporter.Logger().Error("dataset-backfill: suppress error",
						"candidate_id", c.ID, "error", err)
				} else {
					suppressed++
				}
			}
		}
	}

	summary := fmt.Sprintf(
		"examined=%d not_dup=%d true_dup=%d labeled=%d suppressed=%d build_errs=%d (apply=%v)",
		examined, notDup, trueDup, labeled, suppressed, buildErrs, params.Apply,
	)
	reporter.Logger().Info("dataset-backfill complete", "summary", summary)

	if !params.Apply {
		_ = reporter.UpdateProgress(2, 2,
			fmt.Sprintf("Dry-run complete — %d not_dup, %d true_dup of %d examined. Pass apply=true to write.", notDup, trueDup, examined))
	} else {
		_ = reporter.UpdateProgress(2, 2,
			fmt.Sprintf("Complete — %d/%d labeled, %d suppressed. %s", labeled, examined, suppressed, summary))
	}

	return nil
}
