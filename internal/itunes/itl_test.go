// file: internal/itunes/itl_test.go
// version: 1.1.0
// guid: 8a3b9c4d-5e6f-7012-b3c4-d5e6f7a8b9c0

package itunes

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestITLDecryptEncryptRoundTrip(t *testing.T) {
	// Pad to 16-byte multiple
	original := []byte("Hello, ITL world!!!!!!!!!!!!!!!!") // 32 bytes
	assert.Equal(t, 0, len(original)%16)

	encrypted := itlEncrypt("12.0.0", original)
	assert.NotEqual(t, original, encrypted, "encrypted should differ from original")

	decrypted := itlDecrypt("12.0.0", encrypted)
	assert.Equal(t, original, decrypted)
}

func TestITLDecryptEncryptRoundTrip_OldVersion(t *testing.T) {
	original := make([]byte, 256)
	for i := range original {
		original[i] = byte(i % 256)
	}

	encrypted := itlEncrypt("9.0.0", original)
	decrypted := itlDecrypt("9.0.0", encrypted)
	assert.Equal(t, original, decrypted)
}

func TestITLInflateDeflateRoundTrip(t *testing.T) {
	original := []byte("This is some test data for zlib compression round trip testing.")
	compressed := itlDeflate(original)
	assert.NotEqual(t, original, compressed)
	assert.Equal(t, byte(0x78), compressed[0], "zlib data should start with 0x78")

	decompressed, wasCompressed := itlInflate(compressed)
	assert.True(t, wasCompressed)
	assert.Equal(t, original, decompressed)
}

func TestITLInflate_NotCompressed(t *testing.T) {
	data := []byte("not compressed data")
	result, wasCompressed := itlInflate(data)
	assert.False(t, wasCompressed)
	assert.Equal(t, data, result)
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		version string
		major   int
		want    bool
	}{
		{"12.0.0", 10, true},
		{"10.0.0", 10, true},
		{"9.2.1", 10, false},
		{"1.0", 1, true},
		{"", 10, false},
		{"abc", 10, false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.want, isVersionAtLeast(tt.version, tt.major))
		})
	}
}

func TestPidToHexAndBack(t *testing.T) {
	pid := [8]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	hexStr := pidToHex(pid)
	assert.Equal(t, "deadbeefcafebabe", hexStr)

	back, err := hexToPID(hexStr)
	require.NoError(t, err)
	assert.Equal(t, pid, back)
}

func TestHexToPID_Invalid(t *testing.T) {
	_, err := hexToPID("zzzz")
	assert.Error(t, err)

	_, err = hexToPID("deadbeef") // only 4 bytes
	assert.Error(t, err)
}

func TestDecodeHohmString(t *testing.T) {
	// ASCII (flag 0)
	s, err := decodeHohmString([]byte("hello"), 0)
	require.NoError(t, err)
	assert.Equal(t, "hello", s)

	// UTF-8 (flag 2)
	s, err = decodeHohmString([]byte("héllo"), 2)
	require.NoError(t, err)
	assert.Equal(t, "héllo", s)

	// UTF-16BE (flag 1)
	utf16 := make([]byte, 10)
	binary.BigEndian.PutUint16(utf16[0:], 'H')
	binary.BigEndian.PutUint16(utf16[2:], 'e')
	binary.BigEndian.PutUint16(utf16[4:], 'l')
	binary.BigEndian.PutUint16(utf16[6:], 'l')
	binary.BigEndian.PutUint16(utf16[8:], 'o')
	s, err = decodeHohmString(utf16, 1)
	require.NoError(t, err)
	assert.Equal(t, "Hello", s)

	// Windows-1252 (flag 3)
	s, err = decodeHohmString([]byte{0xe9}, 3) // é in Windows-1252
	require.NoError(t, err)
	assert.Equal(t, "é", s)
}

func TestEncodeHohmString(t *testing.T) {
	// ASCII-range -> Windows-1252 (flag 3)
	encoded, flag := encodeHohmString("hello")
	assert.Equal(t, byte(3), flag)
	assert.Equal(t, []byte("hello"), encoded)

	// Non-Latin -> UTF-16BE (flag 1)
	encoded, flag = encodeHohmString("日本語")
	assert.Equal(t, byte(1), flag)
	assert.Equal(t, 6, len(encoded)) // 3 chars * 2 bytes
}

func TestReadTag(t *testing.T) {
	data := []byte("hdfmextra")
	assert.Equal(t, "hdfm", readTag(data, 0))
	assert.Equal(t, "dfme", readTag(data, 1))
	assert.Equal(t, "", readTag(data, 100))
}

func TestMacDateToTime(t *testing.T) {
	// 0 should return zero time
	assert.True(t, macDateToTime(0).IsZero())

	// Known conversion: 2001-01-01 is ~3061152000 seconds from 1904
	expected := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	// Seconds from 1904-01-01 to 2001-01-01
	diff := expected.Sub(macEpoch)
	secs := uint32(diff.Seconds())
	result := macDateToTime(secs)
	assert.Equal(t, expected, result)
}

func TestValidateITL_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Random bytes
	path := filepath.Join(tmpDir, "bad.itl")
	require.NoError(t, os.WriteFile(path, []byte("this is not an ITL file at all"), 0644))
	err := ValidateITL(path)
	assert.Error(t, err)

	// Too small
	path2 := filepath.Join(tmpDir, "tiny.itl")
	require.NoError(t, os.WriteFile(path2, []byte("hdfm"), 0644))
	err = ValidateITL(path2)
	assert.Error(t, err)

	// Nonexistent
	err = ValidateITL(filepath.Join(tmpDir, "nope.itl"))
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Synthetic ITL round-trip test
// ---------------------------------------------------------------------------

func buildSyntheticITL(t *testing.T, version string, compress bool, pid [8]byte, location string) []byte {
	t.Helper()

	// Build inner payload: htim chunk + hohm 0x0D chunk
	// htim must be at least 156 bytes (standard header size)
	// Persistent ID is at offset 128 (8 bytes)
	htimLen := 156
	htim := make([]byte, htimLen)
	copy(htim[0:4], "htim")
	writeUint32BE(htim, 4, uint32(htimLen))
	writeUint32BE(htim, 8, uint32(htimLen)) // recordLength
	writeUint32BE(htim, 16, 42)             // trackID = 42
	copy(htim[128:136], pid[:])             // persistent ID

	// hohm 0x0D: build location string
	encodedStr, encFlag := encodeHohmString(location)
	hohmLen := 40 + len(encodedStr)
	hohm := make([]byte, hohmLen)
	copy(hohm[0:4], "hohm")
	writeUint32BE(hohm, 4, uint32(hohmLen))
	writeUint32BE(hohm, 8, uint32(hohmLen))
	writeUint32BE(hohm, 12, 0x0D) // hohmType = location
	hohm[16+11] = encFlag
	writeUint32BE(hohm, 28, uint32(len(encodedStr)))
	copy(hohm[40:], encodedStr)

	// Combine into payload
	var payload bytes.Buffer
	payload.Write(htim)
	payload.Write(hohm)

	payloadBytes := payload.Bytes()
	if compress {
		payloadBytes = itlDeflate(payloadBytes)
	}
	encrypted := itlEncrypt(version, payloadBytes)

	// Build hdfm header
	// Header: "hdfm"(4) + headerLen(4) + fileLen(4) + unknown(4) + verLen(1) + version(N) = 17 + N
	fileLen := uint32(len(encrypted)) + 17 + uint32(len(version))
	headerRemainder := []byte{}
	hdr := buildHdfmHeader(version, headerRemainder, fileLen, 0)

	var file bytes.Buffer
	file.Write(hdr)
	file.Write(encrypted)
	return file.Bytes()
}

func TestSyntheticITL_ParseAndUpdate(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	originalLoc := "/music/old/song.mp3"
	newLoc := "/music/new/song.mp3"

	for _, compress := range []bool{false, true} {
		name := "uncompressed"
		if compress {
			name = "compressed"
		}
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			itlPath := filepath.Join(tmpDir, "test.itl")
			itlData := buildSyntheticITL(t, "12.0.0", compress, pid, originalLoc)
			require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

			// Parse
			lib, err := ParseITL(itlPath)
			require.NoError(t, err)
			require.Len(t, lib.Tracks, 1)
			assert.Equal(t, originalLoc, lib.Tracks[0].Location)
			assert.Equal(t, 42, lib.Tracks[0].TrackID)
			assert.Equal(t, pid, lib.Tracks[0].PersistentID)

			// Update
			outPath := filepath.Join(tmpDir, "updated.itl")
			result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
				{PersistentID: pidToHex(pid), NewLocation: newLoc},
			})
			require.NoError(t, err)
			assert.Equal(t, 1, result.UpdatedCount)

			// Verify
			lib2, err := ParseITL(outPath)
			require.NoError(t, err)
			require.Len(t, lib2.Tracks, 1)
			assert.Equal(t, newLoc, lib2.Tracks[0].Location)
		})
	}
}

func TestSyntheticITL_Validate(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "valid.itl")
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/test/path.mp3")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	err := ValidateITL(itlPath)
	assert.NoError(t, err)
}

func TestUpdateITLLocations_NoUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.itl")
	result, err := UpdateITLLocations("", outPath, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result.UpdatedCount)
}

// ---------------------------------------------------------------------------
// Synthetic ITL with playlists
// ---------------------------------------------------------------------------

func buildSyntheticITLWithPlaylist(t *testing.T, version string, trackID int, pid [8]byte, location, playlistTitle string) []byte {
	t.Helper()

	// Build track: htim + hohm 0x0D
	htimLen := 156
	htim := make([]byte, htimLen)
	copy(htim[0:4], "htim")
	writeUint32BE(htim, 4, uint32(htimLen))
	writeUint32BE(htim, 8, uint32(htimLen))
	writeUint32BE(htim, 16, uint32(trackID))
	copy(htim[128:136], pid[:])

	encodedStr, encFlag := encodeHohmString(location)
	hohmLen := 40 + len(encodedStr)
	hohm := make([]byte, hohmLen)
	copy(hohm[0:4], "hohm")
	writeUint32BE(hohm, 4, uint32(hohmLen))
	writeUint32BE(hohm, 8, uint32(hohmLen))
	writeUint32BE(hohm, 12, 0x0D)
	hohm[16+11] = encFlag
	writeUint32BE(hohm, 28, uint32(len(encodedStr)))
	copy(hohm[40:], encodedStr)

	// Build playlist: hpim + hohm 0x64 + hptm
	hpimLen := 20 + 428
	hpim := make([]byte, hpimLen)
	copy(hpim[0:4], "hpim")
	writeUint32BE(hpim, 4, uint32(hpimLen))
	writeUint32BE(hpim, 8, uint32(hpimLen))
	writeUint32BE(hpim, 16, 1) // 1 item

	titleEncoded, titleFlag := encodeHohmString(playlistTitle)
	titleHohmLen := 40 + len(titleEncoded)
	titleHohm := make([]byte, titleHohmLen)
	copy(titleHohm[0:4], "hohm")
	writeUint32BE(titleHohm, 4, uint32(titleHohmLen))
	writeUint32BE(titleHohm, 8, uint32(titleHohmLen))
	writeUint32BE(titleHohm, 12, 0x64)
	titleHohm[16+11] = titleFlag
	writeUint32BE(titleHohm, 28, uint32(len(titleEncoded)))
	copy(titleHohm[40:], titleEncoded)

	hptmLen := 28
	hptm := make([]byte, hptmLen)
	copy(hptm[0:4], "hptm")
	writeUint32BE(hptm, 4, uint32(hptmLen))
	writeUint32BE(hptm, 24, uint32(trackID))

	var payload bytes.Buffer
	payload.Write(htim)
	payload.Write(hohm)
	payload.Write(hpim)
	payload.Write(titleHohm)
	payload.Write(hptm)

	encrypted := itlEncrypt(version, payload.Bytes())
	fileLen := uint32(len(encrypted)) + 17 + uint32(len(version))
	hdr := buildHdfmHeader(version, nil, fileLen, 0)

	var file bytes.Buffer
	file.Write(hdr)
	file.Write(encrypted)
	return file.Bytes()
}

func TestParseITL_Playlists(t *testing.T) {
	pid := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	itlData := buildSyntheticITLWithPlaylist(t, "12.0.0", 99, pid, "/music/test.mp3", "My Playlist")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	lib, err := ParseITL(itlPath)
	require.NoError(t, err)

	// Should have 1 track
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, 99, lib.Tracks[0].TrackID)
	assert.Equal(t, "/music/test.mp3", lib.Tracks[0].Location)

	// Should have 1 playlist
	require.Len(t, lib.Playlists, 1)
	assert.Equal(t, "My Playlist", lib.Playlists[0].Title)
	require.Len(t, lib.Playlists[0].Items, 1)
	assert.Equal(t, 99, lib.Playlists[0].Items[0])
}

func TestInsertITLTracks(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/existing.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := InsertITLTracks(itlPath, outPath, []ITLNewTrack{
		{
			Location:    "/music/new_song.mp3",
			Name:        "New Song",
			Album:       "New Album",
			Artist:      "New Artist",
			Genre:       "Rock",
			Kind:        "MPEG audio file",
			Size:        5000000,
			TotalTime:   240000,
			TrackNumber: 1,
			Year:        2025,
			BitRate:     320,
			SampleRate:  44100,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 2)

	// First track is the original
	assert.Equal(t, "/music/existing.mp3", lib.Tracks[0].Location)

	// Second track is the inserted one
	assert.Equal(t, "New Song", lib.Tracks[1].Name)
	assert.Equal(t, "New Album", lib.Tracks[1].Album)
	assert.Equal(t, "New Artist", lib.Tracks[1].Artist)
	assert.Equal(t, "Rock", lib.Tracks[1].Genre)
	assert.Equal(t, "MPEG audio file", lib.Tracks[1].Kind)
	assert.Equal(t, "/music/new_song.mp3", lib.Tracks[1].Location)
	assert.Equal(t, 5000000, lib.Tracks[1].Size)
	assert.Equal(t, 240000, lib.Tracks[1].TotalTime)
	assert.Equal(t, 1, lib.Tracks[1].TrackNumber)
	assert.Equal(t, 2025, lib.Tracks[1].Year)
	assert.Equal(t, 320, lib.Tracks[1].BitRate)
	assert.Equal(t, 44100, lib.Tracks[1].SampleRate)
	assert.Equal(t, 43, lib.Tracks[1].TrackID) // max was 42, new is 43
}

func TestInsertITLTracks_NoTracks(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.itl")
	result, err := InsertITLTracks("", outPath, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, result.UpdatedCount)
}

func TestRewriteITLExtensions(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.flac")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := RewriteITLExtensions(itlPath, outPath, ".flac", ".mp3")
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, "/music/song.mp3", lib.Tracks[0].Location)
}

func TestInsertITLPlaylist(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	itlData := buildSyntheticITL(t, "12.0.0", false, pid, "/music/song.mp3")

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	outPath := filepath.Join(tmpDir, "out.itl")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	result, err := InsertITLPlaylist(itlPath, outPath, ITLNewPlaylist{
		Title:    "Test Playlist",
		TrackIDs: []int{42},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	require.Len(t, lib.Playlists, 1)
	assert.Equal(t, "Test Playlist", lib.Playlists[0].Title)
	require.Len(t, lib.Playlists[0].Items, 1)
	assert.Equal(t, 42, lib.Playlists[0].Items[0])
}

func TestBuildHtimChunk(t *testing.T) {
	track := ITLNewTrack{
		Size:        1234567,
		TotalTime:   300000,
		TrackNumber: 5,
		Year:        2024,
		BitRate:     256,
		SampleRate:  48000,
		DiscNumber:  2,
	}
	htim := buildHtimChunk(100, track)

	// Parse it back
	parsed := parseHtim(htim, 0, len(htim))
	assert.Equal(t, 100, parsed.TrackID)
	assert.Equal(t, 1234567, parsed.Size)
	assert.Equal(t, 300000, parsed.TotalTime)
	assert.Equal(t, 5, parsed.TrackNumber)
	assert.Equal(t, 2024, parsed.Year)
	assert.Equal(t, 256, parsed.BitRate)
	assert.Equal(t, 48000, parsed.SampleRate)
	assert.Equal(t, 2, parsed.DiscNumber)
	// Persistent ID should be non-zero (random)
	assert.NotEqual(t, [8]byte{}, parsed.PersistentID)
}
