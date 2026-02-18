// file: internal/server/covers.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// handleCoverProxy proxies and caches cover images from external URLs.
// GET /api/v1/covers/proxy?url=https://covers.openlibrary.org/...
func (s *Server) handleCoverProxy(c *gin.Context) {
	coverURL := c.Query("url")
	if coverURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url parameter required"})
		return
	}

	// Only allow known cover sources
	if !isAllowedCoverSource(coverURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL not from an allowed cover source"})
		return
	}

	// Generate cache path
	cacheDir := filepath.Join(config.AppConfig.RootDir, ".covers")
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(coverURL)))
	ext := ".jpg"
	if strings.Contains(coverURL, ".png") {
		ext = ".png"
	}
	cachePath := filepath.Join(cacheDir, hash+ext)

	// Serve from cache if exists
	if _, err := os.Stat(cachePath); err == nil {
		c.File(cachePath)
		return
	}

	// Fetch from source
	resp, err := http.Get(coverURL) //nolint:gosec // URL is validated above
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch cover"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{"error": "cover source returned error"})
		return
	}

	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create cache directory"})
		return
	}

	// Write to cache
	f, err := os.Create(cachePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cache cover"})
		return
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(cachePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write cover"})
		return
	}
	f.Close()

	// Serve the cached file
	c.File(cachePath)
}

// handleLocalCover serves locally extracted cover art files.
// GET /api/v1/covers/local/:filename
func (s *Server) handleLocalCover(c *gin.Context) {
	filename := c.Param("filename")

	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	coverDir := filepath.Join(config.AppConfig.RootDir, ".covers")
	coverPath := filepath.Join(coverDir, filename)

	if _, err := os.Stat(coverPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "cover not found"})
		return
	}

	c.File(coverPath)
}

func isAllowedCoverSource(url string) bool {
	allowed := []string{
		"https://covers.openlibrary.org/",
		"https://books.google.com/",
		"https://images-na.ssl-images-amazon.com/",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}
