// file: internal/server/deluge_integration.go
// version: 1.0.0
// guid: 1c9d0e8f-2a3b-4a70-b8c5-3d7e0f1b9a99
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
	"log"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
)

// globalDelugeClient is lazily initialized on first use.
var globalDelugeClient *deluge.Client

// getDelugeClient returns the Deluge client, initializing it if
// needed. Returns nil if Deluge is not configured.
func getDelugeClient() *deluge.Client {
	if globalDelugeClient != nil {
		return globalDelugeClient
	}
	url := config.AppConfig.DelugeWebURL
	pass := config.AppConfig.DelugeWebPassword
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

// NotifyDelugeAfterVersionSwap checks whether the swapped versions
// have torrent hashes and updates Deluge accordingly.
func NotifyDelugeAfterVersionSwap(store database.Store, fromVer, toVer *database.BookVersion, bookFilePath string) {
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
