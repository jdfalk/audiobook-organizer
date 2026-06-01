// file: internal/server/activity_handlers.go
// version: 2.5.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012
// last-edited: 2026-06-01

package server

import (
	"encoding/json"
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

type operationActivityEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	Level         string    `json:"level"`
	OperationID   string    `json:"operation_id"`
	OperationType string    `json:"operation_type"`
	Message       string    `json:"message"`
	Details       string    `json:"details,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
}

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
	if len(entries) == 0 {
		opLogEntries, opLogErr := s.operationActivityFromOpLogs(opID, limit)
		if opLogErr != nil {
			httputil.InternalError(c, "failed to query operation logs", opLogErr)
			return
		}
		if opLogEntries != nil {
			httputil.RespondWithOK(c, struct {
				OperationID string                   `json:"operation_id"`
				Entries     []operationActivityEntry `json:"entries"`
				Total       int                      `json:"total"`
			}{OperationID: opID, Entries: opLogEntries, Total: len(opLogEntries)})
			return
		}
	}
	responseEntries := make([]operationActivityEntry, 0, len(entries))
	for _, e := range entries {
		responseEntries = append(responseEntries, activityEntryToOperationEntry(e))
	}
	httputil.RespondWithOK(c, struct {
		OperationID string                   `json:"operation_id"`
		Entries     []operationActivityEntry `json:"entries"`
		Total       int                      `json:"total"`
	}{OperationID: opID, Entries: responseEntries, Total: total})
}

func (s *Server) operationActivityFromOpLogs(opID string, limit int) ([]operationActivityEntry, error) {
	v2, ok := s.Store().(database.OpsV2Store)
	if !ok {
		return nil, nil
	}
	logs, err := v2.GetOpLogsV2(opID, limit)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return []operationActivityEntry{}, nil
	}
	out := make([]operationActivityEntry, 0, len(logs))
	for _, l := range logs {
		attrs := opLogAttrs(l.Attrs)
		opType, _ := attrs["def_id"].(string)
		out = append(out, operationActivityEntry{
			Timestamp:     l.CreatedAt,
			Level:         l.Level,
			OperationID:   l.OperationID,
			OperationType: opType,
			Message:       l.Message,
			Details:       l.Attrs,
			Tags:          operationLogTags(l.OperationID, l.Level, opType, attrs),
		})
	}
	return out, nil
}

func activityEntryToOperationEntry(e database.ActivityEntry) operationActivityEntry {
	details := ""
	if len(e.Details) > 0 {
		if b, err := json.Marshal(e.Details); err == nil {
			details = string(b)
		}
	}
	return operationActivityEntry{
		Timestamp:     e.Timestamp,
		Level:         e.Level,
		OperationID:   e.OperationID,
		OperationType: e.Type,
		Message:       e.Summary,
		Details:       details,
		Tags:          e.Tags,
	}
}

func opLogAttrs(raw string) map[string]any {
	if raw == "" || raw == "{}" {
		return map[string]any{}
	}
	var attrs map[string]any
	if err := json.Unmarshal([]byte(raw), &attrs); err != nil {
		return map[string]any{}
	}
	return attrs
}

func operationLogTags(opID, level, opType string, attrs map[string]any) []string {
	entry := database.ActivityEntry{
		Tier:        "info",
		Type:        opType,
		Level:       level,
		OperationID: opID,
		Details:     attrs,
		Tags:        []string{"operation"},
	}
	if plugin, ok := attrs["plugin"].(string); ok {
		entry.Source = plugin
		entry.Tags = append(entry.Tags, "plugin:"+plugin)
	}
	if opType != "" {
		entry.Tags = append(entry.Tags, "def:"+opType)
	}
	activity.EnrichTags(&entry)
	return entry.Tags
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
