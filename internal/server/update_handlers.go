// file: internal/server/update_handlers.go
// version: 1.0.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f

package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// getUpdateStatus returns the last update check info.
func (s *Server) getUpdateStatus(c *gin.Context) {
	info := s.updater.LastCheck()
	if info == nil {
		c.JSON(http.StatusOK, gin.H{
			"current_version":  appVersion,
			"latest_version":   "",
			"channel":          config.AppConfig.AutoUpdateChannel,
			"update_available": false,
			"last_checked":     nil,
		})
		return
	}
	c.JSON(http.StatusOK, info)
}

// checkForUpdate triggers an immediate update check and returns the result.
func (s *Server) checkForUpdate(c *gin.Context) {
	channel := config.AppConfig.AutoUpdateChannel
	info, err := s.updater.CheckForUpdate(channel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

// applyUpdate downloads and applies an available update, then exits for restart.
func (s *Server) applyUpdate(c *gin.Context) {
	info := s.updater.LastCheck()
	if info == nil || !info.UpdateAvailable {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no update available"})
		return
	}

	if err := s.updater.DownloadAndReplace(info); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update applied, restarting..."})

	// Exit after response is sent; systemd/launchd restarts the process
	go s.updater.RestartSelf()
}
