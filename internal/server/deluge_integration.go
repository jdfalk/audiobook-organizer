// file: internal/server/deluge_integration.go
// version: 1.6.0
// guid: 1c9d0e8f-2a3b-4a70-b8c5-3d7e0f1b9a99
// last-edited: 2026-05-05
//
// Deluge integration for library centralization (backlog 6.1).
//
// When a book version that came from Deluge is swapped, trashed,
// or its files are reorganized, we need to update the torrent's
// save_path in Deluge so it keeps seeding from the new location.
//
// The Deluge client is optional — if not configured, the
// integration is silently skipped.

package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
)

var (
	globalDelugeClient   *deluge.Client
	globalDelugeClientMu sync.Mutex
)

// getDelugeClient returns the Deluge client, initializing it if
// needed. Returns nil if Deluge is not configured.
func getDelugeClient() *deluge.Client {
	globalDelugeClientMu.Lock()
	defer globalDelugeClientMu.Unlock()
	if globalDelugeClient != nil {
		return globalDelugeClient
	}
	url := config.AppConfig.DelugeWebURL
	pass := config.AppConfig.DelugeWebPassword
	// Fall back to download_client.torrent.deluge config.
	if url == "" {
		dc := config.AppConfig.DownloadClient.Torrent.Deluge
		if dc.Host != "" {
			port := dc.Port
			if port == 0 {
				port = 8112
			}
			url = fmt.Sprintf("http://%s:%d", dc.Host, port)
			pass = dc.Password
		}
	}
	if url == "" {
		return nil
	}
	if pass == "" {
		pass = "deluge"
	}
	c, err := deluge.New(url, pass)
	if err != nil {
		log.Printf("[WARN] failed to create deluge client: %v", err)
		return nil
	}
	globalDelugeClient = c
	return c
}

// NotifyDelugeMoveStorage tells Deluge to move a torrent's storage
// to a new path. Called after a version swap or organize moves files
// that belong to a Deluge-sourced version.
//
// torrentHash is the infohash from BookVersion.TorrentHash.
// newPath is the directory where the files now live.
//
// Silently no-ops if Deluge is not configured or the torrent hash
// is empty.
func NotifyDelugeMoveStorage(torrentHash, newPath string) {
	if torrentHash == "" {
		return
	}
	if !config.AppConfig.DelugeMoveEnabled {
		log.Printf("[INFO] deluge move_storage skipped (deluge_move_enabled=false): %s → %s", torrentHash, newPath)
		return
	}
	c := getDelugeClient()
	if c == nil {
		return
	}
	// Deluge expects the parent directory, not the file path.
	dir := filepath.Dir(newPath)
	if err := c.MoveStorage([]string{torrentHash}, dir); err != nil {
		log.Printf("[WARN] deluge move_storage %s → %s: %v", torrentHash, dir, err)
	} else {
		log.Printf("[INFO] deluge move_storage %s → %s", torrentHash, dir)
	}
}

// handleDelugeTestConnection tests the Deluge Web UI connection.
// POST /api/v1/deluge/test-connection
func (s *Server) handleDelugeTestConnection(c *gin.Context) {
	url := config.AppConfig.DelugeWebURL
	if url == "" {
		httputil.RespondWithBadRequest(c, "deluge_web_url not configured")
		return
	}
	pass := config.AppConfig.DelugeWebPassword
	if pass == "" {
		pass = "deluge"
	}
	client, err := deluge.New(url, pass)
	if err != nil {
		httputil.RespondWithInternalError(c, err.Error())
		return
	}
	if err := client.Login(); err != nil {
		httputil.RespondWithError(c, http.StatusBadGateway, err.Error(), "BAD_GATEWAY")
		return
	}
	connected, err := client.Connected()
	if err != nil {
		httputil.RespondWithError(c, http.StatusBadGateway, err.Error(), "BAD_GATEWAY")
		return
	}
	httputil.RespondWithOK(c, struct {
		Connected bool   `json:"connected"`
		URL       string `json:"url"`
	}{Connected: connected, URL: url})
}

// handleDelugeListTorrents returns all torrents from Deluge.
// GET /api/v1/deluge/torrents
func (s *Server) handleDelugeListTorrents(c *gin.Context) {
	client := getDelugeClient()
	if client == nil {
		httputil.RespondWithBadRequest(c, "deluge not configured")
		return
	}
	torrents, err := client.ListTorrents()
	if err != nil {
		httputil.RespondWithError(c, http.StatusBadGateway, err.Error(), "BAD_GATEWAY")
		return
	}
	httputil.RespondWithOK(c, struct {
		Torrents any `json:"torrents"`
		Count    int `json:"count"`
	}{Torrents: torrents, Count: len(torrents)})
}

// handleDelugeStatus returns Deluge config status.
// GET /api/v1/deluge/status
func (s *Server) handleDelugeStatus(c *gin.Context) {
	url := config.AppConfig.DelugeWebURL
	if url == "" {
		dc := config.AppConfig.DownloadClient.Torrent.Deluge
		if dc.Host != "" {
			port := dc.Port
			if port == 0 {
				port = 8112
			}
			url = fmt.Sprintf("http://%s:%d", dc.Host, port)
		}
	}
	httputil.RespondWithOK(c, struct {
		Configured       bool   `json:"configured"`
		URL              string `json:"url"`
		DiscoveryEnabled bool   `json:"discovery_enabled"`
		MoveEnabled      bool   `json:"move_enabled"`
		DiscoveryLabel   string `json:"discovery_label"`
	}{
		Configured:       url != "",
		URL:              url,
		DiscoveryEnabled: config.AppConfig.DelugeDiscoveryEnabled,
		MoveEnabled:      config.AppConfig.DelugeMoveEnabled,
		DiscoveryLabel:   config.AppConfig.DelugeDiscoveryLabel,
	})
}

// registerDelugeRoutes wires the Deluge integration endpoints.
func (s *Server) registerDelugeRoutes(protected *gin.RouterGroup) {
	dg := protected.Group("/deluge")
	{
		dg.GET("/status", s.perm("integrations.manage"), s.handleDelugeStatus)
		dg.POST("/test-connection", s.perm("integrations.manage"), s.handleDelugeTestConnection)
		dg.GET("/torrents", s.perm("integrations.manage"), s.handleDelugeListTorrents)
		dg.GET("/labels", s.perm("integrations.manage"), s.handleDelugeListLabels)
		dg.GET("/discover", s.perm("integrations.manage"), s.handleDelugeDiscover)
		dg.POST("/discover/import", s.perm("integrations.manage"), s.handleDelugeDiscoverImport)
	}

	// Bulk-import pending Deluge files (settings.manage permission).
	protected.POST("/discovery/import", s.perm("settings.manage"), s.handleDiscoveryImport)
}

// NotifyDelugeAfterUndo checks whether the reverted operation moved
// Deluge-sourced files and updates the torrent storage path.
//
// oldFilePath is the path the file was restored to (the original location
// before the organize operation ran). This is the destination Deluge needs
// to know about — NOT book.FilePath, which may not yet be updated in the DB
// at the point this is called from the undo engine.
func NotifyDelugeAfterUndo(store interface {
	database.BookReader
	database.BookVersionStore
}, bookID, oldFilePath string) {
	if oldFilePath == "" {
		return
	}
	_, _ = store.GetBookByID(bookID) // ensure book exists; ignore result
	versions, _ := store.GetBookVersionsByBookID(bookID)
	for _, v := range versions {
		if v.TorrentHash != "" && v.Status == database.BookVersionStatusActive {
			// Use oldFilePath (the restored destination), not book.FilePath,
			// because the DB FilePath may not have been updated yet when this
			// is called immediately after the file rename-back.
			NotifyDelugeMoveStorage(v.TorrentHash, oldFilePath)
		}
	}
}

// NotifyDelugeAfterVersionSwap checks whether the swapped versions
// have torrent hashes and updates Deluge accordingly.
func NotifyDelugeAfterVersionSwap(store interface {
	database.BookReader
	database.BookVersionStore
}, fromVer, toVer *database.BookVersion, bookFilePath string) {
	if toVer != nil && toVer.TorrentHash != "" {
		NotifyDelugeMoveStorage(toVer.TorrentHash, bookFilePath)
	}
	if fromVer != nil && fromVer.TorrentHash != "" {
		// The "from" version moved into .versions/{id}/ — tell Deluge.
		book, _ := store.GetBookByID(fromVer.BookID)
		if book != nil {
			slotDir := filepath.Join(filepath.Dir(book.FilePath), ".versions", fromVer.ID)
			NotifyDelugeMoveStorage(fromVer.TorrentHash, slotDir)
		}
	}
}
