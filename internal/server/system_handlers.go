// file: internal/server/system_handlers.go
// version: 1.2.0
// guid: 0c5a18be-5744-4e41-a35a-e7e96630833b
//
// System-level HTTP handlers split out of server.go: health, status,
// storage, logs, announcements, reset/factory-reset, config get/update,
// SSE event stream, dashboard, backup CRUD, blocked-hash CRUD, and
// user-preference CRUD.

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
)

// Handler functions (stubs for now)
func (s *Server) healthCheck(c *gin.Context) {
	// Gather basic metrics; tolerate errors (don't fail health entirely)
	var bookCount, authorCount, seriesCount, playlistCount int
	var dbErr error
	if s.Store() != nil {
		if bc, err := s.Store().CountBooks(); err == nil {
			bookCount = bc
		} else {
			dbErr = err
		}
		if authors, err := s.Store().GetAllAuthors(); err == nil {
			authorCount = len(authors)
		} else if dbErr == nil {
			dbErr = err
		}
		if series, err := s.Store().GetAllSeries(); err == nil {
			seriesCount = len(series)
		} else if dbErr == nil {
			dbErr = err
		}
		// Playlist count intentionally omitted — no reliable counting method yet
	}
	resp := gin.H{
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
		"version":       appVersion,
		"database_type": config.AppConfig.DatabaseType,
		"metrics": gin.H{
			"books":     bookCount,
			"authors":   authorCount,
			"series":    seriesCount,
			"playlists": playlistCount,
		},
	}
	if dbErr != nil {
		resp["partial_error"] = dbErr.Error()
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) getSystemStatus(c *gin.Context) {
	status, err := s.systemService.CollectSystemStatus()
	if err != nil {
		internalError(c, "failed to get system status", err)
		return
	}

	// Attach plugin health information
	if s.pluginRegistry != nil {
		pluginHealth := make(map[string]string)
		for id, err := range s.pluginRegistry.HealthCheckAll() {
			if err != nil {
				pluginHealth[id] = err.Error()
			} else {
				pluginHealth[id] = "ok"
			}
		}
		status.PluginHealth = pluginHealth
	}

	c.JSON(http.StatusOK, status)
}

func (s *Server) getSystemAnnouncements(c *gin.Context) {
	type Announcement struct {
		ID       string `json:"id"`
		Severity string `json:"severity"` // info, warning, error
		Message  string `json:"message"`
		Link     string `json:"link,omitempty"`
	}

	var announcements []Announcement

	// Check for duplicate authors
	authors, err := s.Store().GetAllAuthors()
	if err == nil {
		bookCountFn := func(authorID int) int {
			books, err := s.Store().GetBooksByAuthorIDWithRole(authorID)
			if err != nil {
				return 0
			}
			return len(books)
		}
		groups := s.filterReviewedAuthorGroups(dedup.FindDuplicateAuthors(authors, 0.9, bookCountFn))
		if len(groups) > 0 {
			announcements = append(announcements, Announcement{
				ID:       "duplicate-authors",
				Severity: "warning",
				Message:  fmt.Sprintf("You have %d group(s) of duplicate authors to review", len(groups)),
				Link:     "/dedup?tab=authors",
			})
		}
	}

	// Check for missing files (sample first 100 books)
	books, err := s.Store().GetAllBooks(100, 0)
	if err == nil {
		missingCount := 0
		for _, book := range books {
			if book.FilePath != "" {
				if _, statErr := os.Stat(book.FilePath); os.IsNotExist(statErr) {
					missingCount++
				}
			}
		}
		if missingCount > 0 {
			announcements = append(announcements, Announcement{
				ID:       "missing-files",
				Severity: "warning",
				Message:  fmt.Sprintf("%d book(s) have missing files on disk", missingCount),
				Link:     "/library",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"announcements": announcements})
}

func (s *Server) getSystemStorage(c *gin.Context) {
	rootDir := strings.TrimSpace(config.AppConfig.RootDir)
	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}

	totalBytes, freeBytes, err := getDiskStats(rootDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read filesystem stats"})
		return
	}

	usedBytes := totalBytes - freeBytes
	percentUsed := 0.0
	if totalBytes > 0 {
		percentUsed = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	c.JSON(http.StatusOK, gin.H{
		"path":                rootDir,
		"total_bytes":         totalBytes,
		"used_bytes":          usedBytes,
		"free_bytes":          freeBytes,
		"percent_used":        percentUsed,
		"quota_enabled":       config.AppConfig.EnableDiskQuota,
		"quota_percent":       config.AppConfig.DiskQuotaPercent,
		"user_quotas_enabled": config.AppConfig.EnableUserQuotas,
	})
}

func (s *Server) getSystemLogs(c *gin.Context) {
	// For operation-specific logs, redirect to getOperationLogs
	if id := c.Query("operation_id"); id != "" {
		s.getOperationLogs(c)
		return
	}

	level := c.Query("level")
	params := ParsePaginationParams(c)

	logs, total, err := s.systemService.CollectSystemLogs(level, params.Search, params.Limit, params.Offset)
	if err != nil {
		internalError(c, "failed to get system logs", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"limit":  params.Limit,
		"offset": params.Offset,
		"total":  total,
	})
}

func (s *Server) getSystemActivityLog(c *gin.Context) {
	source := c.Query("source")
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	logs, err := s.Store().GetSystemActivityLogs(source, limit)
	if err != nil {
		internalError(c, "failed to get activity log", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
}

func (s *Server) resetSystem(c *gin.Context) {
	// Reset database
	if err := s.Store().Reset(); err != nil {
		internalError(c, "failed to reset database", err)
		return
	}

	// Reset config to defaults
	config.ResetToDefaults()

	// Reset caches
	resetLibrarySizeCache()
	s.dashboardCache.InvalidateAll()

	RespondWithOK(c, gin.H{"message": "System reset successfully"})
}

func (s *Server) factoryReset(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain {\"confirm\": \"RESET\"}"})
		return
	}

	log.Printf("[INFO] Factory reset initiated")

	// Reset database (books, authors, series, settings)
	if err := s.Store().Reset(); err != nil {
		log.Printf("[ERROR] Factory reset: database reset failed: %v", err)
		internalError(c, "failed to reset database", err)
		return
	}
	log.Printf("[INFO] Factory reset: database cleared")

	// Delete OL data (pebble store + dump files)
	if s.olService != nil {
		s.olService.Mu.Lock()
		if s.olService.OLStore != nil {
			s.olService.OLStore.Close()
			s.olService.OLStore = nil
		}
		s.olService.Mu.Unlock()

		targetDir := metafetch.GetOLDumpDir()
		if targetDir != "" {
			if err := os.RemoveAll(targetDir); err != nil {
				log.Printf("[WARN] Factory reset: failed to remove OL data dir: %v", err)
			} else {
				log.Printf("[INFO] Factory reset: OL data deleted")
			}
		}
	}

	// Clear library folder contents (organized audiobooks)
	if config.AppConfig.RootDir != "" {
		libraryDir := config.AppConfig.RootDir
		entries, err := os.ReadDir(libraryDir)
		if err == nil {
			for _, entry := range entries {
				entryPath := filepath.Join(libraryDir, entry.Name())
				if err := os.RemoveAll(entryPath); err != nil {
					log.Printf("[WARN] Factory reset: failed to remove %s: %v", entryPath, err)
				}
			}
			log.Printf("[INFO] Factory reset: library folder cleared (%s)", libraryDir)
		}
	}

	// Reset config to defaults, then clear paths so wizard re-shows
	config.ResetToDefaults()
	config.AppConfig.RootDir = ""
	config.AppConfig.SetupComplete = false
	if err := config.SaveConfigToDatabase(s.Store()); err != nil {
		log.Printf("[WARN] Factory reset: failed to persist config: %v", err)
	}

	// Reset caches
	resetLibrarySizeCache()
	s.dashboardCache.InvalidateAll()

	log.Printf("[INFO] Factory reset complete")
	c.JSON(http.StatusOK, gin.H{"message": "factory reset complete"})
}

func (s *Server) getConfig(c *gin.Context) {
	// Create a copy of config with masked secrets
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	c.JSON(http.StatusOK, gin.H{"config": maskedConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	previousConfig := config.AppConfig
	status, resp := s.configUpdateService.UpdateConfig(payload)
	if status >= 400 {
		config.AppConfig = previousConfig
		c.JSON(status, resp)
		return
	}

	if err := config.AppConfig.Validate(); err != nil {
		config.AppConfig = previousConfig
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	maskedConfig := s.configUpdateService.MaskSecrets(config.AppConfig)
	response := gin.H{"config": maskedConfig}
	if raw, err := json.Marshal(maskedConfig); err == nil {
		var flat map[string]any
		if err := json.Unmarshal(raw, &flat); err == nil {
			for k, v := range flat {
				response[k] = v
			}
		}
	}
	c.JSON(http.StatusOK, response)
}

// handleEvents handles Server-Sent Events (SSE) for real-time updates
func (s *Server) handleEvents(c *gin.Context) {
	if s.hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event hub not initialized"})
		return
	}
	s.hub.HandleSSE(c)
}

// createBackup creates a database backup
func (s *Server) createBackup(c *gin.Context) {
	var req struct {
		MaxBackups *int `json:"max_backups"`
	}
	_ = c.ShouldBindJSON(&req)

	backupConfig := backup.DefaultBackupConfig()
	if req.MaxBackups != nil {
		backupConfig.MaxBackups = *req.MaxBackups
	}

	// Get database path and type from app config
	dbPath := config.AppConfig.DatabasePath
	dbType := config.AppConfig.DatabaseType

	// Resolve backup dir relative to database directory so it's always absolute
	if dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		internalError(c, "failed to create backup", err)
		return
	}

	c.JSON(http.StatusOK, info)
}

// listBackups lists all available backups
func (s *Server) listBackups(c *gin.Context) {
	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	backups, err := backup.ListBackups(backupConfig.BackupDir)
	if err != nil {
		internalError(c, "failed to list backups", err)
		return
	}

	// Ensure we never return null - always return empty array
	if backups == nil {
		backups = []backup.BackupInfo{}
	}

	c.JSON(http.StatusOK, gin.H{
		"backups": backups,
		"count":   len(backups),
	})
}

// restoreBackup restores from a backup file
func (s *Server) restoreBackup(c *gin.Context) {
	var req struct {
		BackupFilename string `json:"backup_filename" binding:"required"`
		TargetPath     string `json:"target_path"`
		Verify         bool   `json:"verify"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	backupPath := filepath.Join(backupConfig.BackupDir, req.BackupFilename)

	// Use current database path as target if not specified
	targetPath := req.TargetPath
	if targetPath == "" {
		targetPath = filepath.Dir(config.AppConfig.DatabasePath)
	}

	if err := backup.RestoreBackup(backupPath, targetPath, req.Verify); err != nil {
		internalError(c, "failed to restore backup", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "backup restored successfully",
		"target":  targetPath,
	})
}

// deleteBackup deletes a backup file
func (s *Server) deleteBackup(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename required"})
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	// Sanitize filename to prevent path traversal
	filename = filepath.Base(filename)
	backupPath := filepath.Join(backupConfig.BackupDir, filename)

	if err := backup.DeleteBackup(backupPath); err != nil {
		internalError(c, "failed to delete backup", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backup deleted successfully"})
}

// getDashboard returns dashboard statistics with size and format distributions
func (s *Server) getDashboard(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Check cache first
	if cached, ok := s.dashboardCache.Get("dashboard"); ok {
		LogServiceCacheHit("Dashboard", "dashboard")
		c.JSON(http.StatusOK, cached)
		return
	}
	LogServiceCacheMiss("Dashboard", "dashboard")

	// Use SQL aggregation instead of loading all books
	stats, err := s.Store().GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve dashboard stats"})
		return
	}

	// Get recent operations
	recentOps, err := s.Store().GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	result := gin.H{
		"formatDistribution": stats.FormatDistribution,
		"stateDistribution":  stats.StateDistribution,
		"recentOperations":   recentOps,
		"totalSize":          stats.TotalSize,
		"totalBooks":         stats.TotalBooks,
		"totalDuration":      stats.TotalDuration,
	}

	s.dashboardCache.Set("dashboard", result)
	c.JSON(http.StatusOK, result)
}

// listBlockedHashes returns all blocked hashes
func (s *Server) listBlockedHashes(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hashes, err := s.Store().GetAllBlockedHashes()
	if err != nil {
		internalError(c, "failed to get blocked hashes", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": hashes,
		"total": len(hashes),
	})
}

// addBlockedHash adds a hash to the blocklist
func (s *Server) addBlockedHash(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Hash   string `json:"hash" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate hash format (should be 64 character hex string for SHA256)
	if len(req.Hash) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash must be 64 characters (SHA256)"})
		return
	}

	err := s.Store().AddBlockedHash(req.Hash, req.Reason)
	if err != nil {
		internalError(c, "failed to add blocked hash", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "hash blocked successfully",
		"hash":    req.Hash,
		"reason":  req.Reason,
	})
}

// removeBlockedHash removes a hash from the blocklist
func (s *Server) removeBlockedHash(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash parameter required"})
		return
	}

	err := s.Store().RemoveBlockedHash(hash)
	if err != nil {
		internalError(c, "failed to remove blocked hash", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "hash unblocked successfully",
		"hash":    hash,
	})
}

// getUserPreference returns a single user preference by key.
func (s *Server) getUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	pref, err := s.Store().GetUserPreference(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get preference"})
		return
	}
	if pref == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "preference not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": pref.Key, "value": pref.Value})
}

// setUserPreference creates or updates a user preference.
func (s *Server) setUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := s.Store().SetUserPreference(key, body.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": body.Value})
}

// deleteUserPreference removes a user preference by setting it to empty.
func (s *Server) deleteUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}
	// Set to empty string to "delete" (store doesn't have a delete method)
	if err := s.Store().SetUserPreference(key, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete preference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "preference deleted"})
}
