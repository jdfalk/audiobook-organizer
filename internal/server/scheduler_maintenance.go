// file: internal/server/scheduler_maintenance.go
// version: 1.0.0
// guid: 8822f62e-ed51-4df4-b9d1-4aa41f62139a
// last-edited: 2026-05-02

package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
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

// RunMaintenanceWindow runs all maintenance-window-eligible tasks in order.
// Step 1: auto-update (if enabled). Step 2+: maintenance tasks in fixed order.
func (ts *TaskScheduler) RunMaintenanceWindow(ctx context.Context) error {
	store := ts.server.Store()
	if store == nil {
		return fmt.Errorf("database not initialized")
	}
	if ts.server.queue == nil {
		return fmt.Errorf("operation queue not initialized")
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "maintenance-window", nil)
	if err != nil {
		return fmt.Errorf("failed to create maintenance-window operation: %w", err)
	}

	// Mark as run NOW to prevent the 60s ticker from re-enqueuing
	// while the async operation is still running.
	ts.saveLastMaintenanceRun()

	_ = ts.server.queue.Enqueue(op.ID, "maintenance-window", operations.PriorityNormal,
		func(innerCtx context.Context, progress operations.ProgressReporter) error {
			ignoreWindow := ctx.Value(ignoreWindowKey) != nil

			// Step 1: Auto-update (if enabled and not already completed post-restart)
			if config.AppConfig.AutoUpdateEnabled {
				updateDone, _ := store.GetSetting("maintenance_window_update_completed")
				today := time.Now().Format("2006-01-02")
				if updateDone == nil || updateDone.Value != today {
					_ = progress.Log("info", "Running auto-update (step 1)", nil)
					_ = progress.UpdateProgress(0, 100, "Running auto-update...")
					_ = store.SetSetting("maintenance_window_update_completed", today, "string", false)
					if ts.server.updater != nil {
						channel := config.AppConfig.AutoUpdateChannel
						info, checkErr := ts.server.updater.CheckForUpdate(channel)
						if checkErr != nil {
							_ = progress.Log("warning", fmt.Sprintf("Auto-update check failed: %v", checkErr), nil)
						} else if info != nil && info.UpdateAvailable {
							_ = progress.Log("info", fmt.Sprintf("Update available: %s, applying...", info.LatestVersion), nil)
							if applyErr := ts.server.updater.DownloadAndReplace(info); applyErr != nil {
								_ = progress.Log("error", fmt.Sprintf("Auto-update apply failed: %v", applyErr), nil)
							} else {
								_ = progress.Log("info", "Update applied, server will restart", nil)
								go ts.server.updater.RestartSelf()
								return nil // Exit — server restarting
							}
						} else {
							_ = progress.Log("info", "No update available", nil)
						}
					}
					_ = progress.Log("info", "Auto-update step complete", nil)
				} else {
					_ = progress.Log("info", "Auto-update already completed today, skipping", nil)
				}
			}

			// Step 2+: Maintenance tasks in order
			var eligible []string
			for _, name := range ts.maintenanceOrder {
				task, ok := ts.tasks[name]
				if !ok {
					continue
				}
				if task.IsEnabled() && task.RunInMaintenanceWindow != nil && task.RunInMaintenanceWindow() {
					eligible = append(eligible, name)
				}
			}

			mwTag := "mw:" + opID
			taskSource := operations.TriggerScheduled
			if ignoreWindow {
				taskSource = operations.TriggerManual
			}
			windowStartTags := []string{activity.Scheduled, mwTag}
			if ignoreWindow {
				windowStartTags = []string{activity.AlwaysShow, mwTag}
			}

			_ = progress.Log("info", fmt.Sprintf("Maintenance window starting: %d tasks eligible: %s", len(eligible), strings.Join(eligible, ", ")), nil)
			activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, "maintenance-window",
				fmt.Sprintf("Maintenance window starting: %d tasks: %s", len(eligible), strings.Join(eligible, ", ")),
				windowStartTags...)

			type taskFailure struct{ name, errMsg string }
			var failures []taskFailure
			var skipped []string
			ran := 0

			for i, name := range eligible {
				// Check if window is still open (skip for manual "Run Now" triggers)
				if !ignoreWindow && !isInMaintenanceWindow() {
					remaining := eligible[i:]
					_ = progress.Log("warning", fmt.Sprintf("Maintenance window closed after task %d/%d, skipping: %s", i, len(eligible), strings.Join(remaining, ", ")), nil)
					skipped = append(skipped, remaining...)
					break
				}

				// Duplicate prevention: skip if already running from interval ticker
				if ts.isTaskRunning(name) {
					_ = progress.Log("info", fmt.Sprintf("Task %s already running (interval), skipping", name), nil)
					skipped = append(skipped, name)
					continue
				}

				_ = progress.UpdateProgress(i, len(eligible), fmt.Sprintf("Running task %d/%d: %s", i+1, len(eligible), name))
				_ = progress.Log("info", fmt.Sprintf("Starting maintenance task: %s", name), nil)
				ran++

				taskOp, taskErr := ts.runTask(name, taskSource)
				if taskErr != nil {
					errMsg := taskErr.Error()
					failures = append(failures, taskFailure{name, errMsg})
					_ = progress.Log("error", fmt.Sprintf("Task %s failed to start: %v", name, taskErr), nil)
					activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, name,
						fmt.Sprintf("Task %s failed to start: %s", name, errMsg),
						activity.Scheduled, mwTag)
				} else if taskOp != nil {
					// Wait for the task operation to complete before starting next
					ts.waitForOperation(innerCtx, taskOp.ID)
					completedOp, _ := store.GetOperationByID(taskOp.ID)
					if completedOp != nil && completedOp.Status == "failed" {
						errMsg := ""
						if completedOp.ErrorMessage != nil {
							errMsg = *completedOp.ErrorMessage
						}
						failures = append(failures, taskFailure{name, errMsg})
						_ = progress.Log("warning", fmt.Sprintf("Task %s operation failed: %s", name, errMsg), nil)
						activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, name,
							fmt.Sprintf("Task %s failed: %s", name, errMsg),
							windowStartTags...)
					} else {
						msg := completedOp.Message
						_ = progress.Log("info", fmt.Sprintf("Task %s completed: %s (op: %s)", name, msg, taskOp.ID), nil)
						activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, name,
							fmt.Sprintf("Task %s ok: %s", name, msg),
							windowStartTags...)
					}
				} else {
					_ = progress.Log("info", fmt.Sprintf("Task %s triggered (no operation)", name), nil)
					activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, name,
						fmt.Sprintf("Task %s triggered", name),
						windowStartTags...)
				}
			}

			summaryParts := []string{fmt.Sprintf("%d/%d tasks ran", ran, len(eligible))}
			if len(failures) > 0 {
				failNames := make([]string, len(failures))
				for i, f := range failures {
					if f.errMsg != "" {
						failNames[i] = f.name + ": " + f.errMsg
					} else {
						failNames[i] = f.name
					}
				}
				summaryParts = append(summaryParts, fmt.Sprintf("%d failed: %s", len(failures), strings.Join(failNames, "; ")))
			}
			if len(skipped) > 0 {
				summaryParts = append(summaryParts, fmt.Sprintf("%d skipped: %s", len(skipped), strings.Join(skipped, ", ")))
			}
			summary := strings.Join(summaryParts, ", ")

			if len(failures) > 0 {
				_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed with errors")
				activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, "maintenance-window",
					"Maintenance window done (errors): "+summary,
					windowStartTags...)
				return fmt.Errorf("maintenance window: %s", summary)
			}
			_ = progress.UpdateProgress(len(eligible), len(eligible), "Maintenance window completed successfully")
			activity.EmitInfo(ts.server.activityWriter, opID, activity.MaintenanceWindow, "maintenance-window",
				"Maintenance window done: "+summary,
				windowStartTags...)
			return nil
		},
	)
	return nil
}

