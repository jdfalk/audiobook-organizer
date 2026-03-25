// file: internal/server/activity_handlers.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012

package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// listActivity handles GET /api/v1/activity.
//
// Supported query parameters:
//
//	limit         – max entries to return (default 50)
//	offset        – pagination offset
//	type          – filter by entry type
//	tier          – filter by tier (realtime|background|debug|audit)
//	level         – filter by level (info|warn|error|debug)
//	operation_id  – filter by operation ID
//	book_id       – filter by book ID
//	since         – RFC3339 lower-bound timestamp (inclusive)
//	until         – RFC3339 upper-bound timestamp (inclusive)
//	tags          – comma-separated list of required tags (AND semantics)
func (s *Server) listActivity(c *gin.Context) {
	if s.activityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "activity log not available"})
		return
	}

	filter := database.ActivityFilter{}

	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		filter.Limit = n
	}

	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		filter.Offset = n
	}

	filter.Type = c.Query("type")
	filter.Tier = c.Query("tier")
	filter.Level = c.Query("level")
	filter.OperationID = c.Query("operation_id")
	filter.BookID = c.Query("book_id")

	if v := c.Query("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since: must be RFC3339"})
			return
		}
		filter.Since = &t
	}

	if v := c.Query("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until: must be RFC3339"})
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

	entries, total, err := s.activityService.Query(filter)
	if err != nil {
		internalError(c, "failed to query activity log", err)
		return
	}

	// Ensure entries is always a JSON array, never null.
	if entries == nil {
		entries = []database.ActivityEntry{}
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
	})
}
