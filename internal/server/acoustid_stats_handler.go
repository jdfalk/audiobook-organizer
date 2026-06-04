// file: internal/server/acoustid_stats_handler.go
// version: 1.0.0
// guid: 12345678-90ab-cdef-1234-567890abcdef
// last-edited: 2026-06-04

package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

func (s *Server) handleGetAcoustIDStats(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "store not initialized")
		return
	}

	stats, err := store.GetAcoustIDStats()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to read AcoustID stats")
		return
	}

	httputil.RespondWithOK(c, stats)
}
