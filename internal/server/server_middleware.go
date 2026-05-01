// file: internal/server/server_middleware.go
// version: 1.0.0
// guid: 6a093405-441a-4c14-a9c5-46326ea767c1
// last-edited: 2026-05-01

package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		allowedOrigin := ""
		isDevMode := gin.Mode() == gin.DebugMode

		if origin != "" {
			// Dev-mode CORS: allow Vite dev server only.
			if isDevMode && (origin == "http://localhost:5173" || origin == "https://localhost:5173") {
				allowedOrigin = origin
			}

			// Always allow same-origin requests.
			host := strings.TrimSpace(c.Request.Host)
			if host != "" {
				if origin == "http://"+host || origin == "https://"+host {
					allowedOrigin = origin
				}
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, Authorization, Cache-Control, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		}

		if c.Request.Method == http.MethodOptions {
			if origin != "" && allowedOrigin == "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func filesCommonDir(files []database.BookFile) string {
	if len(files) == 0 {
		return ""
	}
	common := filepath.Dir(files[0].FilePath)
	for _, f := range files[1:] {
		fDir := filepath.Dir(f.FilePath)
		for common != fDir && !strings.HasPrefix(fDir, common+string(filepath.Separator)) {
			common = filepath.Dir(common)
			if common == "/" || common == "." {
				return common
			}
		}
	}
	return common
}

func isProtectedPath(filePath string) bool {
	absPath, _ := filepath.Abs(filePath)

	// Check import paths
	if database.GetGlobalStore() != nil {
		importPaths, err := database.GetGlobalStore().GetAllImportPaths()
		if err == nil {
			for _, ip := range importPaths {
				ipAbs, _ := filepath.Abs(ip.Path)
				if strings.HasPrefix(absPath, ipAbs+"/") || absPath == ipAbs {
					return true
				}
			}
		}
	}

	// Check iTunes library paths
	if config.AppConfig.ITunesLibraryReadPath != "" {
		itunesDir := filepath.Dir(config.AppConfig.ITunesLibraryReadPath)
		itunesAbs, _ := filepath.Abs(itunesDir)
		if strings.HasPrefix(absPath, itunesAbs+"/") || absPath == itunesAbs {
			return true
		}
	}
	if config.AppConfig.ITunesLibraryWritePath != "" {
		itunesDir := filepath.Dir(config.AppConfig.ITunesLibraryWritePath)
		itunesAbs, _ := filepath.Abs(itunesDir)
		if strings.HasPrefix(absPath, itunesAbs+"/") || absPath == itunesAbs {
			return true
		}
	}

	// Also check if path contains "iTunes Media" as a safety net
	if strings.Contains(absPath, "iTunes Media") || strings.Contains(absPath, "iTunes%20Media") {
		return true
	}

	// Hard-block .failed/ quarantine folder — never write to or move quarantined files.
	if strings.Contains(filepath.ToSlash(absPath), "/.failed/") {
		return true
	}

	return false
}

func loadDismissedDedupGroups(store database.Store) map[string]bool {
	dismissed := map[string]bool{}
	pref, err := store.GetUserPreference("dedup_dismissed_groups")
	if err != nil || pref == nil || pref.Value == nil || *pref.Value == "" {
		return dismissed
	}
	var keys []string
	if err := json.Unmarshal([]byte(*pref.Value), &keys); err != nil {
		return dismissed
	}
	for _, k := range keys {
		dismissed[k] = true
	}
	return dismissed
}

func saveDismissedDedupGroups(store database.Store, dismissed map[string]bool) {
	keys := make([]string, 0, len(dismissed))
	for k := range dismissed {
		keys = append(keys, k)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		log.Printf("[WARN] failed to marshal dismissed dedup groups: %v", err)
		return
	}
	if err := store.SetUserPreference("dedup_dismissed_groups", string(data)); err != nil {
		log.Printf("[WARN] failed to save dismissed dedup groups: %v", err)
	}
}

func (s *Server) triggerITunesSync() {
	if s.Store() == nil || s.queue == nil {
		return
	}

	if !s.itunesSvc.Enabled() {
		return
	}

	// Flush any quarantine-triggered ITL removals before the sync read.
	s.processITunesPurgePending()
	libraryPath := s.itunesSvc.Importer.DiscoverLibraryPath()
	if libraryPath == "" {
		return
	}

	// Check fingerprint — skip if unchanged (quick mtime+size check)
	if rec, err := s.Store().GetLibraryFingerprint(libraryPath); err == nil && rec != nil {
		if info, statErr := os.Stat(libraryPath); statErr == nil {
			if info.Size() == rec.Size && info.ModTime().Equal(rec.ModTime) {
				return // No changes
			}
		}
	}

	itunesTriggerLog := logger.NewWithActivityLog("itunes-scheduler", s.Store())
	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "itunes_sync", &libraryPath)
	if err != nil {
		itunesTriggerLog.Warn("iTunes sync scheduler: failed to create operation: %v", err)
		return
	}

	// Load path mappings from config for the scheduled sync
	var scheduledMappings []itunes.PathMapping
	for _, m := range config.AppConfig.ITunesPathMappings {
		scheduledMappings = append(scheduledMappings, itunes.PathMapping{From: m.From, To: m.To})
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.itunesSvc.Importer.Sync(ctx, libraryPath, scheduledMappings, s.itunesActivityFn, operations.LoggerFromReporter(progress))
	}

	if err := s.queue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
		itunesTriggerLog.Warn("iTunes sync scheduler: failed to enqueue: %v", err)
		return
	}

	itunesTriggerLog.Info("iTunes sync scheduler: enqueued sync operation %s", op.ID)
}
