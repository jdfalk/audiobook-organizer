// file: internal/server/ai_jobs_handlers.go
// version: 1.0.0
// guid: cbb3180d-eb39-40d0-9f14-a2d57e738c0b

package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

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

	store, ok := s.Store().(database.AIJobsStore)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "store does not implement AIJobsStore"})
		return
	}
	jobs, err := store.ListAIJobs(typeF, statusF, limit, offset)
	if err != nil {
		internalError(c, "list ai_jobs", err)
		return
	}
	RespondWithOK(c, gin.H{"jobs": jobs})
}
