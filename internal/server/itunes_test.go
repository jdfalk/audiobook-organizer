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

			book, err := buildBookFromTrack(track, libraryPath, itunes.ImportOptions{})
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
	book, err := buildBookFromTrack(track, libraryPath, itunes.ImportOptions{})
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

	book, err := buildBookFromTrack(track, "/library.xml", itunes.ImportOptions{})
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

// TestExtractSeriesName tests series name extraction from album strings
func TestExtractSeriesName(t *testing.T) {
	tests := []struct {
		album string
		want  string
	}{
		{"", ""},
		{"Simple Album", "Simple Album"},
		{"Series, Book 1", "Series"},
		{"Series - Part 2", "Series"},
		{"Series: Volume 3", "Series"},
		{"  Padded , Book  ", "Padded"},
		{"No Separator Here", "No Separator Here"},
		{"A,B,C", "A,B,C"}, // more than 2 parts, not split on comma
	}
	for _, tt := range tests {
		t.Run(tt.album, func(t *testing.T) {
			got := extractSeriesName(tt.album)
			if got != tt.want {
				t.Errorf("extractSeriesName(%q) = %q, want %q", tt.album, got, tt.want)
			}
		})
	}
}

// TestImportLibraryState tests import mode to library state mapping
func TestImportLibraryState(t *testing.T) {
	tests := []struct {
		mode itunes.ImportMode
		want string
	}{
		{itunes.ImportModeOrganized, "organized"},
		{itunes.ImportModeImport, "imported"},
		{itunes.ImportModeOrganize, "imported"},
	}
	for _, tt := range tests {
		got := importLibraryState(tt.mode)
		if got != tt.want {
			t.Errorf("importLibraryState(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// TestResolveITunesImportMode tests import mode string resolution
func TestResolveITunesImportMode(t *testing.T) {
	tests := []struct {
		input string
		want  itunes.ImportMode
	}{
		{"organized", itunes.ImportModeOrganized},
		{"organize", itunes.ImportModeOrganize},
		{"import", itunes.ImportModeImport},
		{"unknown", itunes.ImportModeImport},
		{"", itunes.ImportModeImport},
	}
	for _, tt := range tests {
		got := resolveITunesImportMode(tt.input)
		if got != tt.want {
			t.Errorf("resolveITunesImportMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCalculatePercent tests percentage calculation
func TestCalculatePercent(t *testing.T) {
	tests := []struct {
		current, total, want int
	}{
		{0, 0, 0},
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{200, 100, 100}, // capped at 100
		{-1, 100, 0},    // negative capped at 0
		{5, 0, 0},       // zero total
	}
	for _, tt := range tests {
		got := calculatePercent(tt.current, tt.total)
		if got != tt.want {
			t.Errorf("calculatePercent(%d, %d) = %d, want %d", tt.current, tt.total, got, tt.want)
		}
	}
}

// TestITunesImportStatusHelpers tests the status tracking functions
func TestITunesImportStatusHelpers(t *testing.T) {
	t.Run("loadITunesImportStatus", func(t *testing.T) {
		opID := "test-load-" + t.Name()
		status := loadITunesImportStatus(opID)
		if status == nil {
			t.Fatal("expected non-nil status")
		}
		if status.Total != 0 || status.Processed != 0 {
			t.Error("expected zeroed status")
		}

		// Loading again should return same status
		status2 := loadITunesImportStatus(opID)
		if status2 != status {
			t.Error("expected same status instance")
		}
		// Cleanup
		itunesImportStatuses.Delete(opID)
	})

	t.Run("setITunesImportTotal", func(t *testing.T) {
		status := &itunesImportStatus{}
		setITunesImportTotal(status, 42)
		if status.Total != 42 {
			t.Errorf("expected Total=42, got %d", status.Total)
		}
	})

	t.Run("updateITunesProcessed", func(t *testing.T) {
		status := &itunesImportStatus{}
		updateITunesProcessed(status, 10)
		if status.Processed != 10 {
			t.Errorf("expected Processed=10, got %d", status.Processed)
		}
	})

	t.Run("updateITunesImported", func(t *testing.T) {
		status := &itunesImportStatus{}
		updateITunesImported(status)
		updateITunesImported(status)
		if status.Imported != 2 {
			t.Errorf("expected Imported=2, got %d", status.Imported)
		}
	})

	t.Run("updateITunesSkipped", func(t *testing.T) {
		status := &itunesImportStatus{}
		updateITunesSkipped(status)
		if status.Skipped != 1 {
			t.Errorf("expected Skipped=1, got %d", status.Skipped)
		}
	})

	t.Run("recordITunesFailure", func(t *testing.T) {
		status := &itunesImportStatus{}
		recordITunesFailure(status, "error1")
		recordITunesFailure(status, "error2")
		if status.Failed != 2 {
			t.Errorf("expected Failed=2, got %d", status.Failed)
		}
		if len(status.Errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(status.Errors))
		}
	})

	t.Run("recordITunesFailure respects limit", func(t *testing.T) {
		status := &itunesImportStatus{}
		for i := 0; i < itunesImportErrorLimit+10; i++ {
			recordITunesFailure(status, "err")
		}
		if len(status.Errors) != itunesImportErrorLimit {
			t.Errorf("expected %d errors, got %d", itunesImportErrorLimit, len(status.Errors))
		}
	})

	t.Run("recordITunesImportError", func(t *testing.T) {
		status := &itunesImportStatus{}
		recordITunesImportError(status, "import error")
		if len(status.Errors) != 1 || status.Errors[0] != "import error" {
			t.Errorf("unexpected errors: %v", status.Errors)
		}
	})

	t.Run("recordITunesImportError respects limit", func(t *testing.T) {
		status := &itunesImportStatus{}
		for i := 0; i < itunesImportErrorLimit+5; i++ {
			recordITunesImportError(status, "err")
		}
		if len(status.Errors) != itunesImportErrorLimit {
			t.Errorf("expected %d errors, got %d", itunesImportErrorLimit, len(status.Errors))
		}
	})
}

// TestSnapshotITunesImportStatus tests status snapshotting
func TestSnapshotITunesImportStatus(t *testing.T) {
	opID := "test-snapshot-" + t.Name()
	defer itunesImportStatuses.Delete(opID)

	status := loadITunesImportStatus(opID)
	setITunesImportTotal(status, 100)
	updateITunesProcessed(status, 50)
	updateITunesImported(status)
	updateITunesSkipped(status)
	recordITunesFailure(status, "test error")

	snapshot := snapshotITunesImportStatus(opID)

	if snapshot.Total != 100 || snapshot.Processed != 50 || snapshot.Imported != 1 || snapshot.Skipped != 1 || snapshot.Failed != 1 {
		t.Errorf("snapshot mismatch: %+v", snapshot)
	}
	if len(snapshot.Errors) != 1 || snapshot.Errors[0] != "test error" {
		t.Errorf("snapshot errors mismatch: %v", snapshot.Errors)
	}

	// Modifying snapshot should not affect original
	snapshot.Total = 999
	if status.Total != 100 {
		t.Error("snapshot modification affected original")
	}
}

// TestBuildITunesSummary tests summary string generation
func TestBuildITunesSummary(t *testing.T) {
	status := &itunesImportStatus{
		Imported: 10,
		Skipped:  3,
		Failed:   2,
	}
	summary := buildITunesSummary(status)
	if !strings.Contains(summary, "10 imported") || !strings.Contains(summary, "3 skipped") || !strings.Contains(summary, "2 failed") {
		t.Errorf("unexpected summary: %q", summary)
	}
}

// TestIntPtr tests int pointer helper
func TestIntPtr(t *testing.T) {
	p := intPtr(42)
	if p == nil || *p != 42 {
		t.Errorf("expected *42, got %v", p)
	}
}

// TestInt64Ptr tests int64 pointer helper
func TestInt64Ptr(t *testing.T) {
	p := int64Ptr(99)
	if p == nil || *p != 99 {
		t.Errorf("expected *99, got %v", p)
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
