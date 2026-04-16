// file: internal/server/operations_handlers.go
// version: 1.1.0
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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
	ulid "github.com/oklog/ulid/v2"
)

func (s *Server) startScan(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath  *string `json:"folder_path"`
		Priority    *int    `json:"priority"`
		ForceUpdate *bool   `json:"force_update"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := s.Store().CreateOperation(id, "scan", req.FolderPath)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function that delegates to service
	scanReq := &ScanRequest{
		FolderPath:  req.FolderPath,
		Priority:    req.Priority,
		ForceUpdate: req.ForceUpdate,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.scanService.PerformScan(ctx, scanReq, operations.LoggerFromReporter(progress))
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "scan", priority, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startOrganize(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath         *string  `json:"folder_path"`
		Priority           *int     `json:"priority"`
		BookIDs            []string `json:"book_ids"`
		FetchMetadataFirst bool     `json:"fetch_metadata_first"`
		SyncITunesFirst    bool     `json:"sync_itunes_first"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := s.Store().CreateOperation(id, "organize", req.FolderPath)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function that delegates to service
	organizeReq := &OrganizeRequest{
		FolderPath:         req.FolderPath,
		Priority:           req.Priority,
		BookIDs:            req.BookIDs,
		FetchMetadataFirst: req.FetchMetadataFirst,
		SyncITunesFirst:    req.SyncITunesFirst,
		OperationID:        op.ID,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.organizeService.PerformOrganize(ctx, organizeReq, operations.LoggerFromReporter(progress))
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "organize", priority, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startTranscode(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		BookID       string `json:"book_id"`
		OutputFormat string `json:"output_format"`
		Bitrate      int    `json:"bitrate"`
		KeepOriginal *bool  `json:"keep_original"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BookID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_id is required"})
		return
	}

	// Verify the book exists
	if _, err := s.Store().GetBookByID(req.BookID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}

	id := ulid.Make().String()
	op, err := s.Store().CreateOperation(id, "transcode", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepOriginal := true
	if req.KeepOriginal != nil {
		keepOriginal = *req.KeepOriginal
	}

	opts := transcode.TranscodeOpts{
		BookID:       req.BookID,
		OutputFormat: req.OutputFormat,
		Bitrate:      req.Bitrate,
		KeepOriginal: keepOriginal,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		outputPath, err := transcode.Transcode(ctx, opts, s.Store(), progress)
		if err != nil {
			return err
		}

		// Get the original book to preserve its data
		originalBook, err := s.Store().GetBookByID(req.BookID)
		if err != nil {
			return fmt.Errorf("failed to get original book: %w", err)
		}

		// Set up version group if not already set
		groupID := ""
		if originalBook.VersionGroupID != nil && *originalBook.VersionGroupID != "" {
			groupID = *originalBook.VersionGroupID
		} else {
			groupID = ulid.Make().String()
		}

		// Mark original as non-primary version (modify fetched book to preserve all fields)
		notPrimary := false
		origNotes := "Original format"
		originalBook.IsPrimaryVersion = &notPrimary
		originalBook.VersionGroupID = &groupID
		originalBook.VersionNotes = &origNotes
		if _, err := s.Store().UpdateBook(req.BookID, originalBook); err != nil {
			progress.Log("warn", fmt.Sprintf("Failed to update original book version info: %v", err), nil)
		}

		// Create a new book record for the M4B version
		m4bFormat := "m4b"
		aacCodec := "aac"
		bitrateVal := opts.Bitrate
		if bitrateVal <= 0 {
			bitrateVal = 128
		}
		isPrimary := true
		m4bNotes := "Transcoded to M4B"

		newBook := &database.Book{
			ID:                   ulid.Make().String(),
			Title:                originalBook.Title,
			FilePath:             outputPath,
			Format:               m4bFormat,
			Codec:                &aacCodec,
			Bitrate:              &bitrateVal,
			AuthorID:             originalBook.AuthorID,
			SeriesID:             originalBook.SeriesID,
			SeriesSequence:       originalBook.SeriesSequence,
			Duration:             originalBook.Duration,
			Narrator:             originalBook.Narrator,
			Publisher:            originalBook.Publisher,
			PrintYear:            originalBook.PrintYear,
			AudiobookReleaseYear: originalBook.AudiobookReleaseYear,
			ISBN10:               originalBook.ISBN10,
			ISBN13:               originalBook.ISBN13,
			ASIN:                 originalBook.ASIN,
			Language:             originalBook.Language,
			CoverURL:             originalBook.CoverURL,
			IsPrimaryVersion:     &isPrimary,
			VersionGroupID:       &groupID,
			VersionNotes:         &m4bNotes,
		}
		if _, err := s.Store().CreateBook(newBook); err != nil {
			// Fallback: update original in-place but preserve all existing fields
			progress.Log("warn", fmt.Sprintf("Failed to create M4B version record, updating original: %v", err), nil)
			isPrim := true
			fallbackNotes := fmt.Sprintf("Transcoded to M4B (in-place, original was at %s)", originalBook.FilePath)
			originalBook.FilePath = outputPath
			originalBook.Format = m4bFormat
			originalBook.Codec = &aacCodec
			originalBook.Bitrate = &bitrateVal
			originalBook.IsPrimaryVersion = &isPrim
			originalBook.VersionGroupID = &groupID
			originalBook.VersionNotes = &fallbackNotes
			if _, updateErr := s.Store().UpdateBook(req.BookID, originalBook); updateErr != nil {
				return updateErr
			}
			return nil
		}

		progress.Log("info", fmt.Sprintf("Created M4B version %s (group %s), original %s demoted to non-primary", newBook.ID, groupID, req.BookID), nil)

		// If iTunes write-back is disabled and the original book came from iTunes,
		// store a deferred update so the path change is applied on the next sync.
		if !config.AppConfig.ITLWriteBackEnabled &&
			originalBook.ITunesPersistentID != nil &&
			*originalBook.ITunesPersistentID != "" {
			if err := s.Store().CreateDeferredITunesUpdate(
				originalBook.ID,
				*originalBook.ITunesPersistentID,
				originalBook.FilePath,
				newBook.FilePath,
				"transcode",
			); err != nil {
				progress.Log("warn", fmt.Sprintf("Failed to create deferred iTunes update: %v", err), nil)
			} else {
				progress.Log("info", "M4B created. iTunes library update deferred until write-back is enabled.", nil)
			}
		}

		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "transcode", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) getOperationStatus(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	op, err := s.Store().GetOperationByID(id)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	c.JSON(http.StatusOK, op)
}

func (s *Server) cancelOperation(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
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
				c.Status(http.StatusNoContent)
				return
			}
		}
	}

	// Try cancel via queue (for running queue operations)
	if operations.GlobalQueue != nil {
		if err := operations.GlobalQueue.Cancel(id); err == nil {
			c.Status(http.StatusNoContent)
			return
		}
	}

	// Fallback: force-update DB status (e.g., stale after restart)
	if dbErr := s.Store().UpdateOperationStatus(id, "canceled", 0, 0, "force canceled (stale operation)"); dbErr != nil {
		internalError(c, "failed to cancel operation", dbErr)
		return
	}
	c.Status(http.StatusNoContent)
}

// clearStaleOperations force-marks all pending/running/queued operations as failed.
func (s *Server) clearStaleOperations(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	ops, err := s.Store().GetRecentOperations(500)
	if err != nil {
		internalError(c, "failed to get operations", err)
		return
	}

	cleared := 0
	for _, op := range ops {
		if op.Status == "pending" || op.Status == "running" || op.Status == "queued" {
			_ = s.Store().UpdateOperationStatus(op.ID, "failed", 0, 0, "force cleared by user")
			cleared++
		}
	}

	c.JSON(http.StatusOK, gin.H{"cleared": cleared})
}

// deleteOperationHistory deletes operations matching the given status(es).
// Query param: ?status=completed or ?status=failed or ?status=completed,failed
func (s *Server) deleteOperationHistory(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	statusParam := c.Query("status")
	if statusParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status parameter required"})
		return
	}

	statuses := strings.Split(statusParam, ",")
	// Only allow deleting terminal statuses
	allowed := map[string]bool{"completed": true, "failed": true, "canceled": true}
	for _, s := range statuses {
		if !allowed[s] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot delete operations with status %q", s)})
			return
		}
	}

	deleted, err := s.Store().DeleteOperationsByStatus(statuses)
	if err != nil {
		internalError(c, "failed to delete operations", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// optimizeDatabase splits &-delimited author/narrator strings and re-extracts empty media info.
func (s *Server) optimizeDatabase(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	books, err := s.Store().GetAllBooks(10000, 0)
	if err != nil {
		internalError(c, "failed to get audiobooks", err)
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

	c.JSON(http.StatusOK, gin.H{
		"books_processed": len(books),
		"authors_split":   authorsSplit,
		"narrators_split": narratorsSplit,
	})
}

func (s *Server) sweepTombstones(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	result, err := SweepTombstones(s.Store())
	if err != nil {
		internalError(c, "failed to sweep tombstones", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) auditFileConsistency(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	result, err := AuditFileConsistency(s.Store())
	if err != nil {
		internalError(c, "failed to audit file consistency", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// listActiveOperations returns a snapshot of currently queued/running operations with basic progress
func (s *Server) listOperations(c *gin.Context) {
	params := ParsePaginationParams(c)
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusOK, gin.H{"items": []database.Operation{}, "total": 0, "limit": params.Limit, "offset": params.Offset})
		return
	}
	ops, total, err := store.ListOperations(params.Limit, params.Offset)
	if err != nil {
		internalError(c, "failed to list operations", err)
		return
	}
	if ops == nil {
		ops = []database.Operation{}
	}
	c.JSON(http.StatusOK, gin.H{"items": ops, "total": total, "limit": params.Limit, "offset": params.Offset})
}

func (s *Server) listActiveOperations(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusOK, gin.H{"operations": []gin.H{}})
		return
	}
	active := operations.GlobalQueue.ActiveOperations()
	results := make([]gin.H, 0, len(active))
	for _, a := range active {
		status := "queued"
		progress := 0
		total := 0
		message := ""
		if s.Store() != nil {
			if op, err := s.Store().GetOperationByID(a.ID); err == nil && op != nil {
				status = op.Status
				progress = op.Progress
				total = op.Total
				message = op.Message
			}
		}
		results = append(results, gin.H{
			"id":       a.ID,
			"type":     a.Type,
			"status":   status,
			"progress": progress,
			"total":    total,
			"message":  message,
		})
	}
	c.JSON(http.StatusOK, gin.H{"operations": results})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list stale operations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"timeout_minutes": timeoutMinutes,
		"count":           len(stale),
		"operations":      stale,
	})
}

// getOperationLogs returns logs for a given operation
func (s *Server) getOperationLogs(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	logs, err := s.Store().GetOperationLogs(id)
	if err != nil {
		internalError(c, "failed to get operation logs", err)
		return
	}
	// Optional tail parameter for last N log lines
	if tailStr := c.Query("tail"); tailStr != "" {
		if n, convErr := strconv.Atoi(tailStr); convErr == nil && n > 0 && n < len(logs) {
			logs = logs[len(logs)-n:]
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
}

func (s *Server) getOperationResult(c *gin.Context) {
	id := c.Param("id")
	store := s.Store()
	op, err := store.GetOperationByID(id)
	if err != nil {
		internalError(c, "failed to get operation", err)
		return
	}
	if op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	if op.ResultData == nil {
		c.JSON(http.StatusOK, gin.H{"result_data": nil})
		return
	}

	// Parse the JSON result data to return as structured JSON
	var resultData json.RawMessage
	if err := json.Unmarshal([]byte(*op.ResultData), &resultData); err != nil {
		c.JSON(http.StatusOK, gin.H{"result_data": *op.ResultData})
		return
	}

	c.JSON(http.StatusOK, gin.H{"result_data": resultData})
}

// getOperationChanges returns change tracking records for an operation.
func (s *Server) getOperationChanges(c *gin.Context) {
	id := c.Param("id")
	changes, err := s.Store().GetOperationChanges(id)
	if err != nil {
		internalError(c, "failed to get operation changes", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

// revertOperation undoes all changes from a given operation.
func (s *Server) revertOperation(c *gin.Context) {
	id := c.Param("id")
	revertSvc := NewRevertService(s.Store())
	if err := revertSvc.RevertOperation(id); err != nil {
		internalError(c, "failed to revert operation", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "operation reverted successfully"})
}

// listTasks returns all registered tasks with their status and schedule.
func (s *Server) listTasks(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	c.JSON(http.StatusOK, s.scheduler.ListTasks())
}

// runTask triggers a task by name.
func (s *Server) runTask(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	name := c.Param("name")
	op, err := s.scheduler.RunTask(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if op == nil {
		c.JSON(http.StatusAccepted, gin.H{"message": "task triggered"})
		return
	}
	c.JSON(http.StatusAccepted, op)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("task %q config is not configurable", name)})
		return
	}

	// Persist to database
	if s.Store() != nil {
		if err := config.SaveConfigToDatabase(s.Store()); err != nil {
			log.Printf("[WARN] Failed to save task config: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "task config updated"})
}

// runMaintenanceWindowNow triggers the full maintenance window sequence immediately.
func (s *Server) runMaintenanceWindowNow(c *gin.Context) {
	if s.scheduler == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scheduler not initialized"})
		return
	}
	ctx := context.WithValue(c.Request.Context(), ignoreWindowKey, true)
	if err := s.scheduler.RunMaintenanceWindow(ctx); err != nil {
		internalError(c, "failed to run maintenance", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "maintenance window triggered"})
}
