// file: internal/database/audiobooks_test.go
// version: 2.1.0
// guid: 3f2e1d0c-4b5a-6978-8899-aabbccddeeff

package database

import (
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/models"
)

// setupLegacyDB initializes the legacy DB connection for audiobooks tests.
func setupLegacyDB(t *testing.T) func() {
	t.Helper()

	prevDB := DB
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "audiobooks.db")

	if err := Initialize(dbPath); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	return func() {
		_ = Close()
		DB = prevDB
	}
}

func insertAudiobookFixture(t *testing.T) (int, int, int) {
	t.Helper()

	result, err := DB.Exec("INSERT INTO authors (name) VALUES ('Test Author')")
	if err != nil {
		t.Fatalf("insert author: %v", err)
	}
	authorID, _ := result.LastInsertId()

	result, err = DB.Exec("INSERT INTO series (name, author_id) VALUES ('Test Series', ?)", authorID)
	if err != nil {
		t.Fatalf("insert series: %v", err)
	}
	seriesID, _ := result.LastInsertId()

	result, err = DB.Exec(`
		INSERT INTO books (title, author_id, series_id, series_sequence, file_path, format, duration)
		VALUES ('Test Book', ?, ?, 1, '/tmp/test.mp3', 'mp3', 3600)
	`, authorID, seriesID)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	bookID, _ := result.LastInsertId()

	return int(bookID), int(authorID), int(seriesID)
}

func TestGetAudiobooksDefaultsAndFilters(t *testing.T) {
	cleanup := setupLegacyDB(t)
	defer cleanup()

	_, _, _ = insertAudiobookFixture(t)

	resp, err := GetAudiobooks(models.AudiobookListRequest{
		Page:    0,
		Limit:   500,
		Search:  "Test",
		Author:  "Test",
		Series:  "Test",
		Format:  "mp3",
		SortBy:  "invalid",
		SortDir: "asc",
	})
	if err != nil {
		t.Fatalf("GetAudiobooks failed: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected Total=1, got %d", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("expected Page=1, got %d", resp.Page)
	}
	if resp.Limit != 200 {
		t.Errorf("expected Limit=200, got %d", resp.Limit)
	}
	if len(resp.Audiobooks) != 1 {
		t.Fatalf("expected 1 audiobook, got %d", len(resp.Audiobooks))
	}
}

func TestAudiobookByIDUpdateAndDelete(t *testing.T) {
	cleanup := setupLegacyDB(t)
	defer cleanup()

	bookID, _, _ := insertAudiobookFixture(t)

	book, err := GetAudiobookByID(bookID)
	if err != nil {
		t.Fatalf("GetAudiobookByID failed: %v", err)
	}
	if book.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got %q", book.Title)
	}

	newTitle := "Updated Title"
	newAuthor := "Updated Author"
	newSeries := "Updated Series"
	seriesSequence := 2
	newFormat := "m4b"
	newDuration := 1800

	updated, err := UpdateAudiobook(bookID, models.AudiobookUpdateRequest{
		Title:          &newTitle,
		Author:         &newAuthor,
		Series:         &newSeries,
		SeriesSequence: &seriesSequence,
		Format:         &newFormat,
		Duration:       &newDuration,
	})
	if err != nil {
		t.Fatalf("UpdateAudiobook failed: %v", err)
	}
	if updated.Title != newTitle {
		t.Errorf("expected updated title %q, got %q", newTitle, updated.Title)
	}

	authors, err := GetAllAuthors()
	if err != nil {
		t.Fatalf("GetAllAuthors failed: %v", err)
	}
	if len(authors) == 0 {
		t.Error("expected authors list to be populated")
	}

	seriesList, err := GetAllSeries()
	if err != nil {
		t.Fatalf("GetAllSeries failed: %v", err)
	}
	if len(seriesList) == 0 {
		t.Error("expected series list to be populated")
	}

	if err := DeleteAudiobook(bookID); err != nil {
		t.Fatalf("DeleteAudiobook failed: %v", err)
	}
	if _, err := GetAudiobookByID(bookID); err == nil {
		t.Error("expected error fetching deleted audiobook")
	}
}
