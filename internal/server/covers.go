// file: internal/server/covers.go
// version: 1.3.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-01

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
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

// handleCoverProxy proxies and caches cover images from external URLs.
// GET /api/v1/covers/proxy?url=https://covers.openlibrary.org/...
func (s *Server) handleCoverProxy(c *gin.Context) {
	coverURL := c.Query("url")
	if coverURL == "" {
		httputil.RespondWithBadRequest(c, "url parameter required")
		return
	}

	// Only allow known cover sources
	if !isAllowedCoverSource(coverURL) {
		httputil.RespondWithBadRequest(c, "URL not from an allowed cover source")
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
		httputil.RespondWithError(c, http.StatusBadGateway, "failed to fetch cover", "BAD_GATEWAY")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		httputil.RespondWithError(c, resp.StatusCode, "cover source returned error", "UPSTREAM_ERROR")
		return
	}

	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0775); err != nil {
		httputil.RespondWithInternalError(c, "failed to create cache directory")
		return
	}

	// Write to cache
	f, err := os.Create(cachePath)
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to cache cover")
		return
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(cachePath)
		httputil.RespondWithInternalError(c, "failed to write cover")
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
		httputil.RespondWithBadRequest(c, "invalid filename")
		return
	}

	// Check both .covers/ (proxy cache) and covers/ (downloaded covers)
	dirs := []string{
		filepath.Join(config.AppConfig.RootDir, ".covers"),
		filepath.Join(config.AppConfig.RootDir, "covers"),
	}
	for _, dir := range dirs {
		coverPath := filepath.Join(dir, filename)
		if _, err := os.Stat(coverPath); err == nil {
			c.File(coverPath)
			return
		}
	}
	httputil.RespondWithNotFound(c, "cover", "")
}

func isAllowedCoverSource(url string) bool {
	allowed := []string{
		"https://covers.openlibrary.org/",
		"http://covers.openlibrary.org/",
		"https://books.google.com/",
		"http://books.google.com/",
		"https://images-na.ssl-images-amazon.com/",
		"http://images-na.ssl-images-amazon.com/",
		"https://images.amazon.com/",
		"http://images.amazon.com/",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}
