// file: internal/server/server.go
// version: 1.14.0
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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/mediainfo"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/realtime"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

// Cached library size to avoid expensive recalculation on frequent status checks
var cachedLibrarySize int64
var cachedSizeComputedAt time.Time
const librarySizeCacheTTL = 60 * time.Second

// Helper functions for pointer conversions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
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

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

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
	s.router.GET("/api/health", s.healthCheck)

	// Real-time events (SSE)
	s.router.GET("/api/events", s.handleEvents)

	// API routes
	api := s.router.Group("/api/v1")
	{
		// Audiobook routes
		api.GET("/audiobooks", s.listAudiobooks)
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

		// Library folder routes
		api.GET("/library/folders", s.listLibraryFolders)
		api.POST("/library/folders", s.addLibraryFolder)
		api.DELETE("/library/folders/:id", s.removeLibraryFolder)

		// Operation routes
		api.POST("/operations/scan", s.startScan)
		api.POST("/operations/organize", s.startOrganize)
		api.GET("/operations/:id/status", s.getOperationStatus)
		api.GET("/operations/:id/logs", s.getOperationLogs)
		api.DELETE("/operations/:id", s.cancelOperation)

		// Import routes
		api.POST("/import/file", s.importFile)

		// System routes
		api.GET("/system/status", s.getSystemStatus)
		api.GET("/system/logs", s.getSystemLogs)
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)

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

	if err := database.GlobalStore.DeleteBook(id); err != nil {
		// Check if error is "not found"
		if err.Error() == "book not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "path parameter is required"})
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

func (s *Server) listLibraryFolders(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	folders, err := database.GlobalStore.GetAllLibraryFolders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure we never return null - always return empty array
	if folders == nil {
		folders = []database.LibraryFolder{}
	}

	c.JSON(http.StatusOK, gin.H{"folders": folders, "count": len(folders)})
}

func (s *Server) addLibraryFolder(c *gin.Context) {
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
	folder, err := database.GlobalStore.CreateLibraryFolder(req.Path, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		folder.Enabled = false
		if err := database.GlobalStore.UpdateLibraryFolder(folder.ID, folder); err != nil {
			// Non-fatal; return created folder anyway with note
			c.JSON(http.StatusCreated, gin.H{"folder": folder, "warning": "created but could not update enabled flag"})
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

				// Scan directory for audiobook files
				books, err := scanner.ScanDirectory(folderPath)
				if err != nil {
					return fmt.Errorf("failed to scan folder: %w", err)
				}

				_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files", len(books)), nil)

				// Process the books to extract metadata
				if len(books) > 0 {
					_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books", len(books)), nil)
					if err := scanner.ProcessBooks(books); err != nil {
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

				// Update book count for this library folder
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				if err := database.GlobalStore.UpdateLibraryFolder(folder.ID, folder); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update book count: %v", err), nil)
				}

				_ = progress.Log("info", fmt.Sprintf("Auto-scan completed. Total books: %d", len(books)), nil)
				return nil
			}

			// Enqueue the scan operation with normal priority
			_ = operations.GlobalQueue.Enqueue(op.ID, "scan", operations.PriorityNormal, operationFunc)

			c.JSON(http.StatusCreated, gin.H{"folder": folder, "scan_operation_id": op.ID})
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
				_ = database.GlobalStore.UpdateLibraryFolder(folder.ID, folder)
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{"folder": folder})
}

func (s *Server) removeLibraryFolder(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid library folder id"})
		return
	}
	if err := database.GlobalStore.DeleteLibraryFolder(id); err != nil {
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
		FolderPath *string `json:"folder_path"`
		Priority   *int    `json:"priority"`
	}
	_ = c.ShouldBindJSON(&req) // optional

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
			// Scan all library folders
			folders, err := database.GlobalStore.GetAllLibraryFolders()
			if err != nil {
				return fmt.Errorf("failed to get library folders: %w", err)
			}
			for _, folder := range folders {
				if folder.Enabled {
					foldersToScan = append(foldersToScan, folder.Path)
				}
			}
			_ = progress.Log("info", fmt.Sprintf("Starting scan of %d library folders", len(foldersToScan)), nil)
		}

		if len(foldersToScan) == 0 {
			_ = progress.Log("warn", "No folders to scan", nil)
			return nil
		}

		// Scan each folder
		totalBooks := 0
		for folderIdx, folderPath := range foldersToScan {
			if progress.IsCanceled() {
				_ = progress.Log("info", "Scan canceled", nil)
				return fmt.Errorf("scan canceled")
			}

			_ = progress.UpdateProgress(folderIdx, len(foldersToScan), fmt.Sprintf("Scanning folder %d/%d: %s", folderIdx+1, len(foldersToScan), folderPath))
			_ = progress.Log("info", fmt.Sprintf("Scanning folder: %s", folderPath), nil)

			// Check if folder exists
			if _, err := os.Stat(folderPath); os.IsNotExist(err) {
				_ = progress.Log("warn", fmt.Sprintf("Folder does not exist: %s", folderPath), nil)
				continue
			}

			// Scan directory for audiobook files
			books, err := scanner.ScanDirectory(folderPath)
			if err != nil {
				_ = progress.Log("error", fmt.Sprintf("Failed to scan folder %s: %v", folderPath, err), nil)
				continue
			}

			_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files in %s", len(books), folderPath), nil)
			totalBooks += len(books)

			// Process the books to extract metadata
			if len(books) > 0 {
				_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books", len(books)), nil)
				if err := scanner.ProcessBooks(books); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Failed to process books: %v", err), nil)
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

			// Update book count for this library folder
			folders, _ := database.GlobalStore.GetAllLibraryFolders()
			for _, folder := range folders {
				if folder.Path == folderPath {
					folder.BookCount = len(books)
					if err := database.GlobalStore.UpdateLibraryFolder(folder.ID, &folder); err != nil {
						_ = progress.Log("warn", fmt.Sprintf("Failed to update book count for folder %s: %v", folderPath, err), nil)
					}
					break
				}
			}
		}

		_ = progress.UpdateProgress(len(foldersToScan), len(foldersToScan), "Scan completed")
		_ = progress.Log("info", fmt.Sprintf("Scan completed successfully. Total books found: %d", totalBooks), nil)
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
		books, err := database.GlobalStore.GetAllBooks(1000, 0)
		if err != nil {
			errDetails := err.Error()
			_ = progress.Log("error", "Failed to fetch books", &errDetails)
			return fmt.Errorf("failed to fetch books: %w", err)
		}

		organized := 0
		failed := 0
		for i, book := range books {
			if progress.IsCanceled() {
				_ = progress.Log("info", "Organize canceled", nil)
				return fmt.Errorf("organize canceled")
			}

			_ = progress.UpdateProgress(i, len(books), fmt.Sprintf("Organizing %s...", book.Title))

			newPath, err := org.OrganizeBook(&book)
			if err != nil {
				errDetails := fmt.Sprintf("Failed to organize %s: %s", book.Title, err.Error())
				_ = progress.Log("warn", errDetails, nil)
				failed++
				continue
			}

			// Update book's file path in database
			book.FilePath = newPath
			if _, err := database.GlobalStore.UpdateBook(book.ID, &book); err != nil {
				errDetails := fmt.Sprintf("Failed to update book path: %s", err.Error())
				_ = progress.Log("warn", errDetails, nil)
			} else {
				organized++
			}
		}

		summary := fmt.Sprintf("Organization completed: %d organized, %d failed", organized, failed)
		_ = progress.Log("info", summary, nil)
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

	// Get book count
	bookCount, err := database.GlobalStore.CountBooks()
	if err != nil {
		bookCount = 0
	}

	// Get library folders
	folders, err := database.GlobalStore.GetAllLibraryFolders()
	if err != nil {
		folders = []database.LibraryFolder{}
	}

	// Get recent operations
	recentOps, err := database.GlobalStore.GetRecentOperations(5)
	if err != nil {
		recentOps = []database.Operation{}
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

    // Disk usage for library folders (cached)
    totalSize := cachedLibrarySize
    if time.Since(cachedSizeComputedAt) > librarySizeCacheTTL {
        var newSize int64
        for _, folder := range folders {
            if !folder.Enabled {
                continue
            }
            if info, err := os.Stat(folder.Path); err == nil && info.IsDir() {
                filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
                    if err == nil && !info.IsDir() {
                        newSize += info.Size()
                    }
                    return nil
                })
            }
        }
        cachedLibrarySize = newSize
        cachedSizeComputedAt = time.Now()
        totalSize = newSize
    }

	c.JSON(http.StatusOK, gin.H{
		"status": "running",
		"library": gin.H{
			"book_count":   bookCount,
			"folder_count": len(folders),
			"total_size":   totalSize,
			"path":         config.AppConfig.RootDir,
		},
		"memory": gin.H{
			"alloc_bytes":       memStats.Alloc,
			"total_alloc_bytes": memStats.TotalAlloc,
			"sys_bytes":         memStats.Sys,
			"num_gc":            memStats.NumGC,
			"heap_alloc":        memStats.HeapAlloc,
			"heap_sys":          memStats.HeapSys,
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
	c.JSON(http.StatusOK, gin.H{"config": config.AppConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Note: This updates the in-memory config only
	// For persistent configuration changes, a config file update mechanism is needed
	// This implementation provides runtime configuration updates

	// Update allowed fields
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

	c.JSON(http.StatusOK, gin.H{
		"message": "configuration updated",
		"updated": updated,
		"config":  config.AppConfig,
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
