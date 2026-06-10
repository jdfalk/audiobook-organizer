// file: internal/playlist/playlist_test.go
// version: 2.0.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e
// last-edited: 2026-06-10

// NOTE(fable5 T022): Tests that relied on database.DB (getBooksInSeries,
// savePlaylistToDatabase, GeneratePlaylistsForSeries with live data) were
// removed; those code paths were deleted with the SQLite store.
// generatePlaylistFile (previously createiTunesPlaylist) is tested below.

package playlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/falkcorp/audiobook-organizer/internal/config"
)

func TestGeneratePlaylistFile(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "First Book", Author: "Author One", FilePath: "/path/to/book1.m4b", Position: 1},
		{BookID: 2, Title: "Second Book", Author: "Author One", FilePath: "/path/to/book2.m4b", Position: 2},
	}

	playlistPath, err := generatePlaylistFile("Test Series - Author One", items)
	if err != nil {
		t.Fatalf("generatePlaylistFile failed: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		t.Fatalf("playlist file not created: %s", playlistPath)
	}

	// Read and verify content
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("failed to read playlist file: %v", err)
	}

	contentStr := string(content)

	// Check header
	if !strings.Contains(contentStr, "#EXTM3U") {
		t.Error("playlist missing #EXTM3U header")
	}

	// Check both books are present
	if !strings.Contains(contentStr, "Author One - First Book") {
		t.Error("playlist missing first book info")
	}
	if !strings.Contains(contentStr, "/path/to/book1.m4b") {
		t.Error("playlist missing first book path")
	}
	if !strings.Contains(contentStr, "Author One - Second Book") {
		t.Error("playlist missing second book info")
	}
	if !strings.Contains(contentStr, "/path/to/book2.m4b") {
		t.Error("playlist missing second book path")
	}
}

func TestGeneratePlaylistFileSpecialChars(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "Book", Author: "Author", FilePath: "/path/to/book.m4b", Position: 1},
	}

	// Test with special characters in playlist name
	playlistPath, err := generatePlaylistFile("Series/With\\Special:Chars", items)
	if err != nil {
		t.Fatalf("generatePlaylistFile failed: %v", err)
	}

	// Verify special characters are replaced
	expectedFilename := "Series-With-Special-Chars.m3u"
	if !strings.Contains(playlistPath, expectedFilename) {
		t.Errorf("expected filename %s, got %s", expectedFilename, filepath.Base(playlistPath))
	}

	// Check file exists
	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		t.Fatalf("playlist file not created: %s", playlistPath)
	}
}

func TestGeneratePlaylistFileCreateError(t *testing.T) {
	tempDir := t.TempDir()
	blocker := filepath.Join(tempDir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}

	config.AppConfig.PlaylistDir = blocker

	_, err := generatePlaylistFile("Blocked Playlist", []PlaylistItem{
		{BookID: 1, Title: "Book", Author: "Author", FilePath: "/path/to/book.m4b", Position: 1},
	})
	if err == nil {
		t.Fatal("expected error when playlist directory is not a folder")
	}
}

func TestGeneratePlaylistFileEmpty(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{}

	playlistPath, err := generatePlaylistFile("Empty Series", items)
	if err != nil {
		t.Fatalf("generatePlaylistFile failed: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		t.Fatalf("playlist file not created: %s", playlistPath)
	}

	// Read and verify content
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("failed to read playlist file: %v", err)
	}

	contentStr := string(content)

	// Should only have header
	if !strings.Contains(contentStr, "#EXTM3U") {
		t.Error("playlist missing #EXTM3U header")
	}

	// Should not have any EXTINF entries
	if strings.Contains(contentStr, "#EXTINF") {
		t.Error("empty playlist should not have EXTINF entries")
	}
}

// TestGeneratePlaylistsForSeriesReturnsError verifies that the legacy
// SQLite-backed GeneratePlaylistsForSeries path now returns an error
// (the implementation was removed in fable5 T022).
func TestGeneratePlaylistsForSeriesReturnsError(t *testing.T) {
	if err := GeneratePlaylistsForSeries(); err == nil {
		t.Error("expected GeneratePlaylistsForSeries to return an error after SQLite removal")
	}
}
