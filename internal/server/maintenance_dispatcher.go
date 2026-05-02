// file: internal/server/maintenance_dispatcher.go
// version: 1.3.0
// guid: 55555555-5555-5555-5555-555555555555
// last-edited: 2026-05-01

package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/maintenance"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// progressAdapter adapts operations.ProgressReporter to maintenance.ProgressReporter.
type progressAdapter struct {
	ops   operations.ProgressReporter
	cur   int
	total int
}

func (a *progressAdapter) SetTotal(n int) { a.total = n }

func (a *progressAdapter) Increment() {
	a.cur++
	_ = a.ops.UpdateProgress(a.cur, a.total, "")
}

func (a *progressAdapter) Log(level, message string, details *string) {
	_ = a.ops.Log(level, message, details)
}

// listMaintenanceJobs returns the catalogue of all registered maintenance jobs.
func (s *Server) listMaintenanceJobs(c *gin.Context) {
	jobs := maintenance.All()
	type jobDef struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		Category      string `json:"category"`
		DefaultParams any    `json:"default_params"`
		CanResume     bool   `json:"can_resume"`
		Permission    string `json:"permission,omitempty"`
	}
	out := make([]jobDef, len(jobs))
	for i, j := range jobs {
		perm := string(auth.PermSettingsManage)
		if pa, ok := j.(maintenance.PermissionAware); ok && pa.Permission() != "" {
			perm = pa.Permission()
		}
		out[i] = jobDef{
			ID:            j.ID(),
			Name:          j.Name(),
			Description:   j.Description(),
			Category:      j.Category(),
			DefaultParams: j.DefaultParams(),
			CanResume:     j.CanResume(),
			Permission:    perm,
		}
	}
	httputil.RespondWithOK(c, struct {
		Jobs []jobDef `json:"jobs"`
	}{Jobs: out})
}

// runMaintenanceJob enqueues the named maintenance job as an async operation.
func (s *Server) runMaintenanceJob(c *gin.Context) {
	jobID := c.Param("job_id")
	job, err := maintenance.Get(jobID)
	if err != nil {
		httputil.RespondWithNotFound(c, "maintenance job", jobID)
		return
	}

	// Enforce per-job access control. Jobs that implement PermissionAware use
	// their own permission; all others default to settings.manage.
	if config.AppConfig.EnableAuth {
		required := auth.Permission(auth.PermSettingsManage)
		if pa, ok := job.(maintenance.PermissionAware); ok && pa.Permission() != "" {
			required = auth.Permission(pa.Permission())
		}
		if !auth.Can(c.Request.Context(), required) {
			if _, hasUser := auth.UserFromContext(c.Request.Context()); !hasUser {
				httputil.RespondWithUnauthorized(c, "authentication required")
			} else {
				httputil.RespondWithForbidden(c, "permission denied: "+string(required))
			}
			return
		}
	}

	var req struct {
		DryRun bool `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)

	opID := ulid.Make().String()
	opType := "maintenance:" + jobID
	store := s.Store()

	// Create the operation record first so it appears in active operations / activity bell.
	if _, err := store.CreateOperation(opID, opType, nil); err != nil {
		httputil.RespondWithInternalError(c, "failed to create operation record")
		return
	}

	enqueueFn := operations.OperationFunc(func(ctx context.Context, reporter operations.ProgressReporter) error {
		ctx = maintenance.WithOperationID(ctx, opID)
		adapter := &progressAdapter{ops: reporter}
		return job.Run(ctx, store, adapter, req.DryRun)
	})

	if err := s.queue.Enqueue(opID, opType, operations.PriorityNormal, enqueueFn); err != nil {
		httputil.RespondWithConflict(c, err.Error())
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, struct {
		OperationID string `json:"operation_id"`
	}{OperationID: opID})
}
