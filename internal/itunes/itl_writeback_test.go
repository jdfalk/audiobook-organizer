// file: internal/itunes/itl_writeback_test.go
// version: 1.0.0
// guid: 3d5f9b2c-ae4e-5c8d-b6g3-9e7f2d1c4b0a

package itunes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// UpdateITLLocations — path format tests
// ---------------------------------------------------------------------------

func TestUpdateITLLocations_PathFormat_WindowsStyle(t *testing.T) {
	// Verify that Windows-style paths (with drive letters, spaces) survive
	// a parse-update-reparse round-trip correctly.
	pid := [8]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	originalLoc := "W:/itunes/iTunes Media/Audiobooks/Author/book.m4b"
	newLoc := "W:/itunes/iTunes Media/Audiobooks/New Author/New Book/chapter01.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	itlData := buildSyntheticITL(t, "12.0.0", false, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, newLoc, lib.Tracks[0].Location, "location should be the exact new path")
}

func TestUpdateITLLocations_PathFormat_UnixAbsolute(t *testing.T) {
	pid := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	originalLoc := "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/old.m4b"
	newLoc := "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/new.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	itlData := buildSyntheticITL(t, "12.0.0", true, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, newLoc, lib.Tracks[0].Location)
}

func TestUpdateITLLocations_PathWithSpaces(t *testing.T) {
	pid := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11}
	originalLoc := "/music/simple.mp3"
	newLoc := "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Brandon Sanderson/01 The Way of Kings.mp3"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	itlData := buildSyntheticITL(t, "12.0.0", false, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, newLoc, lib.Tracks[0].Location)
}

func TestUpdateITLLocations_PathWithUnicode(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	originalLoc := "/music/old.mp3"
	newLoc := "/mnt/books/Audiobooks/Stéphane Mallarmé/L'après-midi d'un faune.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	itlData := buildSyntheticITL(t, "12.0.0", false, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, newLoc, lib.Tracks[0].Location)
}

// ---------------------------------------------------------------------------
// UpdateITLLocations — preserves other tracks
// ---------------------------------------------------------------------------

func TestUpdateITLLocations_PreservesOtherTracks(t *testing.T) {
	// Use the fixture ITL which has 9 tracks. Update one, verify others unchanged.
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	// Update only The Hobbit (index 0)
	hobbitPID := pidToHex(fixtureTracks[0].persistentID)
	newHobbitLoc := "/reorganized/The Hobbit.m4b"

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: hobbitPID, NewLocation: newHobbitLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, len(fixtureTracks))

	// Verify The Hobbit is updated
	assert.Equal(t, newHobbitLoc, lib.Tracks[0].Location)

	// Verify all other tracks are unchanged
	for i := 1; i < len(fixtureTracks); i++ {
		assert.Equal(t, fixtureTracks[i].location, lib.Tracks[i].Location,
			"track %d (%s) should be unchanged", i, fixtureTracks[i].name)
		assert.Equal(t, fixtureTracks[i].name, lib.Tracks[i].Name,
			"track %d name should be unchanged", i)
		assert.Equal(t, fixtureTracks[i].artist, lib.Tracks[i].Artist,
			"track %d artist should be unchanged", i)
		assert.Equal(t, fixtureTracks[i].trackID, lib.Tracks[i].TrackID,
			"track %d trackID should be unchanged", i)
	}
}

func TestUpdateITLLocations_MultipleUpdates(t *testing.T) {
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	// Update The Hobbit and Dune
	updates := []ITLLocationUpdate{
		{PersistentID: pidToHex(fixtureTracks[0].persistentID), NewLocation: "/new/hobbit.m4b"},
		{PersistentID: pidToHex(fixtureTracks[1].persistentID), NewLocation: "/new/dune.mp3"},
	}

	result, err := UpdateITLLocations(itlPath, outPath, updates)
	require.NoError(t, err)
	assert.Equal(t, 2, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	assert.Equal(t, "/new/hobbit.m4b", lib.Tracks[0].Location)
	assert.Equal(t, "/new/dune.mp3", lib.Tracks[1].Location)

	// Others unchanged
	for i := 2; i < len(fixtureTracks); i++ {
		assert.Equal(t, fixtureTracks[i].location, lib.Tracks[i].Location)
	}
}

func TestUpdateITLLocations_NonexistentPID(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: "ffffffffffffffff", NewLocation: "/new/path.mp3"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.UpdatedCount, "non-matching PID should not update anything")

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	assert.Equal(t, "/music/song.mp3", lib.Tracks[0].Location, "original location should be preserved")
}

// ---------------------------------------------------------------------------
// InsertITLTracks — path format tests
// ---------------------------------------------------------------------------

func TestInsertITLTracks_PathFormat_Windows(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/existing.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	newTrack := ITLNewTrack{
		Location:  "W:/itunes/iTunes Media/Audiobooks/Author/New Book.m4b",
		Name:      "New Book",
		Album:     "Book Album",
		Artist:    "Book Author",
		Genre:     "Audiobook",
		Kind:      "Audiobook",
		Size:      100000000,
		TotalTime: 36000000,
	}

	result, err := InsertITLTracks(itlPath, outPath, []ITLNewTrack{newTrack})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 2)
	assert.Equal(t, newTrack.Location, lib.Tracks[1].Location, "Windows path should be stored verbatim")
	assert.Equal(t, newTrack.Name, lib.Tracks[1].Name)
}

func TestInsertITLTracks_PathFormat_UnixWithSpaces(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/existing.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	loc := "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Brandon Sanderson/The Way of Kings.m4b"
	result, err := InsertITLTracks(itlPath, outPath, []ITLNewTrack{
		{Location: loc, Name: "The Way of Kings", Artist: "Brandon Sanderson", Kind: "Audiobook"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 2)
	assert.Equal(t, loc, lib.Tracks[1].Location)
}

// ---------------------------------------------------------------------------
// InsertITLPlaylist — with tracks
// ---------------------------------------------------------------------------

func TestInsertITLPlaylist_WithMultipleTracks(t *testing.T) {
	// Build a fixture with multiple tracks, then insert a playlist referencing them.
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "with_playlist.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	// Create a playlist referencing first 3 track IDs from fixture
	playlist := ITLNewPlaylist{
		Title:    "My Audiobook Picks",
		TrackIDs: []int{fixtureTracks[0].trackID, fixtureTracks[1].trackID, fixtureTracks[3].trackID},
	}

	result, err := InsertITLPlaylist(itlPath, outPath, playlist)
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)

	// Should have the existing fixture playlists plus the new one
	require.GreaterOrEqual(t, len(lib.Playlists), 1)

	// Find our new playlist
	var found *ITLPlaylist
	for i := range lib.Playlists {
		if lib.Playlists[i].Title == "My Audiobook Picks" {
			found = &lib.Playlists[i]
			break
		}
	}
	require.NotNil(t, found, "inserted playlist should be found")
	assert.Equal(t, "My Audiobook Picks", found.Title)
	require.Len(t, found.Items, 3)
	assert.Equal(t, fixtureTracks[0].trackID, found.Items[0])
	assert.Equal(t, fixtureTracks[1].trackID, found.Items[1])
	assert.Equal(t, fixtureTracks[3].trackID, found.Items[2])
}

func TestInsertITLPlaylist_EmptyPlaylist(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := InsertITLPlaylist(itlPath, outPath, ITLNewPlaylist{
		Title:    "Empty Playlist",
		TrackIDs: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Playlists, 1)
	assert.Equal(t, "Empty Playlist", lib.Playlists[0].Title)
	assert.Empty(t, lib.Playlists[0].Items)
}

// ---------------------------------------------------------------------------
// Compression round-trip: verify compressed ITL handles path updates
// ---------------------------------------------------------------------------

func TestUpdateITLLocations_Compressed(t *testing.T) {
	pid := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	originalLoc := "/music/compressed.mp3"
	newLoc := "/reorganized/compressed_new.mp3"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "compressed.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	// Build with compression enabled
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	// Parse to verify it was compressed
	origLib, err := ParseITL(itlPath)
	require.NoError(t, err)
	assert.True(t, origLib.UseCompression, "fixture should be compressed")

	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	assert.Equal(t, newLoc, lib.Tracks[0].Location)
}
