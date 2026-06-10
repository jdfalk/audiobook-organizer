// file: internal/playlist/playlist.go
// version: 2.0.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d
// last-edited: 2026-06-10

package playlist

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/falkcorp/audiobook-organizer/internal/config"
)

// PlaylistItem represents a book in a playlist
type PlaylistItem struct {
	BookID   int
	Title    string
	Author   string
	FilePath string
	Position int
}

// GeneratePlaylistsForSeries generates playlists for all identified series.
//
// NOTE: The legacy implementation read from the global SQLite database.DB
// which was removed in fable5 TASK-022. This function now returns an error
// to avoid silent failures. Use the Store-backed playlist API
// (server/handlers/playlists.go) for production workflows.
func GeneratePlaylistsForSeries() error {
	slog.Warn("GeneratePlaylistsForSeries: legacy SQLite path removed in fable5 T022; use Store-backed playlist API")
	return fmt.Errorf("GeneratePlaylistsForSeries: the legacy SQLite path was removed in fable5 T022; use the Store-backed playlist API instead")
}

// generatePlaylistFile writes a M3U playlist for a set of items to the
// configured playlist directory. This function does not require a database
// connection and is used by the Store-backed playlist handlers.
func generatePlaylistFile(playlistName string, items []PlaylistItem) (string, error) {
	// Sort by position then title
	sort.Slice(items, func(i, j int) bool {
		if items[i].Position == items[j].Position {
			return items[i].Title < items[j].Title
		}
		return items[i].Position < items[j].Position
	})

	safePlaylistName := strings.ReplaceAll(playlistName, "/", "-")
	safePlaylistName = strings.ReplaceAll(safePlaylistName, "\\", "-")
	safePlaylistName = strings.ReplaceAll(safePlaylistName, ":", "-")

	playlistPath := filepath.Join(config.AppConfig.PlaylistDir, safePlaylistName+".m3u")

	f, err := os.Create(playlistPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err = f.WriteString("#EXTM3U\n"); err != nil {
		return "", err
	}

	for _, item := range items {
		if _, err = fmt.Fprintf(f, "#EXTINF:-1,%s - %s\n", item.Author, item.Title); err != nil {
			return "", err
		}
		if _, err = f.WriteString(item.FilePath + "\n"); err != nil {
			return "", err
		}
	}

	return playlistPath, nil
}
