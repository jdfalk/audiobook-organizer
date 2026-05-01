// file: internal/server/pagination.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-456789012345
// last-edited: 2026-05-01

package server

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// paginationFromQuery parses limit/offset query params with sane defaults + caps.
// Default limit = 50, max = 500, default offset = 0.
func paginationFromQuery(c *gin.Context) (int, int) {
	limit, offset := 50, 0
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
