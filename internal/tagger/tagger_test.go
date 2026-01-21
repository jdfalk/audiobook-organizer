// file: internal/tagger/tagger_test.go
// version: 1.0.1
// guid: 8c9d0e1f-2a3b-4c5d-6e7f-8a9b0c1d2e3f

package tagger

import (
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// setupTaggerDB prepares a temporary SQLite database for tagger tests.
func setupTaggerDB(t *testing.T) func() {
	t.Helper()

	prevDB := database.DB
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "tagger.db")

	if err := database.Initialize(dbPath); err != nil {
		t.Fatalf("initialize database: %v", err)
	}

	return func() {
		_ = database.Close()
		database.DB = prevDB
	}
}

func TestUpdateM4BTags(t *testing.T) {
	// Test the placeholder implementation
	filePath := "/test/path/book.m4b"
	seriesTag := "Test Series, Book 1"

	err := updateM4BTags(filePath, seriesTag)
	if err != nil {
		t.Errorf("updateM4BTags failed: %v", err)
	}

	// Since this is a placeholder, it should always succeed
	t.Log("updateM4BTags executed successfully (placeholder)")
}

func TestUpdateMP3Tags(t *testing.T) {
	// Test the placeholder implementation
	filePath := "/test/path/book.mp3"
	seriesTag := "Test Series, Book 1"

	err := updateMP3Tags(filePath, seriesTag)
	if err != nil {
		t.Errorf("updateMP3Tags failed: %v", err)
	}

	t.Log("updateMP3Tags executed successfully (placeholder)")
}

func TestUpdateFLACTags(t *testing.T) {
	// Test the placeholder implementation
	filePath := "/test/path/book.flac"
	seriesTag := "Test Series, Book 1"

	err := updateFLACTags(filePath, seriesTag)
	if err != nil {
		t.Errorf("updateFLACTags failed: %v", err)
	}

	t.Log("updateFLACTags executed successfully (placeholder)")
}

func TestUpdateFileTags_M4B(t *testing.T) {
	filePath := "/test/path/book.m4b"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err != nil {
		t.Errorf("updateFileTags for M4B failed: %v", err)
	}
}

func TestUpdateFileTags_MP3(t *testing.T) {
	filePath := "/test/path/book.mp3"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err != nil {
		t.Errorf("updateFileTags for MP3 failed: %v", err)
	}
}

func TestUpdateFileTags_FLAC(t *testing.T) {
	filePath := "/test/path/book.flac"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err != nil {
		t.Errorf("updateFileTags for FLAC failed: %v", err)
	}
}

func TestUpdateFileTags_M4A(t *testing.T) {
	filePath := "/test/path/book.m4a"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err != nil {
		t.Errorf("updateFileTags for M4A failed: %v", err)
	}
}

func TestUpdateFileTags_AAC(t *testing.T) {
	filePath := "/test/path/book.aac"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err != nil {
		t.Errorf("updateFileTags for AAC failed: %v", err)
	}
}

func TestUpdateFileTags_UnsupportedFormat(t *testing.T) {
	filePath := "/test/path/book.wav"
	title := "Test Book"
	seriesTag := "Test Series, Book 1"

	err := updateFileTags(filePath, title, seriesTag)
	if err == nil {
		t.Error("Expected error for unsupported format")
	}

	if err.Error() != "unsupported file format: .wav" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestUpdateFileTags_CaseInsensitive(t *testing.T) {
	tests := []struct {
		filePath string
		ext      string
	}{
		{"/test/book.M4B", ".M4B"},
		{"/test/book.Mp3", ".Mp3"},
		{"/test/book.FLAC", ".FLAC"},
		{"/test/book.M4A", ".M4A"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			err := updateFileTags(tt.filePath, "Title", "Series")
			if err != nil {
				t.Errorf("updateFileTags should handle case-insensitive extensions: %v", err)
			}
		})
	}
}

// TestUpdateSeriesTags exercises the series tag update flow with mixed inputs.
func TestUpdateSeriesTags(t *testing.T) {
	cleanup := setupTaggerDB(t)
	defer cleanup()

	result, err := database.DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = database.DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	_, err = database.DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path)
		VALUES
			('Book One', ?, ?, 1, '/tmp/book-one.m4b'),
			('Book Two', ?, ?, NULL, '/tmp/book-two.wav')
	`, authorID, seriesID, authorID, seriesID)
	if err != nil {
		t.Fatalf("insert books: %v", err)
	}

	if err := UpdateSeriesTags(); err != nil {
		t.Fatalf("UpdateSeriesTags failed: %v", err)
	}
}
