// file: internal/itunes/itunes_test.go
// version: 1.0.0
// guid: f3a7c891-2d4e-5b6f-8a9c-0d1e2f3a4b5c

package itunes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdataPath returns the absolute path to the testdata directory.
func testdataPath(t *testing.T) string {
	t.Helper()
	// The test binary runs from the package directory, so testdata is relative.
	path, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("failed to resolve testdata path: %v", err)
	}
	return path
}

// testLibraryPath returns the path to the synthetic test library XML.
func testLibraryPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testdataPath(t), "test_library.xml")
}

// ---------- ParseLibrary / parsePlist tests ----------

func TestParseLibrary_Success(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	if library == nil {
		t.Fatal("ParseLibrary() returned nil library")
	}

	// Verify library metadata
	if library.MajorVersion != 1 {
		t.Errorf("MajorVersion = %d, want 1", library.MajorVersion)
	}
	if library.MinorVersion != 1 {
		t.Errorf("MinorVersion = %d, want 1", library.MinorVersion)
	}
	if library.ApplicationVersion != "12.9.5.5" {
		t.Errorf("ApplicationVersion = %q, want %q", library.ApplicationVersion, "12.9.5.5")
	}
	if !strings.Contains(library.MusicFolder, "iTunes") {
		t.Errorf("MusicFolder = %q, expected it to contain 'iTunes'", library.MusicFolder)
	}
}

func TestParseLibrary_Tracks(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	if len(library.Tracks) != 4 {
		t.Fatalf("expected 4 tracks, got %d", len(library.Tracks))
	}

	// Find The Hobbit track
	var hobbit *Track
	for _, track := range library.Tracks {
		if track.Name == "The Hobbit" {
			hobbit = track
			break
		}
	}

	if hobbit == nil {
		t.Fatal("expected to find 'The Hobbit' track")
	}

	if hobbit.TrackID != 100 {
		t.Errorf("TrackID = %d, want 100", hobbit.TrackID)
	}
	if hobbit.PersistentID != "ABCD1234EFGH5678" {
		t.Errorf("PersistentID = %q, want %q", hobbit.PersistentID, "ABCD1234EFGH5678")
	}
	if hobbit.Artist != "J.R.R. Tolkien" {
		t.Errorf("Artist = %q, want %q", hobbit.Artist, "J.R.R. Tolkien")
	}
	if hobbit.AlbumArtist != "Rob Inglis" {
		t.Errorf("AlbumArtist = %q, want %q", hobbit.AlbumArtist, "Rob Inglis")
	}
	if hobbit.Album != "Middle-earth, Book 1" {
		t.Errorf("Album = %q, want %q", hobbit.Album, "Middle-earth, Book 1")
	}
	if hobbit.Genre != "Audiobook" {
		t.Errorf("Genre = %q, want %q", hobbit.Genre, "Audiobook")
	}
	if hobbit.Kind != "Audiobook" {
		t.Errorf("Kind = %q, want %q", hobbit.Kind, "Audiobook")
	}
	if hobbit.Year != 1997 {
		t.Errorf("Year = %d, want 1997", hobbit.Year)
	}
	if hobbit.Size != 524288000 {
		t.Errorf("Size = %d, want 524288000", hobbit.Size)
	}
	if hobbit.TotalTime != 39600000 {
		t.Errorf("TotalTime = %d, want 39600000", hobbit.TotalTime)
	}
	if hobbit.PlayCount != 3 {
		t.Errorf("PlayCount = %d, want 3", hobbit.PlayCount)
	}
	if hobbit.Rating != 80 {
		t.Errorf("Rating = %d, want 80", hobbit.Rating)
	}
	if hobbit.Bookmark != 1200000 {
		t.Errorf("Bookmark = %d, want 1200000", hobbit.Bookmark)
	}
	if !hobbit.Bookmarkable {
		t.Error("Bookmarkable = false, want true")
	}
}

func TestParseLibrary_Playlists(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	if len(library.Playlists) != 4 {
		t.Fatalf("expected 4 playlists, got %d", len(library.Playlists))
	}

	// Find "Sci-Fi Favorites" playlist
	var scifi *Playlist
	for _, pl := range library.Playlists {
		if pl.Name == "Sci-Fi Favorites" {
			scifi = pl
			break
		}
	}

	if scifi == nil {
		t.Fatal("expected to find 'Sci-Fi Favorites' playlist")
	}

	if len(scifi.TrackIDs) != 2 {
		t.Errorf("Sci-Fi Favorites has %d tracks, want 2", len(scifi.TrackIDs))
	}
}

func TestParseLibrary_FileNotFound(t *testing.T) {
	_, err := ParseLibrary("/nonexistent/path/Library.xml")
	if err == nil {
		t.Error("ParseLibrary() expected error for nonexistent file, got nil")
	}
}

func TestParseLibrary_InvalidXML(t *testing.T) {
	// Create a temp file with invalid XML
	tmpFile := filepath.Join(t.TempDir(), "invalid.xml")
	if err := os.WriteFile(tmpFile, []byte("<not>valid plist</not>"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := ParseLibrary(tmpFile)
	if err == nil {
		t.Error("ParseLibrary() expected error for invalid XML, got nil")
	}
}

// ---------- IsAudiobook tests (audiobook detection from parsed tracks) ----------

func TestIsAudiobook_FromParsedLibrary(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	audiobookCount := 0
	musicCount := 0
	for _, track := range library.Tracks {
		if IsAudiobook(track) {
			audiobookCount++
		} else {
			musicCount++
		}
	}

	// Tracks 100, 200, 400 are audiobooks; track 300 is music
	if audiobookCount != 3 {
		t.Errorf("expected 3 audiobooks, got %d", audiobookCount)
	}
	if musicCount != 1 {
		t.Errorf("expected 1 music track, got %d", musicCount)
	}
}

// ---------- ExtractPlaylistTags tests ----------

func TestExtractPlaylistTags(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	// Track 100 (The Hobbit) is in "Audiobooks" (built-in, filtered) and "Sci-Fi Favorites"
	tags := ExtractPlaylistTags(100, library.Playlists)

	// Should only contain "sci-fi favorites" (not "Audiobooks" which is built-in)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d: %v", len(tags), tags)
	}
	if tags[0] != "sci-fi favorites" {
		t.Errorf("expected tag %q, got %q", "sci-fi favorites", tags[0])
	}
}

func TestExtractPlaylistTags_NoPlaylists(t *testing.T) {
	tags := ExtractPlaylistTags(999, nil)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags for nonexistent track, got %d", len(tags))
	}
}

func TestExtractPlaylistTags_BuiltInFiltered(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	// Track 300 (Bohemian Rhapsody) is only in "Music" (built-in)
	tags := ExtractPlaylistTags(300, library.Playlists)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags (all built-in playlists), got %d: %v", len(tags), tags)
	}
}

func TestExtractPlaylistTags_RecentlyAddedFiltered(t *testing.T) {
	library, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	// Track 400 is in "Audiobooks" (built-in) and "Recently Added" (built-in)
	tags := ExtractPlaylistTags(400, library.Playlists)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags (all built-in), got %d: %v", len(tags), tags)
	}
}

// ---------- isBuiltInPlaylist tests ----------

func TestIsBuiltInPlaylist(t *testing.T) {
	builtIns := []string{
		"Music", "Movies", "TV Shows", "Podcasts", "Audiobooks",
		"iTunes U", "Books", "Genius", "Recently Added",
		"Recently Played", "Top 25 Most Played",
	}

	for _, name := range builtIns {
		if !isBuiltInPlaylist(name) {
			t.Errorf("isBuiltInPlaylist(%q) = false, want true", name)
		}
	}

	customs := []string{"Sci-Fi Favorites", "My Audiobooks", "Road Trip", ""}
	for _, name := range customs {
		if isBuiltInPlaylist(name) {
			t.Errorf("isBuiltInPlaylist(%q) = true, want false", name)
		}
	}
}

// ---------- ConvertTrack tests ----------

func TestConvertTrack(t *testing.T) {
	// Create a temporary audiobook file so os.Stat succeeds
	tmpDir := t.TempDir()
	fakeFile := filepath.Join(tmpDir, "The Hobbit.m4b")
	if err := os.WriteFile(fakeFile, []byte("fake audio data"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	track := &Track{
		TrackID:      100,
		PersistentID: "ABCD1234EFGH5678",
		Name:         "The Hobbit",
		Artist:       "J.R.R. Tolkien",
		AlbumArtist:  "Rob Inglis",
		Album:        "Middle-earth, Book 1",
		Genre:        "Audiobook",
		Kind:         "Audiobook",
		Year:         1997,
		Comments:     "Unabridged Edition",
		Location:     EncodeLocation(fakeFile),
		TotalTime:    39600000,
		PlayCount:    3,
		PlayDate:     1700000000,
		Rating:       80,
		Bookmark:     1200000,
	}

	opts := ImportOptions{
		LibraryPath:    "/fake/Library.xml",
		ImportMode:     ImportModeImport,
		SkipDuplicates: true,
	}

	book, err := ConvertTrack(track, opts)
	if err != nil {
		t.Fatalf("ConvertTrack() error = %v", err)
	}

	if book.Title != "The Hobbit" {
		t.Errorf("Title = %q, want %q", book.Title, "The Hobbit")
	}
	if book.FilePath != fakeFile {
		t.Errorf("FilePath = %q, want %q", book.FilePath, fakeFile)
	}
	if book.Format != "m4b" {
		t.Errorf("Format = %q, want %q", book.Format, "m4b")
	}

	// Duration should be converted from milliseconds to seconds
	if book.Duration == nil || *book.Duration != 39600 {
		t.Errorf("Duration = %v, want 39600", book.Duration)
	}

	// Narrator should be extracted from AlbumArtist (different from Artist)
	if book.Narrator == nil || *book.Narrator != "Rob Inglis" {
		t.Errorf("Narrator = %v, want %q", book.Narrator, "Rob Inglis")
	}

	// Edition from Comments
	if book.Edition == nil || *book.Edition != "Unabridged Edition" {
		t.Errorf("Edition = %v, want %q", book.Edition, "Unabridged Edition")
	}

	// Year
	if book.AudiobookReleaseYear == nil || *book.AudiobookReleaseYear != 1997 {
		t.Errorf("AudiobookReleaseYear = %v, want 1997", book.AudiobookReleaseYear)
	}

	// iTunes-specific fields
	if book.ITunesPersistentID == nil || *book.ITunesPersistentID != "ABCD1234EFGH5678" {
		t.Errorf("ITunesPersistentID = %v, want %q", book.ITunesPersistentID, "ABCD1234EFGH5678")
	}
	if book.ITunesPlayCount == nil || *book.ITunesPlayCount != 3 {
		t.Errorf("ITunesPlayCount = %v, want 3", book.ITunesPlayCount)
	}
	if book.ITunesRating == nil || *book.ITunesRating != 80 {
		t.Errorf("ITunesRating = %v, want 80", book.ITunesRating)
	}
	if book.ITunesBookmark == nil || *book.ITunesBookmark != 1200000 {
		t.Errorf("ITunesBookmark = %v, want 1200000", book.ITunesBookmark)
	}
	if book.ITunesLastPlayed == nil {
		t.Error("ITunesLastPlayed should not be nil when PlayDate > 0")
	}
}

func TestConvertTrack_NoNarrator(t *testing.T) {
	// When AlbumArtist == Artist, narrator should not be set
	tmpDir := t.TempDir()
	fakeFile := filepath.Join(tmpDir, "book.mp3")
	os.WriteFile(fakeFile, []byte("data"), 0644)

	track := &Track{
		Name:        "Some Book",
		Artist:      "Author Name",
		AlbumArtist: "Author Name", // Same as Artist
		Location:    EncodeLocation(fakeFile),
	}

	book, err := ConvertTrack(track, ImportOptions{})
	if err != nil {
		t.Fatalf("ConvertTrack() error = %v", err)
	}

	if book.Narrator != nil {
		t.Errorf("Narrator should be nil when AlbumArtist == Artist, got %q", *book.Narrator)
	}
}

func TestConvertTrack_MissingFile(t *testing.T) {
	track := &Track{
		Name:     "Missing Book",
		Location: EncodeLocation("/nonexistent/path/book.m4b"),
	}

	_, err := ConvertTrack(track, ImportOptions{})
	if err == nil {
		t.Error("ConvertTrack() expected error for missing file, got nil")
	}
}

// ---------- ValidateImport tests ----------

func TestValidateImport_WithTestLibrary(t *testing.T) {
	opts := ImportOptions{
		LibraryPath:    testLibraryPath(t),
		ImportMode:     ImportModeImport,
		SkipDuplicates: false,
	}

	result, err := ValidateImport(opts)
	if err != nil {
		t.Fatalf("ValidateImport() error = %v", err)
	}

	if result.TotalTracks != 4 {
		t.Errorf("TotalTracks = %d, want 4", result.TotalTracks)
	}
	if result.AudiobookTracks != 3 {
		t.Errorf("AudiobookTracks = %d, want 3", result.AudiobookTracks)
	}
	// All files should be missing since they're fake paths
	if result.FilesMissing != 3 {
		t.Errorf("FilesMissing = %d, want 3", result.FilesMissing)
	}
	if result.FilesFound != 0 {
		t.Errorf("FilesFound = %d, want 0", result.FilesFound)
	}
	if result.EstimatedTime == "" {
		t.Error("EstimatedTime should not be empty")
	}
}

func TestValidateImport_WithRealFiles(t *testing.T) {
	// Create temp files matching the expected paths
	tmpDir := t.TempDir()
	fakeFile := filepath.Join(tmpDir, "audiobook.m4b")
	os.WriteFile(fakeFile, []byte("fake audio"), 0644)

	// Create a minimal test library pointing to our temp file
	libraryContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Major Version</key><integer>1</integer>
	<key>Minor Version</key><integer>1</integer>
	<key>Tracks</key>
	<dict>
		<key>1</key>
		<dict>
			<key>Track ID</key><integer>1</integer>
			<key>Name</key><string>Test Book</string>
			<key>Kind</key><string>Audiobook</string>
			<key>Location</key><string>` + EncodeLocation(fakeFile) + `</string>
		</dict>
	</dict>
	<key>Playlists</key><array/>
</dict>
</plist>`

	libraryFile := filepath.Join(tmpDir, "Library.xml")
	os.WriteFile(libraryFile, []byte(libraryContent), 0644)

	opts := ImportOptions{
		LibraryPath:    libraryFile,
		ImportMode:     ImportModeImport,
		SkipDuplicates: true,
	}

	result, err := ValidateImport(opts)
	if err != nil {
		t.Fatalf("ValidateImport() error = %v", err)
	}

	if result.AudiobookTracks != 1 {
		t.Errorf("AudiobookTracks = %d, want 1", result.AudiobookTracks)
	}
	if result.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1", result.FilesFound)
	}
	if result.FilesMissing != 0 {
		t.Errorf("FilesMissing = %d, want 0", result.FilesMissing)
	}
}

func TestValidateImport_InvalidLibrary(t *testing.T) {
	_, err := ValidateImport(ImportOptions{
		LibraryPath: "/nonexistent/Library.xml",
	})
	if err == nil {
		t.Error("ValidateImport() expected error for nonexistent library")
	}
}

func TestValidateImport_EstimatedTime(t *testing.T) {
	// Verify estimated time formatting with test library (0 files found = "0 seconds")
	opts := ImportOptions{
		LibraryPath: testLibraryPath(t),
		ImportMode:  ImportModeImport,
	}

	result, err := ValidateImport(opts)
	if err != nil {
		t.Fatalf("ValidateImport() error = %v", err)
	}

	if result.EstimatedTime != "0 seconds" {
		t.Errorf("EstimatedTime = %q, want %q", result.EstimatedTime, "0 seconds")
	}
}

// ---------- WriteBack / writePlist tests ----------

func TestWriteBack_RoundTrip(t *testing.T) {
	// Copy test library to temp dir
	tmpDir := t.TempDir()
	srcData, err := os.ReadFile(testLibraryPath(t))
	if err != nil {
		t.Fatalf("failed to read test library: %v", err)
	}
	tmpLibrary := filepath.Join(tmpDir, "Library.xml")
	if err := os.WriteFile(tmpLibrary, srcData, 0644); err != nil {
		t.Fatalf("failed to write temp library: %v", err)
	}

	// Also create the new path file so ValidateWriteBack would be happy
	newPath := filepath.Join(tmpDir, "organized", "The Hobbit.m4b")
	os.MkdirAll(filepath.Dir(newPath), 0755)
	os.WriteFile(newPath, []byte("data"), 0644)

	opts := WriteBackOptions{
		LibraryPath:  tmpLibrary,
		CreateBackup: true,
		Updates: []*WriteBackUpdate{
			{
				ITunesPersistentID: "ABCD1234EFGH5678",
				OldPath:            "/Users/testuser/Music/iTunes/Audiobooks/The Hobbit.m4b",
				NewPath:            newPath,
			},
		},
	}

	result, err := WriteBack(opts)
	if err != nil {
		t.Fatalf("WriteBack() error = %v", err)
	}

	if !result.Success {
		t.Error("WriteBack() Success = false, want true")
	}
	if result.UpdatedCount != 1 {
		t.Errorf("UpdatedCount = %d, want 1", result.UpdatedCount)
	}
	if result.BackupPath == "" {
		t.Error("BackupPath should not be empty when CreateBackup=true")
	}

	// Verify backup was created
	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Error("backup file was not created")
	}

	// Parse the updated library and verify the location was changed
	updated, err := ParseLibrary(tmpLibrary)
	if err != nil {
		t.Fatalf("failed to parse updated library: %v", err)
	}

	for _, track := range updated.Tracks {
		if track.PersistentID == "ABCD1234EFGH5678" {
			decoded, _ := DecodeLocation(track.Location)
			if decoded != newPath {
				t.Errorf("updated location = %q, want %q", decoded, newPath)
			}
			return
		}
	}
	t.Error("track ABCD1234EFGH5678 not found in updated library")
}

func TestWriteBack_NoBackup(t *testing.T) {
	tmpDir := t.TempDir()
	srcData, _ := os.ReadFile(testLibraryPath(t))
	tmpLibrary := filepath.Join(tmpDir, "Library.xml")
	os.WriteFile(tmpLibrary, srcData, 0644)

	opts := WriteBackOptions{
		LibraryPath:  tmpLibrary,
		CreateBackup: false,
		Updates: []*WriteBackUpdate{
			{
				ITunesPersistentID: "WXYZ9876LMNO5432",
				NewPath:            "/new/path/Dune.mp3",
			},
		},
	}

	result, err := WriteBack(opts)
	if err != nil {
		t.Fatalf("WriteBack() error = %v", err)
	}

	if result.BackupPath != "" {
		t.Errorf("BackupPath = %q, want empty when CreateBackup=false", result.BackupPath)
	}
}

func TestWriteBack_CustomBackupPath(t *testing.T) {
	tmpDir := t.TempDir()
	srcData, _ := os.ReadFile(testLibraryPath(t))
	tmpLibrary := filepath.Join(tmpDir, "Library.xml")
	os.WriteFile(tmpLibrary, srcData, 0644)

	customBackup := filepath.Join(tmpDir, "my-backup.xml")

	opts := WriteBackOptions{
		LibraryPath:  tmpLibrary,
		CreateBackup: true,
		BackupPath:   customBackup,
		Updates: []*WriteBackUpdate{
			{
				ITunesPersistentID: "ABCD1234EFGH5678",
				NewPath:            "/new/hobbit.m4b",
			},
		},
	}

	result, err := WriteBack(opts)
	if err != nil {
		t.Fatalf("WriteBack() error = %v", err)
	}

	if result.BackupPath != customBackup {
		t.Errorf("BackupPath = %q, want %q", result.BackupPath, customBackup)
	}
	if _, err := os.Stat(customBackup); os.IsNotExist(err) {
		t.Error("custom backup file was not created")
	}
}

func TestWriteBack_EmptyLibraryPath(t *testing.T) {
	_, err := WriteBack(WriteBackOptions{
		LibraryPath: "",
		Updates:     []*WriteBackUpdate{{NewPath: "/foo"}},
	})
	if err == nil {
		t.Error("WriteBack() expected error for empty library path")
	}
}

func TestWriteBack_NoUpdates(t *testing.T) {
	_, err := WriteBack(WriteBackOptions{
		LibraryPath: testLibraryPath(t),
		Updates:     nil,
	})
	if err == nil {
		t.Error("WriteBack() expected error for nil updates")
	}
}

func TestWriteBack_NonexistentLibrary(t *testing.T) {
	_, err := WriteBack(WriteBackOptions{
		LibraryPath: "/nonexistent/Library.xml",
		Updates:     []*WriteBackUpdate{{NewPath: "/foo"}},
	})
	if err == nil {
		t.Error("WriteBack() expected error for nonexistent library")
	}
}

func TestWriteBack_UnmatchedPersistentID(t *testing.T) {
	tmpDir := t.TempDir()
	srcData, _ := os.ReadFile(testLibraryPath(t))
	tmpLibrary := filepath.Join(tmpDir, "Library.xml")
	os.WriteFile(tmpLibrary, srcData, 0644)

	opts := WriteBackOptions{
		LibraryPath: tmpLibrary,
		Updates: []*WriteBackUpdate{
			{
				ITunesPersistentID: "NONEXISTENT_ID",
				NewPath:            "/some/path.m4b",
			},
		},
	}

	result, err := WriteBack(opts)
	if err != nil {
		t.Fatalf("WriteBack() error = %v", err)
	}

	// Should succeed but with 0 updates
	if result.UpdatedCount != 0 {
		t.Errorf("UpdatedCount = %d, want 0 for unmatched ID", result.UpdatedCount)
	}
}

// ---------- ValidateWriteBack tests ----------

func TestValidateWriteBack(t *testing.T) {
	tmpDir := t.TempDir()
	srcData, _ := os.ReadFile(testLibraryPath(t))
	tmpLibrary := filepath.Join(tmpDir, "Library.xml")
	os.WriteFile(tmpLibrary, srcData, 0644)

	// Create one real file, leave one as nonexistent
	realFile := filepath.Join(tmpDir, "real_book.m4b")
	os.WriteFile(realFile, []byte("data"), 0644)

	opts := WriteBackOptions{
		LibraryPath: tmpLibrary,
		Updates: []*WriteBackUpdate{
			{
				ITunesPersistentID: "ABCD1234EFGH5678",
				NewPath:            realFile,
			},
			{
				ITunesPersistentID: "NONEXISTENT_ID",
				NewPath:            "/missing/path.m4b",
			},
		},
	}

	warnings, err := ValidateWriteBack(opts)
	if err != nil {
		t.Fatalf("ValidateWriteBack() error = %v", err)
	}

	// The nonexistent persistent ID generates a warning and continues (skips file check).
	// The valid persistent ID with a real file path generates no warning.
	// So we expect exactly 1 warning for the nonexistent persistent ID.
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if len(warnings) > 0 && !strings.Contains(warnings[0], "NONEXISTENT_ID") {
		t.Errorf("expected warning about NONEXISTENT_ID, got %q", warnings[0])
	}
}

func TestValidateWriteBack_NonexistentLibrary(t *testing.T) {
	_, err := ValidateWriteBack(WriteBackOptions{
		LibraryPath: "/nonexistent/Library.xml",
	})
	if err == nil {
		t.Error("ValidateWriteBack() expected error for nonexistent library")
	}
}

// ---------- WritePlist round-trip tests ----------

func TestWritePlist_PreservesAllTracks(t *testing.T) {
	// Parse, write, re-parse, verify all tracks preserved
	original, err := ParseLibrary(testLibraryPath(t))
	if err != nil {
		t.Fatalf("ParseLibrary() error = %v", err)
	}

	tmpFile := filepath.Join(t.TempDir(), "roundtrip.xml")
	if err := writePlist(original, tmpFile); err != nil {
		t.Fatalf("writePlist() error = %v", err)
	}

	reparsed, err := ParseLibrary(tmpFile)
	if err != nil {
		t.Fatalf("ParseLibrary(roundtrip) error = %v", err)
	}

	if len(reparsed.Tracks) != len(original.Tracks) {
		t.Errorf("round-trip lost tracks: got %d, want %d", len(reparsed.Tracks), len(original.Tracks))
	}

	// Verify track details survived
	for id, origTrack := range original.Tracks {
		newTrack, ok := reparsed.Tracks[id]
		if !ok {
			t.Errorf("track %s missing after round-trip", id)
			continue
		}
		if newTrack.Name != origTrack.Name {
			t.Errorf("track %s Name = %q, want %q", id, newTrack.Name, origTrack.Name)
		}
		if newTrack.Location != origTrack.Location {
			t.Errorf("track %s Location mismatch", id)
		}
	}

	if len(reparsed.Playlists) != len(original.Playlists) {
		t.Errorf("round-trip lost playlists: got %d, want %d", len(reparsed.Playlists), len(original.Playlists))
	}
}

// ---------- copyFile tests ----------

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "subdir", "dest.txt")

	content := "hello world test content"
	os.WriteFile(src, []byte(content), 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}

	if string(data) != content {
		t.Errorf("copied content = %q, want %q", string(data), content)
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/src", filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Error("copyFile() expected error for nonexistent source")
	}
}

// ---------- computeFileHash tests ----------

func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(file, []byte("hello"), 0644)

	hash, err := computeFileHash(file)
	if err != nil {
		t.Fatalf("computeFileHash() error = %v", err)
	}

	if hash == "" {
		t.Error("computeFileHash() returned empty hash")
	}

	// Same content should produce same hash
	file2 := filepath.Join(tmpDir, "test2.txt")
	os.WriteFile(file2, []byte("hello"), 0644)

	hash2, err := computeFileHash(file2)
	if err != nil {
		t.Fatalf("computeFileHash() error = %v", err)
	}

	if hash != hash2 {
		t.Error("same content produced different hashes")
	}

	// Different content should produce different hash
	file3 := filepath.Join(tmpDir, "test3.txt")
	os.WriteFile(file3, []byte("world"), 0644)

	hash3, _ := computeFileHash(file3)
	if hash == hash3 {
		t.Error("different content produced same hash")
	}
}

func TestComputeFileHash_NonexistentFile(t *testing.T) {
	_, err := computeFileHash("/nonexistent/file")
	if err == nil {
		t.Error("computeFileHash() expected error for nonexistent file")
	}
}

// ---------- ImportMode constants test ----------

func TestImportModeValues(t *testing.T) {
	if ImportModeOrganized != "organized" {
		t.Errorf("ImportModeOrganized = %q, want %q", ImportModeOrganized, "organized")
	}
	if ImportModeImport != "import" {
		t.Errorf("ImportModeImport = %q, want %q", ImportModeImport, "import")
	}
	if ImportModeOrganize != "organize" {
		t.Errorf("ImportModeOrganize = %q, want %q", ImportModeOrganize, "organize")
	}
}
