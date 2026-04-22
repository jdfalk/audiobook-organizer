// file: internal/server/deluge_discovery.go
// version: 1.2.0
// guid: e6f7a8b9-c0d1-2e3f-4a5b-6c7d8e9f0a1b
//
// Deluge label-based audiobook discovery.
//
// Fetches torrents with a configured label from Deluge, then
// cross-references their save_path against known Book.FilePath values
// in the database. Torrents whose save_path is not a prefix of any
// tracked file are surfaced as "unimported" candidates.

package server

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	delugeclient "github.com/jdfalk/audiobook-organizer/internal/deluge"
)

// DiscoveredTorrent is a Deluge torrent not yet tracked in the library.
type DiscoveredTorrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"` // filepath.Join(save_path, name) — import this
	Label       string  `json:"label"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	TotalSize   int64   `json:"total_size"`
}

// discoverUnimported fetches labeled torrents from Deluge and returns those
// whose save_path does not match any file path already in the library.
func (s *Server) discoverUnimported(client *delugeclient.Client, label string) ([]DiscoveredTorrent, error) {
	torrents, err := client.ListTorrentsByLabel(label)
	if err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return []DiscoveredTorrent{}, nil
	}

	// Build a set of known file path prefixes from the DB.
	// Page through all books — library sizes are typically < 100K so a
	// single 100K-limit fetch is fine; no pagination needed here.
	books, err := s.Store().GetAllBooks(100000, 0)
	if err != nil {
		log.Printf("[WARN] deluge discovery: failed to load books: %v", err)
		books = nil
	}
	known := make(map[string]struct{}, len(books))
	for _, b := range books {
		if b.FilePath != "" {
			known[b.FilePath] = struct{}{}
		}
	}

	var unimported []DiscoveredTorrent
	for _, t := range torrents {
		// Deluge saves multi-file torrents to {save_path}/{name}/.
		// Single-file torrents land at {save_path}/{name} directly.
		// Either way, filepath.Join(save_path, name) is the unique
		// content root for this torrent — not save_path alone, which
		// is a shared download directory.
		contentPath := filepath.Join(t.SavePath, t.Name)
		if !isTracked(contentPath, known) {
			unimported = append(unimported, DiscoveredTorrent{
				Hash:        t.Hash,
				Name:        t.Name,
				SavePath:    t.SavePath,
				ContentPath: contentPath,
				Label:       t.Label,
				State:       t.State,
				Progress:    t.Progress,
				TotalSize:   t.TotalSize,
			})
		}
	}
	return unimported, nil
}

// isTracked returns true if contentPath is a prefix of any known file path,
// meaning the torrent's content directory is already represented in the library.
//
// Callers MUST pass filepath.Join(save_path, torrent_name) — NOT save_path
// alone. A shared download directory (e.g. /mnt/downloads) is itself a prefix
// of every file in it, so passing raw save_path would make every torrent appear
// tracked once any file from that directory is imported.
func isTracked(contentPath string, known map[string]struct{}) bool {
	if contentPath == "" {
		return false
	}
	// Normalize trailing slash so prefix check is consistent.
	prefix := strings.TrimRight(contentPath, "/") + "/"
	for p := range known {
		if strings.HasPrefix(p, prefix) || p == contentPath {
			return true
		}
	}
	return false
}

// handleDelugeDiscover returns Deluge torrents not yet in the library.
// GET /api/v1/deluge/discover?label=audiobooks
func (s *Server) handleDelugeDiscover(c *gin.Context) {
	if !config.AppConfig.DelugeDiscoveryEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "deluge discovery is disabled (set deluge_discovery_enabled=true to enable)"})
		return
	}
	client := getDelugeClient()
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deluge not configured"})
		return
	}

	label := c.Query("label")
	if label == "" {
		label = config.AppConfig.DelugeDiscoveryLabel
	}

	unimported, err := s.discoverUnimported(client, label)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"label":      label,
		"candidates": unimported,
		"count":      len(unimported),
	})
}

// handleDelugeListLabels returns all labels from the Deluge Label plugin.
// GET /api/v1/deluge/labels
func (s *Server) handleDelugeListLabels(c *gin.Context) {
	client := getDelugeClient()
	if client == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deluge not configured"})
		return
	}
	labels, err := client.ListLabels()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"labels": labels, "count": len(labels)})
}

// handleDelugeDiscoverImport triggers an import of a discovered torrent's
// content_path into the library. Reuses the existing ImportFile pipeline.
// POST /api/v1/deluge/discover/import
// Body: { "content_path": "/mnt/downloads/Dune by Frank Herbert", "torrent_hash": "abc123" }
func (s *Server) handleDelugeDiscoverImport(c *gin.Context) {
	if !config.AppConfig.DelugeDiscoveryEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "deluge discovery is disabled (set deluge_discovery_enabled=true to enable)"})
		return
	}
	var req struct {
		ContentPath string `json:"content_path" binding:"required"`
		TorrentHash string `json:"torrent_hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if s.importService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "import service not initialized"})
		return
	}

	resp, err := s.importService.ImportFile(&ImportFileRequest{
		FilePath: req.ContentPath,
		Organize: false,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"book":         resp,
		"torrent_hash": req.TorrentHash,
	})
}
