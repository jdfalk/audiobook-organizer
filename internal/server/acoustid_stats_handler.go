// file: internal/server/acoustid_stats_handler.go
// version: 1.0.0
// guid: 3c3e1a5b-5d59-4cd8-a772-ff8b4cb1e5be
// last-edited: 2026-06-06

package server

import (
	"github.com/gin-gonic/gin"

	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

// handleGetAcoustIDStats returns fingerprint coverage stats.
// GET /api/v1/maintenance/acoustid-stats
func (s *Server) handleGetAcoustIDStats(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	stats, err := store.GetAcoustIDStats()
	if err != nil {
		httputil.InternalError(c, "failed to get AcoustID stats", err)
		return
	}

	httputil.RespondWithOK(c, stats)
}
