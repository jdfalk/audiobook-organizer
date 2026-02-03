// file: internal/server/itunes_test.go
// version: 1.1.0
// guid: 57e871fa-41b4-4fe6-9ed6-457ae78f0a07

package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// TestBuildBookFromTrack verifies field mapping from iTunes tracks.
func TestBuildBookFromTrack(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "itunes-track-*.m4b")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	content := bytes.Repeat([]byte("a"), 2048)
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to stat temp file: %v", err)
	}
	filePath := tmpFile.Name()
	location := itunes.EncodeLocation(filePath)
	now := time.Now().UTC()
	playDate := now.Add(-time.Hour).Unix()
	libraryPath := "/tmp/iTunes Library.xml"

	tests := []struct {
		name         string
		trackSize    int64
		wantFileSize int64
	}{
		{name: "uses track size", trackSize: 4096, wantFileSize: 4096},
		{name: "falls back to stat size", trackSize: 0, wantFileSize: info.Size()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := &itunes.Track{
				Location:     location,
				Name:         "",
				PersistentID: "ABC123",
				TotalTime:    123000,
				Year:         2000,
				PlayCount:    2,
				Rating:       80,
				Bookmark:     5000,
				DateAdded:    now,
				PlayDate:     playDate,
				AlbumArtist:  "Narrator",
				Artist:       "Author",
				Comments:     "First edition",
				Size:         tt.trackSize,
			}

			book, err := buildBookFromTrack(track, libraryPath)
			if err != nil {
				t.Fatalf("buildBookFromTrack error: %v", err)
			}

			wantTitle := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			if book.Title != wantTitle {
				t.Fatalf("title = %q, want %q", book.Title, wantTitle)
			}
			if book.ITunesPersistentID == nil || *book.ITunesPersistentID != "ABC123" {
				t.Fatalf("persistent ID not set correctly")
			}
			if book.ITunesDateAdded == nil || !book.ITunesDateAdded.Equal(now) {
				t.Fatalf("date added not set correctly")
			}
			if book.ITunesLastPlayed == nil || book.ITunesLastPlayed.Unix() != playDate {
				t.Fatalf("last played not set correctly")
			}
			if book.ITunesPlayCount == nil || *book.ITunesPlayCount != 2 {
				t.Fatalf("play count not set correctly")
			}
			if book.ITunesRating == nil || *book.ITunesRating != 80 {
				t.Fatalf("rating not set correctly")
			}
			if book.ITunesBookmark == nil || *book.ITunesBookmark != 5000 {
				t.Fatalf("bookmark not set correctly")
			}
			if book.ITunesImportSource == nil || *book.ITunesImportSource != libraryPath {
				t.Fatalf("import source not set correctly")
			}
			if book.Narrator == nil || *book.Narrator != "Narrator" {
				t.Fatalf("narrator not set correctly")
			}
			if book.Edition == nil || *book.Edition != "First edition" {
				t.Fatalf("edition not set correctly")
			}
			if book.FileSize == nil || *book.FileSize != tt.wantFileSize {
				t.Fatalf("file size = %d, want %d", valueOrZero(book.FileSize), tt.wantFileSize)
			}
		})
	}
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// TestValidateITunesLibrary tests library validation endpoint
func TestValidateITunesLibrary(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Use test data iTunes library
	libPath := filepath.Join("../../testdata/itunes", "iTunes Music Library.xml")

	// Verify the file exists before testing
	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		t.Skipf("iTunes test library not found at %s", libPath)
	}

	// Test with valid library path
	payload := map[string]interface{}{
		"library_path": libPath,
	}
	body := marshal(t, payload)

	req := httptest.NewRequest("POST", "/api/v1/itunes/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// We expect it to process the file, even if empty or has no audiobooks
	if w.Code != 200 && w.Code != 400 {
		t.Errorf("unexpected status code: %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBuildBookFromTrack_AllFields verifies complete field mapping
func TestBuildBookFromTrack_AllFields(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "complete-track-*.m4b")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	filePath := tmpFile.Name()
	location := itunes.EncodeLocation(filePath)
	now := time.Now().UTC()

	track := &itunes.Track{
		Location:     location,
		Name:         "Test Track Title",
		PersistentID: "DEF456",
		TotalTime:    456000,
		Year:         2021,
		PlayCount:    5,
		Rating:       100,
		Bookmark:     10000,
		DateAdded:    now,
		PlayDate:     now.Unix(),
		AlbumArtist:  "Test Narrator",
		Artist:       "Test Author",
		Album:        "Test Series",
		Comments:     "Test Edition",
		Size:         8192,
	}

	libraryPath := "/path/to/iTunes Library.xml"
	book, err := buildBookFromTrack(track, libraryPath)
	if err != nil {
		t.Fatalf("buildBookFromTrack error: %v", err)
	}

	// Verify all key fields are set
	if book.ITunesPersistentID == nil || *book.ITunesPersistentID != "DEF456" {
		t.Error("persistent ID not set")
	}
	if book.ITunesDateAdded == nil {
		t.Error("date added not set")
	}
	if book.ITunesPlayCount == nil || *book.ITunesPlayCount != 5 {
		t.Error("play count not set correctly")
	}
	if book.ITunesRating == nil || *book.ITunesRating != 100 {
		t.Error("rating not set correctly")
	}
	if book.ITunesBookmark == nil || *book.ITunesBookmark != 10000 {
		t.Error("bookmark not set correctly")
	}
	if book.ITunesImportSource == nil || *book.ITunesImportSource != libraryPath {
		t.Error("import source not set")
	}
}

// TestBuildBookFromTrack_MinimalTrack verifies handling of minimal track data
func TestBuildBookFromTrack_MinimalTrack(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "minimal-track-*.m4b")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	filePath := tmpFile.Name()
	location := itunes.EncodeLocation(filePath)

	// Create minimal track with only required fields
	track := &itunes.Track{
		Location:     location,
		PersistentID: "MIN123",
	}

	book, err := buildBookFromTrack(track, "/library.xml")
	if err != nil {
		t.Fatalf("buildBookFromTrack error: %v", err)
	}

	// Should still produce a valid book with defaults
	if book.ITunesPersistentID == nil {
		t.Error("persistent ID should be set")
	}
	if book.FilePath == "" {
		t.Error("file path should be decoded")
	}
}

// Helper functions
func marshal(t *testing.T, v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return b
}
