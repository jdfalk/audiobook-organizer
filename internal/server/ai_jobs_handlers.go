// file: internal/server/ai_jobs_handlers.go
// version: 1.2.0
// guid: cbb3180d-eb39-40d0-9f14-a2d57e738c0b
// last-edited: 2026-05-01

package server

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

// unwrapAIJobsStore peels Store decorator layers (anything with Unwrap()) until
// it finds one that satisfies AIJobsStore, mirroring the errors.As() pattern.
func unwrapAIJobsStore(s database.Store) (database.AIJobsStore, bool) {
	type unwrapper interface{ Unwrap() database.Store }
	for s != nil {
		if ai, ok := s.(database.AIJobsStore); ok {
			return ai, true
		}
		u, ok := s.(unwrapper)
		if !ok {
			break
		}
		s = u.Unwrap()
	}
	return nil, false
}

// handleListAIJobs serves GET /api/v1/ai-jobs with optional type/status filters.
// Query params: type, status, limit (default 100, max 500), offset (default 0).
func (s *Server) handleListAIJobs(c *gin.Context) {
	typeF := c.Query("type")
	statusF := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	store, ok := unwrapAIJobsStore(s.Store())
	if !ok {
		httputil.RespondWithInternalError(c, "store does not implement AIJobsStore")
		return
	}
	jobs, err := store.ListAIJobs(typeF, statusF, limit, offset)
	if err != nil {
		httputil.InternalError(c, "list ai_jobs", err)
		return
	}
	httputil.RespondWithOK(c, struct {
		Jobs any `json:"jobs"`
	}{Jobs: jobs})
}
