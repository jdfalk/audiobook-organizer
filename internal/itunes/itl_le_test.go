// file: internal/itunes/itl_le_test.go
// version: 1.0.0
// guid: c5f9e038-7d4a-4b92-af13-g8c4d9e5f67b

package itunes

import (
	"encoding/hex"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers — build synthetic LE binary data
// ---------------------------------------------------------------------------

// putUint32LE writes a uint32 in little-endian to buf at offset.
func putUint32LE(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
	buf[offset+2] = byte(val >> 16)
	buf[offset+3] = byte(val >> 24)
}

// putUint16LE writes a uint16 in little-endian to buf at offset.
func putUint16LE(buf []byte, offset int, val uint16) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
}

// buildMhohLE builds a little-endian mhoh chunk for a given hohmType and ASCII string.
func buildMhohLE(hohmType uint32, value string) []byte {
	encoded := []byte(value)
	chunkLen := 40 + len(encoded)
	buf := make([]byte, chunkLen)
	copy(buf[0:4], "mhoh")
	putUint32LE(buf, 4, uint32(chunkLen))
	putUint32LE(buf, 8, uint32(chunkLen))
	putUint32LE(buf, 12, hohmType)
	buf[16+11] = 0 // ASCII encoding
	putUint32LE(buf, 28, uint32(len(encoded)))
	copy(buf[40:], encoded)
	return buf
}

// buildMithLE builds a little-endian mith chunk with the given fields.
func buildMithLE(trackID int, pid [8]byte, size, totalTime int) []byte {
	mithLen := 156
	buf := make([]byte, mithLen)
	copy(buf[0:4], "mith")
	putUint32LE(buf, 4, uint32(mithLen))
	putUint32LE(buf, 16, uint32(trackID))
	putUint32LE(buf, 36, uint32(size))
	putUint32LE(buf, 40, uint32(totalTime))
	putUint16LE(buf, 44, 3)  // TrackNumber
	putUint16LE(buf, 48, 12) // TrackCount
	putUint16LE(buf, 54, 2024) // Year
	putUint16LE(buf, 58, 320)  // BitRate
	putUint16LE(buf, 60, 44100) // SampleRate
	putUint32LE(buf, 76, 5)    // PlayCount
	putUint16LE(buf, 104, 1)   // DiscNumber
	putUint16LE(buf, 106, 2)   // DiscCount
	buf[108] = 80               // Rating
	copy(buf[128:136], pid[:])
	return buf
}

// buildMsdhLE builds an msdh container wrapping the given content.
func buildMsdhLE(blockType uint32, content []byte) []byte {
	headerLen := 16
	totalLen := headerLen + len(content)
	buf := make([]byte, totalLen)
	copy(buf[0:4], "msdh")
	putUint32LE(buf, 4, uint32(headerLen))
	putUint32LE(buf, 8, uint32(totalLen))
	putUint32LE(buf, 12, blockType)
	copy(buf[headerLen:], content)
	return buf
}

// buildMiphLE builds a little-endian miph chunk with a persistent ID.
func buildMiphLE(pid [8]byte) []byte {
	// miph: same layout as hpim — PID at remaining[420:428] = offset 20+420=440
	miphLen := 20 + 428
	buf := make([]byte, miphLen)
	copy(buf[0:4], "miph")
	putUint32LE(buf, 4, uint32(miphLen))
	putUint32LE(buf, 8, uint32(miphLen))
	putUint32LE(buf, 16, 1) // item count
	copy(buf[440:448], pid[:])
	return buf
}

// buildMtphLE builds a little-endian mtph chunk referencing a track ID.
func buildMtphLE(trackID int) []byte {
	mtphLen := 28
	buf := make([]byte, mtphLen)
	copy(buf[0:4], "mtph")
	putUint32LE(buf, 4, uint32(mtphLen))
	putUint32LE(buf, 24, uint32(trackID))
	return buf
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestWalkChunksLE_ParsesTracks(t *testing.T) {
	// LE byte order: reversed from XML hex "aabbccddeeff1122"
	pid := [8]byte{0x22, 0x11, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}
	location := "/music/audiobooks/test.m4b"

	// Build track content: mith + mhoh(name) + mhoh(location)
	mith := buildMithLE(42, pid, 1024000, 360000)
	nameMhoh := buildMhohLE(0x02, "Test Audiobook")
	albumMhoh := buildMhohLE(0x03, "Test Album")
	artistMhoh := buildMhohLE(0x04, "Test Author")
	genreMhoh := buildMhohLE(0x05, "Audiobooks")
	locMhoh := buildMhohLE(0x0D, location)

	var content []byte
	content = append(content, mith...)
	content = append(content, nameMhoh...)
	content = append(content, albumMhoh...)
	content = append(content, artistMhoh...)
	content = append(content, genreMhoh...)
	content = append(content, locMhoh...)

	data := buildMsdhLE(0x01, content)

	lib := &ITLLibrary{}
	walkChunksLEImpl(data, lib)

	if len(lib.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(lib.Tracks))
	}

	tr := lib.Tracks[0]
	if tr.TrackID != 42 {
		t.Errorf("TrackID: expected 42, got %d", tr.TrackID)
	}
	if tr.Name != "Test Audiobook" {
		t.Errorf("Name: expected 'Test Audiobook', got %q", tr.Name)
	}
	if tr.Album != "Test Album" {
		t.Errorf("Album: expected 'Test Album', got %q", tr.Album)
	}
	if tr.Artist != "Test Author" {
		t.Errorf("Artist: expected 'Test Author', got %q", tr.Artist)
	}
	if tr.Genre != "Audiobooks" {
		t.Errorf("Genre: expected 'Audiobooks', got %q", tr.Genre)
	}
	if tr.Location != location {
		t.Errorf("Location: expected %q, got %q", location, tr.Location)
	}
	if tr.Size != 1024000 {
		t.Errorf("Size: expected 1024000, got %d", tr.Size)
	}
	if tr.TotalTime != 360000 {
		t.Errorf("TotalTime: expected 360000, got %d", tr.TotalTime)
	}
	if tr.TrackNumber != 3 {
		t.Errorf("TrackNumber: expected 3, got %d", tr.TrackNumber)
	}
	if tr.TrackCount != 12 {
		t.Errorf("TrackCount: expected 12, got %d", tr.TrackCount)
	}
	if tr.Year != 2024 {
		t.Errorf("Year: expected 2024, got %d", tr.Year)
	}
	if tr.BitRate != 320 {
		t.Errorf("BitRate: expected 320, got %d", tr.BitRate)
	}
	if tr.SampleRate != 44100 {
		t.Errorf("SampleRate: expected 44100, got %d", tr.SampleRate)
	}
	if tr.PlayCount != 5 {
		t.Errorf("PlayCount: expected 5, got %d", tr.PlayCount)
	}
	if tr.DiscNumber != 1 {
		t.Errorf("DiscNumber: expected 1, got %d", tr.DiscNumber)
	}
	if tr.DiscCount != 2 {
		t.Errorf("DiscCount: expected 2, got %d", tr.DiscCount)
	}
	if tr.Rating != 80 {
		t.Errorf("Rating: expected 80, got %d", tr.Rating)
	}
	pidHex := hex.EncodeToString(tr.PersistentID[:])
	expectedPID := "aabbccddeeff1122"
	if pidHex != expectedPID {
		t.Errorf("PersistentID: expected %s, got %s", expectedPID, pidHex)
	}
}

func TestRewriteChunksLE_UpdatesLocation(t *testing.T) {
	// LE byte order: reversed from XML hex "aabbccddeeff1122"
	pid := [8]byte{0x22, 0x11, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}
	oldLocation := "/old/path/book.m4b"
	newLocation := "/new/path/book.m4b"

	// Build track content
	mith := buildMithLE(42, pid, 1024000, 360000)
	locMhoh := buildMhohLE(0x0D, oldLocation)

	var content []byte
	content = append(content, mith...)
	content = append(content, locMhoh...)

	data := buildMsdhLE(0x01, content)

	// updateMap uses XML-format (BE) hex — pidToHexLE reverses the LE bytes to match
	updateMap := map[string]string{
		"aabbccddeeff1122": newLocation,
	}

	rewritten, count := rewriteChunksLEImpl(data, updateMap)
	if count != 1 {
		t.Fatalf("expected 1 update, got %d", count)
	}

	// Parse the rewritten data to verify
	lib := &ITLLibrary{}
	walkChunksLEImpl(rewritten, lib)

	if len(lib.Tracks) != 1 {
		t.Fatalf("expected 1 track after rewrite, got %d", len(lib.Tracks))
	}
	if lib.Tracks[0].Location != newLocation {
		t.Errorf("Location after rewrite: expected %q, got %q", newLocation, lib.Tracks[0].Location)
	}
	if lib.Tracks[0].TrackID != 42 {
		t.Errorf("TrackID after rewrite: expected 42, got %d", lib.Tracks[0].TrackID)
	}
}

func TestWalkChunksLE_ParsesPlaylists(t *testing.T) {
	// LE order: reverses to "1122334455667788"
	playlistPID := [8]byte{0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11}

	// Build playlist content: miph + mhoh(title) + mtph + mtph
	miph := buildMiphLE(playlistPID)
	titleMhoh := buildMhohLE(0x64, "My Audiobooks")
	mtph1 := buildMtphLE(42)
	mtph2 := buildMtphLE(99)

	var content []byte
	content = append(content, miph...)
	content = append(content, titleMhoh...)
	content = append(content, mtph1...)
	content = append(content, mtph2...)

	data := buildMsdhLE(0x02, content)

	lib := &ITLLibrary{}
	walkChunksLEImpl(data, lib)

	if len(lib.Playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(lib.Playlists))
	}

	pl := lib.Playlists[0]
	if pl.Title != "My Audiobooks" {
		t.Errorf("Playlist title: expected 'My Audiobooks', got %q", pl.Title)
	}
	if len(pl.Items) != 2 {
		t.Fatalf("expected 2 playlist items, got %d", len(pl.Items))
	}
	if pl.Items[0] != 42 {
		t.Errorf("Playlist item 0: expected 42, got %d", pl.Items[0])
	}
	if pl.Items[1] != 99 {
		t.Errorf("Playlist item 1: expected 99, got %d", pl.Items[1])
	}
	pidHex := hex.EncodeToString(pl.PersistentID[:])
	if pidHex != "1122334455667788" {
		t.Errorf("Playlist PID: expected 1122334455667788, got %s", pidHex)
	}
}

func TestWalkChunksLE_MultipleMsdhContainers(t *testing.T) {
	trackPID := [8]byte{0x22, 0x11, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}
	playlistPID := [8]byte{0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11}

	// Build track msdh
	mith := buildMithLE(42, trackPID, 1024000, 360000)
	locMhoh := buildMhohLE(0x0D, "/path/to/book.m4b")
	var trackContent []byte
	trackContent = append(trackContent, mith...)
	trackContent = append(trackContent, locMhoh...)
	trackMsdh := buildMsdhLE(0x01, trackContent)

	// Build playlist msdh
	miph := buildMiphLE(playlistPID)
	titleMhoh := buildMhohLE(0x64, "Favorites")
	mtph := buildMtphLE(42)
	var playlistContent []byte
	playlistContent = append(playlistContent, miph...)
	playlistContent = append(playlistContent, titleMhoh...)
	playlistContent = append(playlistContent, mtph...)
	playlistMsdh := buildMsdhLE(0x02, playlistContent)

	// Concatenate both containers
	var data []byte
	data = append(data, trackMsdh...)
	data = append(data, playlistMsdh...)

	lib := &ITLLibrary{}
	walkChunksLEImpl(data, lib)

	if len(lib.Tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(lib.Tracks))
	}
	if len(lib.Playlists) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(lib.Playlists))
	}
	if lib.Tracks[0].Location != "/path/to/book.m4b" {
		t.Errorf("Track location: expected '/path/to/book.m4b', got %q", lib.Tracks[0].Location)
	}
	if lib.Playlists[0].Title != "Favorites" {
		t.Errorf("Playlist title: expected 'Favorites', got %q", lib.Playlists[0].Title)
	}
}

func TestRewriteChunksLE_NoMatchReturnsUnchanged(t *testing.T) {
	pid := [8]byte{0x22, 0x11, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}
	mith := buildMithLE(42, pid, 1024000, 360000)
	locMhoh := buildMhohLE(0x0D, "/original/path.m4b")

	var content []byte
	content = append(content, mith...)
	content = append(content, locMhoh...)
	data := buildMsdhLE(0x01, content)

	// Update map with a different PID
	updateMap := map[string]string{
		"0000000000000000": "/new/path.m4b",
	}

	rewritten, count := rewriteChunksLEImpl(data, updateMap)
	if count != 0 {
		t.Fatalf("expected 0 updates, got %d", count)
	}

	// Parse and verify location unchanged
	lib := &ITLLibrary{}
	walkChunksLEImpl(rewritten, lib)
	if len(lib.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(lib.Tracks))
	}
	if lib.Tracks[0].Location != "/original/path.m4b" {
		t.Errorf("Location should be unchanged, got %q", lib.Tracks[0].Location)
	}
}

func TestRewriteChunksLE_LocalURLUpdate(t *testing.T) {
	pid := [8]byte{0x22, 0x11, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}
	mith := buildMithLE(42, pid, 1024000, 360000)
	localURLMhoh := buildMhohLE(0x0B, "file://localhost/old/path.m4b")

	var content []byte
	content = append(content, mith...)
	content = append(content, localURLMhoh...)
	data := buildMsdhLE(0x01, content)

	updateMap := map[string]string{
		"aabbccddeeff1122": "/new/path.m4b",
	}

	rewritten, count := rewriteChunksLEImpl(data, updateMap)
	if count != 1 {
		t.Fatalf("expected 1 update, got %d", count)
	}

	// Parse and verify the LocalURL was updated with file:// prefix
	lib := &ITLLibrary{}
	walkChunksLEImpl(rewritten, lib)
	if len(lib.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(lib.Tracks))
	}
	if !strings.HasPrefix(lib.Tracks[0].LocalURL, "file://localhost/") {
		t.Errorf("LocalURL should start with file://localhost/, got %q", lib.Tracks[0].LocalURL)
	}
}
