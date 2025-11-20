// file: internal/server/static_embed.go
// version: 1.2.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d
//go:build embed_frontend

package server

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

// webFS holds the embedded filesystem passed from main package
var webFS embed.FS

// SetEmbeddedFS sets the embedded filesystem for serving static files
func SetEmbeddedFS(fs embed.FS) {
	webFS = fs
}

// setupStaticFiles serves the embedded React frontend
func (s *Server) setupStaticFiles() {
	// Get the subdirectory for web/dist
	webDist, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		// If embedded files not available, fall back to placeholder
		s.setupPlaceholder()
		return
	}

	// Serve all static files (assets/, vite.svg, etc.) directly from webDist root
	// This allows /assets/vendor.js to map to assets/vendor.js in the filesystem
	httpFS := http.FS(webDist)

	// Try to serve files from the filesystem first
	fileServer := http.FileServer(httpFS)
	s.router.NoRoute(func(c *gin.Context) {
		// Return 404 for unknown API routes
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
			return
		}

		// Try to serve the file from the embedded filesystem
		path := c.Request.URL.Path
		if _, err := webDist.Open(path[1:]); err == nil {
			// File exists, serve it
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// File not found, serve index.html for SPA routing
		indexData, err := fs.ReadFile(webDist, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to load frontend")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
	})
}

// setupPlaceholder serves the API documentation placeholder page (fallback)
func (s *Server) setupPlaceholder() {
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
        .error-box { background: #f8d7da; border-left: 4px solid #dc3545; padding: 15px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎧 Audiobook Organizer</h1>
        <div class="error-box">
            <strong>⚠️ Error Loading Frontend</strong><br>
            The embedded frontend files could not be loaded. Please check that the web/dist directory was properly embedded during build.
        </div>
        <p>The API server is still available at <code>/api</code> endpoints.</p>
    </div>
</body>
</html>
		`
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})
}
