// file: internal/server/middleware/request_size.go
// version: 1.0.0
// guid: f2129ae7-cf11-4888-bd4f-ab4b578f8f18

package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func methodHasBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func selectBodyLimit(path string, jsonLimitBytes, uploadLimitBytes int64) int64 {
	if strings.Contains(path, "/import/") || strings.Contains(path, "/backup/") {
		return uploadLimitBytes
	}
	// OL dump uploads can be multi-GB — no practical limit
	if strings.Contains(path, "/openlibrary/upload") {
		return 20 * 1024 * 1024 * 1024 // 20GB
	}
	return jsonLimitBytes
}

// MaxRequestBodySize enforces request body limits by route class.
func MaxRequestBodySize(jsonLimitBytes, uploadLimitBytes int64) gin.HandlerFunc {
	if jsonLimitBytes < 1 {
		jsonLimitBytes = 1 << 20
	}
	if uploadLimitBytes < jsonLimitBytes {
		uploadLimitBytes = jsonLimitBytes
	}

	return func(c *gin.Context) {
		if !methodHasBody(c.Request.Method) {
			c.Next()
			return
		}

		limit := selectBodyLimit(c.Request.URL.Path, jsonLimitBytes, uploadLimitBytes)
		if c.Request.ContentLength > limit && c.Request.ContentLength > 0 {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
