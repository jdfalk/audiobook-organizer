// file: internal/server/server.go
// version: 1.2.0
// guid: 4c5d6e7f-8a9b-0c1d-2e3f-4a5b6c7d8e9f

package server

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// webFS holds embedded web assets (will be populated when frontend is built)
// TODO: Add go:embed directive when web/dist directory exists
var webFS embed.FS

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

		// System routes
		api.GET("/system/status", s.getSystemStatus)
		api.GET("/system/logs", s.getSystemLogs)
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)
	}

	// Serve static files (React frontend)
	s.setupStaticFiles()
}

// setupStaticFiles serves the React frontend
func (s *Server) setupStaticFiles() {
	// For now, just serve a simple index page at root
	// TODO: Implement proper static file serving when frontend is built
	s.router.GET("/", func(c *gin.Context) {
		html := `
<!DOCTYPE html>
<html>
<head>
    <title>Audiobook Organizer</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background-color: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        .api-list { background: #f8f9fa; padding: 20px; border-radius: 4px; margin: 20px 0; }
        .api-endpoint { font-family: 'Courier New', monospace; background: #e9ecef; padding: 4px 8px; margin: 2px 0; border-radius: 3px; display: block; }
        .method { color: #007bff; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎧 Audiobook Organizer Web Interface</h1>
        <p>The web interface is currently under development. You can use the API endpoints below:</p>

        <div class="api-list">
            <h3>Available API Endpoints:</h3>
            <code class="api-endpoint"><span class="method">GET</span> /api/health - Health check</code>
            <code class="api-endpoint"><span class="method">GET</span> /api/v1/audiobooks - List audiobooks</code>
            <code class="api-endpoint"><span class="method">GET</span> /api/v1/authors - List authors</code>
            <code class="api-endpoint"><span class="method">GET</span> /api/v1/series - List series</code>
            <code class="api-endpoint"><span class="method">GET</span> /api/v1/config - Get configuration</code>
            <code class="api-endpoint"><span class="method">POST</span> /api/v1/operations/scan - Start scan</code>
        </div>

        <p>Try the health check: <a href="/api/health" target="_blank">/api/health</a></p>
        <p>View configuration: <a href="/api/v1/config" target="_blank">/api/v1/config</a></p>
    </div>
</body>
</html>
		`
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})

	// Catch-all route for SPA (Single Page Application)
	s.router.NoRoute(func(c *gin.Context) {
		// Return 404 for unknown API routes
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		// Redirect other routes to home page for now
		c.Redirect(http.StatusFound, "/")
	})
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

	var books []database.Book
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
	if books == nil && err == nil { // fall back to generic list
		books, err = database.GlobalStore.GetAllBooks(limit, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books), "limit": limit, "offset": offset})
}

func (s *Server) getAudiobook(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audiobook id"})
		return
	}
	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}
	c.JSON(http.StatusOK, book)
}

func (s *Server) updateAudiobook(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Update audiobook - not implemented yet"})
}

func (s *Server) deleteAudiobook(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Delete audiobook - not implemented yet"})
}

func (s *Server) batchUpdateAudiobooks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Batch update audiobooks - not implemented yet"})
}

func (s *Server) listAuthors(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "List authors - not implemented yet"})
}

func (s *Server) listSeries(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "List series - not implemented yet"})
}

func (s *Server) browseFilesystem(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Browse filesystem - not implemented yet"})
}

func (s *Server) createExclusion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Create exclusion - not implemented yet"})
}

func (s *Server) removeExclusion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Remove exclusion - not implemented yet"})
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
	c.JSON(http.StatusOK, gin.H{"items": folders, "count": len(folders)})
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
	var req struct {
		FolderPath *string `json:"folder_path"`
	}
	_ = c.ShouldBindJSON(&req) // optional
	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "scan", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = database.GlobalStore.UpdateOperationStatus(op.ID, "queued", 0, 0, "scan requested")
	c.JSON(http.StatusAccepted, op)
}

func (s *Server) startOrganize(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var req struct {
		FolderPath *string `json:"folder_path"`
	}
	_ = c.ShouldBindJSON(&req)
	id := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(id, "organize", req.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = database.GlobalStore.UpdateOperationStatus(op.ID, "queued", 0, 0, "organize requested")
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
	id := c.Param("id")
	if err := database.GlobalStore.UpdateOperationStatus(id, "canceled", 0, 0, "operation canceled by user"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) getSystemStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Get system status - not implemented yet"})
}

func (s *Server) getSystemLogs(c *gin.Context) {
	// For now, redirect to operation logs when an operation_id is provided
	if id := c.Query("operation_id"); id != "" {
		s.getOperationLogs(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "system logs not implemented; pass operation_id to query operation logs"})
}

func (s *Server) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"config": config.AppConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Update config - not implemented yet"})
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
