// file: internal/server/system_handlers.go
// version: 2.2.6
// last-edited: 2026-05-18
// guid: 0c5a18be-5744-4e41-a35a-e7e96630833b
//
// System-level HTTP handlers split out of server.go: health, status,
// storage, logs, announcements, reset/factory-reset, config get/update,
// SSE event stream, dashboard, backup CRUD, blocked-hash CRUD, and
// user-preference CRUD. Migrated to use RespondWith* helpers.

package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/policy"
	"github.com/jdfalk/audiobook-organizer/internal/security/pathvalidation"
)

// Handler functions (stubs for now)
func (s *Server) healthCheck(c *gin.Context) {
	// Gather basic metrics; tolerate errors (don't fail health entirely)
	var bookCount, authorCount, seriesCount, playlistCount int
	var dbErr error
	var brokenFileCount int
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

		// Try to read broken file count from underlying store (PebbleStore)
		if gf, ok := s.Store().(interface{ GetBrokenFileCount() (int, error) }); ok {
			if cnt, err := gf.GetBrokenFileCount(); err == nil {
				brokenFileCount = cnt
			}
		} else if uw, ok := s.Store().(interface{ Unwrap() database.Store }); ok {
			if inner, ok2 := uw.Unwrap().(interface{ GetBrokenFileCount() (int, error) }); ok2 {
				if cnt, err := inner.GetBrokenFileCount(); err == nil {
					brokenFileCount = cnt
				}
			}
		}
	}
	resp := gin.H{
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
		"version":       appVersion,
		"database_type": config.AppConfig.DatabaseType,
		"metrics": gin.H{
			"books":             bookCount,
			"authors":           authorCount,
			"series":            seriesCount,
			"playlists":         playlistCount,
			"broken_file_count": brokenFileCount,
		},
		"broken_file_count": brokenFileCount,
	}
	if dbErr != nil {
		resp["partial_error"] = dbErr.Error()
	}
	httputil.RespondWithOK(c, resp)
}

func (s *Server) getSystemStatus(c *gin.Context) {
	status, err := s.systemService.CollectSystemStatus()
	if err != nil {
		httputil.InternalError(c, "failed to get system status", err)
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

	// Attach broken file count — PebbleStore implements GetBrokenFileCount but it
	// is not part of SystemServiceStore, so we probe via interface assertion here.
	if s.Store() != nil {
		bfc := func() (int, bool) {
			if gf, ok := s.Store().(interface{ GetBrokenFileCount() (int, error) }); ok {
				if cnt, err := gf.GetBrokenFileCount(); err == nil {
					return cnt, true
				}
			} else if uw, ok := s.Store().(interface{ Unwrap() database.Store }); ok {
				if inner, ok2 := uw.Unwrap().(interface{ GetBrokenFileCount() (int, error) }); ok2 {
					if cnt, err := inner.GetBrokenFileCount(); err == nil {
						return cnt, true
					}
				}
			}
			return 0, false
		}
		if cnt, ok := bfc(); ok {
			status.BrokenFileCount = &cnt
		}
	}

	httputil.RespondWithOK(c, status)
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

	httputil.RespondWithOK(c, gin.H{"announcements": announcements})
}

func (s *Server) getSystemStorage(c *gin.Context) {
	rootDir := strings.TrimSpace(config.AppConfig.RootDir)
	if rootDir == "" {
		httputil.RespondWithBadRequest(c, "root_dir is not configured")
		return
	}

	totalBytes, freeBytes, err := getDiskStats(rootDir)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to read filesystem stats")
		return
	}

	usedBytes := totalBytes - freeBytes
	percentUsed := 0.0
	if totalBytes > 0 {
		percentUsed = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	httputil.RespondWithOK(c, gin.H{
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
	params := httputil.ParsePaginationParams(c)

	logs, total, err := s.systemService.CollectSystemLogs(level, params.Search, params.Limit, params.Offset)
	if err != nil {
		httputil.InternalError(c, "failed to get system logs", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{
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
		httputil.InternalError(c, "failed to get activity log", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"items": logs, "count": len(logs)})
}

func (s *Server) resetSystem(c *gin.Context) {
	// Reset database
	if err := s.Store().Reset(); err != nil {
		httputil.InternalError(c, "failed to reset database", err)
		return
	}

	// Reset config to defaults
	config.ResetToDefaults()

	// Reset caches
	resetLibrarySizeCache()
	s.Store().InvalidateLibraryStats()

	httputil.RespondWithOK(c, gin.H{"message": "System reset successfully"})
}

func (s *Server) factoryReset(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Confirm != "RESET" {
		httputil.RespondWithBadRequest(c, "request body must contain {\"confirm\": \"RESET\"}")
		return
	}

	slog.Info("Factory reset initiated")

	// Reset database (books, authors, series, settings)
	if err := s.Store().Reset(); err != nil {
		slog.Error("Factory reset: database reset failed: %v", err)
		httputil.InternalError(c, "failed to reset database", err)
		return
	}
	slog.Info("Factory reset: database cleared")

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
				slog.Warn("Factory reset: failed to remove OL data dir: %v", err)
			} else {
				slog.Info("Factory reset: OL data deleted")
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
					slog.Warn("Factory reset: failed to remove %s: %v", entryPath, err)
				}
			}
			slog.Info("Factory reset: library folder cleared (%s)", libraryDir)
		}
	}

	// Reset config to defaults, then clear paths so wizard re-shows
	config.ResetToDefaults()
	config.AppConfig.RootDir = ""
	config.AppConfig.SetupComplete = false
	if err := config.SaveConfigToDatabase(s.Store()); err != nil {
		slog.Warn("Factory reset: failed to persist config: %v", err)
	}

	// Reset caches
	resetLibrarySizeCache()
	s.Store().InvalidateLibraryStats()

	slog.Info("Factory reset complete")
	httputil.RespondWithOK(c, gin.H{"message": "factory reset complete"})
}

func (s *Server) getConfig(c *gin.Context) {
	// Create a copy of config with masked secrets
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	httputil.RespondWithOK(c, gin.H{"config": maskedConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	previousConfig := config.AppConfig
	status, resp := s.configUpdateService.UpdateConfig(payload)
	if status >= 400 {
		config.AppConfig = previousConfig
		errMsg, _ := resp["error"].(string)
		httputil.RespondWithError(c, status, errMsg, "CONFIG_ERROR")
		return
	}

	if err := config.AppConfig.Validate(); err != nil {
		config.AppConfig = previousConfig
		httputil.RespondWithBadRequest(c, err.Error())
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
	httputil.RespondWithOK(c, response)
}

// handleEvents handles Server-Sent Events (SSE) for real-time updates
func (s *Server) handleEvents(c *gin.Context) {
	if s.hub == nil {
		httputil.RespondWithError(c, 503, "event hub not initialized", "SERVICE_UNAVAILABLE")
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
		httputil.InternalError(c, "failed to create backup", err)
		return
	}

	httputil.RespondWithOK(c, info)
}

// listBackups lists all available backups
func (s *Server) listBackups(c *gin.Context) {
	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	backups, err := backup.ListBackups(backupConfig.BackupDir)
	if err != nil {
		httputil.InternalError(c, "failed to list backups", err)
		return
	}

	// Ensure we never return null - always return empty array
	if backups == nil {
		backups = []backup.BackupInfo{}
	}

	httputil.RespondWithOK(c, gin.H{
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
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	safeFilename := pathvalidation.SanitizeFilename(req.BackupFilename)
	backupPath := filepath.Join(backupConfig.BackupDir, safeFilename)

	// Use current database path as target if not specified
	var targetPath string
	if req.TargetPath != "" {
		cleanTarget, err := pathvalidation.CleanAbsolutePath(req.TargetPath)
		if err != nil {
			httputil.RespondWithBadRequest(c, "invalid target_path: "+err.Error())
			return
		}
		targetPath = cleanTarget
	} else {
		targetPath = filepath.Dir(config.AppConfig.DatabasePath)
	}

	if err := backup.RestoreBackup(backupPath, targetPath, req.Verify); err != nil {
		httputil.InternalError(c, "failed to restore backup", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{
		"message": "backup restored successfully",
		"target":  targetPath,
	})
}

// deleteBackup deletes a backup file
func (s *Server) deleteBackup(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		httputil.RespondWithBadRequest(c, "filename required")
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if dbPath := config.AppConfig.DatabasePath; dbPath != "" && !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}
	filename = pathvalidation.SanitizeFilename(filename)
	backupPath := filepath.Join(backupConfig.BackupDir, filename)

	if err := backup.DeleteBackup(backupPath); err != nil {
		httputil.InternalError(c, "failed to delete backup", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{"message": "backup deleted successfully"})
}

// getDashboard returns dashboard statistics. The store handles caching internally
// (PebbleDB: stats:library key with 10-min TTL; SQLite: SQL aggregation directly).
func (s *Server) getDashboard(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	stats, err := s.Store().GetDashboardStats()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to retrieve dashboard stats")
		return
	}

	recentOps, err := s.Store().GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	// Try to read broken file count from underlying store (PebbleStore)
	brokenFileCount := 0
	if s.Store() != nil {
		if gf, ok := s.Store().(interface{ GetBrokenFileCount() (int, error) }); ok {
			if cnt, err := gf.GetBrokenFileCount(); err == nil {
				brokenFileCount = cnt
			}
		} else if uw, ok := s.Store().(interface{ Unwrap() database.Store }); ok {
			if inner, ok2 := uw.Unwrap().(interface{ GetBrokenFileCount() (int, error) }); ok2 {
				if cnt, err := inner.GetBrokenFileCount(); err == nil {
					brokenFileCount = cnt
				}
			}
		}
	}

	httputil.RespondWithOK(c, gin.H{
		"formatDistribution": stats.FormatDistribution,
		"stateDistribution":  stats.StateDistribution,
		"recentOperations":   recentOps,
		"totalSize":          stats.TotalSize,
		"totalBooks":         stats.TotalBooks,
		"totalDuration":      stats.TotalDuration,
		"organizedBooks":     stats.OrganizedBooks,
		"unorganizedBooks":   stats.UnorganizedBooks,
		"broken_file_count":  brokenFileCount,
	})
}

// listBlockedHashes returns all blocked hashes
func (s *Server) listBlockedHashes(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	hashes, err := s.Store().GetAllBlockedHashes()
	if err != nil {
		httputil.InternalError(c, "failed to get blocked hashes", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{
		"items": hashes,
		"total": len(hashes),
	})
}

// addBlockedHash adds a hash to the blocklist
func (s *Server) addBlockedHash(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		Hash   string `json:"hash" binding:"required"`
		Reason string `json:"reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	// Validate hash format (should be 64 character hex string for SHA256)
	if len(req.Hash) != 64 {
		httputil.RespondWithBadRequest(c, "hash must be 64 characters (SHA256)")
		return
	}

	err := s.Store().AddBlockedHash(req.Hash, req.Reason)
	if err != nil {
		httputil.InternalError(c, "failed to add blocked hash", err)
		return
	}

	httputil.RespondWithCreated(c, gin.H{
		"message": "hash blocked successfully",
		"hash":    req.Hash,
		"reason":  req.Reason,
	})
}

// removeBlockedHash removes a hash from the blocklist
func (s *Server) removeBlockedHash(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		httputil.RespondWithBadRequest(c, "hash parameter required")
		return
	}

	err := s.Store().RemoveBlockedHash(hash)
	if err != nil {
		httputil.InternalError(c, "failed to remove blocked hash", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{
		"message": "hash unblocked successfully",
		"hash":    hash,
	})
}

// getUserPreference returns a single user preference by key.
// Unset preferences return 200 with an empty value rather than 404 so that
// browsers don't surface "Failed to load resource" console noise for
// optional client-side prefs (library_column_config, dialog state, etc.)
// that legitimately have no saved value yet. Clients should treat an
// empty `value` as "not set" — matching the existing frontend pattern.
func (s *Server) getUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httputil.RespondWithBadRequest(c, "key is required")
		return
	}
	pref, err := s.Store().GetUserPreference(key)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to get preference")
		return
	}
	if pref == nil {
		httputil.RespondWithOK(c, gin.H{"key": key, "value": ""})
		return
	}
	httputil.RespondWithOK(c, gin.H{"key": pref.Key, "value": pref.Value})
}

// setUserPreference creates or updates a user preference.
func (s *Server) setUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httputil.RespondWithBadRequest(c, "key is required")
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}
	if err := s.Store().SetUserPreference(key, body.Value); err != nil {
		httputil.RespondWithInternalError(c, "failed to save preference")
		return
	}
	httputil.RespondWithOK(c, gin.H{"key": key, "value": body.Value})
}

// deleteUserPreference removes a user preference by setting it to empty.
func (s *Server) deleteUserPreference(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		httputil.RespondWithBadRequest(c, "key is required")
		return
	}
	// Set to empty string to "delete" (store doesn't have a delete method)
	if err := s.Store().SetUserPreference(key, ""); err != nil {
		httputil.RespondWithInternalError(c, "failed to delete preference")
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "preference deleted"})
}

// handlePolicyTags returns the catalogue of recognised policy tags.
// GET /api/v1/policy/tags
func (s *Server) handlePolicyTags(c *gin.Context) {
	httputil.RespondWithOK(c, policy.KnownPolicyTags())
}