// file: internal/server/scheduler_ops.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

// scheduler_ops wires the 4 scheduled maintenance operations into the
// UOS v2 OperationDef registry. The corresponding TriggerFns in
// scheduler_tasks.go have been reduced to the hybrid pattern: they still
// create a v1 op record (so callers can poll v1 status) but delegate
// execution to opRegistry.EnqueueOp.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/reconcile"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
)

// maintenanceOpParams is shared by all 4 scheduler maintenance ops.
// The single field carries the v1 operation ID created by the TriggerFn so
// the Run func can store results back via store.UpdateOperationResultData.
type maintenanceOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// RegisterReconcileScanOp registers the maintenance.reconcile-scan OperationDef.
func (s *Server) RegisterReconcileScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "maintenance.reconcile-scan",
		Plugin:          "maintenance",
		DisplayName:     "Reconcile Scan",
		Description:     "Find books with missing files and match them to untracked files on disk.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "maintenance.reconcile-scan",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p maintenanceOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("reconcile-scan: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("reconcile-scan: database not initialized")
			}

			progress := registryProgressAdapter{r: reporter}
			reconcileLog := logger.New("reconcile_scan")
			result, scanErr := reconcile.BuildReconcilePreviewWithProgress(store, reconcileLog)
			if scanErr != nil {
				return fmt.Errorf("reconcile scan failed: %w", scanErr)
			}
			resultJSON, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal scan results: %w", marshalErr)
			}
			if err := store.UpdateOperationResultData(p.LegacyOpID, string(resultJSON)); err != nil {
				return fmt.Errorf("failed to store scan results: %w", err)
			}
			summary := fmt.Sprintf("Found %d broken records, %d matches, %d unmatched",
				len(result.BrokenRecords), len(result.Matches), len(result.UnmatchedBooks))
			_ = progress.Log("info", summary, nil)
			return nil
		},
	})
}

// RegisterAIDedupBatchOp registers the maintenance.ai-dedup-batch OperationDef.
func (s *Server) RegisterAIDedupBatchOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "maintenance.ai-dedup-batch",
		Plugin:          "maintenance",
		DisplayName:     "AI Author Dedup Batch",
		Description:     "Run AI author deduplication via OpenAI Batch API (50% cheaper, up to 24h).",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         25 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "maintenance.ai-dedup-batch",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapNetworkOpenAI},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p maintenanceOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("ai-dedup-batch: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("ai-dedup-batch: database not initialized")
			}

			progress := registryProgressAdapter{r: reporter}
			opID := p.LegacyOpID

			parser := ai.NewOpenAIParser(&config.AppConfig, config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
			if !parser.IsEnabled() {
				return fmt.Errorf("AI parsing is not enabled")
			}

			_ = progress.Log("info", "Building author list for batch AI dedup", nil)
			_ = progress.UpdateProgress(0, 100, "Loading authors...")

			allAuthors, err := store.GetAllAuthors()
			if err != nil {
				return fmt.Errorf("failed to get authors: %w", err)
			}

			var inputs []ai.AuthorDiscoveryInput
			for _, author := range allAuthors {
				var sampleTitles []string
				books, bErr := store.GetBooksByAuthorIDWithRole(author.ID)
				if bErr == nil {
					for j, b := range books {
						if j >= 3 {
							break
						}
						sampleTitles = append(sampleTitles, b.Title)
					}
				}
				inputs = append(inputs, ai.AuthorDiscoveryInput{
					ID: author.ID, Name: author.Name,
					BookCount: len(books), SampleTitles: sampleTitles,
				})
			}

			if len(inputs) == 0 {
				_ = progress.Log("info", "No authors to process", nil)
				return nil
			}

			_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Submitting %d authors to OpenAI Batch API...", len(inputs)))

			batchID, err := parser.CreateBatchAuthorDedup(ctx, inputs)
			if err != nil {
				return fmt.Errorf("failed to create batch: %w", err)
			}

			_ = progress.Log("info", fmt.Sprintf("Batch created: %s — polling for completion", batchID), nil)

			// Poll for completion (up to 24h, check every 5 min)
			pollInterval := 5 * time.Minute
			maxPolls := 288 // 24h / 5min
			for i := 0; i < maxPolls; i++ {
				if progress.IsCanceled() {
					return fmt.Errorf("cancelled")
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(pollInterval):
				}

				status, outputFileID, sErr := parser.CheckBatchStatus(ctx, batchID)
				if sErr != nil {
					_ = progress.Log("warn", fmt.Sprintf("Poll error: %v", sErr), nil)
					continue
				}

				_ = progress.UpdateProgress(10+i, maxPolls, fmt.Sprintf("Batch status: %s", status))

				switch status {
				case "completed":
					_ = progress.Log("info", "Batch completed, downloading results", nil)
					discoveries, dErr := parser.DownloadBatchResults(ctx, outputFileID)
					if dErr != nil {
						return fmt.Errorf("failed to download results: %w", dErr)
					}
					resultPayload := map[string]any{
						"mode":        "batch-full",
						"suggestions": discoveries,
						"batch_id":    batchID,
					}
					resultJSON, jErr := json.Marshal(resultPayload)
					if jErr != nil {
						return fmt.Errorf("failed to marshal results: %w", jErr)
					}
					if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
						return fmt.Errorf("failed to store results: %w", err)
					}
					_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Batch complete: %d suggestions", len(discoveries)))
					return nil

				case "failed", "expired", "cancelled":
					return fmt.Errorf("batch %s: %s", batchID, status)
				}
			}
			return fmt.Errorf("batch timed out after 24h")
		},
	})
}

// RegisterPurgeOldLogsOp registers the maintenance.purge-old-logs OperationDef.
func (s *Server) RegisterPurgeOldLogsOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "maintenance.purge-old-logs",
		Plugin:          "maintenance",
		DisplayName:     "Purge Old Logs",
		Description:     "Prune operation logs and system activity logs older than the configured retention period.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "maintenance.purge-old-logs",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			store := s.Store()
			if store == nil {
				return fmt.Errorf("purge-old-logs: database not initialized")
			}
			retLog := logger.New("purge_old_logs")
			_, err := logger.PruneOldLogs(store, config.AppConfig.LogRetentionDays, retLog)
			return err
		},
	})
}

// RegisterCleanupActivityLogOp registers the maintenance.cleanup-activity-log OperationDef.
func (s *Server) RegisterCleanupActivityLogOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "maintenance.cleanup-activity-log",
		Plugin:          "maintenance",
		DisplayName:     "Cleanup Activity Log",
		Description:     "Summarize old change entries and prune old debug entries from the activity log.",
		DefaultPriority: opsregistry.PriorityLow,
		Cancellable:     false,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "maintenance.cleanup-activity-log",
		Permissions:     []auth.Permission{auth.PermSettingsManage},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			if s.activityService == nil {
				return nil
			}

			// Step 1: Compact old entries into daily digests
			compactionDays := config.AppConfig.ActivityLogCompactionDays
			if compactionDays <= 0 {
				compactionDays = 14
			}
			compactionCutoff := time.Now().AddDate(0, 0, -compactionDays)
			compacted, err := s.activityService.CompactByDay(ctx, compactionCutoff)
			if err != nil {
				return fmt.Errorf("compact activity: %w", err)
			}

			// Step 2: Summarize remaining old change entries
			changeDays := config.AppConfig.ActivityLogRetentionChangeDays
			if changeDays <= 0 {
				changeDays = 90
			}
			changeCutoff := time.Now().AddDate(0, 0, -changeDays)
			summarized, err := s.activityService.Summarize(ctx, changeCutoff, "change")
			if err != nil {
				return fmt.Errorf("summarize activity: %w", err)
			}

			// Step 3: Prune old debug entries
			debugDays := config.AppConfig.ActivityLogRetentionDebugDays
			if debugDays <= 0 {
				debugDays = 30
			}
			debugCutoff := time.Now().AddDate(0, 0, -debugDays)
			pruned, err := s.activityService.Prune(debugCutoff, "debug")
			if err != nil {
				return fmt.Errorf("prune activity: %w", err)
			}

			log.Printf("Activity log cleanup: compacted %d days (%d entries), summarized %d, pruned %d",
				compacted.DaysCompacted, compacted.EntriesDeleted, summarized, pruned)
			return nil
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterReconcileScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterAIDedupBatchOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterPurgeOldLogsOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterCleanupActivityLogOp(reg) })
}
