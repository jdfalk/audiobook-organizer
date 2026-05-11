// file: internal/server/scheduler_maintenance.go
// version: 1.1.0
// guid: 8822f62e-ed51-4df4-b9d1-4aa41f62139a
// last-edited: 2026-05-02

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	ulid "github.com/oklog/ulid/v2"
)

// --- Maintenance Window ---

// maintenanceCtxKey is a typed context key to avoid string-key collisions.
type maintenanceCtxKey string

const ignoreWindowKey maintenanceCtxKey = "ignore_window"

// isInMaintenanceWindowAt checks if a given hour falls within the configured window.
// Supports midnight-spanning windows (e.g., start=23, end=2).
func isInMaintenanceWindowAt(hour int) bool {
	if !config.AppConfig.MaintenanceWindowEnabled {
		return false
	}
	start := config.AppConfig.MaintenanceWindowStart
	end := config.AppConfig.MaintenanceWindowEnd

	if start < end {
		return hour >= start && hour < end
	}
	// Midnight spanning: e.g., start=23, end=2 → 23,0,1 are in window
	return hour >= start || hour < end
}

// isInMaintenanceWindow checks if the current time falls within the configured window.
func isInMaintenanceWindow() bool {
	return isInMaintenanceWindowAt(time.Now().Hour())
}

// loadLastMaintenanceRun reads the persisted last-run date from the database.
func (ts *TaskScheduler) loadLastMaintenanceRun() {
	store := ts.server.Store()
	if store == nil {
		return
	}
	setting, err := store.GetSetting("maintenance_window_last_run")
	if err != nil || setting == nil {
		return
	}
	t, err := time.Parse("2006-01-02", setting.Value)
	if err != nil {
		return
	}
	ts.lastMaintenanceRun = t
}

// saveLastMaintenanceRun persists today's date as the last-run date.
func (ts *TaskScheduler) saveLastMaintenanceRun() {
	store := ts.server.Store()
	if store == nil {
		return
	}
	today := time.Now().Format("2006-01-02")
	_ = store.SetSetting("maintenance_window_last_run", today, "string", false)
	ts.lastMaintenanceRun = time.Now()
}

// GetLastMaintenanceRunDate returns the last-run date as "2006-01-02", or "" if never run.
func (ts *TaskScheduler) GetLastMaintenanceRunDate() string {
	if ts.lastMaintenanceRun.IsZero() {
		return ""
	}
	return ts.lastMaintenanceRun.Format("2006-01-02")
}

// IsMaintenanceRunning returns true if a maintenance-window operation is active.
func (ts *TaskScheduler) IsMaintenanceRunning() bool {
	store := ts.server.Store()
	if store == nil {
		return false
	}
	ops, _, err := store.ListOperations(20, 0)
	if err != nil {
		return false
	}
	for _, op := range ops {
		if op.Type == "maintenance-window" && (op.Status == "running" || op.Status == "pending") {
			return true
		}
	}
	return false
}

// hasRunToday checks if the maintenance window has already run today.
func (ts *TaskScheduler) hasRunToday() bool {
	today := time.Now().Format("2006-01-02")
	return ts.lastMaintenanceRun.Format("2006-01-02") == today
}

// isTaskRunning checks if a task's operation is currently in progress.
func (ts *TaskScheduler) isTaskRunning(name string) bool {
	store := ts.server.Store()
	if store == nil {
		return false
	}
	ops, _, err := store.ListOperations(100, 0)
	if err != nil {
		return false
	}
	opTypeMap := map[string]string{
		"library_scan": "scan", "library_organize": "organize",
		"dedup_refresh": "author-dedup-scan", "dedup_llm_review": "dedup-llm-review", "series_prune": "series-prune",
		"isbn_enrichment":   "isbn-enrichment",
		"author_split_scan": "author-split-scan", "db_optimize": "db-optimize",
		"purge_deleted": "purge-deleted", "tombstone_cleanup": "tombstone-cleanup",
		"reconcile_scan": "reconcile_scan", "purge_old_logs": "purge_old_logs",
		"cleanup_old_backups": "cleanup-old-backups",
		"metadata_refresh":    "metadata-refresh",
	}
	opType, ok := opTypeMap[name]
	if !ok {
		return false
	}
	for _, op := range ops {
		if op.Type == opType && (op.Status == "running" || op.Status == "pending") {
			return true
		}
	}
	return false
}

// RunMaintenanceWindow enqueues the maintenance-window operation via the v2 registry.
// Step 1: auto-update (if enabled). Step 2+: maintenance tasks in fixed order.
func (ts *TaskScheduler) RunMaintenanceWindow(ctx context.Context) error {
	store := ts.server.Store()
	if store == nil {
		return fmt.Errorf("database not initialized")
	}
	if ts.server.opRegistry == nil {
		return fmt.Errorf("operation registry not initialized")
	}

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "maintenance-window", nil); err != nil {
		return fmt.Errorf("failed to create maintenance-window operation: %w", err)
	}

	// Mark as run NOW to prevent the 60s ticker from re-enqueuing
	// while the async operation is still running.
	ts.saveLastMaintenanceRun()

	ignoreWindow := ctx.Value(ignoreWindowKey) != nil
	if _, err := ts.server.opRegistry.EnqueueOp(context.Background(), "maintenance.window", maintenanceWindowOpParams{
		LegacyOpID:   opID,
		IgnoreWindow: ignoreWindow,
	}); err != nil {
		return fmt.Errorf("failed to enqueue maintenance-window: %w", err)
	}
	return nil
}

