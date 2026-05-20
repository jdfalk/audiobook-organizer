// file: internal/server/activity_handlers.go
// version: 2.4.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012
// last-edited: 2026-05-20

package server

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

// activityHandlerDeps documents the narrow Server surface needed by the activity
// log handlers in this file. *Server satisfies this interface automatically via
// ActivityService().
type activityHandlerDeps interface {
	ActivityService() *activity.Service
}

var _ activityHandlerDeps = (*Server)(nil)

// listActivity handles GET /api/v1/activity.
//
// Supported query parameters:
//
//	limit            – max entries to return (default 50)
//	offset           – pagination offset
//	type             – filter by entry type
//	tier             – filter by tier (realtime|background|debug|audit)
//	level            – filter by level (info|warn|error|debug)
//	operation_id     – filter by operation ID
//	book_id          – filter by book ID
//	since            – RFC3339 lower-bound timestamp (inclusive)
//	until            – RFC3339 upper-bound timestamp (inclusive)
//	tags             – comma-separated list of required tags (AND semantics)
//	search           – substring match on summary
//	source           – show only entries from this source
//	exclude_sources  – comma-separated list of sources to hide
func (s *Server) listActivity(c *gin.Context) {
	if s.activityService == nil {
		httputil.RespondWithInternalError(c, "activity log not available")
		return
	}

	filter := database.ActivityFilter{}

	params := httputil.ParsePaginationParams(c)
	filter.Limit = params.Limit
	filter.Offset = params.Offset

	filter.Type = c.Query("type")
	filter.Tier = c.Query("tier")
	filter.Level = c.Query("level")
	filter.OperationID = c.Query("operation_id")
	filter.BookID = c.Query("book_id")

	if v := c.Query("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httputil.RespondWithBadRequest(c, "invalid since: must be RFC3339")
			return
		}
		filter.Since = &t
	}

	if v := c.Query("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httputil.RespondWithBadRequest(c, "invalid until: must be RFC3339")
			return
		}
		filter.Until = &t
	}

	if v := c.Query("tags"); v != "" {
		for _, tag := range strings.Split(v, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				filter.Tags = append(filter.Tags, tag)
			}
		}
	}

	filter.Search = c.Query("search")
	filter.Source = c.Query("source")
	if v := c.Query("exclude_sources"); v != "" {
		for _, src := range strings.Split(v, ",") {
			src = strings.TrimSpace(src)
			if src != "" {
				filter.ExcludeSources = append(filter.ExcludeSources, src)
			}
		}
	}
	if v := c.Query("exclude_tiers"); v != "" {
		for _, tier := range strings.Split(v, ",") {
			tier = strings.TrimSpace(tier)
			if tier != "" {
				filter.ExcludeTiers = append(filter.ExcludeTiers, tier)
			}
		}
	}
	if v := c.Query("exclude_tags"); v != "" {
		for _, tag := range strings.Split(v, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				filter.ExcludeTags = append(filter.ExcludeTags, tag)
			}
		}
	}

	entries, total, err := s.activityService.Query(filter)
	if err != nil {
		httputil.InternalError(c, "failed to query activity log", err)
		return
	}

	// Ensure entries is always a JSON array, never null.
	if entries == nil {
		entries = []database.ActivityEntry{}
	}

	httputil.RespondWithOK(c, struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}{Entries: entries, Total: total})
}

// listActivitySources handles GET /api/v1/activity/sources.
//
// Returns distinct sources with their entry counts, filtered by the same
// tier/level/since/until parameters as listActivity.
func (s *Server) listActivitySources(c *gin.Context) {
	if s.activityService == nil {
		httputil.RespondWithInternalError(c, "activity log not available")
		return
	}
	filter := database.ActivityFilter{
		Tier:  c.Query("tier"),
		Level: c.Query("level"),
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
		}
	}
	sources, err := s.activityService.GetDistinctSources(filter)
	if err != nil {
		httputil.InternalError(c, "failed to get sources", err)
		return
	}
	if sources == nil {
		sources = []database.SourceCount{}
	}
	httputil.RespondWithOK(c, struct {
		Sources []database.SourceCount `json:"sources"`
	}{Sources: sources})
}

// listOperationActivity handles GET /api/v1/operations/:id/activity.
//
// Returns all activity log entries for the given operation ID, ordered by
// timestamp ASC (oldest first) so the response reads as a chronological
// transcript of the operation. Supports an optional `limit` query parameter
// (default 1000, max 10000).
func (s *Server) listOperationActivity(c *gin.Context) {
	if s.activityService == nil {
		httputil.RespondWithInternalError(c, "activity log not available")
		return
	}
	opID := strings.TrimSpace(c.Param("id"))
	if opID == "" {
		httputil.RespondWithBadRequest(c, "operation id required")
		return
	}

	limit := 1000
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 10000 {
				n = 10000
			}
			limit = n
		}
	}

	filter := database.ActivityFilter{
		OperationID: opID,
		Limit:       limit,
	}
	entries, total, err := s.activityService.Query(filter)
	if err != nil {
		httputil.InternalError(c, "failed to query activity log", err)
		return
	}

	// Query returns entries newest-first; reverse for ASC chronological order.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	if entries == nil {
		entries = []database.ActivityEntry{}
	}
	httputil.RespondWithOK(c, struct {
		OperationID string                   `json:"operation_id"`
		Entries     []database.ActivityEntry `json:"entries"`
		Total       int                      `json:"total"`
	}{OperationID: opID, Entries: entries, Total: total})
}

// recompactDigests handles POST /api/v1/admin/recompact-digests.
//
// One-shot endpoint that re-derives type, tier, and tags on every stored
// daily-digest entry whose items were compacted before 2026-05-20.
// Returns { touched, skipped }. Safe to call multiple times (idempotent).
func (s *Server) recompactDigests(c *gin.Context) {
	if s.activityService == nil {
		httputil.RespondWithInternalError(c, "activity log not available")
		return
	}

	result, err := s.activityService.RecompactDigests(c.Request.Context())
	if err != nil {
		httputil.InternalError(c, "recompact digests failed", err)
		return
	}

	httputil.RespondWithOK(c, result)
}

// compactActivity handles POST /api/v1/activity/compact.
func (s *Server) compactActivity(c *gin.Context) {
	if s.activityService == nil {
		httputil.RespondWithInternalError(c, "activity log not available")
		return
	}

	var req struct {
		OlderThanDays int `json:"older_than_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.OlderThanDays < 0 {
		httputil.RespondWithBadRequest(c, "older_than_days must be zero or positive")
		return
	}

	// 0 means "compact everything up to now"
	cutoff := time.Now().AddDate(0, 0, -req.OlderThanDays)
	result, err := s.activityService.CompactByDay(c.Request.Context(), cutoff)
	if err != nil {
		httputil.InternalError(c, "activity compaction failed", err)
		return
	}

	httputil.RespondWithOK(c, result)
}
