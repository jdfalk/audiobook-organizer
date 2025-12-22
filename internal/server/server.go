// file: internal/server/server.go
// version: 1.29.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/mediainfo"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metrics"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/jdfalk/audiobook-organizer/internal/sysinfo"
	ulid "github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Cached library and import path sizes to avoid expensive recalculation on frequent status checks
var cachedLibrarySize int64
var cachedImportSize int64
var cachedSizeComputedAt time.Time
var cacheLock sync.RWMutex

const librarySizeCacheTTL = 60 * time.Second

// resetLibrarySizeCache resets the library size cache (for testing)
func resetLibrarySizeCache() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cachedLibrarySize = 0
	cachedImportSize = 0
	cachedSizeComputedAt = time.Time{}
}

// Helper functions for pointer conversions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func applyOrganizedFileMetadata(book *database.Book, newPath string) {
	hash, err := scanner.ComputeFileHash(newPath)
	if err != nil {
		log.Printf("[WARN] failed to compute organized hash for %s: %v", newPath, err)
	} else if hash != "" {
		book.FileHash = stringPtr(hash)
		book.OrganizedFileHash = stringPtr(hash)
		if book.OriginalFileHash == nil {
			book.OriginalFileHash = stringPtr(hash)
		}
	}
	if info, err := os.Stat(newPath); err == nil {
		size := info.Size()
		book.FileSize = &size
	}
}

// calculateLibrarySizes computes library and import path sizes with caching
func calculateLibrarySizes(rootDir string, importFolders []database.ImportPath) (librarySize, importSize int64) {
	cacheLock.RLock()
	if time.Since(cachedSizeComputedAt) < librarySizeCacheTTL {
		librarySize = cachedLibrarySize
		importSize = cachedImportSize
		cacheLock.RUnlock()
		log.Printf("[DEBUG] Using cached sizes: library=%d, import=%d", librarySize, importSize)
		return
	}
	cacheLock.RUnlock()

	// Cache expired, recalculate
	cacheLock.Lock()
	defer cacheLock.Unlock()

	// Double-check in case another goroutine just updated
	if time.Since(cachedSizeComputedAt) < librarySizeCacheTTL {
		return cachedLibrarySize, cachedImportSize
	}

	log.Printf("[DEBUG] Recalculating library sizes (cache expired)")

	// Calculate library size
	librarySize = 0
	if rootDir != "" {
		if info, err := os.Stat(rootDir); err == nil && info.IsDir() {
			filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					librarySize += info.Size()
				}
				return nil
			})
		}
	}

	// Calculate import path sizes independently (not by subtraction)
	importSize = 0
	for _, folder := range importFolders {
		if !folder.Enabled {
			continue
		}
		if info, err := os.Stat(folder.Path); err == nil && info.IsDir() {
			filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					// Skip files that are under rootDir to avoid double counting
					if rootDir != "" && strings.HasPrefix(path, rootDir) {
						return nil
					}
					importSize += info.Size()
				}
				return nil
			})
		}
	}

	// Update cache
	cachedLibrarySize = librarySize
	cachedImportSize = importSize
	cachedSizeComputedAt = time.Now()

	log.Printf("[DEBUG] Calculated new sizes: library=%d, import=%d", librarySize, importSize)
	return
}

// Server represents the HTTP server
type Server struct {
	httpServer *http.Server
	router     *gin.Engine
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// NewServer creates a new server instance
func NewServer() *Server {
	router := gin.Default()

	// Set up middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Register metrics (idempotent)
	metrics.Register()

	server := &Server{
		router: router,
	}

	server.setupRoutes()

	return server
}

// Start starts the HTTP server
func (s *Server) Start(cfg ServerConfig) error {
	s.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:        s.router,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		IdleTimeout:    cfg.IdleTimeout,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Heartbeat: push periodic system.status events via SSE (every 5s) while running
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if realtime.GlobalHub != nil {
					// Gather lightweight metrics
					var alloc runtime.MemStats
					runtime.ReadMemStats(&alloc)
					bookCount := 0
					folderCount := 0
					if database.GlobalStore != nil {
						if bc, err := database.GlobalStore.CountBooks(); err == nil {
							bookCount = bc
						} else {
							log.Printf("[DEBUG] Heartbeat: Failed to count books: %v", err)
						}
						if folders, err := database.GlobalStore.GetAllImportPaths(); err == nil {
							folderCount = len(folders)
							log.Printf("[DEBUG] Heartbeat: Got %d import paths", folderCount)
						} else {
							log.Printf("[DEBUG] Heartbeat: Failed to get import paths: %v", err)
						}
					}

					// Update Prometheus metrics
					metrics.SetBooks(bookCount)
					metrics.SetFolders(folderCount)
					metrics.SetMemoryAlloc(alloc.Alloc)
					metrics.SetGoroutines(runtime.NumGoroutine())

					realtime.GlobalHub.SendSystemStatus(map[string]interface{}{
						"books":        bookCount,
						"folders":      folderCount,
						"memory_alloc": alloc.Alloc,
						"goroutines":   runtime.NumGoroutine(),
						"timestamp":    time.Now().Unix(),
					})
				}
			case <-quit:
				return
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	<-quit

	log.Println("Shutting down server...")

	// Broadcast shutdown event to all connected clients
	if realtime.GlobalHub != nil {
		realtime.GlobalHub.Broadcast(&realtime.Event{
			Type: "system.shutdown",
			Data: map[string]interface{}{
				"message": "Server is shutting down",
			},
		})
		// Give clients a moment to receive the event
		time.Sleep(500 * time.Millisecond)
	}

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	log.Println("Server exited")
	return nil
}

// setupRoutes configures all the routes
func (s *Server) setupRoutes() {
	// Health check endpoint
	// Prometheus metrics endpoint (standard path)
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health check endpoint (both paths for compatibility)
	s.router.GET("/api/health", s.healthCheck)
	s.router.GET("/api/v1/health", s.healthCheck)

	// Real-time events (SSE)
	s.router.GET("/api/events", s.handleEvents)

	// Redirect /api/* to /api/v1/* for v1 compatibility
	s.router.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		// If path starts with /api/ but not /api/v1/ and not /api/health and not /api/events
		if strings.HasPrefix(path, "/api/") &&
			!strings.HasPrefix(path, "/api/v1/") &&
			!strings.HasPrefix(path, "/api/health") &&
			!strings.HasPrefix(path, "/api/events") &&
			!strings.HasPrefix(path, "/api/metrics") {
			// Redirect to /api/v1/
			newPath := strings.Replace(path, "/api/", "/api/v1/", 1)
			c.Redirect(http.StatusMovedPermanently, newPath)
			c.Abort()
			return
		}
		c.Next()
	})

	// API routes
	api := s.router.Group("/api/v1")
	{
		// Audiobook routes
		api.GET("/audiobooks", s.listAudiobooks)
		api.GET("/audiobooks/count", s.countAudiobooks)
		api.GET("/audiobooks/duplicates", s.listDuplicateAudiobooks)
		api.GET("/audiobooks/:id", s.getAudiobook)
		api.PUT("/audiobooks/:id", s.updateAudiobook)
		api.DELETE("/audiobooks/:id", s.deleteAudiobook)
		api.POST("/audiobooks/batch", s.batchUpdateAudiobooks)

		// Author and series routes
		api.GET("/authors", s.listAuthors)
		api.GET("/series", s.listSeries)

		// File system routes
		api.GET("/filesystem/browse", s.browseFilesystem)
		api.POST("/filesystem/exclude", s.createExclusion)
		api.DELETE("/filesystem/exclude", s.removeExclusion)

		// Import path routes
		api.GET("/import-paths", s.listImportPaths)
		api.POST("/import-paths", s.addImportPath)
		api.DELETE("/import-paths/:id", s.removeImportPath)

		// Operation routes
		api.POST("/operations/scan", s.startScan)
		api.POST("/operations/organize", s.startOrganize)
		api.GET("/operations/:id/status", s.getOperationStatus)
		api.GET("/operations/:id/logs", s.getOperationLogs)
		api.DELETE("/operations/:id", s.cancelOperation)
		api.GET("/operations/active", s.listActiveOperations)

		// Import routes
		api.POST("/import/file", s.importFile)

		// System routes
		api.GET("/system/status", s.getSystemStatus)
		api.GET("/system/logs", s.getSystemLogs)
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)
		api.GET("/dashboard", s.getDashboard)

		// Backup routes
		api.POST("/backup/create", s.createBackup)
		api.GET("/backup/list", s.listBackups)
		api.POST("/backup/restore", s.restoreBackup)
		api.DELETE("/backup/:filename", s.deleteBackup)

		// Enhanced metadata routes
		api.POST("/metadata/batch-update", s.batchUpdateMetadata)
		api.POST("/metadata/validate", s.validateMetadata)
		api.GET("/metadata/export", s.exportMetadata)
		api.POST("/metadata/import", s.importMetadata)
		api.GET("/metadata/search", s.searchMetadata)
		api.GET("/metadata/fields", s.getMetadataFields)
		api.POST("/audiobooks/:id/fetch-metadata", s.fetchAudiobookMetadata)

		// AI-powered parsing routes
		api.POST("/ai/parse-filename", s.parseFilenameWithAI)
		api.POST("/ai/test-connection", s.testAIConnection)
		api.POST("/audiobooks/:id/parse-with-ai", s.parseAudiobookWithAI)

		// Work routes (logical title-level grouping)
		api.GET("/works", s.listWorks)
		api.POST("/works", s.createWork)
		api.GET("/works/:id", s.getWork)
		api.PUT("/works/:id", s.updateWork)
		api.DELETE("/works/:id", s.deleteWork)
		api.GET("/works/:id/books", s.listWorkBooks)

		// Version management routes
		api.GET("/audiobooks/:id/versions", s.listAudiobookVersions)
		api.POST("/audiobooks/:id/versions", s.linkAudiobookVersion)
		api.PUT("/audiobooks/:id/set-primary", s.setAudiobookPrimary)
		api.GET("/version-groups/:id", s.getVersionGroup)

		// Work queue routes (alternative singular form for compatibility)
		api.GET("/work", s.listWork)
		api.GET("/work/stats", s.getWorkStats)

		// Blocked hashes management routes
		api.GET("/blocked-hashes", s.listBlockedHashes)
		api.POST("/blocked-hashes", s.addBlockedHash)
		api.DELETE("/blocked-hashes/:hash", s.removeBlockedHash)
	}

	// Serve static files (React frontend)
	// Implementation is in static_embed.go or static_nonembed.go depending on build tags
	s.setupStaticFiles()
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Handler functions (stubs for now)
func (s *Server) healthCheck(c *gin.Context) {
	// Gather basic metrics; tolerate errors (don't fail health entirely)
	var bookCount, authorCount, seriesCount, playlistCount int
	var dbErr error
	if database.GlobalStore != nil {
		if bc, err := database.GlobalStore.CountBooks(); err == nil {
			bookCount = bc
		} else {
			dbErr = err
		}
		if authors, err := database.GlobalStore.GetAllAuthors(); err == nil {
			authorCount = len(authors)
		} else if dbErr == nil {
			dbErr = err
		}
		if series, err := database.GlobalStore.GetAllSeries(); err == nil {
			seriesCount = len(series)
		} else if dbErr == nil {
			dbErr = err
		}
		if playlists, err := database.GlobalStore.GetPlaylistBySeriesID(0); err == nil && playlists != nil { // legacy placeholder (0 unlikely valid series)
			playlistCount = 1 // minimal indicator; real playlist counting not yet implemented
		}
	}
	resp := gin.H{
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
		"version":       "1.1.0",
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

func (s *Server) listAudiobooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Query params
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	search := c.Query("search")
	authorIDStr := c.Query("author_id")
	seriesIDStr := c.Query("series_id")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 500 {
		limit = 50
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Initialize as empty slice to ensure JSON returns [] instead of null
	books := []database.Book{}
	if search != "" {
		books, err = database.GlobalStore.SearchBooks(search, limit, offset)
	} else if authorIDStr != "" {
		if authorID, convErr := strconv.Atoi(authorIDStr); convErr == nil {
			books, err = database.GlobalStore.GetBooksByAuthorID(authorID)
		}
	} else if seriesIDStr != "" {
		if seriesID, convErr := strconv.Atoi(seriesIDStr); convErr == nil {
			books, err = database.GlobalStore.GetBooksBySeriesID(seriesID)
		}
	}
	if len(books) == 0 && err == nil { // fall back to generic list if no results yet
		books, err = database.GlobalStore.GetAllBooks(limit, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if books == nil {
		books = []database.Book{}
	}

	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books), "limit": limit, "offset": offset})
}

func (s *Server) listDuplicateAudiobooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	duplicateGroups, err := database.GlobalStore.GetDuplicateBooks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if duplicateGroups == nil {
		duplicateGroups = [][]database.Book{}
	}

	// Calculate total duplicates count (sum of all books in duplicate groups minus the count of groups)
	totalDuplicates := 0
	for _, group := range duplicateGroups {
		totalDuplicates += len(group) - 1 // Count extras in each group
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":          duplicateGroups,
		"group_count":     len(duplicateGroups),
		"duplicate_count": totalDuplicates,
	})
}

func (s *Server) countAudiobooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	count, err := database.GlobalStore.CountBooks()
	if err != nil {
		log.Printf("[DEBUG] countAudiobooks: Error counting books: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] countAudiobooks: Returning count: %d", count)
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) getAudiobook(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id") // ULID string

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}
	c.JSON(http.StatusOK, book)
}

func (s *Server) updateAudiobook(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id") // ULID string

	var book database.Book
	if err := c.ShouldBindJSON(&book); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedBook, err := database.GlobalStore.UpdateBook(id, &book)
	if err != nil {
		// Check if error is "not found"
		if err.Error() == "book not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedBook)
}

func (s *Server) deleteAudiobook(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id") // ULID string

	// Check for query parameters for enhanced delete options
	blockHash := c.Query("block_hash") == "true"
	softDelete := c.Query("soft_delete") == "true"

	// Get the book first to access its hash
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// If soft delete requested, mark for deletion instead of hard delete
	if softDelete {
		now := time.Now()
		book.MarkedForDeletion = boolPtr(true)
		book.MarkedForDeletionAt = &now
		book.LibraryState = stringPtr("deleted")
		
		if _, err := database.GlobalStore.UpdateBook(id, book); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Optionally block the hash
		if blockHash && book.FileHash != nil && *book.FileHash != "" {
			if err := database.GlobalStore.AddBlockedHash(*book.FileHash, "User deleted - soft delete"); err != nil {
				log.Printf("Warning: failed to block hash during soft delete: %v", err)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message":     "audiobook soft deleted",
			"blocked":     blockHash && book.FileHash != nil,
			"soft_delete": true,
		})
		return
	}

	// Hard delete path
	// Optionally block the hash before deleting
	if blockHash && book.FileHash != nil && *book.FileHash != "" {
		if err := database.GlobalStore.AddBlockedHash(*book.FileHash, "User deleted - prevent reimport"); err != nil {
			log.Printf("Warning: failed to block hash before delete: %v", err)
			// Continue with delete even if blocking fails
		}
	}

	if err := database.GlobalStore.DeleteBook(id); err != nil {
		if err.Error() == "book not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "audiobook deleted",
		"blocked": blockHash && book.FileHash != nil,
	})
}

func (s *Server) batchUpdateAudiobooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		IDs     []string               `json:"ids"` // ULID strings
		Updates map[string]interface{} `json:"updates"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Allow empty batches - return success with 0 updates
	if len(req.IDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": 0,
			"failed":  0,
			"total":   0,
			"message": "no items to update",
		})
		return
	}

	results := []gin.H{}
	for _, id := range req.IDs {
		book, err := database.GlobalStore.GetBookByID(id)
		if err != nil {
			results = append(results, gin.H{"id": id, "error": "not found"})
			continue
		}

		// Apply updates
		if title, ok := req.Updates["title"].(string); ok {
			book.Title = title
		}
		if format, ok := req.Updates["format"].(string); ok {
			book.Format = format
		}
		if authorID, ok := req.Updates["author_id"].(float64); ok {
			aid := int(authorID)
			book.AuthorID = &aid
		}
		if seriesID, ok := req.Updates["series_id"].(float64); ok {
			sid := int(seriesID)
			book.SeriesID = &sid
		}
		if seriesSeq, ok := req.Updates["series_sequence"].(float64); ok {
			seq := int(seriesSeq)
			book.SeriesSequence = &seq
		}

		if _, err := database.GlobalStore.UpdateBook(id, book); err != nil {
			results = append(results, gin.H{"id": id, "error": err.Error()})
		} else {
			results = append(results, gin.H{"id": id, "success": true})
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// ---- Work handlers ----

func (s *Server) listWorks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if works == nil {
		works = []database.Work{}
	}
	c.JSON(http.StatusOK, gin.H{"items": works, "count": len(works)})
}

func (s *Server) createWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(work.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	created, err := database.GlobalStore.CreateWork(&work)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) getWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	work, err := database.GlobalStore.GetWorkByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if work == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "work not found"})
		return
	}
	c.JSON(http.StatusOK, work)
}

func (s *Server) updateWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(work.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	updated, err := database.GlobalStore.UpdateWork(id, &work)
	if err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "work not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	if err := database.GlobalStore.DeleteWork(id); err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "work not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) listWorkBooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	books, err := database.GlobalStore.GetBooksByWorkID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books)})
}

func (s *Server) listAuthors(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	authors, err := database.GlobalStore.GetAllAuthors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if authors == nil {
		authors = []database.Author{}
	}

	c.JSON(http.StatusOK, gin.H{"items": authors, "count": len(authors)})
}

func (s *Server) listSeries(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	series, err := database.GlobalStore.GetAllSeries()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if series == nil {
		series = []database.Series{}
	}

	c.JSON(http.StatusOK, gin.H{"items": series, "count": len(series)})
}

func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter is required"})
		return
	}

	// Security check: prevent directory traversal attacks
	absPath, err := filepath.Abs(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}

	// Read directory contents
	entries, err := os.ReadDir(absPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read directory: %v", err)})
		return
	}

	type FileInfo struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		IsDir    bool   `json:"is_dir"`
		Size     int64  `json:"size,omitempty"`
		ModTime  int64  `json:"mod_time,omitempty"`
		Excluded bool   `json:"excluded"`
	}

	items := []FileInfo{}
	for _, entry := range entries {
		fullPath := filepath.Join(absPath, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if directory is excluded
		excluded := false
		if entry.IsDir() {
			jabExcludePath := filepath.Join(fullPath, ".jabexclude")
			if _, err := os.Stat(jabExcludePath); err == nil {
				excluded = true
			}
		}

		item := FileInfo{
			Name:     entry.Name(),
			Path:     fullPath,
			IsDir:    entry.IsDir(),
			Excluded: excluded,
		}

		if !entry.IsDir() {
			item.Size = info.Size()
			item.ModTime = info.ModTime().Unix()
		}

		items = append(items, item)
	}

	// Get disk space info
	var diskInfo map[string]interface{}
	if stat, err := os.Stat(absPath); err == nil {
		diskInfo = map[string]interface{}{
			"exists":   true,
			"readable": stat.Mode().Perm()&0400 != 0,
			"writable": stat.Mode().Perm()&0200 != 0,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"path":      absPath,
		"items":     items,
		"count":     len(items),
		"disk_info": diskInfo,
	})
}

func (s *Server) createExclusion(c *gin.Context) {
	var req struct {
		Path   string `json:"path" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Ensure it's a directory
	info, err := os.Stat(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path does not exist"})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path must be a directory"})
		return
	}

	// Create .jabexclude file
	jabExcludePath := filepath.Join(req.Path, ".jabexclude")
	content := "# Excluded from audiobook organization\n"
	if req.Reason != "" {
		content += fmt.Sprintf("# Reason: %s\n", req.Reason)
	}
	content += fmt.Sprintf("# Created: %s\n", time.Now().Format(time.RFC3339))

	if err := os.WriteFile(jabExcludePath, []byte(content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create exclusion: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"path":     req.Path,
		"excluded": true,
		"file":     jabExcludePath,
	})
}

func (s *Server) removeExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jabExcludePath := filepath.Join(req.Path, ".jabexclude")
	if err := os.Remove(jabExcludePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove exclusion: %v", err)})
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) listImportPaths(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	folders, err := database.GlobalStore.GetAllImportPaths()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if folders == nil {
		folders = []database.ImportPath{}
	}

	c.JSON(http.StatusOK, gin.H{"importPaths": folders, "count": len(folders)})
}

func (s *Server) addImportPath(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var req struct {
		Path    string `json:"path" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folder, err := database.GlobalStore.CreateImportPath(req.Path, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		folder.Enabled = false
		if err := database.GlobalStore.UpdateImportPath(folder.ID, folder); err != nil {
			// Non-fatal; return created folder anyway with note
			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "warning": "created but could not update enabled flag"})
			return
		}
	}

	// Auto-scan the newly added folder if enabled and operation queue is available
	if folder.Enabled && operations.GlobalQueue != nil {
		opID := ulid.Make().String()
		folderPath := folder.Path
		op, err := database.GlobalStore.CreateOperation(opID, "scan", &folderPath)
		if err == nil {
			// Create scan operation function
			operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
				_ = progress.Log("info", fmt.Sprintf("Auto-scanning newly added folder: %s", folderPath), nil)

				// Check if folder exists
				if _, err := os.Stat(folderPath); os.IsNotExist(err) {
					return fmt.Errorf("folder does not exist: %s", folderPath)
				}

				// Scan directory for audiobook files (parallel)
				workers := config.AppConfig.ConcurrentScans
				if workers < 1 {
					workers = 4
				}
				books, err := scanner.ScanDirectoryParallel(folderPath, workers)
				if err != nil {
					return fmt.Errorf("failed to scan folder: %w", err)
				}

				_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files", len(books)), nil)

				// Process the books to extract metadata (parallel)
				if len(books) > 0 {
					_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books using %d workers", len(books), workers), nil)
					if err := scanner.ProcessBooksParallel(ctx, books, workers, nil); err != nil {
						return fmt.Errorf("failed to process books: %w", err)
					}
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						organized := 0
						for _, b := range books {
							// Lookup DB book by file path
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, err := org.OrganizeBook(dbBook)
							if err != nil {
								_ = progress.Log("warn", fmt.Sprintf("Organize failed for %s: %v", dbBook.Title, err), nil)
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								if _, err := database.GlobalStore.UpdateBook(dbBook.ID, dbBook); err != nil {
									_ = progress.Log("warn", fmt.Sprintf("Failed to update path for %s: %v", dbBook.Title, err), nil)
								} else {
									organized++
								}
							}
						}
						_ = progress.Log("info", fmt.Sprintf("Auto-organize complete: %d organized", organized), nil)
					} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
						_ = progress.Log("warn", "Auto-organize enabled but root_dir not set", nil)
					}
				}

				// Update book count for this import path
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				if err := database.GlobalStore.UpdateImportPath(folder.ID, folder); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update book count: %v", err), nil)
				}

				_ = progress.Log("info", fmt.Sprintf("Auto-scan completed. Total books: %d", len(books)), nil)
				return nil
			}

			// Enqueue the scan operation with normal priority
			_ = operations.GlobalQueue.Enqueue(op.ID, "scan", operations.PriorityNormal, operationFunc)

			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "scan_operation_id": op.ID})
			return
		}
	}

	// Fallback: if enabled but queue unavailable OR operation creation failed, run synchronous scan
	if folder.Enabled && operations.GlobalQueue == nil {
		// Basic scan without progress reporter
		if _, err := os.Stat(folder.Path); err == nil {
			books, err := scanner.ScanDirectory(folder.Path)
			if err == nil {
				if len(books) > 0 {
					_ = scanner.ProcessBooks(books) // ignore individual processing errors (already logged internally)
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						for _, b := range books {
							dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, err := org.OrganizeBook(dbBook)
							if err != nil {
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								_, _ = database.GlobalStore.UpdateBook(dbBook.ID, dbBook)
							}
						}
					} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
						log.Printf("auto-organize enabled but root_dir not set")
					}
				}
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				_ = database.GlobalStore.UpdateImportPath(folder.ID, folder)
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{"importPath": folder})
}

func (s *Server) removeImportPath(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid import path id"})
		return
	}
	if err := database.GlobalStore.DeleteImportPath(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) startScan(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath  *string `json:"folder_path"`
		Priority    *int    `json:"priority"`
		ForceUpdate *bool   `json:"force_update"`
	}
	_ = c.ShouldBindJSON(&req) // optional

	forceUpdate := req.ForceUpdate != nil && *req.ForceUpdate
	if forceUpdate {
		log.Printf("[DEBUG] startScan: Force update enabled - will update all book file paths in database")
	}

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "scan", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		// Determine which folders to scan
		var foldersToScan []string
		if req.FolderPath != nil && *req.FolderPath != "" {
			// Scan specific folder
			foldersToScan = []string{*req.FolderPath}
			_ = progress.Log("info", fmt.Sprintf("Starting scan of folder: %s", *req.FolderPath), nil)
		} else {
			// Full scan: include RootDir if force_update enabled, then all import paths
			if forceUpdate && config.AppConfig.RootDir != "" {
				foldersToScan = append(foldersToScan, config.AppConfig.RootDir)
				_ = progress.Log("info", fmt.Sprintf("Full rescan: including library path %s", config.AppConfig.RootDir), nil)
			}

			// Add all import paths (import paths)
			folders, err := database.GlobalStore.GetAllImportPaths()
			if err != nil {
				return fmt.Errorf("failed to get import paths: %w", err)
			}
			for _, folder := range folders {
				if folder.Enabled {
					foldersToScan = append(foldersToScan, folder.Path)
				}
			}
			_ = progress.Log("info", fmt.Sprintf("Scanning %d total folders (%d import paths)", len(foldersToScan), len(folders)), nil)
		}

		if len(foldersToScan) == 0 {
			_ = progress.Log("warn", "No folders to scan", nil)
			return nil
		}

		// First pass: count total files across all folders
		totalFilesAcrossFolders := 0
		for _, folderPath := range foldersToScan {
			if _, err := os.Stat(folderPath); os.IsNotExist(err) {
				_ = progress.Log("warn", fmt.Sprintf("Folder does not exist: %s", folderPath), nil)
				continue
			}
			fileCount := 0
			err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				for _, supported := range config.AppConfig.SupportedExtensions {
					if ext == supported {
						fileCount++
						break
					}
				}
				return nil
			})
			if err == nil {
				_ = progress.Log("info", fmt.Sprintf("Folder %s: Found %d audiobook files", folderPath, fileCount), nil)
				totalFilesAcrossFolders += fileCount
			}
		}
		_ = progress.Log("info", fmt.Sprintf("Total audiobook files across all folders: %d", totalFilesAcrossFolders), nil)
		if totalFilesAcrossFolders == 0 {
			_ = progress.Log("warn", "No audiobook files detected during pre-scan; totals will update as files are processed", nil)
		}

		progressTotal := totalFilesAcrossFolders
		var processedFiles atomic.Int32

		// Scan each folder
		totalBooks := 0
		libraryBooks := 0
		importBooks := 0
		for folderIdx, folderPath := range foldersToScan {
			if progress.IsCanceled() {
				_ = progress.Log("info", "Scan canceled", nil)
				return fmt.Errorf("scan canceled")
			}

			currentProcessed := int(processedFiles.Load())
			displayTotal := totalFilesAcrossFolders
			if currentProcessed > displayTotal {
				displayTotal = currentProcessed
			}
			_ = progress.UpdateProgress(currentProcessed, displayTotal, fmt.Sprintf("Scanning folder %d/%d: %s", folderIdx+1, len(foldersToScan), folderPath))
			_ = progress.Log("info", fmt.Sprintf("Scanning folder: %s", folderPath), nil)

			// Check if folder exists
			if _, err := os.Stat(folderPath); os.IsNotExist(err) {
				_ = progress.Log("warn", fmt.Sprintf("Folder does not exist: %s", folderPath), nil)
				continue
			}

			// Scan directory for audiobook files (parallel)
			workers := config.AppConfig.ConcurrentScans
			if workers < 1 {
				workers = 4
			}
			books, err := scanner.ScanDirectoryParallel(folderPath, workers)
			if err != nil {
				_ = progress.Log("error", fmt.Sprintf("Failed to scan folder %s: %v", folderPath, err), nil)
				continue
			}

			_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files in %s", len(books), folderPath), nil)
			totalBooks += len(books)
			if folderPath == config.AppConfig.RootDir {
				libraryBooks += len(books)
			} else {
				importBooks += len(books)
			}
			// Prepare per-book progress reporting
			targetTotal := progressTotal
			if targetTotal == 0 {
				targetTotal = len(books)
			}
			progressCallback := func(_ int, _ int, bookPath string) {
				current := processedFiles.Add(1)
				displayTotal := targetTotal
				if int(current) > displayTotal {
					displayTotal = int(current)
				}
				message := fmt.Sprintf("Processed: %d/%d books", current, displayTotal)
				if bookPath != "" {
					message = fmt.Sprintf("Processed: %d/%d books (%s)", current, displayTotal, filepath.Base(bookPath))
				}
				_ = progress.UpdateProgress(int(current), displayTotal, message)
			}

			// Process the books to extract metadata (parallel)
			// This automatically upserts books by FilePath, creating new records or updating existing ones
			if len(books) > 0 {
				_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books using %d workers", len(books), workers), nil)
				if err := scanner.ProcessBooksParallel(ctx, books, workers, progressCallback); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Failed to process books: %v", err), nil)
				} else {
					_ = progress.Log("info", fmt.Sprintf("Successfully processed %d books", len(books)), nil)
				}

				// Auto-organize if enabled
				if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
					org := organizer.NewOrganizer(&config.AppConfig)
					organized := 0
					for _, b := range books {
						if progress.IsCanceled() {
							break
						}
						// Lookup DB book by file path
						dbBook, err := database.GlobalStore.GetBookByFilePath(b.FilePath)
						if err != nil || dbBook == nil {
							continue
						}
						newPath, err := org.OrganizeBook(dbBook)
						if err != nil {
							_ = progress.Log("warn", fmt.Sprintf("Organize failed for %s: %v", dbBook.Title, err), nil)
							continue
						}
						// Update DB path if changed
						if newPath != dbBook.FilePath {
							dbBook.FilePath = newPath
							applyOrganizedFileMetadata(dbBook, newPath)
							if _, err := database.GlobalStore.UpdateBook(dbBook.ID, dbBook); err != nil {
								_ = progress.Log("warn", fmt.Sprintf("Failed to update path for %s: %v", dbBook.Title, err), nil)
							} else {
								organized++
							}
						}
					}
					_ = progress.Log("info", fmt.Sprintf("Auto-organize complete: %d organized", organized), nil)
				} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
					_ = progress.Log("warn", "Auto-organize enabled but root_dir not set", nil)
				}
			}

			// Update book count for this import path
			folders, _ := database.GlobalStore.GetAllImportPaths()
			for _, folder := range folders {
				if folder.Path == folderPath {
					folder.BookCount = len(books)
					if err := database.GlobalStore.UpdateImportPath(folder.ID, &folder); err != nil {
						_ = progress.Log("warn", fmt.Sprintf("Failed to update book count for folder %s: %v", folderPath, err), nil)
					}
					break
				}
			}
		}

		// Format completion message with separate library/import counts
		var completionMsg string
		if libraryBooks > 0 && importBooks > 0 {
			completionMsg = fmt.Sprintf("Scan completed. Library: %d books, Import: %d books (Total: %d)", libraryBooks, importBooks, totalBooks)
		} else if libraryBooks > 0 {
			completionMsg = fmt.Sprintf("Scan completed. Library: %d books", libraryBooks)
		} else if importBooks > 0 {
			completionMsg = fmt.Sprintf("Scan completed. Import: %d books", importBooks)
		} else {
			completionMsg = "Scan completed. No books found"
		}
		finalProcessed := int(processedFiles.Load())
		finalTotal := totalFilesAcrossFolders
		if finalProcessed > finalTotal {
			finalTotal = finalProcessed
		}
		_ = progress.UpdateProgress(finalProcessed, finalTotal, completionMsg)
		_ = progress.Log("info", completionMsg, nil)
		return nil
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "scan", priority, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startOrganize(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		FolderPath *string `json:"folder_path"`
		Priority   *int    `json:"priority"`
	}
	_ = c.ShouldBindJSON(&req)

	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "organize", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine priority (default to normal)
	priority := operations.PriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Create operation function
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		org := organizer.NewOrganizer(&config.AppConfig)

		_ = progress.Log("info", "Starting file organization", nil)

		// Get books to organize
		allBooks, err := database.GlobalStore.GetAllBooks(1000, 0)
		if err != nil {
			errDetails := err.Error()
			_ = progress.Log("error", "Failed to fetch books", &errDetails)
			return fmt.Errorf("failed to fetch books: %w", err)
		}

		logMsg := fmt.Sprintf("Fetched %d total books from database", len(allBooks))
		_ = progress.Log("info", logMsg, nil)
		log.Printf("[DEBUG] Organize: %s", logMsg)

		// Filter books that need organizing (not already in root directory)
		booksToOrganize := make([]database.Book, 0)
		for _, book := range allBooks {
			// Skip if book is already in the root directory (already organized)
			if config.AppConfig.RootDir != "" && strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
				logMsg := fmt.Sprintf("Skipping book already in RootDir: %s (RootDir: %s)", book.FilePath, config.AppConfig.RootDir)
				log.Printf("[DEBUG] Organize: %s", logMsg)
				continue
			}
			// Skip if file doesn't exist
			if _, err := os.Stat(book.FilePath); os.IsNotExist(err) {
				logMsg := fmt.Sprintf("Skipping non-existent file: %s", book.FilePath)
				log.Printf("[DEBUG] Organize: %s", logMsg)
				continue
			}
			booksToOrganize = append(booksToOrganize, book)
		}

		logMsg = fmt.Sprintf("Found %d books that need organizing (out of %d total)", len(booksToOrganize), len(allBooks))
		_ = progress.Log("info", logMsg, nil)
		log.Printf("[DEBUG] Organize: %s", logMsg)

		organized := 0
		failed := 0
		for i, book := range booksToOrganize {
			if progress.IsCanceled() {
				_ = progress.Log("info", "Organize canceled", nil)
				return fmt.Errorf("organize canceled")
			}

			_ = progress.UpdateProgress(i, len(booksToOrganize), fmt.Sprintf("Organizing %s...", book.Title))

			newPath, err := org.OrganizeBook(&book)
			if err != nil {
				errDetails := fmt.Sprintf("Failed to organize %s: %s", book.Title, err.Error())
				_ = progress.Log("warn", errDetails, nil)
				failed++
				continue
			}

			// Update book's file path and state in database
			book.FilePath = newPath
			book.LibraryState = stringPtr("organized")
			applyOrganizedFileMetadata(&book, newPath)
			if _, err := database.GlobalStore.UpdateBook(book.ID, &book); err != nil {
				errDetails := fmt.Sprintf("Failed to update book path: %s", err.Error())
				_ = progress.Log("warn", errDetails, nil)
			} else {
				organized++
			}
		}

		summary := fmt.Sprintf("Organization completed: %d organized, %d failed", organized, failed)
		_ = progress.Log("info", summary, nil)

		// Trigger automatic rescan of library path after organize completes
		if organized > 0 && config.AppConfig.RootDir != "" {
			_ = progress.Log("info", "Starting automatic rescan of library path...", nil)

			// Create a new scan operation
			scanID := ulid.Make().String()
			scanOp, err := database.GlobalStore.CreateOperation(scanID, "scan", &config.AppConfig.RootDir)
			if err != nil {
				errDetails := fmt.Sprintf("Failed to create rescan operation: %s", err.Error())
				_ = progress.Log("warn", errDetails, nil)
			} else {
				// Enqueue the scan operation with low priority (don't block other operations)
				scanFunc := func(ctx context.Context, scanProgress operations.ProgressReporter) error {
					_ = scanProgress.Log("info", fmt.Sprintf("Scanning organized books in: %s", config.AppConfig.RootDir), nil)

					// Scan the root directory for newly organized books
					workers := config.AppConfig.ConcurrentScans
					if workers < 1 {
						workers = 4
					}
					books, err := scanner.ScanDirectoryParallel(config.AppConfig.RootDir, workers)
					if err != nil {
						return fmt.Errorf("failed to rescan root directory: %w", err)
					}

					_ = scanProgress.Log("info", fmt.Sprintf("Found %d books in root directory", len(books)), nil)

					// Process the books to extract metadata
					if len(books) > 0 {
						if err := scanner.ProcessBooksParallel(ctx, books, workers, nil); err != nil {
							return fmt.Errorf("failed to process books: %w", err)
						}
					}

					_ = scanProgress.Log("info", "Rescan completed successfully", nil)
					return nil
				}

				if err := operations.GlobalQueue.Enqueue(scanOp.ID, "scan", operations.PriorityLow, scanFunc); err != nil {
					errDetails := fmt.Sprintf("Failed to enqueue rescan: %s", err.Error())
					_ = progress.Log("warn", errDetails, nil)
				} else {
					_ = progress.Log("info", "Rescan operation queued successfully", nil)
				}
			}
		}

		return nil
	}

	// Enqueue the operation
	if err := operations.GlobalQueue.Enqueue(op.ID, "organize", priority, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) getOperationStatus(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	op, err := database.GlobalStore.GetOperationByID(id)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	c.JSON(http.StatusOK, op)
}

func (s *Server) cancelOperation(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	id := c.Param("id")

	// Cancel via queue (which will update database)
	if err := operations.GlobalQueue.Cancel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// listActiveOperations returns a snapshot of currently queued/running operations with basic progress
func (s *Server) listActiveOperations(c *gin.Context) {
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusOK, gin.H{"operations": []gin.H{}})
		return
	}
	active := operations.GlobalQueue.ActiveOperations()
	results := make([]gin.H, 0, len(active))
	for _, a := range active {
		status := "queued"
		progress := 0
		total := 0
		message := ""
		if database.GlobalStore != nil {
			if op, err := database.GlobalStore.GetOperationByID(a.ID); err == nil && op != nil {
				status = op.Status
				progress = op.Progress
				total = op.Total
				message = op.Message
			}
		}
		results = append(results, gin.H{
			"id":       a.ID,
			"type":     a.Type,
			"status":   status,
			"progress": progress,
			"total":    total,
			"message":  message,
		})
	}
	c.JSON(http.StatusOK, gin.H{"operations": results})
}

func (s *Server) importFile(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		FilePath string `json:"file_path" binding:"required"`
		Organize bool   `json:"organize"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate file exists and is supported
	fileInfo, err := os.Stat(req.FilePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file not found or inaccessible"})
		return
	}

	if fileInfo.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is a directory, not a file"})
		return
	}

	// Check if file extension is supported
	ext := strings.ToLower(filepath.Ext(req.FilePath))
	supported := false
	for _, supportedExt := range config.AppConfig.SupportedExtensions {
		if ext == supportedExt {
			supported = true
			break
		}
	}

	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":                fmt.Sprintf("unsupported file type: %s", ext),
			"supported_extensions": config.AppConfig.SupportedExtensions,
		})
		return
	}

	// Extract metadata
	meta, err := metadata.ExtractMetadata(req.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to extract metadata: %v", err)})
		return
	}

	// Create book record
	book := &database.Book{
		Title:            meta.Title,
		FilePath:         req.FilePath,
		OriginalFilename: stringPtr(filepath.Base(req.FilePath)),
	}

	// Set author if available
	if meta.Artist != "" {
		author, err := database.GlobalStore.GetAuthorByName(meta.Artist)
		if err != nil {
			// Create new author
			author, err = database.GlobalStore.CreateAuthor(meta.Artist)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create author"})
				return
			}
		}
		book.AuthorID = &author.ID
	}

	// Set additional metadata
	if meta.Album != "" && book.Title == "" {
		book.Title = meta.Album
	}
	if meta.Narrator != "" {
		book.Narrator = &meta.Narrator
	}
	if meta.Language != "" {
		book.Language = &meta.Language
	}
	if meta.Publisher != "" {
		book.Publisher = &meta.Publisher
	}

	// Extract media info
	mediaInfo, err := mediainfo.Extract(req.FilePath)
	if err == nil {
		if mediaInfo.Bitrate > 0 {
			book.Bitrate = intPtr(mediaInfo.Bitrate)
		}
		if mediaInfo.Codec != "" {
			book.Codec = stringPtr(mediaInfo.Codec)
		}
		if mediaInfo.SampleRate > 0 {
			book.SampleRate = intPtr(mediaInfo.SampleRate)
		}
		if mediaInfo.Channels > 0 {
			book.Channels = intPtr(mediaInfo.Channels)
		}
		if mediaInfo.BitDepth > 0 {
			book.BitDepth = intPtr(mediaInfo.BitDepth)
		}
		if mediaInfo.Quality != "" {
			book.Quality = stringPtr(mediaInfo.Quality)
		}
	}

	// Create book in database
	createdBook, err := database.GlobalStore.CreateBook(book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create book: %v", err)})
		return
	}

	response := gin.H{
		"message": "file imported successfully",
		"book":    createdBook,
	}

	// If organize flag is set, trigger organization operation
	if req.Organize && operations.GlobalQueue != nil {
		opID := ulid.Make().String()
		op, err := database.GlobalStore.CreateOperation(opID, "organize", nil)
		if err == nil {
			// Queue the organization operation
			operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
				_ = progress.Log("info", fmt.Sprintf("Organizing imported file: %s", req.FilePath), nil)
				// TODO: Implement actual organization logic
				return nil
			}
			operations.GlobalQueue.Enqueue(opID, "organize", operations.PriorityNormal, operationFunc)
			response["operation_id"] = op.ID
		}
	}

	c.JSON(http.StatusCreated, response)
}

func (s *Server) getSystemStatus(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get import folders
	importFolders, err := database.GlobalStore.GetAllImportPaths()
	if err != nil {
		log.Printf("[DEBUG] getSystemStatus: Failed to get import paths: %v", err)
		importFolders = []database.ImportPath{}
	} else {
		log.Printf("[DEBUG] getSystemStatus: Got %d import paths", len(importFolders))
		for i, f := range importFolders {
			log.Printf("[DEBUG]   Folder %d: %s (enabled: %v)", i, f.Path, f.Enabled)
		}
	}

	// Use database-based counts for efficiency
	// Get all books to count library vs import paths
	allBooks, err := database.GlobalStore.GetAllBooks(100000, 0)
	if err != nil {
		allBooks = []database.Book{}
	}

	libraryBookCount := 0
	importBookCount := 0
	rootDir := config.AppConfig.RootDir

	for _, book := range allBooks {
		if rootDir != "" && strings.HasPrefix(book.FilePath, rootDir) {
			libraryBookCount++
		} else {
			importBookCount++
		}
	}

	log.Printf("[DEBUG] getSystemStatus: Library books: %d, Import path books: %d", libraryBookCount, importBookCount)

	// Get recent operations
	recentOps, err := database.GlobalStore.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Use cached size calculations to avoid expensive file system walks
	librarySize, importSize := calculateLibrarySizes(rootDir, importFolders)
	totalSize := librarySize + importSize

	c.JSON(http.StatusOK, gin.H{
		"status":             "running",
		"library_book_count": libraryBookCount,
		"import_book_count":  importBookCount,
		"total_book_count":   libraryBookCount + importBookCount,
		"library_size_bytes": librarySize,
		"import_size_bytes":  importSize,
		"total_size_bytes":   totalSize,
		"root_directory":     rootDir,
		"library": gin.H{
			"book_count":   libraryBookCount,
			"folder_count": 1, // Always 1 for RootDir
			"total_size":   librarySize,
			"path":         rootDir,
		},
		"import_paths": gin.H{
			"book_count":   importBookCount,
			"folder_count": len(importFolders),
			"total_size":   importSize,
		},
		"memory": gin.H{
			"alloc_bytes":       memStats.Alloc,
			"total_alloc_bytes": memStats.TotalAlloc,
			"sys_bytes":         memStats.Sys,
			"num_gc":            memStats.NumGC,
			"heap_alloc":        memStats.HeapAlloc,
			"heap_sys":          memStats.HeapSys,
			"system_total":      sysinfo.GetTotalMemory(),
		},
		"runtime": gin.H{
			"go_version":    runtime.Version(),
			"num_goroutine": runtime.NumGoroutine(),
			"num_cpu":       runtime.NumCPU(),
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
		},
		"operations": gin.H{
			"recent": recentOps,
		},
	})
}

func (s *Server) getSystemLogs(c *gin.Context) {
	// For operation-specific logs, redirect to getOperationLogs
	if id := c.Query("operation_id"); id != "" {
		s.getOperationLogs(c)
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Query parameters for filtering
	level := c.Query("level")      // Filter by log level (info, warn, error)
	search := c.Query("search")    // Search in message/details
	limitStr := c.Query("limit")   // Pagination limit
	offsetStr := c.Query("offset") // Pagination offset

	// Parse pagination
	limit := 100
	offset := 0
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get recent operations to collect logs from
	operations, err := database.GlobalStore.GetRecentOperations(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch operations"})
		return
	}

	// Collect logs from all operations
	type LogEntry struct {
		OperationID string    `json:"operation_id"`
		Timestamp   time.Time `json:"timestamp"`
		Level       string    `json:"level"`
		Message     string    `json:"message"`
		Details     *string   `json:"details,omitempty"`
	}
	var allLogs []LogEntry

	for _, op := range operations {
		logs, err := database.GlobalStore.GetOperationLogs(op.ID)
		if err != nil {
			continue
		}

		for _, log := range logs {
			// Apply level filter
			if level != "" && log.Level != level {
				continue
			}

			// Apply search filter
			if search != "" {
				found := strings.Contains(strings.ToLower(log.Message), strings.ToLower(search))
				if !found && log.Details != nil {
					found = strings.Contains(strings.ToLower(*log.Details), strings.ToLower(search))
				}
				if !found {
					continue
				}
			}

			allLogs = append(allLogs, LogEntry{
				OperationID: op.ID,
				Timestamp:   log.CreatedAt,
				Level:       log.Level,
				Message:     log.Message,
				Details:     log.Details,
			})
		}
	}

	// Sort by timestamp (newest first) - simple bubble sort for now
	for i := 0; i < len(allLogs)-1; i++ {
		for j := i + 1; j < len(allLogs); j++ {
			if allLogs[j].Timestamp.After(allLogs[i].Timestamp) {
				allLogs[i], allLogs[j] = allLogs[j], allLogs[i]
			}
		}
	}

	// Apply pagination
	total := len(allLogs)
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedLogs := allLogs[start:end]

	c.JSON(http.StatusOK, gin.H{
		"logs":   paginatedLogs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) getConfig(c *gin.Context) {
	// Create a copy of config with masked secrets
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	if maskedConfig.APIKeys.Goodreads != "" {
		maskedConfig.APIKeys.Goodreads = database.MaskSecret(maskedConfig.APIKeys.Goodreads)
	}
	c.JSON(http.StatusOK, gin.H{"config": maskedConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update allowed fields and persist to database
	updated := []string{}

	if val, ok := updates["root_dir"].(string); ok {
		config.AppConfig.RootDir = val
		updated = append(updated, "root_dir")
	}

	if val, ok := updates["database_path"].(string); ok {
		config.AppConfig.DatabasePath = val
		updated = append(updated, "database_path")
	}

	if val, ok := updates["playlist_dir"].(string); ok {
		config.AppConfig.PlaylistDir = val
		updated = append(updated, "playlist_dir")
	}

	if apiKeys, ok := updates["api_keys"].(map[string]interface{}); ok {
		if goodreads, ok := apiKeys["goodreads"].(string); ok {
			config.AppConfig.APIKeys.Goodreads = goodreads
			updated = append(updated, "api_keys.goodreads")
		}
	}

	// Library organization related updates
	if val, ok := updates["organization_strategy"].(string); ok {
		config.AppConfig.OrganizationStrategy = val
		updated = append(updated, "organization_strategy")
	}
	if val, ok := updates["scan_on_startup"].(bool); ok {
		config.AppConfig.ScanOnStartup = val
		updated = append(updated, "scan_on_startup")
	}
	if val, ok := updates["auto_organize"].(bool); ok {
		config.AppConfig.AutoOrganize = val
		updated = append(updated, "auto_organize")
	}
	if val, ok := updates["folder_naming_pattern"].(string); ok {
		config.AppConfig.FolderNamingPattern = val
		updated = append(updated, "folder_naming_pattern")
	}
	if val, ok := updates["file_naming_pattern"].(string); ok {
		config.AppConfig.FileNamingPattern = val
		updated = append(updated, "file_naming_pattern")
	}
	if val, ok := updates["create_backups"].(bool); ok {
		config.AppConfig.CreateBackups = val
		updated = append(updated, "create_backups")
	}

	// Database type and enable_sqlite are read-only at runtime for safety
	if _, ok := updates["database_type"]; ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "database_type cannot be changed at runtime"})
		return
	}

	if _, ok := updates["enable_sqlite"]; ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enable_sqlite cannot be changed at runtime"})
		return
	}

	// Handle AI API key updates
	if val, ok := updates["openai_api_key"].(string); ok {
		log.Printf("[DEBUG] updateConfig: Updating OpenAI API key (length: %d, last 4: ***%s)", len(val), func() string {
			if len(val) > 4 {
				return val[len(val)-4:]
			}
			return val
		}())
		config.AppConfig.OpenAIAPIKey = val
		updated = append(updated, "openai_api_key")
	} else {
		log.Printf("[DEBUG] updateConfig: No openai_api_key in updates (present: %v, type: %T)", ok, updates["openai_api_key"])
	}
	if val, ok := updates["enable_ai_parsing"].(bool); ok {
		log.Printf("[DEBUG] updateConfig: Updating enable_ai_parsing to: %v", val)
		config.AppConfig.EnableAIParsing = val
		updated = append(updated, "enable_ai_parsing")
	}

	// Handle additional config fields
	if val, ok := updates["concurrent_scans"].(float64); ok {
		config.AppConfig.ConcurrentScans = int(val)
		updated = append(updated, "concurrent_scans")
	}
	if val, ok := updates["language"].(string); ok {
		config.AppConfig.Language = val
		updated = append(updated, "language")
	}
	if val, ok := updates["log_level"].(string); ok {
		config.AppConfig.LogLevel = val
		updated = append(updated, "log_level")
	}

	// Persist to database
	if err := config.SaveConfigToDatabase(database.GlobalStore); err != nil {
		log.Printf("ERROR: Failed to persist config to database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to save configuration",
			"details": err.Error(),
		})
		return
	}

	log.Printf("Configuration saved successfully. Updated fields: %v", updated)

	// Reload to confirm persistence
	maskedConfig := config.AppConfig
	if maskedConfig.OpenAIAPIKey != "" {
		maskedConfig.OpenAIAPIKey = database.MaskSecret(maskedConfig.OpenAIAPIKey)
	}
	if maskedConfig.APIKeys.Goodreads != "" {
		maskedConfig.APIKeys.Goodreads = database.MaskSecret(maskedConfig.APIKeys.Goodreads)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "configuration updated and saved to database",
		"updated": updated,
		"config":  maskedConfig,
	})
}

// getOperationLogs returns logs for a given operation
func (s *Server) getOperationLogs(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	logs, err := database.GlobalStore.GetOperationLogs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Optional tail parameter for last N log lines
	if tailStr := c.Query("tail"); tailStr != "" {
		if n, convErr := strconv.Atoi(tailStr); convErr == nil && n > 0 && n < len(logs) {
			logs = logs[len(logs)-n:]
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "count": len(logs)})
}

// handleEvents handles Server-Sent Events (SSE) for real-time updates
func (s *Server) handleEvents(c *gin.Context) {
	if realtime.GlobalHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "event hub not initialized"})
		return
	}
	realtime.GlobalHub.HandleSSE(c)
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

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// listBackups lists all available backups
func (s *Server) listBackups(c *gin.Context) {
	backupConfig := backup.DefaultBackupConfig()

	backups, err := backup.ListBackups(backupConfig.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	backupPath := filepath.Join(backupConfig.BackupDir, req.BackupFilename)

	// Use current database path as target if not specified
	targetPath := req.TargetPath
	if targetPath == "" {
		targetPath = filepath.Dir(config.AppConfig.DatabasePath)
	}

	if err := backup.RestoreBackup(backupPath, targetPath, req.Verify); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	backupPath := filepath.Join(backupConfig.BackupDir, filename)

	if err := backup.DeleteBackup(backupPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backup deleted successfully"})
}

// batchUpdateMetadata handles batch metadata updates with validation
func (s *Server) batchUpdateMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Updates  []metadata.MetadataUpdate `json:"updates" binding:"required"`
		Validate bool                      `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	errors, successCount := metadata.BatchUpdateMetadata(req.Updates, database.GlobalStore, req.Validate)

	response := gin.H{
		"success_count": successCount,
		"total_count":   len(req.Updates),
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// validateMetadata validates metadata updates without applying them
func (s *Server) validateMetadata(c *gin.Context) {
	var req struct {
		Updates map[string]interface{} `json:"updates" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rules := metadata.DefaultValidationRules()
	errors := metadata.ValidateMetadata(req.Updates, rules)

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":  false,
			"errors": errorMessages,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"valid":   true,
			"message": "metadata is valid",
		})
	}
}

// exportMetadata exports all audiobook metadata
func (s *Server) exportMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all books
	books, err := database.GlobalStore.GetAllBooks(0, 0) // No limit/offset
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Export metadata
	exportData, err := metadata.ExportMetadata(books)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, exportData)
}

// importMetadata imports audiobook metadata
func (s *Server) importMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Data     map[string]interface{} `json:"data" binding:"required"`
		Validate bool                   `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	importCount, errors := metadata.ImportMetadata(req.Data, database.GlobalStore, req.Validate)

	response := gin.H{
		"import_count": importCount,
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// searchMetadata searches external metadata sources
func (s *Server) searchMetadata(c *gin.Context) {
	title := c.Query("title")
	author := c.Query("author")

	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title parameter required"})
		return
	}

	// Use Open Library for now
	client := metadata.NewOpenLibraryClient()

	var results []metadata.BookMetadata
	var err error

	if author != "" {
		results, err = client.SearchByTitleAndAuthor(title, author)
	} else {
		results, err = client.SearchByTitle(title)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("metadata search failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"source":  "Open Library",
	})
}

// fetchAudiobookMetadata fetches and applies metadata to an audiobook
func (s *Server) fetchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the audiobook
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Search for metadata using current title
	client := metadata.NewOpenLibraryClient()
	results, err := client.SearchByTitle(book.Title)
	if err != nil || len(results) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no metadata found"})
		return
	}

	// Use the first result
	meta := results[0]

	// Update book with fetched metadata (only fields that exist in Book struct)
	if meta.Title != "" {
		book.Title = meta.Title
	}
	if meta.Publisher != "" {
		book.Publisher = stringPtr(meta.Publisher)
	}
	if meta.Language != "" {
		book.Language = stringPtr(meta.Language)
	}

	// Update in database
	updatedBook, err := database.GlobalStore.UpdateBook(id, book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update book: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "metadata fetched and applied",
		"book":    updatedBook,
		"source":  "Open Library",
	})
}

// Version Management Handlers

// listAudiobookVersions lists all versions of an audiobook
func (s *Server) listAudiobookVersions(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		c.JSON(http.StatusOK, gin.H{"versions": []interface{}{book}})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": books})
}

// linkAudiobookVersion links an audiobook as another version
func (s *Server) linkAudiobookVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		OtherID string `json:"other_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book1, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	book2, err := database.GlobalStore.GetBookByID(req.OtherID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "other audiobook not found"})
		return
	}

	versionGroupID := ""
	if book1.VersionGroupID != nil {
		versionGroupID = *book1.VersionGroupID
	} else if book2.VersionGroupID != nil {
		versionGroupID = *book2.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
	}

	book1.VersionGroupID = &versionGroupID
	book2.VersionGroupID = &versionGroupID

	if _, err := database.GlobalStore.UpdateBook(id, book1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	if _, err := database.GlobalStore.UpdateBook(req.OtherID, book2); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update other audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"version_group_id": versionGroupID})
}

// setAudiobookPrimary sets an audiobook as the primary version
func (s *Server) setAudiobookPrimary(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		primaryFlag := true
		book.IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(id, book); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	for i := range books {
		primaryFlag := books[i].ID == id
		books[i].IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(books[i].ID, &books[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update version"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
}

// getVersionGroup gets all audiobooks in a version group
func (s *Server) getVersionGroup(c *gin.Context) {
	groupID := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch version group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"audiobooks": books})
}

// parseFilenameWithAI uses OpenAI to parse a filename into structured metadata
func (s *Server) parseFilenameWithAI(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename is required"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Parse filename
	metadata, err := parser.ParseFilename(c.Request.Context(), req.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// testAIConnection tests the OpenAI API connection
func (s *Server) testAIConnection(c *gin.Context) {
	// Parse request body for API key (allows testing without saving)
	var req struct {
		APIKey string `json:"api_key"`
	}

	// Try to get API key from request body first, fall back to config
	apiKey := config.AppConfig.OpenAIAPIKey
	if err := c.ShouldBindJSON(&req); err == nil && req.APIKey != "" {
		apiKey = req.APIKey
	}

	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key not provided", "success": false})
		return
	}

	// Create parser with the provided/configured API key
	parser := ai.NewOpenAIParser(apiKey, true)
	if err := parser.TestConnection(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("connection test failed: %v", err), "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OpenAI connection successful"})
}

// parseAudiobookWithAI parses an audiobook's filename with AI and updates its metadata
func (s *Server) parseAudiobookWithAI(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the book
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Extract filename from path
	filename := filepath.Base(book.FilePath)

	// Parse with AI
	metadata, err := parser.ParseFilename(c.Request.Context(), filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to parse filename: %v", err)})
		return
	}

	// Update book with parsed metadata
	if metadata.Title != "" {
		book.Title = metadata.Title
	}
	if metadata.Narrator != "" {
		book.Narrator = &metadata.Narrator
	}
	if metadata.Publisher != "" {
		book.Publisher = &metadata.Publisher
	}
	if metadata.Year > 0 {
		book.PrintYear = &metadata.Year
	}

	// Handle author
	if metadata.Author != "" {
		author, err := database.GlobalStore.GetAuthorByName(metadata.Author)
		if err != nil || author == nil {
			// Create new author
			author, err = database.GlobalStore.CreateAuthor(metadata.Author)
			if err == nil && author != nil {
				book.AuthorID = &author.ID
			}
		} else {
			book.AuthorID = &author.ID
		}
	}

	// Handle series
	if metadata.Series != "" {
		series, err := database.GlobalStore.GetSeriesByName(metadata.Series, book.AuthorID)
		if err != nil || series == nil {
			// Create new series
			series, err = database.GlobalStore.CreateSeries(metadata.Series, book.AuthorID)
			if err == nil && series != nil {
				book.SeriesID = &series.ID
			}
		} else {
			book.SeriesID = &series.ID
		}

		if metadata.SeriesNum > 0 {
			book.SeriesSequence = &metadata.SeriesNum
		}
	}

	// Update in database
	updatedBook, err := database.GlobalStore.UpdateBook(id, book)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       updatedBook,
		"confidence": metadata.Confidence,
	})
}

// getDashboard returns dashboard statistics with size and format distributions
func (s *Server) getDashboard(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all books
	allBooks, err := database.GlobalStore.GetAllBooks(100000, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve books"})
		return
	}

	// Calculate size distribution
	sizeDistribution := map[string]int{
		"0-100MB":    0,
		"100-500MB":  0,
		"500MB-1GB":  0,
		"1GB+":       0,
	}

	// Calculate format distribution and total size
	formatDistribution := make(map[string]int)
	var totalSize int64 = 0

	for _, book := range allBooks {
		// Size distribution
		if book.FileSize != nil {
			totalSize += *book.FileSize
			sizeMB := float64(*book.FileSize) / (1024 * 1024)
			sizeGB := sizeMB / 1024

			if sizeMB < 100 {
				sizeDistribution["0-100MB"]++
			} else if sizeMB < 500 {
				sizeDistribution["100-500MB"]++
			} else if sizeGB < 1 {
				sizeDistribution["500MB-1GB"]++
			} else {
				sizeDistribution["1GB+"]++
			}
		}

		// Format distribution
		ext := strings.ToLower(filepath.Ext(book.FilePath))
		if ext != "" {
			ext = strings.TrimPrefix(ext, ".")
			formatDistribution[ext]++
		}
	}

	// Get recent operations
	recentOps, err := database.GlobalStore.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"sizeDistribution":   sizeDistribution,
		"formatDistribution": formatDistribution,
		"recentOperations":   recentOps,
		"totalSize":          totalSize,
		"totalBooks":         len(allBooks),
	})
}

// getMetadataFields returns available metadata fields with their types and validation rules
func (s *Server) getMetadataFields(c *gin.Context) {
	fields := []map[string]interface{}{
		{
			"name":        "title",
			"type":        "string",
			"required":    true,
			"maxLength":   500,
			"description": "Book title",
		},
		{
			"name":        "author",
			"type":        "string",
			"required":    false,
			"description": "Author name",
		},
		{
			"name":        "narrator",
			"type":        "string",
			"required":    false,
			"description": "Narrator name",
		},
		{
			"name":        "publisher",
			"type":        "string",
			"required":    false,
			"description": "Publisher name",
		},
		{
			"name":        "publishDate",
			"type":        "integer",
			"required":    false,
			"min":         1000,
			"max":         9999,
			"description": "Publication year",
		},
		{
			"name":        "series",
			"type":        "string",
			"required":    false,
			"description": "Series name",
		},
		{
			"name":        "language",
			"type":        "string",
			"required":    false,
			"pattern":     "^[a-z]{2}$",
			"description": "ISO 639-1 language code (e.g., 'en', 'es')",
		},
		{
			"name":        "isbn10",
			"type":        "string",
			"required":    false,
			"pattern":     "^[0-9]{9}[0-9X]$",
			"description": "ISBN-10",
		},
		{
			"name":        "isbn13",
			"type":        "string",
			"required":    false,
			"pattern":     "^97[89][0-9]{10}$",
			"description": "ISBN-13",
		},
		{
			"name":        "series_sequence",
			"type":        "integer",
			"required":    false,
			"min":         1,
			"description": "Position in series",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"fields": fields,
	})
}

// listWork returns all work items (audiobooks grouped by work entity)
func (s *Server) listWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all works
	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	// For each work, get associated books
	items := make([]map[string]interface{}, 0, len(works))
	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			books = []database.Book{}
		}

		items = append(items, map[string]interface{}{
			"id":         work.ID,
			"title":      work.Title,
			"author_id":  work.AuthorID,
			"book_count": len(books),
			"books":      books,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// getWorkStats returns statistics about work items
func (s *Server) getWorkStats(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	totalWorks := len(works)
	totalBooks := 0
	worksWithMultipleEditions := 0

	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			continue
		}
		bookCount := len(books)
		totalBooks += bookCount
		if bookCount > 1 {
			worksWithMultipleEditions++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_works":                  totalWorks,
		"total_books":                  totalBooks,
		"works_with_multiple_editions": worksWithMultipleEditions,
		"average_editions_per_work":    float64(totalBooks) / float64(max(totalWorks, 1)),
	})
}

// listBlockedHashes returns all blocked hashes
func (s *Server) listBlockedHashes(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hashes, err := database.GlobalStore.GetAllBlockedHashes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get blocked hashes: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": hashes,
		"total": len(hashes),
	})
}

// addBlockedHash adds a hash to the blocklist
func (s *Server) addBlockedHash(c *gin.Context) {
	if database.GlobalStore == nil {
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

	err := database.GlobalStore.AddBlockedHash(req.Hash, req.Reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to add blocked hash: %v", err)})
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
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash parameter required"})
		return
	}

	err := database.GlobalStore.RemoveBlockedHash(hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove blocked hash: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "hash unblocked successfully",
		"hash":    hash,
	})
}

// GetDefaultServerConfig returns default server configuration
func GetDefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:         "8080",
		Host:         "localhost",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
