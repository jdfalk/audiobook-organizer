// file: internal/server/server.go
// version: 1.0.0
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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
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
		c.HTML(http.StatusOK, "", `
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
		`)
	})

	// Catch-all route for SPA (Single Page Application)
	s.router.NoRoute(func(c *gin.Context) {
		// Return 404 for unknown API routes
		if c.Request.URL.Path[:4] == "/api" {
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
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0",
	})
}

func (s *Server) listAudiobooks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "List audiobooks - not implemented yet"})
}

func (s *Server) getAudiobook(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Get audiobook - not implemented yet"})
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
	c.JSON(http.StatusOK, gin.H{"message": "List library folders - not implemented yet"})
}

func (s *Server) addLibraryFolder(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Add library folder - not implemented yet"})
}

func (s *Server) removeLibraryFolder(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Remove library folder - not implemented yet"})
}

func (s *Server) startScan(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Start scan - not implemented yet"})
}

func (s *Server) startOrganize(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Start organize - not implemented yet"})
}

func (s *Server) getOperationStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Get operation status - not implemented yet"})
}

func (s *Server) cancelOperation(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Cancel operation - not implemented yet"})
}

func (s *Server) getSystemStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Get system status - not implemented yet"})
}

func (s *Server) getSystemLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Get system logs - not implemented yet"})
}

func (s *Server) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"config": config.AppConfig})
}

func (s *Server) updateConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Update config - not implemented yet"})
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
