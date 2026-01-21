// file: internal/playlist/playlist_test.go
// version: 1.0.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

package playlist

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// setupTestDB initializes a test database with required schema
func setupTestDB(t *testing.T) {
	t.Helper()

	// Create a temporary database file
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	database.DB = db

	// Create required tables - execute each separately for SQLite compatibility
	tables := []string{
		`CREATE TABLE IF NOT EXISTS authors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS series (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			author_id INTEGER,
			FOREIGN KEY (author_id) REFERENCES authors(id)
		)`,
		`CREATE TABLE IF NOT EXISTS books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			author_id INTEGER,
			series_id INTEGER,
			series_sequence INTEGER,
			file_path TEXT,
			FOREIGN KEY (author_id) REFERENCES authors(id),
			FOREIGN KEY (series_id) REFERENCES series(id)
		)`,
		`CREATE TABLE IF NOT EXISTS playlists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			series_id INTEGER,
			file_path TEXT,
			FOREIGN KEY (series_id) REFERENCES series(id)
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			playlist_id INTEGER,
			book_id INTEGER,
			position INTEGER,
			FOREIGN KEY (playlist_id) REFERENCES playlists(id),
			FOREIGN KEY (book_id) REFERENCES books(id)
		)`,
	}

	for _, table := range tables {
		_, err = db.Exec(table)
		if err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
	}

	// Verify tables were created
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tableCount)
	if err != nil {
		t.Fatalf("failed to query tables: %v", err)
	}
	if tableCount < 5 {
		t.Fatalf("expected at least 5 tables, got %d", tableCount)
	}
}

// cleanupTestDB closes the test database
func cleanupTestDB(t *testing.T) {
	t.Helper()
	if database.DB != nil {
		database.DB.Close()
	}
}

func TestGetBooksInSeries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Insert test data
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	// Insert books
	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES
			('Book 1', ?, ?, 1, '/path/to/book1.m4b'),
			('Book 2', ?, ?, 2, '/path/to/book2.m4b'),
			('Book 3', ?, ?, 3, '/path/to/book3.m4b')
	`, authorID, seriesID, authorID, seriesID, authorID, seriesID)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Test getBooksInSeries
	items, err := getBooksInSeries(int(seriesID))
	if err != nil {
		t.Fatalf("getBooksInSeries failed: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 books, got %d", len(items))
	}

	// Verify order
	if items[0].Title != "Book 1" || items[0].Position != 1 {
		t.Errorf("expected first book to be 'Book 1' with position 1, got %s with position %d", items[0].Title, items[0].Position)
	}

	if items[1].Title != "Book 2" || items[1].Position != 2 {
		t.Errorf("expected second book to be 'Book 2' with position 2, got %s with position %d", items[1].Title, items[1].Position)
	}
}

func TestGetBooksInSeriesEmpty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Test with non-existent series
	items, err := getBooksInSeries(9999)
	if err != nil {
		t.Fatalf("getBooksInSeries failed: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("expected 0 books for non-existent series, got %d", len(items))
	}
}

func TestGetBooksInSeriesNullSequence(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Insert test data
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	// Insert book with NULL sequence
	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES ('Book Without Sequence', ?, ?, NULL, '/path/to/book.m4b')
	`, authorID, seriesID)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	items, err := getBooksInSeries(int(seriesID))
	if err != nil {
		t.Fatalf("getBooksInSeries failed: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 book, got %d", len(items))
	}

	if items[0].Position != 0 {
		t.Errorf("expected position 0 for NULL sequence, got %d", items[0].Position)
	}
}

func TestCreateiTunesPlaylist(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "First Book", Author: "Author One", FilePath: "/path/to/book1.m4b", Position: 1},
		{BookID: 2, Title: "Second Book", Author: "Author One", FilePath: "/path/to/book2.m4b", Position: 2},
	}

	playlistPath, err := createiTunesPlaylist("Test Series - Author One", items)
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
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

func TestCreateiTunesPlaylistSpecialChars(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{
		{BookID: 1, Title: "Book", Author: "Author", FilePath: "/path/to/book.m4b", Position: 1},
	}

	// Test with special characters in playlist name
	playlistPath, err := createiTunesPlaylist("Series/With\\Special:Chars", items)
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
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

func TestCreateiTunesPlaylistEmpty(t *testing.T) {
	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	items := []PlaylistItem{}

	playlistPath, err := createiTunesPlaylist("Empty Series", items)
	if err != nil {
		t.Fatalf("createiTunesPlaylist failed: %v", err)
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

func TestSavePlaylistToDatabase(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Insert test data
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	// Insert books
	result, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES ('Book 1', ?, ?, 1, '/path/to/book1.m4b')
	`, authorID, seriesID)
	if err != nil {
		t.Fatalf("failed to insert book: %v", err)
	}

	// Test savePlaylistToDatabase
	err = savePlaylistToDatabase(int(seriesID), "Test Playlist", "/path/to/playlist.m3u")
	if err != nil {
		t.Fatalf("savePlaylistToDatabase failed: %v", err)
	}

	// Verify playlist was saved
	var playlistID int
	var name, path string
	err = database.DB.QueryRow("SELECT id, name, file_path FROM playlists WHERE series_id = ?", seriesID).Scan(&playlistID, &name, &path)
	if err != nil {
		t.Fatalf("failed to query playlist: %v", err)
	}

	if name != "Test Playlist" {
		t.Errorf("expected playlist name 'Test Playlist', got %s", name)
	}
	if path != "/path/to/playlist.m3u" {
		t.Errorf("expected playlist path '/path/to/playlist.m3u', got %s", path)
	}

	// Verify playlist item was saved
	var count int
	err = database.DB.QueryRow("SELECT COUNT(*) FROM playlist_items WHERE playlist_id = ?", playlistID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count playlist items: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 playlist item, got %d", count)
	}
}

func TestSavePlaylistToDatabaseUpdate(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// Insert test data
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	// Insert books
	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES
			('Book 1', ?, ?, 1, '/path/to/book1.m4b'),
			('Book 2', ?, ?, 2, '/path/to/book2.m4b')
	`, authorID, seriesID, authorID, seriesID)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Save playlist first time
	err = savePlaylistToDatabase(int(seriesID), "Test Playlist", "/path/to/playlist.m3u")
	if err != nil {
		t.Fatalf("savePlaylistToDatabase failed (first save): %v", err)
	}

	// Save playlist second time (update)
	err = savePlaylistToDatabase(int(seriesID), "Updated Playlist", "/path/to/updated_playlist.m3u")
	if err != nil {
		t.Fatalf("savePlaylistToDatabase failed (update): %v", err)
	}

	// Verify playlist was updated
	var name, path string
	err = database.DB.QueryRow("SELECT name, file_path FROM playlists WHERE series_id = ?", seriesID).Scan(&name, &path)
	if err != nil {
		t.Fatalf("failed to query playlist: %v", err)
	}

	if name != "Updated Playlist" {
		t.Errorf("expected playlist name 'Updated Playlist', got %s", name)
	}
	if path != "/path/to/updated_playlist.m3u" {
		t.Errorf("expected playlist path '/path/to/updated_playlist.m3u', got %s", path)
	}

	// Verify only one playlist exists
	var count int
	err = database.DB.QueryRow("SELECT COUNT(*) FROM playlists WHERE series_id = ?", seriesID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count playlists: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 playlist, got %d", count)
	}
}

func TestGeneratePlaylistsForSeries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	// Insert test data
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	// Insert books
	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES
			('Book 1', ?, ?, 1, '/path/to/book1.m4b'),
			('Book 2', ?, ?, 2, '/path/to/book2.m4b')
	`, authorID, seriesID, authorID, seriesID)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Test GeneratePlaylistsForSeries
	err = GeneratePlaylistsForSeries()
	if err != nil {
		t.Fatalf("GeneratePlaylistsForSeries failed: %v", err)
	}

	// Verify playlist file was created
	playlistFiles, err := filepath.Glob(filepath.Join(tempDir, "*.m3u"))
	if err != nil {
		t.Fatalf("failed to list playlist files: %v", err)
	}

	if len(playlistFiles) != 1 {
		t.Errorf("expected 1 playlist file, got %d", len(playlistFiles))
	}

	// Verify playlist was saved to database
	var playlistID int
	err = database.DB.QueryRow("SELECT id FROM playlists WHERE series_id = ?", seriesID).Scan(&playlistID)
	if err != nil {
		t.Fatalf("failed to query playlist: %v", err)
	}

	// Verify playlist items were saved
	var itemCount int
	err = database.DB.QueryRow("SELECT COUNT(*) FROM playlist_items WHERE playlist_id = ?", playlistID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("failed to count playlist items: %v", err)
	}

	if itemCount != 2 {
		t.Errorf("expected 2 playlist items, got %d", itemCount)
	}
}

func TestGeneratePlaylistsForSeriesNoBooks(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	// Insert test data - series without books
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("failed to insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	_, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Empty Series', ?)", authorID)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Test GeneratePlaylistsForSeries - should not fail
	err = GeneratePlaylistsForSeries()
	if err != nil {
		t.Fatalf("GeneratePlaylistsForSeries failed: %v", err)
	}

	// Verify no playlist file was created
	playlistFiles, err := filepath.Glob(filepath.Join(tempDir, "*.m3u"))
	if err != nil {
		t.Fatalf("failed to list playlist files: %v", err)
	}

	if len(playlistFiles) != 0 {
		t.Errorf("expected 0 playlist files for empty series, got %d", len(playlistFiles))
	}
}

func TestGeneratePlaylistsForSeriesNoSeries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	// Test with empty database - should not fail
	err := GeneratePlaylistsForSeries()
	if err != nil {
		t.Fatalf("GeneratePlaylistsForSeries failed on empty database: %v", err)
	}

	// Verify no playlist files were created
	playlistFiles, err := filepath.Glob(filepath.Join(tempDir, "*.m3u"))
	if err != nil {
		t.Fatalf("failed to list playlist files: %v", err)
	}

	if len(playlistFiles) != 0 {
		t.Errorf("expected 0 playlist files, got %d", len(playlistFiles))
	}
}

func TestGeneratePlaylistsForSeriesMultiple(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	tempDir := t.TempDir()
	config.AppConfig.PlaylistDir = tempDir

	// Insert multiple series with books
	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Author One'), ('Author Two')")
	if err != nil {
		t.Fatalf("failed to insert authors: %v", err)
	}

	// Get author IDs
	var author1ID, author2ID int64
	err = database.DB.QueryRow("SELECT id FROM authors WHERE name = 'Author One'").Scan(&author1ID)
	if err != nil {
		t.Fatalf("failed to get author1 ID: %v", err)
	}
	err = database.DB.QueryRow("SELECT id FROM authors WHERE name = 'Author Two'").Scan(&author2ID)
	if err != nil {
		t.Fatalf("failed to get author2 ID: %v", err)
	}

	// Insert series
	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Series A', ?)", author1ID)
	if err != nil {
		t.Fatalf("failed to insert series A: %v", err)
	}
	seriesAID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Series B', ?)", author2ID)
	if err != nil {
		t.Fatalf("failed to insert series B: %v", err)
	}
	seriesBID, _ := result.LastInsertId()

	// Insert books
	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES
			('Book A1', ?, ?, 1, '/path/to/a1.m4b'),
			('Book B1', ?, ?, 1, '/path/to/b1.m4b')
	`, author1ID, seriesAID, author2ID, seriesBID)
	if err != nil {
		t.Fatalf("failed to insert books: %v", err)
	}

	// Test GeneratePlaylistsForSeries
	err = GeneratePlaylistsForSeries()
	if err != nil {
		t.Fatalf("GeneratePlaylistsForSeries failed: %v", err)
	}

	// Verify two playlist files were created
	playlistFiles, err := filepath.Glob(filepath.Join(tempDir, "*.m3u"))
	if err != nil {
		t.Fatalf("failed to list playlist files: %v", err)
	}

	if len(playlistFiles) != 2 {
		t.Errorf("expected 2 playlist files, got %d", len(playlistFiles))
	}
}
