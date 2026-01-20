// file: internal/playlist/playlist_test.go
// version: 2.0.0
// guid: 9c0d1e2f-3a4b-5c6d-7e8f-9a0b1c2d3e4f
// last-edited: 2026-01-19

package playlist

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// TestPlaylistItemStructure tests the PlaylistItem struct
func TestPlaylistItemStructure(t *testing.T) {
	// Arrange
	item := PlaylistItem{
		BookID:   1,
		Title:    "Test Book",
		Author:   "Test Author",
		FilePath: "/path/to/book.m4b",
		Position: 1,
	}

	// Act & Assert
	if item.BookID != 1 {
		t.Errorf("Expected BookID 1, got %d", item.BookID)
	}
	if item.Title != "Test Book" {
		t.Errorf("Expected Title 'Test Book', got '%s'", item.Title)
	}
	if item.Author != "Test Author" {
		t.Errorf("Expected Author 'Test Author', got '%s'", item.Author)
	}
	if item.FilePath != "/path/to/book.m4b" {
		t.Errorf("Expected FilePath '/path/to/book.m4b', got '%s'", item.FilePath)
	}
	if item.Position != 1 {
		t.Errorf("Expected Position 1, got %d", item.Position)
	}
}

// TestPlaylistItemSorting tests sorting of playlist items
func TestPlaylistItemSorting(t *testing.T) {
	// Arrange
	items := []PlaylistItem{
		{BookID: 3, Title: "Book C", Position: 3},
		{BookID: 1, Title: "Book A", Position: 1},
		{BookID: 4, Title: "Book B", Position: 3}, // Same position as Book C
		{BookID: 2, Title: "Book D", Position: 2},
	}

	// Act
	sort.Slice(items, func(i, j int) bool {
		if items[i].Position == items[j].Position {
			return items[i].Title < items[j].Title
		}
		return items[i].Position < items[j].Position
	})

	// Assert
	expectedOrder := []string{"Book A", "Book D", "Book B", "Book C"}
	for i, expected := range expectedOrder {
		if items[i].Title != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, items[i].Title)
		}
	}
}

// TestPlaylistNameSanitization tests that unsafe characters are removed
func TestPlaylistNameSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "forward slash",
			input:    "Series/Name",
			expected: "Series-Name",
		},
		{
			name:     "backslash",
			input:    "Series\\Name",
			expected: "Series-Name",
		},
		{
			name:     "colon",
			input:    "Series: The Beginning",
			expected: "Series- The Beginning",
		},
		{
			name:     "multiple unsafe chars",
			input:    "Series/Name: Book\\1",
			expected: "Series-Name- Book-1",
		},
		{
			name:     "safe name",
			input:    "Series Name - Book 1",
			expected: "Series Name - Book 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act - Apply same sanitization as createiTunesPlaylist
			result := strings.ReplaceAll(tt.input, "/", "-")
			result = strings.ReplaceAll(result, "\\", "-")
			result = strings.ReplaceAll(result, ":", "-")

			// Assert
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestCreateiTunesPlaylistFileCreation tests that playlist file is created
func TestCreateiTunesPlaylistFileCreation(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "Book 1", Author: "Author A", FilePath: "/path/book1.m4b", Position: 1},
		{BookID: 2, Title: "Book 2", Author: "Author A", FilePath: "/path/book2.m4b", Position: 2},
	}
	playlistName := "Test Series - Author A"

	// Act
	playlistPath, err := createiTunesPlaylist(playlistName, items)

	// Assert
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
	}

	if !strings.HasSuffix(playlistPath, ".m3u") {
		t.Errorf("Expected .m3u extension, got '%s'", playlistPath)
	}

	// Verify file exists
	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		t.Error("Playlist file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("Failed to read playlist file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "#EXTM3U") {
		t.Error("Playlist missing M3U header")
	}
	if !strings.Contains(contentStr, "Book 1") {
		t.Error("Playlist missing Book 1")
	}
	if !strings.Contains(contentStr, "Book 2") {
		t.Error("Playlist missing Book 2")
	}
	if !strings.Contains(contentStr, "/path/book1.m4b") {
		t.Error("Playlist missing book1 path")
	}
}

// TestCreateiTunesPlaylistEmpty tests handling of empty playlist
func TestCreateiTunesPlaylistEmpty(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{}
	playlistName := "Empty Playlist"

	// Act
	playlistPath, err := createiTunesPlaylist(playlistName, items)

	// Assert
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
	}

	// Verify file exists and has header
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("Failed to read playlist file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "#EXTM3U") {
		t.Error("Empty playlist missing M3U header")
	}
}

// TestCreateiTunesPlaylistInvalidDir tests error handling for invalid directory
func TestCreateiTunesPlaylistInvalidDir(t *testing.T) {
	// Arrange
	config.AppConfig.PlaylistDir = "/nonexistent/directory/that/should/not/exist"

	items := []PlaylistItem{
		{BookID: 1, Title: "Book 1", Author: "Author A", FilePath: "/path/book1.m4b", Position: 1},
	}
	playlistName := "Test Playlist"

	// Act
	_, err := createiTunesPlaylist(playlistName, items)

	// Assert
	if err == nil {
		t.Error("Expected error for invalid directory")
	}
}

// TestCreateiTunesPlaylistUnsafeChars tests that unsafe characters are handled
func TestCreateiTunesPlaylistUnsafeChars(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "Book 1", Author: "Author A", FilePath: "/path/book1.m4b", Position: 1},
	}
	playlistName := "Test/Series\\Name:Title"

	// Act
	playlistPath, err := createiTunesPlaylist(playlistName, items)

	// Assert
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
	}

	// Verify filename has safe characters
	filename := filepath.Base(playlistPath)
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, ":") {
		t.Errorf("Filename contains unsafe characters: %s", filename)
	}

	expectedFilename := "Test-Series-Name-Title.m3u"
	if filename != expectedFilename {
		t.Errorf("Expected filename '%s', got '%s'", expectedFilename, filename)
	}
}

// TestCreateiTunesPlaylistM3UFormat tests M3U format compliance
func TestCreateiTunesPlaylistM3UFormat(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "Book 1", Author: "Author A", FilePath: "/path/book1.m4b", Position: 1},
		{BookID: 2, Title: "Book 2", Author: "Author B", FilePath: "/path/book2.m4b", Position: 2},
	}
	playlistName := "Test Playlist"

	// Act
	playlistPath, err := createiTunesPlaylist(playlistName, items)
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
	}

	// Assert
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("Failed to read playlist file: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	// Check header
	if len(lines) < 1 || lines[0] != "#EXTM3U" {
		t.Error("First line should be #EXTM3U")
	}

	// Check EXTINF entries
	extinfCount := 0
	pathCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXTINF:") {
			extinfCount++
		} else if strings.HasPrefix(line, "/path/") {
			pathCount++
		}
	}

	if extinfCount != 2 {
		t.Errorf("Expected 2 EXTINF entries, got %d", extinfCount)
	}
	if pathCount != 2 {
		t.Errorf("Expected 2 file paths, got %d", pathCount)
	}
}

// TestGeneratePlaylistsForSeriesNilDB tests error handling with nil database
// Note: This test is skipped as GeneratePlaylistsForSeries requires full DB setup
func TestGeneratePlaylistsForSeriesNilDB(t *testing.T) {
	t.Skip("GeneratePlaylistsForSeries requires database setup, tested in integration tests")
	// This function depends on database.DB which needs proper initialization
	// and would panic with nil pointer if called directly
}
