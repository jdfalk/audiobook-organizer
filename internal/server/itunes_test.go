// file: internal/server/itunes_test.go
// version: 2.2.0
// guid: 57e871fa-41b4-4fe6-9ed6-457ae78f0a07

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// TestBuildBookFromAlbumGroup verifies field mapping from iTunes tracks.
func TestBuildBookFromAlbumGroup(t *testing.T) {
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

	filePath := tmpFile.Name()
	location := itunes.EncodeLocation(filePath)
	now := time.Now().UTC()
	playDate := now.Add(-time.Hour).Unix()
	libraryPath := "/tmp/iTunes Library.xml"

	track := &itunes.Track{
		Location:     location,
		Name:         "Chapter 1",
		Album:        "My Audiobook",
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
		Size:         4096,
	}

	group := albumGroup{key: "Author|My Audiobook", tracks: []*itunes.Track{track}}
	book, err := buildBookFromAlbumGroup(group, libraryPath, itunes.ImportOptions{})
	if err != nil {
		t.Fatalf("buildBookFromAlbumGroup error: %v", err)
	}

	// Single-track group uses Album as title
	if book.Title != "My Audiobook" {
		t.Fatalf("title = %q, want %q", book.Title, "My Audiobook")
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
	if book.FileSize == nil || *book.FileSize != 4096 {
		t.Fatalf("file size = %d, want 4096", valueOrZero(book.FileSize))
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

// TestBuildBookFromAlbumGroup_AllFields verifies complete field mapping
func TestBuildBookFromAlbumGroup_AllFields(t *testing.T) {
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
	group := albumGroup{key: "Test Author|Test Series", tracks: []*itunes.Track{track}}
	book, err := buildBookFromAlbumGroup(group, libraryPath, itunes.ImportOptions{})
	if err != nil {
		t.Fatalf("buildBookFromAlbumGroup error: %v", err)
	}

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

// TestBuildBookFromAlbumGroup_MinimalTrack verifies handling of minimal track data
func TestBuildBookFromAlbumGroup_MinimalTrack(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "minimal-track-*.m4b")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	filePath := tmpFile.Name()
	location := itunes.EncodeLocation(filePath)

	track := &itunes.Track{
		Location:     location,
		PersistentID: "MIN123",
	}

	group := albumGroup{key: "|", tracks: []*itunes.Track{track}}
	book, err := buildBookFromAlbumGroup(group, "/library.xml", itunes.ImportOptions{})
	if err != nil {
		t.Fatalf("buildBookFromAlbumGroup error: %v", err)
	}

	if book.ITunesPersistentID == nil {
		t.Error("persistent ID should be set")
	}
	if book.FilePath == "" {
		t.Error("file path should be decoded")
	}
}

// TestGroupTracksByAlbum verifies multi-track grouping
func TestGroupTracksByAlbum(t *testing.T) {
	library := &itunes.Library{
		Tracks: map[string]*itunes.Track{
			"1": {TrackID: 1, Name: "Chapter 1", Artist: "Author A", Album: "Book One", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 1},
			"2": {TrackID: 2, Name: "Chapter 2", Artist: "Author A", Album: "Book One", Kind: "Audiobook", TrackNumber: 2, DiscNumber: 1},
			"3": {TrackID: 3, Name: "Chapter 1", Artist: "Author B", Album: "Book Two", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 1},
			"4": {TrackID: 4, Name: "Music Track", Artist: "Singer", Album: "Pop Album", Kind: "MPEG audio file"},
		},
	}

	groups := groupTracksByAlbum(library)

	// Should have 2 groups (not 3 - the music track is filtered out)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Find the "Author A|Book One" group
	var bookOneGroup *albumGroup
	for i := range groups {
		if groups[i].key == "Author A|Book One" {
			bookOneGroup = &groups[i]
			break
		}
	}
	if bookOneGroup == nil {
		t.Fatal("expected to find 'Author A|Book One' group")
	}
	if len(bookOneGroup.tracks) != 2 {
		t.Errorf("expected 2 tracks in Book One group, got %d", len(bookOneGroup.tracks))
	}
	// Verify sorted by track number
	if bookOneGroup.tracks[0].TrackNumber != 1 || bookOneGroup.tracks[1].TrackNumber != 2 {
		t.Error("tracks not sorted by track number")
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

// TestGroupTracksByAlbum_MultiTrackBooks tests grouping with multi-track test data
func TestGroupTracksByAlbum_MultiTrackBooks(t *testing.T) {
	library := &itunes.Library{
		Tracks: map[string]*itunes.Track{
			"500": {TrackID: 500, Name: "Chapter 1 - Loomings", Artist: "Herman Melville", Album: "Moby Dick", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 1, TotalTime: 3600000, Size: 50000000},
			"501": {TrackID: 501, Name: "Chapter 2 - The Carpet-Bag", Artist: "Herman Melville", Album: "Moby Dick", Kind: "Audiobook", TrackNumber: 2, DiscNumber: 1, TotalTime: 3200000, Size: 45000000},
			"502": {TrackID: 502, Name: "Chapter 3 - The Spouter-Inn", Artist: "Herman Melville", Album: "Moby Dick", Kind: "Audiobook", TrackNumber: 3, DiscNumber: 1, TotalTime: 3400000, Size: 48000000},
			"600": {TrackID: 600, Name: "Part 1", Artist: "Jane Austen", Album: "Pride and Prejudice", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 1, TotalTime: 5400000, Size: 60000000},
			"601": {TrackID: 601, Name: "Part 2", Artist: "Jane Austen", Album: "Pride and Prejudice", Kind: "Audiobook", TrackNumber: 2, DiscNumber: 1, TotalTime: 4800000, Size: 55000000},
		},
	}

	groups := groupTracksByAlbum(library)

	// Should have 2 groups
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Find each group
	var mobyGroup, prideGroup *albumGroup
	for i := range groups {
		switch groups[i].key {
		case "Herman Melville|Moby Dick":
			mobyGroup = &groups[i]
		case "Jane Austen|Pride and Prejudice":
			prideGroup = &groups[i]
		}
	}

	if mobyGroup == nil {
		t.Fatal("expected to find 'Herman Melville|Moby Dick' group")
	}
	if len(mobyGroup.tracks) != 3 {
		t.Errorf("expected 3 tracks in Moby Dick group, got %d", len(mobyGroup.tracks))
	}
	// Verify sorted by track number
	for i, track := range mobyGroup.tracks {
		if track.TrackNumber != i+1 {
			t.Errorf("Moby Dick track %d has TrackNumber %d, want %d", i, track.TrackNumber, i+1)
		}
	}

	if prideGroup == nil {
		t.Fatal("expected to find 'Jane Austen|Pride and Prejudice' group")
	}
	if len(prideGroup.tracks) != 2 {
		t.Errorf("expected 2 tracks in Pride and Prejudice group, got %d", len(prideGroup.tracks))
	}
}

// TestBuildBookFromAlbumGroup_MultiTrack tests that multi-track albums sum duration and size
func TestBuildBookFromAlbumGroup_MultiTrack(t *testing.T) {
	tmpDir := t.TempDir()
	// Create 3 temp files for the multi-track album
	var tracks []*itunes.Track
	for i := 1; i <= 3; i++ {
		f, err := os.Create(filepath.Join(tmpDir, fmt.Sprintf("chapter%d.m4b", i)))
		if err != nil {
			t.Fatal(err)
		}
		f.Write(bytes.Repeat([]byte("x"), 100))
		f.Close()

		tracks = append(tracks, &itunes.Track{
			TrackID:     500 + i - 1,
			Name:        fmt.Sprintf("Chapter %d", i),
			Artist:      "Herman Melville",
			Album:       "Moby Dick",
			Kind:        "Audiobook",
			TrackNumber: i,
			DiscNumber:  1,
			TotalTime:   int64(3000000 + i*100000),
			Size:        int64(40000000 + i*5000000),
			Location:    itunes.EncodeLocation(filepath.Join(tmpDir, fmt.Sprintf("chapter%d.m4b", i))),
		})
	}

	group := albumGroup{key: "Herman Melville|Moby Dick", tracks: tracks}
	book, err := buildBookFromAlbumGroup(group, "/library.xml", itunes.ImportOptions{})
	if err != nil {
		t.Fatalf("buildBookFromAlbumGroup error: %v", err)
	}

	// Title should be album name
	if book.Title != "Moby Dick" {
		t.Errorf("title = %q, want %q", book.Title, "Moby Dick")
	}

	// FilePath should be common parent dir for multi-track
	if book.FilePath != tmpDir {
		t.Errorf("filePath = %q, want common parent dir %q", book.FilePath, tmpDir)
	}

	// Duration should be summed: (3100000 + 3200000 + 3300000) / 1000 = 9600 seconds
	expectedDuration := int((3100000 + 3200000 + 3300000) / 1000)
	if book.Duration == nil || *book.Duration != expectedDuration {
		t.Errorf("duration = %v, want %d", book.Duration, expectedDuration)
	}

	// FileSize should be summed
	expectedSize := int64(45000000 + 50000000 + 55000000)
	if book.FileSize == nil || *book.FileSize != expectedSize {
		t.Errorf("fileSize = %v, want %d", book.FileSize, expectedSize)
	}
}

// TestGroupTracksByAlbum_DiscSorting tests that tracks are sorted by disc then track number
func TestGroupTracksByAlbum_DiscSorting(t *testing.T) {
	library := &itunes.Library{
		Tracks: map[string]*itunes.Track{
			"1": {TrackID: 1, Name: "D2T1", Artist: "Author", Album: "Book", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 2},
			"2": {TrackID: 2, Name: "D1T2", Artist: "Author", Album: "Book", Kind: "Audiobook", TrackNumber: 2, DiscNumber: 1},
			"3": {TrackID: 3, Name: "D1T1", Artist: "Author", Album: "Book", Kind: "Audiobook", TrackNumber: 1, DiscNumber: 1},
		},
	}

	groups := groupTracksByAlbum(library)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	tracks := groups[0].tracks
	// Expected order: D1T1, D1T2, D2T1
	expected := []struct{ disc, track int }{{1, 1}, {1, 2}, {2, 1}}
	for i, e := range expected {
		if tracks[i].DiscNumber != e.disc || tracks[i].TrackNumber != e.track {
			t.Errorf("track %d: got disc=%d track=%d, want disc=%d track=%d",
				i, tracks[i].DiscNumber, tracks[i].TrackNumber, e.disc, e.track)
		}
	}
}

// copyLibraryWithCleanModTime copies an iTunes library XML to a temp dir
// and sets its modTime to a whole-second value so it survives RFC3339 round-trip.
func copyLibraryWithCleanModTime(t *testing.T, srcPath string) string {
	t.Helper()
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read library: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "iTunes Music Library.xml")
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("failed to write library copy: %v", err)
	}
	// Set modTime to a clean second so RFC3339 round-trip is lossless
	cleanTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dst, cleanTime, cleanTime); err != nil {
		t.Fatalf("failed to set modtime: %v", err)
	}
	return dst
}

// TestITunesSyncForceFlag_NoChanges tests that force=false returns "no changes detected"
// when the library fingerprint matches the stored fingerprint.
func TestITunesSyncForceFlag_NoChanges(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	srcPath := filepath.Join("../../testdata/itunes", "iTunes Music Library.xml")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skipf("iTunes test library not found at %s", srcPath)
	}

	libPath := copyLibraryWithCleanModTime(t, srcPath)

	// Store a fingerprint that matches the file's size and modTime
	info, err := os.Stat(libPath)
	if err != nil {
		t.Fatalf("failed to stat library file: %v", err)
	}
	err = database.GlobalStore.SaveLibraryFingerprint(libPath, info.Size(), info.ModTime(), 0)
	if err != nil {
		t.Fatalf("failed to save fingerprint: %v", err)
	}

	// Sync with force=false — should get "no changes detected"
	payload := map[string]interface{}{
		"library_path": libPath,
		"force":        false,
	}
	body := marshal(t, payload)

	req := httptest.NewRequest("POST", "/api/v1/itunes/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "no changes detected") {
		t.Errorf("expected 'no changes detected' in message, got %q", msg)
	}
	opID, _ := resp["operation_id"].(string)
	if opID != "" {
		t.Errorf("expected empty operation_id, got %q", opID)
	}
}

// TestITunesSyncForceFlag_Bypass tests that force=true bypasses the fingerprint check
// even when the stored fingerprint matches.
func TestITunesSyncForceFlag_Bypass(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	srcPath := filepath.Join("../../testdata/itunes", "iTunes Music Library.xml")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skipf("iTunes test library not found at %s", srcPath)
	}

	libPath := copyLibraryWithCleanModTime(t, srcPath)

	// Store a fingerprint that matches the file's size and modTime
	info, err := os.Stat(libPath)
	if err != nil {
		t.Fatalf("failed to stat library file: %v", err)
	}
	err = database.GlobalStore.SaveLibraryFingerprint(libPath, info.Size(), info.ModTime(), 0)
	if err != nil {
		t.Fatalf("failed to save fingerprint: %v", err)
	}

	// Sync with force=true — should bypass fingerprint check and queue sync
	payload := map[string]interface{}{
		"library_path": libPath,
		"force":        true,
	}
	body := marshal(t, payload)

	req := httptest.NewRequest("POST", "/api/v1/itunes/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Fatalf("expected 202 (Accepted), got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	opID, _ := resp["operation_id"].(string)
	if opID == "" {
		t.Errorf("expected non-empty operation_id when force=true")
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "queued") {
		t.Errorf("expected 'queued' in message, got %q", msg)
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
