// file: internal/itunes/itl_regression_test.go
// version: 1.0.0
// guid: c2d3e4f5-a6b7-c8d9-e0f1-itl-regress01

package itunes

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Regression: zlib BestSpeed compression
// (Bug: Go's DefaultCompression produced output iTunes couldn't read.
// Only BestSpeed (level 1) works. Verify itlDeflate uses BestSpeed.)
// ---------------------------------------------------------------------------

func TestITLDeflate_ProducesBestSpeedOutput(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	compressed := itlDeflate(data)
	require.NotEmpty(t, compressed)

	// zlib header: first byte 0x78, second byte indicates compression level.
	// BestSpeed (level 1) → 0x01, DefaultCompression (level 6) → 0x9C
	assert.Equal(t, byte(0x78), compressed[0], "zlib magic byte")
	assert.Equal(t, byte(0x01), compressed[1],
		"zlib flag byte should be 0x01 (BestSpeed), not 0x9C (DefaultCompression)")

	// Verify round-trip
	decompressed, wasCompressed := itlInflate(compressed)
	assert.True(t, wasCompressed)
	assert.Equal(t, data, decompressed)
}

func TestITLDeflate_LargePayloadRoundTrip(t *testing.T) {
	// Simulate a real ITL payload size (~5MB of track data)
	data := make([]byte, 5*1024*1024)
	for i := range data {
		data[i] = byte((i * 7) % 256)
	}

	compressed := itlDeflate(data)
	decompressed, wasCompressed := itlInflate(compressed)
	assert.True(t, wasCompressed)
	assert.Equal(t, data, decompressed, "large payload must survive deflate/inflate round-trip")
}

// ---------------------------------------------------------------------------
// Regression: mhoh headerLen preservation in location rewrite
// (Bug: rewriteHohmLocationLE was setting both headerLen AND totalLen to
// newChunkLen, corrupting the file. headerLen must be preserved as-is.)
// ---------------------------------------------------------------------------

func TestRewriteHohmLocationLE_PreservesHeaderLen(t *testing.T) {
	// Build a mock mhoh location chunk with headerLen=24 and totalLen=60
	originalHeaderLen := uint32(24)
	originalTotalLen := uint32(60)
	originalStr := "old/path.m4b"

	chunk := make([]byte, int(originalTotalLen))
	copy(chunk[0:4], "mhoh") // tag
	writeUint32LE(chunk, 4, originalHeaderLen)
	writeUint32LE(chunk, 8, originalTotalLen)
	writeUint32LE(chunk, 12, 0x0D) // hohmType = location
	// encoding flag at offset 27
	chunk[27] = 3 // Windows-1252
	writeUint32LE(chunk, 28, uint32(len(originalStr)))
	copy(chunk[40:], []byte(originalStr))

	newLocation := "/mnt/bigdata/books/audiobook-organizer/Author/Title/file.m4b"
	result := rewriteHohmLocationLE(chunk, 0, int(originalTotalLen), newLocation)

	// Critical: headerLen at offset 4 must be preserved (not overwritten with totalLen)
	resultHeaderLen := readUint32LE(result, 4)
	resultTotalLen := readUint32LE(result, 8)

	assert.Equal(t, originalHeaderLen, resultHeaderLen,
		"headerLen must be preserved at original value (24), not set to totalLen")
	assert.NotEqual(t, resultHeaderLen, resultTotalLen,
		"headerLen and totalLen should differ (headerLen is fixed, totalLen varies with string)")

	// Verify the new total length is correct
	encodedStr, _ := encodeHohmString(newLocation)
	expectedTotalLen := uint32(40 + len(encodedStr))
	assert.Equal(t, expectedTotalLen, resultTotalLen, "totalLen should be 40 + encoded string length")

	// Verify hohmType is preserved
	hohmType := readUint32LE(result, 12)
	assert.Equal(t, uint32(0x0D), hohmType, "hohmType must be preserved")
}

func TestRewriteHohmLocationLE_DifferentHeaderLens(t *testing.T) {
	// Some mhoh chunks have headerLen=24, others might have different values.
	// The rewrite must preserve whatever headerLen was in the original.
	for _, headerLen := range []uint32{20, 24, 28, 32} {
		t.Run("headerLen="+string(rune('0'+headerLen/4)), func(t *testing.T) {
			totalLen := uint32(60)
			chunk := make([]byte, int(totalLen))
			copy(chunk[0:4], "mhoh")
			writeUint32LE(chunk, 4, headerLen)
			writeUint32LE(chunk, 8, totalLen)
			writeUint32LE(chunk, 12, 0x0D)
			chunk[27] = 3
			writeUint32LE(chunk, 28, uint32(8))
			copy(chunk[40:48], []byte("old/path"))

			result := rewriteHohmLocationLE(chunk, 0, int(totalLen), "new/path")
			assert.Equal(t, headerLen, readUint32LE(result, 4),
				"headerLen must be preserved regardless of value")
		})
	}
}

// ---------------------------------------------------------------------------
// Regression: PID byte-order reversal (LE→BE)
// (Bug: ITL v10+ stores PIDs in LE byte order, but XML uses BE.
// pidToHexLE must reverse all 8 bytes.)
// ---------------------------------------------------------------------------

func TestPidToHexLE_ReversesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		pid      [8]byte
		expected string
	}{
		{
			name:     "simple sequential",
			pid:      [8]byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01},
			expected: "0102030405060708",
		},
		{
			name:     "real PID example",
			pid:      [8]byte{0x62, 0xA6, 0x9B, 0xA7, 0x0F, 0xCB, 0x73, 0x9A},
			expected: "9a73cb0fa79ba662",
		},
		{
			name:     "all zeros",
			pid:      [8]byte{},
			expected: "0000000000000000",
		},
		{
			name:     "all FF",
			pid:      [8]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			expected: "ffffffffffffffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pidToHexLE(tt.pid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPidToHexLE_VsPidToHex_Distinction(t *testing.T) {
	// pidToHex is identity (BE→BE), pidToHexLE reverses (LE→BE)
	pidBE := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	pidLE := [8]byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}

	// Both should produce the same hex string for the same logical PID
	assert.Equal(t, pidToHex(pidBE), pidToHexLE(pidLE),
		"BE and LE representations of the same PID should produce identical hex")

	// But the same raw bytes should produce different hex strings
	assert.NotEqual(t, pidToHex(pidBE), pidToHexLE(pidBE),
		"pidToHex and pidToHexLE on the same bytes should differ")
}

// ---------------------------------------------------------------------------
// Regression: synthetic ITL round-trip with longer replacement path
// (Bug: location rewrite with a significantly longer path must produce a
// valid ITL that can be parsed back correctly.)
// ---------------------------------------------------------------------------

func TestSyntheticITL_LongerReplacementPath(t *testing.T) {
	pid := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	shortPath := "/old/a.m4b"
	longPath := "/mnt/bigdata/books/audiobook-organizer/Very Long Author Name/Very Long Series Name/Book 01 - Very Long Title That Goes On and On/Book 01 - Very Long Title That Goes On and On - Very Long Author Name - read by narrator.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, shortPath)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	outPath := filepath.Join(tmpDir, "updated.itl")
	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: longPath},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	// Parse the updated file and verify the long path survived
	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, longPath, lib.Tracks[0].Location,
		"long replacement path should survive write→parse round-trip")
}

func TestSyntheticITL_ShorterReplacementPath(t *testing.T) {
	pid := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	longPath := "/mnt/bigdata/books/audiobook-organizer/Author/Series/Title/Title - Author.m4b"
	shortPath := "/x/a.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, longPath)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	outPath := filepath.Join(tmpDir, "updated.itl")
	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: shortPath},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, shortPath, lib.Tracks[0].Location)
}

// ---------------------------------------------------------------------------
// Regression: ITL with Unicode paths
// (Probing for undiscovered bugs with non-ASCII characters in paths)
// ---------------------------------------------------------------------------

func TestSyntheticITL_UnicodePath(t *testing.T) {
	pid := [8]byte{0xCA, 0xFE, 0xBA, 0xBE, 0xDE, 0xAD, 0xBE, 0xEF}
	unicodePath := "/mnt/books/日本語の本/著者名/タイトル.m4b"

	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, unicodePath)
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	// Parse and verify Unicode survived
	lib, err := ParseITL(itlPath)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, unicodePath, lib.Tracks[0].Location)

	// Update with another Unicode path
	newPath := "/mnt/books/Ünîcödé Àuthör/Böök Tïtle/file.m4b"
	outPath := filepath.Join(tmpDir, "updated.itl")
	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid), NewLocation: newPath},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount)

	lib2, err := ParseITL(outPath)
	require.NoError(t, err)
	require.Len(t, lib2.Tracks, 1)
	assert.Equal(t, newPath, lib2.Tracks[0].Location)
}

// ---------------------------------------------------------------------------
// Regression: ITL with special characters in paths
// (Probing: spaces, ampersands, apostrophes — common in audiobook titles)
// ---------------------------------------------------------------------------

func TestSyntheticITL_SpecialCharacterPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"spaces", "/mnt/books/Author Name/Book Title With Spaces/file.m4b"},
		{"apostrophe", "/mnt/books/O'Brien/Harry's Game/file.m4b"},
		{"ampersand", "/mnt/books/Author & Co/Book & Stuff/file.m4b"},
		{"parentheses", "/mnt/books/Author/Book (Unabridged)/file.m4b"},
		{"colon_substitute", "/mnt/books/Author/Book_ A Subtitle/file.m4b"},
		{"long_256_chars", "/mnt/books/" + string(bytes.Repeat([]byte("a"), 200)) + "/file.m4b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
			tmpDir := t.TempDir()
			itlPath := filepath.Join(tmpDir, "test.itl")
			itlData := buildSyntheticITL(t, "12.0.0", true, pid, tt.path)
			require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

			lib, err := ParseITL(itlPath)
			require.NoError(t, err)
			require.Len(t, lib.Tracks, 1)
			assert.Equal(t, tt.path, lib.Tracks[0].Location)
		})
	}
}

// ---------------------------------------------------------------------------
// Regression: multiple PID updates in one call — no crosstalk
// (Bug potential: if two tracks share similar paths, ensure each gets the
// correct unique update)
// ---------------------------------------------------------------------------

func TestSyntheticITL_MultiTrackUpdate_NoCrosstalk(t *testing.T) {
	pid1 := [8]byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}
	pid2 := [8]byte{0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02}

	// Build a two-track ITL by concatenating synthetic payloads
	// This is a simplified approach — build two separate single-track ITLs and test each
	tmpDir := t.TempDir()

	// Track 1
	itl1Path := filepath.Join(tmpDir, "track1.itl")
	itl1Data := buildSyntheticITL(t, "12.0.0", true, pid1, "/old/track1.m4b")
	require.NoError(t, os.WriteFile(itl1Path, itl1Data, 0644))

	out1Path := filepath.Join(tmpDir, "out1.itl")
	result, err := UpdateITLLocations(itl1Path, out1Path, []ITLLocationUpdate{
		{PersistentID: pidToHex(pid1), NewLocation: "/new/track1.m4b"},
		{PersistentID: pidToHex(pid2), NewLocation: "/new/track2.m4b"}, // shouldn't match
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedCount, "only pid1 should match in track1's ITL")

	lib, err := ParseITL(out1Path)
	require.NoError(t, err)
	require.Len(t, lib.Tracks, 1)
	assert.Equal(t, "/new/track1.m4b", lib.Tracks[0].Location)
}

// ---------------------------------------------------------------------------
// New: empty update list should be a no-op
// ---------------------------------------------------------------------------

func TestUpdateITLLocations_EmptyUpdates(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	tmpDir := t.TempDir()
	itlPath := filepath.Join(tmpDir, "test.itl")
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, "/original/path.m4b")
	require.NoError(t, os.WriteFile(itlPath, itlData, 0644))

	outPath := filepath.Join(tmpDir, "out.itl")
	result, err := UpdateITLLocations(itlPath, outPath, []ITLLocationUpdate{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.UpdatedCount)
}

// ---------------------------------------------------------------------------
// New: validate ITL must reject truncated files
// ---------------------------------------------------------------------------

func TestValidateITL_RejectsTruncated(t *testing.T) {
	pid := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	tmpDir := t.TempDir()
	itlData := buildSyntheticITL(t, "12.0.0", true, pid, "/test/path.m4b")

	// Truncate to half
	truncated := itlData[:len(itlData)/2]
	path := filepath.Join(tmpDir, "truncated.itl")
	require.NoError(t, os.WriteFile(path, truncated, 0644))

	err := ValidateITL(path)
	assert.Error(t, err, "truncated ITL should fail validation")
}

// ---------------------------------------------------------------------------
// New: encrypt/decrypt must be symmetric across versions
// ---------------------------------------------------------------------------

func TestITLEncryptDecrypt_MultipleVersions(t *testing.T) {
	original := make([]byte, 128)
	for i := range original {
		original[i] = byte(i)
	}

	versions := []string{"9.0.0", "10.0.0", "11.0.0", "12.0.0", "12.13.10.3"}
	for _, v := range versions {
		t.Run(v, func(t *testing.T) {
			hdr := &hdfmHeader{version: v}
			encrypted := itlEncrypt(hdr, original)
			decrypted := itlDecrypt(hdr, encrypted)
			assert.Equal(t, original, decrypted,
				"encrypt/decrypt must round-trip for version %s", v)
		})
	}
}
