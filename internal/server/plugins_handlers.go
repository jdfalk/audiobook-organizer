// file: internal/server/plugins_handlers.go
// version: 2.1.0
// last-edited: 2026-05-01
// guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e

package server

import (
	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
)

// pluginInfo is the JSON shape returned for each plugin.
type pluginInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Enabled      bool     `json:"enabled"`
	Initialized  bool     `json:"initialized"`
	Health       string   `json:"health,omitempty"`
}

func pluginToInfo(p plugin.Plugin, reg *plugin.Registry) pluginInfo {
	caps := make([]string, len(p.Capabilities()))
	for i, c := range p.Capabilities() {
		caps[i] = string(c)
	}
	info := pluginInfo{
		ID:           p.ID(),
		Name:         p.Name(),
		Version:      p.Version(),
		Capabilities: caps,
		Enabled:      reg.IsEnabled(p.ID()),
	}
	if err := p.HealthCheck(); err != nil {
		info.Health = err.Error()
	} else {
		info.Health = "ok"
		info.Initialized = true
	}
	return info
}

// listPlugins handles GET /api/v1/plugins
func (s *Server) listPlugins(c *gin.Context) {
	if s.pluginRegistry == nil {
		httputil.RespondWithOK(c, gin.H{"plugins": []pluginInfo{}})
		return
	}
	all := s.pluginRegistry.All()
	result := make([]pluginInfo, 0, len(all))
	for _, p := range all {
		result = append(result, pluginToInfo(p, s.pluginRegistry))
	}
	httputil.RespondWithOK(c, gin.H{"plugins": result})
}

// getPlugin handles GET /api/v1/plugins/:id
func (s *Server) getPlugin(c *gin.Context) {
	id := c.Param("id")
	if s.pluginRegistry == nil {
		httputil.RespondWithNotFound(c, "plugin system", "not initialized")
		return
	}
	p, ok := s.pluginRegistry.Get(id)
	if !ok {
		httputil.RespondWithNotFound(c, "plugin", id)
		return
	}
	httputil.RespondWithOK(c, pluginToInfo(p, s.pluginRegistry))
}

// enablePlugin handles POST /api/v1/plugins/:id/enable
func (s *Server) enablePlugin(c *gin.Context) {
	id := c.Param("id")
	if s.pluginRegistry == nil {
		httputil.RespondWithInternalError(c, "plugin system not initialized")
		return
	}
	if _, ok := s.pluginRegistry.Get(id); !ok {
		httputil.RespondWithNotFound(c, "plugin", id)
		return
	}
	s.pluginRegistry.Enable(id)

	// Persist to config.
	cfg := config.AppConfig.Plugins[id]
	cfg.Enabled = true
	if config.AppConfig.Plugins == nil {
		config.AppConfig.Plugins = make(map[string]config.PluginConfig)
	}
	config.AppConfig.Plugins[id] = cfg

	httputil.RespondWithOK(c, gin.H{"id": id, "enabled": true})
}

// disablePlugin handles POST /api/v1/plugins/:id/disable
func (s *Server) disablePlugin(c *gin.Context) {
	id := c.Param("id")
	if s.pluginRegistry == nil {
		httputil.RespondWithInternalError(c, "plugin system not initialized")
		return
	}
	if _, ok := s.pluginRegistry.Get(id); !ok {
		httputil.RespondWithNotFound(c, "plugin", id)
		return
	}
	s.pluginRegistry.Disable(id)

	cfg := config.AppConfig.Plugins[id]
	cfg.Enabled = false
	if config.AppConfig.Plugins == nil {
		config.AppConfig.Plugins = make(map[string]config.PluginConfig)
	}
	config.AppConfig.Plugins[id] = cfg

	httputil.RespondWithOK(c, gin.H{"id": id, "enabled": false})
}

// pluginHealth handles GET /api/v1/plugins/:id/health
func (s *Server) pluginHealth(c *gin.Context) {
	id := c.Param("id")
	if s.pluginRegistry == nil {
		httputil.RespondWithSuccess(c, 503, gin.H{"id": id, "health": "plugin system not initialized"})
		return
	}
	p, ok := s.pluginRegistry.Get(id)
	if !ok {
		httputil.RespondWithNotFound(c, "plugin", id)
		return
	}
	if err := p.HealthCheck(); err != nil {
		httputil.RespondWithOK(c, gin.H{"id": id, "health": err.Error(), "ok": false})
		return
	}
	httputil.RespondWithOK(c, gin.H{"id": id, "health": "ok", "ok": true})
}

// updatePluginSettings handles PUT /api/v1/plugins/:id/settings
func (s *Server) updatePluginSettings(c *gin.Context) {
	id := c.Param("id")
	if s.pluginRegistry == nil {
		httputil.RespondWithInternalError(c, "plugin system not initialized")
		return
	}
	if _, ok := s.pluginRegistry.Get(id); !ok {
		httputil.RespondWithNotFound(c, "plugin", id)
		return
	}

	var settings map[string]string
	if err := c.ShouldBindJSON(&settings); err != nil {
		httputil.RespondWithBadRequest(c, "invalid settings: "+err.Error())
		return
	}

	if config.AppConfig.Plugins == nil {
		config.AppConfig.Plugins = make(map[string]config.PluginConfig)
	}
	cfg := config.AppConfig.Plugins[id]
	cfg.Settings = settings
	config.AppConfig.Plugins[id] = cfg

	httputil.RespondWithOK(c, gin.H{"id": id, "settings": settings})
}
