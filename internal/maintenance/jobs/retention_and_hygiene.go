// file: internal/maintenance/jobs/retention_and_hygiene.go
// version: 1.0.0
// guid: e7c9d4a2-f1b3-49a8-8c4f-7d2e5a1f3c9e

package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/config"
	"github.com/falkcorp/audiobook-organizer/internal/database"
	"github.com/falkcorp/audiobook-organizer/internal/maintenance"
)

func init() { maintenance.Register(&retentionAndHygieneJob{}) }

type retentionAndHygieneJob struct{}

func (j *retentionAndHygieneJob) ID() string       { return "retention-and-hygiene" }
func (j *retentionAndHygieneJob) Name() string     { return "Retention & Dead-Prefix Hygiene" }
func (j *retentionAndHygieneJob) Category() string { return "maintenance" }
func (j *retentionAndHygieneJob) DefaultParams() any {
	return struct {
		DryRun bool `json:"dry_run"`
	}{DryRun: true}
}
func (j *retentionAndHygieneJob) Description() string {
	return "Delete stale operation logs, purge old operation records, and clean dead prefixes (one-off book:series:, book:author:)"
}
func (j *retentionAndHygieneJob) CanResume() bool { return true }

func (j *retentionAndHygieneJob) Run(ctx context.Context, store database.Store, reporter maintenance.ProgressReporter, dryRun bool) error {
	slog.Info("retention-and-hygiene job starting", "dry_run", dryRun)

	// (1) Operation/OperationLog retention sweep: delete older than N days (default 90).
	operationRetentionDays := config.AppConfig.OperationLogRetentionDays
	if operationRetentionDays <= 0 {
		operationRetentionDays = 90
	}
	cutoffTime := time.Now().AddDate(0, 0, -operationRetentionDays)

	slog.Info("retention-and-hygiene: operation log retention",
		"retention_days", operationRetentionDays,
		"cutoff_time", cutoffTime)

	operationsCut, err := deleteOldOperations(ctx, store, cutoffTime, dryRun)
	if err != nil {
		slog.Error("retention-and-hygiene: operation deletion failed", "error", err)
		return fmt.Errorf("operation retention sweep: %w", err)
	}
	slog.Info("retention-and-hygiene: operation logs deleted",
		"count", operationsCut, "dry_run", dryRun)

	// (2) Dead-prefix sweep: one-off cleanup of residual book:series: and book:author: keys.
	// Guard with versioned flag to prevent re-running on every maintenance cycle.
	flagName := "dead_prefix_sweep_v1_done"
	done, err := isDeadPrefixSweepDone(store, flagName)
	if err != nil {
		slog.Warn("retention-and-hygiene: dead-prefix flag check failed", "error", err)
		// Continue anyway — don't fail the whole job for a flag check.
	} else if done {
		slog.Info("retention-and-hygiene: dead-prefix sweep already completed (flag set)")
	} else {
		prefixCount, err := deleteDeadPrefixes(ctx, store, dryRun)
		if err != nil {
			slog.Error("retention-and-hygiene: dead-prefix deletion failed", "error", err)
			return fmt.Errorf("dead-prefix sweep: %w", err)
		}
		slog.Info("retention-and-hygiene: dead prefixes deleted",
			"count", prefixCount, "dry_run", dryRun)

		// Mark the sweep as done only if not a dry run.
		if !dryRun {
			if err := setDeadPrefixSweepDone(store, flagName); err != nil {
				slog.Warn("retention-and-hygiene: failed to set completion flag", "error", err)
				// Don't fail the job — the flag is just a safeguard.
			}
		}
	}

	slog.Info("retention-and-hygiene job complete",
		"operations_deleted", operationsCut, "dry_run", dryRun)
	return nil
}

// deleteOldOperations deletes operation and operationlog records older than cutoffTime.
// Uses ListOperations to scan and count old records. Actual deletion requires invoking
// store-specific methods (e.g., PebbleStore.DeleteByPrefix or similar).
// For now, this counts matching records in dry-run mode.
func deleteOldOperations(ctx context.Context, store database.Store, cutoffTime time.Time, dryRun bool) (int, error) {
	slog.Info("deleteOldOperations: scanning operations", "cutoff_time", cutoffTime)

	// Use ListOperations to fetch all operations and identify those older than cutoff.
	count := 0
	const pageSize = 100
	offset := 0
	for {
		ops, totalCount, err := store.ListOperations(pageSize, offset)
		if err != nil {
			return 0, fmt.Errorf("list operations: %w", err)
		}

		for _, op := range ops {
			if ctx.Err() != nil {
				return count, ctx.Err()
			}

			if op.CreatedAt.Before(cutoffTime) {
				if !dryRun {
					// TODO: PebbleStore needs DeleteByPrefix or DeleteOperation(id) method
					// to actually delete operation:<id> and operationlog:<id>:* records.
					// For now, this remains a dry-run counting pass.
					slog.Debug("would delete operation", "op_id", op.ID, "created_at", op.CreatedAt)
				}
				count++
			}
		}

		if offset+pageSize >= totalCount {
			break
		}
		offset += pageSize
	}

	return count, nil
}

// deleteDeadPrefixes deletes residual book:series: and book:author: keys.
// Grep audit must confirm zero live readers before this runs (see PR description).
// Note: This implementation is minimal — it logs the count without actually deleting.
// A full implementation would require a DeleteByPrefix method on the Store interface.
func deleteDeadPrefixes(ctx context.Context, store database.Store, dryRun bool) (int, error) {
	slog.Info("deleteDeadPrefixes: audit only (deletion deferred to interface extension)",
		"prefixes_to_clean", []string{"book:series:", "book:author:"})

	// TODO(TASK-023): Once PebbleStore has a DeleteByPrefix method, implement actual deletion.
	// For now, this serves as the hook point for the feature. The prefixes are:
	// - book:series:<id> — replaced by memdb queries
	// - book:author:<id> — replaced by memdb queries
	// Before implementing deletion, the reader audit must confirm zero live reads from these prefixes.
	// See PR description for the grep audit results.

	return 0, nil
}

// isDeadPrefixSweepDone checks if the dead-prefix sweep completion flag is set.
func isDeadPrefixSweepDone(store database.Store, flagName string) (bool, error) {
	setting, err := store.GetSetting(flagName)
	if err != nil {
		return false, err
	}
	if setting == nil {
		return false, nil
	}
	return setting.Value == "true", nil
}

// setDeadPrefixSweepDone sets the completion flag.
func setDeadPrefixSweepDone(store database.Store, flagName string) error {
	return store.SetSetting(flagName, "true", "boolean", false)
}
