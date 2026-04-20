// file: internal/plugin/router_test.go
// version: 1.0.0

package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPluginRouter_ScopedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "test-plugin")
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/test-plugin/status", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginRouter_Group(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "my-plugin")
	sub := router.Group("/torrents")
	sub.GET("/list", func(c *gin.Context) {
		c.JSON(200, gin.H{"torrents": []string{}})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/my-plugin/torrents/list", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginRouter_WrongPath_404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	parent := engine.Group("/api/v1/plugins")

	router := NewPluginRouter(parent, "my-plugin")
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/plugins/other-plugin/status", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
