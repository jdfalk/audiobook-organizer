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

// NOTE (TASK-005 / K12): buildSyntheticITL produces BIG-ENDIAN ("hohm"/"htim")
// fixtures. BE writeback is now REFUSED (ErrBEWritebackUnsupported) — the BE
// writer shared CRIT-1's foreign +27 flag invention with no corpus to validate
// against, and production is LE. These tests therefore assert the refusal rather
// than a successful round-trip. LE path-format coverage lives in the
// itl_le_metadata_update / itl_convert / mhoh_string suites.

func TestUpdateITLLocations_PathFormat_WindowsStyle(t *testing.T) {
	pid := [8]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	originalLoc := "W:/itunes/iTunes Media/Audiobooks/Author/book.m4b"
	newLoc := "W:/itunes/iTunes Media/Audiobooks/New Author/New Book/chapter01.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")

	itlData := buildSyntheticITL(t, "12.0.0", false, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
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

	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
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

	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
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

	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
}

// ---------------------------------------------------------------------------
// UpdateITLLocations — preserves other tracks
// ---------------------------------------------------------------------------

func TestUpdateITLLocations_PreservesOtherTracks(t *testing.T) {
	// BE fixture (buildFixtureITL) — BE writeback refused (K12).
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	hobbitPID := pidToHex(fixtureTracks[0].persistentID)
	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: hobbitPID, NewLocation: "/reorganized/The Hobbit.m4b"},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
}

func TestUpdateITLLocations_MultipleUpdates(t *testing.T) {
	// BE fixture (buildFixtureITL) — BE writeback refused (K12).
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	updates := []ITLLocationUpdate{
		{PersistentID: pidToHex(fixtureTracks[0].persistentID), NewLocation: "/new/hobbit.m4b"},
		{PersistentID: pidToHex(fixtureTracks[1].persistentID), NewLocation: "/new/dune.mp3"},
	}
	_, err := UpdateITLLocations(itlPath, outPath, updates)
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
}

func TestUpdateITLLocations_NonexistentPID(t *testing.T) {
	// BE fixture — BE writeback refused (K12) before any PID matching.
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "updated.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	_, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: "ffffffffffffffff", NewLocation: "/new/path.mp3"},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
}

// ---------------------------------------------------------------------------
// InsertITLTracks — path format tests
// ---------------------------------------------------------------------------

// TestInsertITLTracks_PathFormat_Windows: buildSyntheticITL emits a big-endian
// payload, which the SafeWriteITL chokepoint now refuses (TASK-004, SPEC §3
// step 1 / K12). The path-format conversion logic is covered by the LE tests in
// itl_le_*_test.go; here we assert the BE refusal contract.
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

	_, err := InsertITLTracks(itlPath, outPath, []ITLNewTrack{newTrack})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported)
	_, statErr := os.Stat(outPath)
	require.Error(t, statErr, "refused BE write must not create output")
}

// TestInsertITLTracks_PathFormat_UnixWithSpaces: BE fixture → refused (see above).
func TestInsertITLTracks_PathFormat_UnixWithSpaces(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/existing.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	loc := "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Brandon Sanderson/The Way of Kings.m4b"
	_, err := InsertITLTracks(itlPath, outPath, []ITLNewTrack{
		{Location: loc, Name: "The Way of Kings", Artist: "Brandon Sanderson", Kind: "Audiobook"},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported)
	_, statErr := os.Stat(outPath)
	require.Error(t, statErr, "refused BE write must not create output")
}

// ---------------------------------------------------------------------------
// InsertITLPlaylist — with tracks
// ---------------------------------------------------------------------------

// TestInsertITLPlaylist_WithMultipleTracks: buildFixtureITL emits a big-endian
// payload, refused by the SafeWriteITL chokepoint (TASK-004, SPEC §3 step 1).
func TestInsertITLPlaylist_WithMultipleTracks(t *testing.T) {
	fixtureData := buildFixtureITL()
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "fixture.itl")
	outPath := filepath.Join(tmpDir, "with_playlist.itl")
	require.NoError(t, os.WriteFile(itlPath, fixtureData, 0644))

	playlist := ITLNewPlaylist{
		Title:    "My Audiobook Picks",
		TrackIDs: []int{fixtureTracks[0].trackID, fixtureTracks[1].trackID, fixtureTracks[3].trackID},
	}

	_, err := InsertITLPlaylist(itlPath, outPath, playlist)
	require.ErrorIs(t, err, ErrBEWritebackUnsupported)
	_, statErr := os.Stat(outPath)
	require.Error(t, statErr, "refused BE write must not create output")
}

// TestInsertITLPlaylist_EmptyPlaylist: BE fixture → refused (see above).
func TestInsertITLPlaylist_EmptyPlaylist(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	_, err := InsertITLPlaylist(itlPath, outPath, ITLNewPlaylist{
		Title:    "Empty Playlist",
		TrackIDs: nil,
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported)
	_, statErr := os.Stat(outPath)
	require.Error(t, statErr, "refused BE write must not create output")
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

	// Build with compression enabled (BE fixture).
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, originalLoc)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	// Parse to verify it was compressed
	origLib, err := ParseITL(itlPath)
	require.NoError(t, err)
	assert.True(t, origLib.UseCompression, "fixture should be compressed")

	// BE writeback refused (K12) — even through the compressed read path.
	_, err = UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newLoc},
	})
	require.ErrorIs(t, err, ErrBEWritebackUnsupported, "BE writeback must be refused (K12)")
}
