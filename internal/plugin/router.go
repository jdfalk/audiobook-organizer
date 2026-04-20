// file: internal/plugin/router.go
// version: 1.0.0

package plugin

import "github.com/gin-gonic/gin"

// PluginRouter provides scoped HTTP route registration.
// All routes are automatically prefixed with /api/v1/plugins/{pluginID}/.
type PluginRouter interface {
	GET(path string, handler gin.HandlerFunc)
	POST(path string, handler gin.HandlerFunc)
	PUT(path string, handler gin.HandlerFunc)
	DELETE(path string, handler gin.HandlerFunc)
	Group(path string) PluginRouter
}

// ginPluginRouter wraps a gin.RouterGroup to implement PluginRouter.
type ginPluginRouter struct {
	group *gin.RouterGroup
}

// NewPluginRouter creates a scoped router for a plugin.
func NewPluginRouter(parent *gin.RouterGroup, pluginID string) PluginRouter {
	return &ginPluginRouter{group: parent.Group("/" + pluginID)}
}

func (r *ginPluginRouter) GET(path string, handler gin.HandlerFunc)    { r.group.GET(path, handler) }
func (r *ginPluginRouter) POST(path string, handler gin.HandlerFunc)   { r.group.POST(path, handler) }
func (r *ginPluginRouter) PUT(path string, handler gin.HandlerFunc)    { r.group.PUT(path, handler) }
func (r *ginPluginRouter) DELETE(path string, handler gin.HandlerFunc) { r.group.DELETE(path, handler) }

func (r *ginPluginRouter) Group(path string) PluginRouter {
	return &ginPluginRouter{group: r.group.Group(path)}
}
