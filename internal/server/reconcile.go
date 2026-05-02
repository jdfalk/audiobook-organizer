// file: internal/server/reconcile.go
// version: 3.0.0
// guid: e7f8a9b0-c1d2-3e4f-5a6b-7c8d9e0f1a2b

package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/reconcile"
	"github.com/oklog/ulid/v2"
)

// reconcilePreview handles GET /api/v1/operations/reconcile/preview (sync, kept for backward compat)
func (s *Server) reconcilePreview(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	result, err := reconcile.BuildReconcilePreview(store)
	if err != nil {
		httputil.InternalError(c, "failed to build reconcile preview", err)
		return
	}
	httputil.RespondWithOK(c, result)
}

// startReconcileScan handles POST /api/v1/operations/reconcile/scan — async background scan
func (s *Server) startReconcileScan(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		httputil.RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, "reconcile_scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return reconcile.RunReconcileScan(store, ctx, id, progress)
	}

	if err := s.queue.Enqueue(op.ID, "reconcile_scan", operations.PriorityNormal, operationFunc); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, http.StatusAccepted, op)
}

// latestReconcileScan handles GET /api/v1/operations/reconcile/scan/latest
func (s *Server) latestReconcileScan(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	// Find the most recent reconcile_scan operation
	ops, _, err := store.ListOperations(50, 0)
	if err != nil {
		httputil.InternalError(c, "failed to list operations", err)
		return
	}

	for _, op := range ops {
		if op.Type != "reconcile_scan" {
			continue
		}
		// Return the operation with its result_data if completed
		if op.Status == "completed" && op.ResultData != nil {
			var preview reconcile.ReconcilePreviewResult
			if err := json.Unmarshal([]byte(*op.ResultData), &preview); err == nil {
				httputil.RespondWithOK(c, gin.H{
					"operation": op,
					"preview":   preview,
				})
				return
			}
		}
		// Return op status if still running or failed
		httputil.RespondWithOK(c, gin.H{
			"operation": op,
			"preview":   nil,
		})
		return
	}

	httputil.RespondWithOK(c, gin.H{"operation": nil, "preview": nil})
}

// startReconcile handles POST /api/v1/operations/reconcile
func (s *Server) startReconcile(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		httputil.RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	var req struct {
		Matches []reconcile.ReconcileApplyItem `json:"matches"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if len(req.Matches) == 0 {
		httputil.RespondWithBadRequest(c, "no matches provided")
		return
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, "reconcile", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	matches := req.Matches
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return reconcile.ExecuteReconcile(ctx, store, id, matches, operations.LoggerFromReporter(progress))
	}

	if err := s.queue.Enqueue(op.ID, "reconcile", operations.PriorityNormal, operationFunc); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, http.StatusAccepted, op)
}

// cleanupDuplicateVersionGroupsHandler is the HTTP handler for POST /api/v1/operations/cleanup-version-groups
func (s *Server) cleanupDuplicateVersionGroupsHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := reconcile.CleanupDuplicateVersionGroups(s.Store(), config.AppConfig.RootDir, dryRun)
	if err != nil {
		httputil.InternalError(c, "failed to cleanup version groups", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// markBrokenSegmentBooksHandler handles POST /api/v1/operations/mark-broken-segments
func (s *Server) markBrokenSegmentBooksHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := reconcile.FindBrokenSegmentBooks(s.Store(), dryRun)
	if err != nil {
		httputil.InternalError(c, "failed to find broken segments", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// mergeNoVGDuplicatesHandler handles POST /api/v1/operations/merge-novg-duplicates
func (s *Server) mergeNoVGDuplicatesHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := reconcile.MergeNoVGDuplicates(s.Store(), config.AppConfig.RootDir, dryRun)
	if err != nil {
		httputil.InternalError(c, "failed to merge duplicates", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// assignOrphanVGsHandler handles POST /api/v1/operations/assign-orphan-vgs
func (s *Server) assignOrphanVGsHandler(c *gin.Context) {
	result, err := reconcile.AssignOrphanVGs(s.Store(), config.AppConfig.RootDir)
	if err != nil {
		httputil.InternalError(c, "failed to assign version groups", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"result": result})
}
