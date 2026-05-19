// file: internal/plugins/maintenance/batch_poller.go
// version: 1.0.1
// guid: c9d0e1f2-a3b4-5678-2345-890123456789
// last-edited: 2026-05-07

package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jdfalk/audiobook-organizer/pkg/plugin/sdk"
)

func (p *Plugin) batchPollerDef() sdk.OperationDef {
	sched := "*/5 * * * *" // every 5 minutes
	return sdk.OperationDef{
		ID:              "maintenance.batch-poller",
		Plugin:          "maintenance",
		DisplayName:     "OpenAI batch poller",
		Description:     "Polls OpenAI for completed batch jobs and routes results to the appropriate handler.",
		ResumePolicy:    sdk.ResumeDrop,
		DefaultPriority: sdk.PriorityLow,
		ConcurrencyKey:  "maintenance.batch-poller",
		Cancellable:     false,
		Isolate:         false,
		Timeout:         5 * time.Minute,
		Schedule:        &sched,
		Capabilities:    []sdk.Capability{sdk.CapLibraryRead, sdk.CapLibraryWrite, sdk.CapNetworkOpenAI},
		Run:             p.runBatchPoller,
	}
}

func (p *Plugin) runBatchPoller(ctx context.Context, _ json.RawMessage, reporter sdk.Reporter) error {
	if !p.deps.HasBatchPoller() {
		_ = reporter.Log(slog.LevelInfo, "Batch poller not configured, skipping")
		return nil
	}
	processed, err := p.deps.PollBatch(ctx)
	if err != nil {
		slog.Warn("batch-poller", "err", err)
	}
	if processed > 0 {
		msg := fmt.Sprintf("Processed %d completed batches", processed)
		slog.Info("batch-poller", "msg", msg)
		_ = reporter.Log(slog.LevelInfo, msg)
	}
	return nil
}
