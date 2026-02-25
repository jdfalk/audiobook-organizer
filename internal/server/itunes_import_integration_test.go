// file: internal/server/itunes_import_integration_test.go
// version: 1.0.0
// guid: c2d3e4f5-a6b7-8901-cdef-234567890abc

package server

import (
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLibraryPath(t *testing.T) string {
	t.Helper()
	root := testutil.FindRepoRoot(t)
	return filepath.Join(root, "internal", "itunes", "testdata", "test_library.xml")
}

// TestITunesImport_ParseAndGroupTestLibrary parses the real test library XML
// and verifies groupTracksByAlbum filters and groups correctly.
func TestITunesImport_ParseAndGroupTestLibrary(t *testing.T) {
	library, err := itunes.ParseLibrary(testLibraryPath(t))
	require.NoError(t, err)
	require.NotNil(t, library)

	groups := groupTracksByAlbum(library)

	// Test library has 9 tracks total; track 300 (Queen) is music, not audiobook.
	// Audiobook tracks: 100 (Hobbit), 200 (Dune), 400 (Art of War),
	//   500/501/502 (Moby Dick x3), 600/601 (Pride x2)
	// Grouping by Artist|Album:
	//   "J.R.R. Tolkien|Middle-earth, Book 1" (1 track)
	//   "Frank Herbert|Dune Chronicles" (1 track)
	//   "Sun Tzu|The Art of War" (1 track) -- album empty, falls back to track name
	//   "Herman Melville|Moby Dick" (3 tracks)
	//   "Jane Austen|Pride and Prejudice" (2 tracks)
	require.Len(t, groups, 5, "should have 5 audiobook groups (Queen filtered out)")

	// Build a lookup by key for easier assertions
	byKey := make(map[string]albumGroup)
	for _, g := range groups {
		byKey[g.key] = g
	}

	// Multi-track: Moby Dick = 3 tracks
	moby, ok := byKey["Herman Melville|Moby Dick"]
	require.True(t, ok, "Moby Dick group should exist")
	assert.Len(t, moby.tracks, 3)
	// Verify sorted by track number
	assert.Equal(t, 1, moby.tracks[0].TrackNumber)
	assert.Equal(t, 2, moby.tracks[1].TrackNumber)
	assert.Equal(t, 3, moby.tracks[2].TrackNumber)

	// Multi-track: Pride and Prejudice = 2 tracks
	pride, ok := byKey["Jane Austen|Pride and Prejudice"]
	require.True(t, ok, "Pride and Prejudice group should exist")
	assert.Len(t, pride.tracks, 2)

	// Single-track audiobooks
	_, ok = byKey["J.R.R. Tolkien|Middle-earth, Book 1"]
	assert.True(t, ok, "Hobbit group should exist")

	_, ok = byKey["Frank Herbert|Dune Chronicles"]
	assert.True(t, ok, "Dune group should exist")

	// Art of War has empty album, so key uses track Name as album
	_, ok = byKey["Sun Tzu|The Art of War"]
	assert.True(t, ok, "Art of War group should exist")
}

// TestITunesImport_BuildAndSaveBooks is an integration test that parses the
// test library, groups tracks, builds books with temp files, and saves to real SQLite.
func TestITunesImport_BuildAndSaveBooks(t *testing.T) {
	env, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	library, err := itunes.ParseLibrary(testLibraryPath(t))
	require.NoError(t, err)

	groups := groupTracksByAlbum(library)
	require.NotEmpty(t, groups)

	// Create temp files matching each track's decoded location so buildBookFromAlbumGroup
	// can os.Stat them. We remap the iTunes paths to our temp dir.
	tempAudioDir := filepath.Join(env.TempDir, "audiobooks")
	require.NoError(t, os.MkdirAll(tempAudioDir, 0755))

	// Build path mapping: rewrite /Users/testuser/Music/iTunes to our temp dir
	opts := itunes.ImportOptions{
		PathMappings: []itunes.PathMapping{
			{From: "file://localhost/Users/testuser/Music/iTunes", To: "file://localhost" + tempAudioDir},
		},
	}

	// Create fake files for every audiobook track
	for _, track := range library.Tracks {
		if !itunes.IsAudiobook(track) {
			continue
		}
		remapped := opts.RemapPath(track.Location)
		decoded, err := itunes.DecodeLocation(remapped)
		require.NoError(t, err, "decode location for track %d", track.TrackID)
		require.NoError(t, os.MkdirAll(filepath.Dir(decoded), 0755))
		require.NoError(t, os.WriteFile(decoded, []byte("fake-audio-data"), 0644))
	}

	var savedBooks []*database.Book
	for _, group := range groups {
		book, err := buildBookFromAlbumGroup(group, testLibraryPath(t), opts)
		require.NoError(t, err, "build book for group %s", group.key)

		created, err := database.GlobalStore.CreateBook(book)
		require.NoError(t, err, "save book %s", book.Title)
		savedBooks = append(savedBooks, created)
	}

	assert.Len(t, savedBooks, 5)

	// Verify books exist in DB with correct titles
	for _, book := range savedBooks {
		fetched, err := database.GlobalStore.GetBookByID(book.ID)
		require.NoError(t, err)
		require.NotNil(t, fetched)
		assert.Equal(t, book.Title, fetched.Title)
	}

	// Verify multi-track Moby Dick has summed duration
	// Moby Dick tracks: 3600000 + 3200000 + 3400000 = 10200000ms = 10200s
	for _, book := range savedBooks {
		if book.Title == "Moby Dick" {
			require.NotNil(t, book.Duration)
			assert.Equal(t, 10200, *book.Duration, "Moby Dick should have summed duration")
			require.NotNil(t, book.FileSize)
			assert.Equal(t, int64(50000000+45000000+48000000), *book.FileSize, "Moby Dick should have summed file size")
		}
	}

	// Verify Pride and Prejudice summed duration: 5400000 + 4800000 = 10200000ms = 10200s
	for _, book := range savedBooks {
		if book.Title == "Pride and Prejudice" {
			require.NotNil(t, book.Duration)
			assert.Equal(t, 10200, *book.Duration)
		}
	}
}

// TestITunesImport_AuthorAndSeriesCreation tests assignAuthorAndSeries with real SQLite.
func TestITunesImport_AuthorAndSeriesCreation(t *testing.T) {
	_, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	// Test 1: create author and series from a track
	book1 := &database.Book{Title: "Dune"}
	track1 := &itunes.Track{
		Artist: "Frank Herbert",
		Album:  "Dune Chronicles",
	}
	assignAuthorAndSeries(book1, track1)

	require.NotNil(t, book1.AuthorID, "AuthorID should be set")
	author, err := database.GlobalStore.GetAuthorByName("Frank Herbert")
	require.NoError(t, err)
	require.NotNil(t, author)
	assert.Equal(t, *book1.AuthorID, author.ID)

	// "Dune Chronicles" has no separator with exactly 2 parts, so extractSeriesName
	// returns the whole album name
	require.NotNil(t, book1.SeriesID, "SeriesID should be set")
	series, err := database.GlobalStore.GetSeriesByName("Dune Chronicles", book1.AuthorID)
	require.NoError(t, err)
	require.NotNil(t, series)
	assert.Equal(t, *book1.SeriesID, series.ID)

	// Test 2: call again with same artist, verify no duplicate
	book2 := &database.Book{Title: "Dune Messiah"}
	track2 := &itunes.Track{
		Artist: "Frank Herbert",
		Album:  "Dune Messiah",
	}
	assignAuthorAndSeries(book2, track2)

	require.NotNil(t, book2.AuthorID)
	assert.Equal(t, *book1.AuthorID, *book2.AuthorID, "same author should be reused")

	// Test 3: series with comma separator
	book3 := &database.Book{Title: "The Hobbit"}
	track3 := &itunes.Track{
		Artist: "J.R.R. Tolkien",
		Album:  "Middle-earth, Book 1",
	}
	assignAuthorAndSeries(book3, track3)

	require.NotNil(t, book3.SeriesID)
	// extractSeriesName("Middle-earth, Book 1") should return "Middle-earth"
	seriesMiddle, err := database.GlobalStore.GetSeriesByName("Middle-earth", book3.AuthorID)
	require.NoError(t, err)
	require.NotNil(t, seriesMiddle)
	assert.Equal(t, *book3.SeriesID, seriesMiddle.ID)
}

// TestITunesImport_AuthorDedup verifies that two tracks by the same artist
// result in a single author record in the database.
func TestITunesImport_AuthorDedup(t *testing.T) {
	_, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	bookA := &database.Book{Title: "Book A"}
	trackA := &itunes.Track{Artist: "Isaac Asimov", Album: "Foundation"}
	assignAuthorAndSeries(bookA, trackA)

	bookB := &database.Book{Title: "Book B"}
	trackB := &itunes.Track{Artist: "Isaac Asimov", Album: "I, Robot"}
	assignAuthorAndSeries(bookB, trackB)

	require.NotNil(t, bookA.AuthorID)
	require.NotNil(t, bookB.AuthorID)
	assert.Equal(t, *bookA.AuthorID, *bookB.AuthorID, "both books should share the same author ID")

	// Verify only one author named "Isaac Asimov" exists
	authors, err := database.GlobalStore.GetAllAuthors()
	require.NoError(t, err)
	count := 0
	for _, a := range authors {
		if a.Name == "Isaac Asimov" {
			count++
		}
	}
	assert.Equal(t, 1, count, "should have exactly one Isaac Asimov author record")
}

// TestITunesImport_SeriesExtraction tests extractSeriesName and verifies
// series records are created in the DB.
func TestITunesImport_SeriesExtraction(t *testing.T) {
	_, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	tests := []struct {
		album          string
		expectedSeries string
	}{
		{"Middle-earth, Book 1", "Middle-earth"},         // comma separator
		{"The Dark Tower: Book 3", "The Dark Tower"},     // colon separator
		{"Wheel of Time - Book 5", "Wheel of Time"},      // dash separator
		{"Dune Chronicles", "Dune Chronicles"},           // no separator, whole name
		{"", ""},                                          // empty
	}

	for _, tc := range tests {
		result := extractSeriesName(tc.album)
		assert.Equal(t, tc.expectedSeries, result, "extractSeriesName(%q)", tc.album)
	}

	// Integration: verify series records created in DB
	book := &database.Book{Title: "Test Book"}
	track := &itunes.Track{
		Artist: "Test Author",
		Album:  "Middle-earth, Book 1",
	}
	assignAuthorAndSeries(book, track)

	require.NotNil(t, book.SeriesID)
	series, err := database.GlobalStore.GetSeriesByID(*book.SeriesID)
	require.NoError(t, err)
	require.NotNil(t, series)
	assert.Equal(t, "Middle-earth", series.Name)

	// Another book in same series should get same series ID
	book2 := &database.Book{Title: "Test Book 2"}
	track2 := &itunes.Track{
		Artist: "Test Author",
		Album:  "Middle-earth, Book 2",
	}
	assignAuthorAndSeries(book2, track2)
	require.NotNil(t, book2.SeriesID)
	assert.Equal(t, *book.SeriesID, *book2.SeriesID, "same series name should yield same series ID")
}

// TestITunesImport_MultiTrackBookSegments tests creating BookSegments for
// multi-track albums, mimicking what executeITunesImport does.
func TestITunesImport_MultiTrackBookSegments(t *testing.T) {
	env, cleanup := testutil.SetupIntegration(t)
	defer cleanup()

	library, err := itunes.ParseLibrary(testLibraryPath(t))
	require.NoError(t, err)

	groups := groupTracksByAlbum(library)

	// Find the Moby Dick group (3 tracks)
	var mobyGroup *albumGroup
	for i, g := range groups {
		if g.key == "Herman Melville|Moby Dick" {
			mobyGroup = &groups[i]
			break
		}
	}
	require.NotNil(t, mobyGroup, "Moby Dick group should exist")
	require.Len(t, mobyGroup.tracks, 3)

	// Create temp files for Moby Dick tracks
	tempDir := filepath.Join(env.TempDir, "audiobooks")
	opts := itunes.ImportOptions{
		PathMappings: []itunes.PathMapping{
			{From: "file://localhost/Users/testuser/Music/iTunes", To: "file://localhost" + tempDir},
		},
	}

	for _, track := range mobyGroup.tracks {
		remapped := opts.RemapPath(track.Location)
		decoded, err := itunes.DecodeLocation(remapped)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(decoded), 0755))
		require.NoError(t, os.WriteFile(decoded, []byte("fake-audio"), 0644))
	}

	book, err := buildBookFromAlbumGroup(*mobyGroup, testLibraryPath(t), opts)
	require.NoError(t, err)

	created, err := database.GlobalStore.CreateBook(book)
	require.NoError(t, err)

	// Create segments as executeITunesImport does
	bookNumericID := int(crc32.ChecksumIEEE([]byte(created.ID)))
	for _, track := range mobyGroup.tracks {
		trackLoc := opts.RemapPath(track.Location)
		trackPath, err := itunes.DecodeLocation(trackLoc)
		require.NoError(t, err)

		trackNum := track.TrackNumber
		totalTracks := len(mobyGroup.tracks)
		segment := &database.BookSegment{
			FilePath:    trackPath,
			Format:      "m4b",
			SizeBytes:   track.Size,
			DurationSec: int(track.TotalTime / 1000),
			TrackNumber: &trackNum,
			TotalTracks: &totalTracks,
			Active:      true,
		}
		_, err = database.GlobalStore.CreateBookSegment(bookNumericID, segment)
		require.NoError(t, err)
	}

	// Verify segments
	segments, err := database.GlobalStore.ListBookSegments(bookNumericID)
	require.NoError(t, err)
	require.Len(t, segments, 3)

	// Verify track numbers and durations
	for _, seg := range segments {
		require.NotNil(t, seg.TrackNumber)
		require.NotNil(t, seg.TotalTracks)
		assert.Equal(t, 3, *seg.TotalTracks)
		assert.True(t, seg.Active)
	}
}

// TestITunesImport_PlaylistTagExtraction tests ExtractPlaylistTags against
// the test library's playlists.
func TestITunesImport_PlaylistTagExtraction(t *testing.T) {
	library, err := itunes.ParseLibrary(testLibraryPath(t))
	require.NoError(t, err)

	// Built-in playlists that should be filtered: Music, Audiobooks, Recently Added
	// User playlists: Sci-Fi Favorites

	// Track 100 (Hobbit): in Audiobooks (built-in, filtered) and Sci-Fi Favorites (kept)
	tags100 := itunes.ExtractPlaylistTags(100, library.Playlists)
	assert.Contains(t, tags100, "sci-fi favorites", "track 100 should have sci-fi favorites tag")
	assert.NotContains(t, tags100, "audiobooks", "built-in Audiobooks should be filtered")
	assert.NotContains(t, tags100, "music", "Music playlist should be filtered")

	// Track 200 (Dune): in Audiobooks (filtered) and Sci-Fi Favorites (kept)
	tags200 := itunes.ExtractPlaylistTags(200, library.Playlists)
	assert.Contains(t, tags200, "sci-fi favorites")
	assert.NotContains(t, tags200, "audiobooks")

	// Track 400 (Art of War): in Audiobooks (filtered) and Recently Added (filtered)
	tags400 := itunes.ExtractPlaylistTags(400, library.Playlists)
	assert.NotContains(t, tags400, "audiobooks")
	assert.NotContains(t, tags400, "recently added")
	assert.Empty(t, tags400, "Art of War should have no user playlist tags")

	// Track 300 (Queen): in Music only (filtered)
	tags300 := itunes.ExtractPlaylistTags(300, library.Playlists)
	assert.Empty(t, tags300)
}
