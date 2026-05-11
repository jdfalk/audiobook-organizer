// file: internal/server/operations_handlers.go
// version: 2.6.0
// guid: 9326aa39-ca40-4db3-a3be-7e76e6e2a23f
//
// Background-operation HTTP handlers split out of server.go: the
// long-running scan / organize / transcode starters, generic
// operation status/cancel/listing/revert, maintenance chores
// (optimize DB, sweep tombstones, audit, clear stale, delete
// history), and the task scheduler endpoints.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/scheduler"
	"github.com/jdfalk/audiobook-organizer/internal/sweep"
)

func (s *Server) startScan(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operations registry not initialized")
		return
	}
	body, _ := c.GetRawData()
	if len(body) == 0 {
		body = []byte("{}")
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "library.scan", body)
	if err != nil {
		httputil.InternalError(c, "enqueue failed", err)
		return
	}
	c.JSON(202, gin.H{"op_id": opID, "id": opID})
}

func (s *Server) startOrganize(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operations registry not initialized")
		return
	}
	body, _ := c.GetRawData()
	if len(body) == 0 {
		body = []byte("{}")
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "library.organize", body)
	if err != nil {
		httputil.InternalError(c, "enqueue failed", err)
		return
	}
	c.JSON(202, gin.H{"op_id": opID, "id": opID})
}

func (s *Server) startTranscode(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operations registry not initialized")
		return
	}
	body, _ := c.GetRawData()
	var check struct {
		BookID string `json:"book_id"`
	}
	if err := json.Unmarshal(body, &check); err != nil || check.BookID == "" {
		httputil.RespondWithBadRequest(c, "book_id is required")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "library.transcode", body)
	if err != nil {
		httputil.InternalError(c, "enqueue failed", err)
		return
	}
	c.JSON(202, gin.H{"op_id": opID, "id": opID})
}

func (s *Server) getOperationStatus(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	id := c.Param("id")
	op, err := s.Store().GetOperationByID(id)
	if err != nil || op == nil {
		httputil.RespondWithNotFound(c, "operation", id)
		return
	}
	httputil.RespondWithOK(c, op)
}

func (s *Server) cancelOperation(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	id := c.Param("id")

	// Check if this is an AI scan operation — cancel via pipeline manager
	if s.pipelineManager != nil && s.aiScanStore != nil {
		scans, _ := s.aiScanStore.ListScans()
		for _, scan := range scans {
			if scan.OperationID == id {
				if err := s.pipelineManager.CancelScan(scan.ID); err != nil {
					log.Printf("[cancelOperation] AI scan %d cancel warning: %v", scan.ID, err)
				}
				httputil.RespondWithNoContent(c)
				return
			}
		}
	}

	// Try cancel via v2 registry (running and queued v2 ops).
	if s.opRegistry != nil {
		if err := s.opRegistry.Cancel(id); err == nil {
			httputil.RespondWithNoContent(c)
			return
		}
	}

	// Fallback: force-update DB status (e.g., stale after restart)
	if dbErr := s.Store().UpdateOperationStatus(id, "canceled", 0, 0, "force canceled (stale operation)"); dbErr != nil {
		httputil.InternalError(c, "failed to cancel operation", dbErr)
		return
	}
	httputil.RespondWithNoContent(c)
}

// clearStaleOperations force-marks all pending/running/queued operations as failed.
func (s *Server) clearStaleOperations(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	ops, err := s.Store().GetRecentOperations(500)
	if err != nil {
		httputil.InternalError(c, "failed to get operations", err)
		return
	}

	cleared := 0
	for _, op := range ops {
		if op.Status == "pending" || op.Status == "running" || op.Status == "queued" {
			_ = s.Store().UpdateOperationStatus(op.ID, "failed", 0, 0, "force cleared by user")
			cleared++
		}
	}

	httputil.RespondWithOK(c, gin.H{"cleared": cleared})
}

// deleteOperationHistory deletes operations matching the given status(es).
// Query param: ?status=completed or ?status=failed or ?status=completed,failed
func (s *Server) deleteOperationHistory(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	statusParam := c.Query("status")
	if statusParam == "" {
		httputil.RespondWithBadRequest(c, "status parameter required")
		return
	}

	statuses := strings.Split(statusParam, ",")
	// Only allow deleting terminal statuses
	allowed := map[string]bool{"completed": true, "failed": true, "canceled": true}
	for _, s := range statuses {
		if !allowed[s] {
			httputil.RespondWithBadRequest(c, fmt.Sprintf("cannot delete operations with status %q", s))
			return
		}
	}

	deleted, err := s.Store().DeleteOperationsByStatus(statuses)
	if err != nil {
		httputil.InternalError(c, "failed to delete operations", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{"deleted": deleted})
}

// optimizeDatabase splits &-delimited author/narrator strings and re-extracts empty media info.
func (s *Server) optimizeDatabase(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	books, err := s.Store().GetAllBooks(10000, 0)
	if err != nil {
		httputil.InternalError(c, "failed to get audiobooks", err)
		return
	}

	authorsSplit := 0
	narratorsSplit := 0

	for _, book := range books {
		// Split compound author names into individual book_authors
		if book.AuthorID != nil {
			author, err := s.Store().GetAuthorByID(*book.AuthorID)
			if err == nil && author != nil && strings.Contains(author.Name, " & ") {
				names := splitMultipleNames(author.Name)
				if len(names) > 1 {
					var bookAuthors []database.BookAuthor
					for _, name := range names {
						a, err := s.Store().GetAuthorByName(name)
						if err != nil || a == nil {
							a, err = s.Store().CreateAuthor(name)
							if err != nil {
								continue
							}
						}
						bookAuthors = append(bookAuthors, database.BookAuthor{
							AuthorID: a.ID,
							Role:     "author",
						})
					}
					if len(bookAuthors) > 0 {
						if err := s.Store().SetBookAuthors(book.ID, bookAuthors); err == nil {
							authorsSplit++
						}
					}
				}
			}
		}

		// Split compound narrator names into individual book_narrators
		if book.Narrator != nil && strings.Contains(*book.Narrator, " & ") {
			names := splitMultipleNames(*book.Narrator)
			if len(names) > 1 {
				var bookNarrators []database.BookNarrator
				for _, name := range names {
					n, err := s.Store().GetNarratorByName(name)
					if err != nil || n == nil {
						n, err = s.Store().CreateNarrator(name)
						if err != nil {
							continue
						}
					}
					bookNarrators = append(bookNarrators, database.BookNarrator{
						NarratorID: n.ID,
					})
				}
				if len(bookNarrators) > 0 {
					if err := s.Store().SetBookNarrators(book.ID, bookNarrators); err == nil {
						narratorsSplit++
					}
				}
			}
		}
	}

	httputil.RespondWithOK(c, gin.H{
		"books_processed": len(books),
		"authors_split":   authorsSplit,
		"narrators_split": narratorsSplit,
	})
}

func (s *Server) sweepTombstones(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	result, err := sweep.SweepTombstones(s.Store())
	if err != nil {
		httputil.InternalError(c, "failed to sweep tombstones", err)
		return
	}
	httputil.RespondWithOK(c, result)
}

// setInternalFlag sets an arbitrary internal settings flag in PebbleDB.
// Useful for injecting skip/done flags without direct DB access.
func (s *Server) setInternalFlag(c *gin.Context) {
	var req struct {
		Key   string `json:"key" binding:"required"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if err := s.Store().SetSetting(req.Key, req.Value, "string", false); err != nil {
		httputil.InternalError(c, "failed to set flag", err)
		return
	}
	log.Printf("[INFO] setInternalFlag: %s = %q", req.Key, req.Value)
	httputil.RespondWithOK(c, gin.H{"key": req.Key, "value": req.Value})
}

func (s *Server) auditFileConsistency(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	result, err := sweep.AuditFileConsistency(s.Store())
	if err != nil {
		httputil.InternalError(c, "failed to audit file consistency", err)
		return
	}
	httputil.RespondWithOK(c, result)
}

// listActiveOperations returns a snapshot of currently queued/running operations with basic progress
func (s *Server) listOperations(c *gin.Context) {
	params := httputil.ParsePaginationParams(c)
	store := s.Store()
	if store == nil {
		httputil.RespondWithOK(c, gin.H{"items": []database.Operation{}, "total": 0, "limit": params.Limit, "offset": params.Offset})
		return
	}
	ops, total, err := store.ListOperations(params.Limit, params.Offset)
	if err != nil {
		httputil.InternalError(c, "failed to list operations", err)
		return
	}
	if ops == nil {
		ops = []database.Operation{}
	}
	httputil.RespondWithOK(c, gin.H{"items": ops, "total": total, "limit": params.Limit, "offset": params.Offset})
}

func (s *Server) listActiveOperations(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithOK(c, gin.H{"operations": []gin.H{}})
		return
	}
	// GetRecentOperations returns the most-recent 200 ops; filter to active states.
	// Active ops are always in the recent window so no separate indexed query is needed.
	all, err := store.GetRecentOperations(200)
	if err != nil {
		httputil.InternalError(c, "failed to list active operations", err)
		return
	}
	results := make([]gin.H, 0)
	for _, op := range all {
		if op.Status != "running" && op.Status != "queued" {
			continue
		}
		results = append(results, gin.H{
			"id":       op.ID,
			"type":     op.Type,
			"status":   op.Status,
			"progress": op.Progress,
			"total":    op.Total,
			"message":  op.Message,
		})
	}
	httputil.RespondWithOK(c, gin.H{"operations": results})
}

func (s *Server) listStaleOperations(c *gin.Context) {
	timeoutMinutes := config.AppConfig.OperationTimeoutMinutes
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30
	}
	if raw := strings.TrimSpace(c.Query("timeout_minutes")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutMinutes = parsed
		}
	}

	stale, err := s.collectStaleOperations(time.Duration(timeoutMinutes) * time.Minute)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to list stale operations")
		return
	}
	httputil.RespondWithOK(c, gin.H{
		"timeout_minutes": timeoutMinutes,
		"count":           len(stale),
		"operations":      stale,
	})
}

// getOperationLogs returns logs for a given operation
func (s *Server) getOperationLogs(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	id := c.Param("id")
	logs, err := s.Store().GetOperationLogs(id)
	if err != nil {
		httputil.InternalError(c, "failed to get operation logs", err)
		return
	}
	// Optional tail parameter for last N log lines
	if tailStr := c.Query("tail"); tailStr != "" {
		if n, convErr := strconv.Atoi(tailStr); convErr == nil && n > 0 && n < len(logs) {
			logs = logs[len(logs)-n:]
		}
	}
	httputil.RespondWithOK(c, gin.H{"items": logs, "count": len(logs)})
}

func (s *Server) getOperationResult(c *gin.Context) {
	id := c.Param("id")
	store := s.Store()
	op, err := store.GetOperationByID(id)
	if err != nil {
		httputil.InternalError(c, "failed to get operation", err)
		return
	}
	if op == nil {
		httputil.RespondWithNotFound(c, "operation", id)
		return
	}

	if op.ResultData == nil {
		httputil.RespondWithOK(c, gin.H{"result_data": nil})
		return
	}

	// Parse the JSON result data to return as structured JSON
	var resultData json.RawMessage
	if err := json.Unmarshal([]byte(*op.ResultData), &resultData); err != nil {
		httputil.RespondWithOK(c, gin.H{"result_data": *op.ResultData})
		return
	}

	httputil.RespondWithOK(c, gin.H{"result_data": resultData})
}

// getOperationChanges returns change tracking records for an operation.
func (s *Server) getOperationChanges(c *gin.Context) {
	id := c.Param("id")
	changes, err := s.Store().GetOperationChanges(id)
	if err != nil {
		httputil.InternalError(c, "failed to get operation changes", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"changes": changes})
}

// undoPreflightHandler checks for conflicts before executing an undo.
// GET /api/v1/operations/:id/undo/preflight
func (s *Server) undoPreflightHandler(c *gin.Context) {
	id := c.Param("id")
	report, err := PreflightUndoConflicts(s.Store(), id)
	if err != nil {
		httputil.InternalError(c, "failed to check conflicts", err)
		return
	}
	httputil.RespondWithOK(c, report)
}

// revertOperation undoes all changes from a given operation.
func (s *Server) revertOperation(c *gin.Context) {
	id := c.Param("id")
	revertSvc := NewRevertService(s.Store())
	if err := revertSvc.RevertOperation(id); err != nil {
		httputil.InternalError(c, "failed to revert operation", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "operation reverted successfully"})
}

// listTasks returns all registered tasks with their status and schedule.
func (s *Server) listTasks(c *gin.Context) {
	if s.scheduler == nil {
		httputil.RespondWithInternalError(c, "scheduler not initialized")
		return
	}
	httputil.RespondWithOK(c, s.scheduler.ListTasks())
}

// runTask triggers a task by name.
func (s *Server) runTask(c *gin.Context) {
	if s.scheduler == nil {
		httputil.RespondWithInternalError(c, "scheduler not initialized")
		return
	}
	name := c.Param("name")
	op, err := s.scheduler.RunTaskManual(name)
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if op == nil {
		httputil.RespondWithSuccess(c, 202, gin.H{"message": "task triggered"})
		return
	}
	httputil.RespondWithSuccess(c, 202, op)
}

// updateTaskConfig updates schedule config for a task.
func (s *Server) updateTaskConfig(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Enabled                *bool `json:"enabled"`
		IntervalMinutes        *int  `json:"interval_minutes"`
		RunOnStartup           *bool `json:"run_on_startup"`
		RunInMaintenanceWindow *bool `json:"run_in_maintenance_window"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	// Map task name to config fields and apply
	switch name {
	case "dedup_refresh":
		if req.Enabled != nil {
			config.AppConfig.ScheduledDedupRefreshEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledDedupRefreshInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledDedupRefreshOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowDedupRefresh = *req.RunInMaintenanceWindow
		}
	case "author_split_scan":
		if req.Enabled != nil {
			config.AppConfig.ScheduledAuthorSplitEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledAuthorSplitInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledAuthorSplitOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowAuthorSplit = *req.RunInMaintenanceWindow
		}
	case "db_optimize":
		if req.Enabled != nil {
			config.AppConfig.ScheduledDbOptimizeEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledDbOptimizeInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledDbOptimizeOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowDbOptimize = *req.RunInMaintenanceWindow
		}
	case "metadata_refresh":
		if req.Enabled != nil {
			config.AppConfig.ScheduledMetadataRefreshEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledMetadataRefreshInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledMetadataRefreshOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowMetadataRefresh = *req.RunInMaintenanceWindow
		}
	case "itunes_sync":
		if req.Enabled != nil {
			config.AppConfig.ITunesSyncEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ITunesSyncInterval = *req.IntervalMinutes
		}
	case "series_prune":
		if req.Enabled != nil {
			config.AppConfig.ScheduledSeriesPruneEnabled = *req.Enabled
		}
		if req.IntervalMinutes != nil {
			config.AppConfig.ScheduledSeriesPruneInterval = *req.IntervalMinutes
		}
		if req.RunOnStartup != nil {
			config.AppConfig.ScheduledSeriesPruneOnStartup = *req.RunOnStartup
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowSeriesPrune = *req.RunInMaintenanceWindow
		}
	case "purge_deleted":
		if req.IntervalMinutes != nil {
			// purge interval is fixed at 6h, but we can update retention days
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowPurgeDeleted = *req.RunInMaintenanceWindow
		}
	case "tombstone_cleanup":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowTombstoneCleanup = *req.RunInMaintenanceWindow
		}
	case "reconcile_scan":
		if req.Enabled != nil {
			config.AppConfig.ScheduledReconcileEnabled = *req.Enabled
		}
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowReconcile = *req.RunInMaintenanceWindow
		}
	case "purge_old_logs":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowPurgeOldLogs = *req.RunInMaintenanceWindow
		}
	case "library_scan":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowLibraryScan = *req.RunInMaintenanceWindow
		}
	case "library_organize":
		if req.RunInMaintenanceWindow != nil {
			config.AppConfig.MaintenanceWindowLibraryOrganize = *req.RunInMaintenanceWindow
		}
	default:
		httputil.RespondWithBadRequest(c, fmt.Sprintf("task %q config is not configurable", name))
		return
	}

	// Persist to database
	if s.Store() != nil {
		if err := config.SaveConfigToDatabase(s.Store()); err != nil {
			log.Printf("[WARN] Failed to save task config: %v", err)
		}
	}

	httputil.RespondWithOK(c, gin.H{"message": "task config updated"})
}

// runMaintenanceWindowNow triggers the full maintenance window sequence immediately.
func (s *Server) runMaintenanceWindowNow(c *gin.Context) {
	if s.scheduler == nil {
		httputil.RespondWithInternalError(c, "scheduler not initialized")
		return
	}
	ctx := context.WithValue(c.Request.Context(), scheduler.IgnoreWindowKey, true)
	if err := s.scheduler.RunMaintenanceWindow(ctx); err != nil {
		httputil.InternalError(c, "failed to run maintenance", err)
		return
	}
	httputil.RespondWithSuccess(c, 202, gin.H{"message": "maintenance window triggered"})
}

// getMaintenanceWindowStatus returns current schedule config and live running status.
func (s *Server) getMaintenanceWindowStatus(c *gin.Context) {
	if s.scheduler == nil {
		httputil.RespondWithInternalError(c, "scheduler not initialized")
		return
	}
	cfg := config.AppConfig
	httputil.RespondWithOK(c, gin.H{
		"enabled":           cfg.MaintenanceWindowEnabled,
		"window_start":      cfg.MaintenanceWindowStart,
		"window_end":        cfg.MaintenanceWindowEnd,
		"last_run_date":     s.scheduler.GetLastMaintenanceRunDate(),
		"next_run_estimate": calculateNextWindowRun(cfg.MaintenanceWindowStart),
		"currently_running": s.scheduler.IsMaintenanceRunning(),
	})
}

// calculateNextWindowRun returns the next RFC3339 timestamp when startHour occurs locally.
func calculateNextWindowRun(startHour int) string {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), startHour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Format(time.RFC3339)
}

type maintenanceWindowConfigReq struct {
	Enabled     bool `json:"enabled"`
	WindowStart int  `json:"window_start"`
	WindowEnd   int  `json:"window_end"`
}

// updateMaintenanceWindowConfig persists maintenance window schedule settings.
func (s *Server) updateMaintenanceWindowConfig(c *gin.Context) {
	var req maintenanceWindowConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if req.WindowStart < 0 || req.WindowStart > 23 || req.WindowEnd < 0 || req.WindowEnd > 23 {
		httputil.RespondWithBadRequest(c, "window_start and window_end must be 0-23")
		return
	}
	config.AppConfig.MaintenanceWindowEnabled = req.Enabled
	config.AppConfig.MaintenanceWindowStart = req.WindowStart
	config.AppConfig.MaintenanceWindowEnd = req.WindowEnd
	if s.Store() != nil {
		if err := config.SaveConfigToDatabase(s.Store()); err != nil {
			httputil.InternalError(c, "failed to save maintenance window config", err)
			return
		}
	}
	httputil.RespondWithOK(c, gin.H{"ok": true})
}
