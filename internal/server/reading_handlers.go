// file: internal/server/reading_handlers.go
// version: 1.3.0
// last-edited: 2026-06-01
// guid: 7f2c4a1d-5b8e-4f70-a9d6-2e8c0f1b9a57
//
// HTTP endpoints for the per-user read/unread tracking system
// (spec 3.6). All endpoints scope to the calling user from
// auth.UserFromContext — users can only read/write their own
// state. Anonymous / first-run bootstrap requests use a synthetic
// "_local" user id so the endpoints remain functional before
// multi-user login ships.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/readstatus"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/server/handlers"
	svrmw "github.com/jdfalk/audiobook-organizer/internal/server/middleware"
)

// readingHandlerDeps documents the narrow Server surface needed by the reading
// progress handlers in this file. *Server satisfies this interface automatically.
type readingHandlerDeps interface {
	Store() database.Store
}

var _ readingHandlerDeps = (*Server)(nil)

// callingUserID pulls the authenticated user's ID from context.
// Falls back to "_local" for unauthenticated / bootstrap mode so
// single-user installs can exercise the endpoints before running
// the 3.7 setup wizard.
func callingUserID(c *gin.Context) string {
	if u, ok := auth.UserFromContext(c.Request.Context()); ok && u != nil {
		return u.ID
	}
	if u, ok := svrmw.CurrentUser(c); ok && u != nil {
		return u.ID
	}
	return "_local"
}

// handleSetPosition records one position heartbeat and recomputes
// derived UserBookState. POST /api/v1/books/:id/position
func (s *Server) handleSetPosition(c *gin.Context) {
	bookID := c.Param("id")
	if bookID == "" {
		httputil.RespondWithBadRequest(c, "book id required")
		return
	}
	var req handlers.SetPositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	userID := callingUserID(c)
	if err := s.Store().SetUserPosition(userID, bookID, req.SegmentID, req.PositionSeconds); err != nil {
		httputil.InternalError(c, "failed to record position", err)
		return
	}
	state, err := readstatus.RecomputeUserBookState(s.Store(), userID, bookID)
	if err != nil {
		httputil.InternalError(c, "failed to recompute book state", err)
		return
	}
	httputil.RespondWithOK(c, state)
}

// handleGetPosition returns the latest position for the calling user.
// GET /api/v1/books/:id/position
func (s *Server) handleGetPosition(c *gin.Context) {
	bookID := c.Param("id")
	if bookID == "" {
		httputil.RespondWithBadRequest(c, "book id required")
		return
	}
	pos, err := s.Store().GetUserPosition(callingUserID(c), bookID)
	if err != nil {
		httputil.InternalError(c, "failed to load position", err)
		return
	}
	httputil.RespondWithOK(c, pos)
}

// handleGetBookState returns the derived UserBookState — status +
// progress percent + last activity — for the calling user.
// GET /api/v1/books/:id/state
func (s *Server) handleGetBookState(c *gin.Context) {
	bookID := c.Param("id")
	if bookID == "" {
		httputil.RespondWithBadRequest(c, "book id required")
		return
	}
	state, err := s.Store().GetUserBookState(callingUserID(c), bookID)
	if err != nil {
		httputil.InternalError(c, "failed to load state", err)
		return
	}
	httputil.RespondWithOK(c, state)
}

// handleSetBookStatus sets a manual status override. User-forced
// status takes precedence over auto-derived in future recomputes.
// PATCH /api/v1/books/:id/status
func (s *Server) handleSetBookStatus(c *gin.Context) {
	bookID := c.Param("id")
	if bookID == "" {
		httputil.RespondWithBadRequest(c, "book id required")
		return
	}
	var req handlers.PatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	switch req.Status {
	case database.UserBookStatusFinished,
		database.UserBookStatusInProgress,
		database.UserBookStatusUnstarted,
		database.UserBookStatusAbandoned:
		// ok
	default:
		httputil.RespondWithBadRequest(c, "invalid status: "+req.Status)
		return
	}
	state, err := readstatus.SetManualStatus(s.Store(), callingUserID(c), bookID, req.Status)
	if err != nil {
		httputil.InternalError(c, "failed to set status", err)
		return
	}
	httputil.RespondWithOK(c, state)
}

// handleClearBookStatus clears the manual override — next recompute
// derives a fresh status from positions.
// DELETE /api/v1/books/:id/status
func (s *Server) handleClearBookStatus(c *gin.Context) {
	bookID := c.Param("id")
	if bookID == "" {
		httputil.RespondWithBadRequest(c, "book id required")
		return
	}
	state, err := readstatus.SetManualStatus(s.Store(), callingUserID(c), bookID, "")
	if err != nil {
		httputil.InternalError(c, "failed to clear status", err)
		return
	}
	httputil.RespondWithOK(c, state)
}

// handleListByStatus returns the calling user's books filtered by
// status, paginated. GET /api/v1/me/{in-progress|finished|abandoned|unstarted}
func (s *Server) handleListByStatus(c *gin.Context) {
	status := c.Param("status")
	switch status {
	case database.UserBookStatusInProgress,
		database.UserBookStatusFinished,
		database.UserBookStatusAbandoned,
		database.UserBookStatusUnstarted:
	default:
		httputil.RespondWithBadRequest(c, "invalid status")
		return
	}
	p := httputil.ParsePaginationParams(c)
	list, err := s.Store().ListUserBookStatesByStatus(callingUserID(c), status, p.Limit, p.Offset)
	if err != nil {
		httputil.InternalError(c, "failed to list states", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"states": list, "count": len(list), "limit": p.Limit, "offset": p.Offset})
}

// registerReadingRoutes wires the read/unread endpoints onto the
// given router group.
func (s *Server) registerReadingRoutes(protected *gin.RouterGroup) {
	protected.POST("/books/:id/position", s.handleSetPosition)
	protected.GET("/books/:id/position", s.handleGetPosition)
	protected.GET("/books/:id/state", s.handleGetBookState)
	protected.PATCH("/books/:id/status", s.handleSetBookStatus)
	protected.DELETE("/books/:id/status", s.handleClearBookStatus)
	protected.GET("/me/:status", s.handleListByStatus)
}

// Silence unused-import if the reading code block ever gets compiled
// without touching time — handler test paths reach here.
var _ = time.Now
