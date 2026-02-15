// file: internal/server/middleware/ratelimit_test.go
// version: 1.0.0
// guid: b31f3de0-b0bc-4cbf-8448-7309df38f7c0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewIPRateLimiter_Defaults(t *testing.T) {
	t.Parallel()

	limiter := NewIPRateLimiter(0, 0)
	assert.Equal(t, 1, limiter.requestsPerMin)
	assert.Equal(t, 1, limiter.burst)
}

func TestIPRateLimiter_Middleware(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewIPRateLimiter(1, 1).Middleware())
	router.GET("/limited", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req1 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req1.RemoteAddr = "192.0.2.1:1234"
	resp1 := httptest.NewRecorder()
	router.ServeHTTP(resp1, req1)
	assert.Equal(t, http.StatusOK, resp1.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req2.RemoteAddr = "192.0.2.1:1234"
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	assert.Equal(t, http.StatusTooManyRequests, resp2.Code)
	assert.Contains(t, resp2.Body.String(), "rate limit exceeded")

	// Different IP should have its own bucket.
	req3 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req3.RemoteAddr = "198.51.100.3:4321"
	resp3 := httptest.NewRecorder()
	router.ServeHTTP(resp3, req3)
	assert.Equal(t, http.StatusOK, resp3.Code)
}
