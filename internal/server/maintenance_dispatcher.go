// file: internal/server/maintenance_dispatcher.go
// version: 1.0.0
// guid: 55555555-5555-5555-5555-555555555555
// last-edited: 2026-05-03

package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
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
		ID          string `json:"id"`
		Description string `json:"description"`
		CanResume   bool   `json:"can_resume"`
	}
	out := make([]jobDef, len(jobs))
	for i, j := range jobs {
		out[i] = jobDef{
			ID:          j.ID(),
			Description: j.Description(),
			CanResume:   j.CanResume(),
		}
	}
	c.JSON(http.StatusOK, gin.H{"jobs": out})
}

// runMaintenanceJob enqueues the named maintenance job as an async operation.
func (s *Server) runMaintenanceJob(c *gin.Context) {
	jobID := c.Param("job_id")
	job, err := maintenance.Get(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		DryRun bool `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)

	opID := ulid.Make().String()
	opType := "maintenance:" + jobID
	store := s.Store()

	enqueueFn := operations.OperationFunc(func(ctx context.Context, reporter operations.ProgressReporter) error {
		ctx = maintenance.WithOperationID(ctx, opID)
		adapter := &progressAdapter{ops: reporter}
		return job.Run(ctx, store, adapter, req.DryRun)
	})

	if err := s.queue.Enqueue(opID, opType, operations.PriorityNormal, enqueueFn); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operation_id": opID})
}
