// file: internal/itunes/path_mapping_test.go
// version: 1.0.0
// guid: b3c4d5e6-f7a8-9012-bcde-f34567890abc

package itunes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RemapPath tests (method on ImportOptions)
// ---------------------------------------------------------------------------

func TestRemapPath_WindowsToLinux(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
		},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic windows path",
			input:    "W:/itunes/iTunes Media/Audiobooks/Author/book.m4b",
			expected: "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/book.m4b",
		},
		{
			name:     "backslash windows path",
			input:    "W:\\itunes\\iTunes Media\\Audiobooks\\Author\\book.m4b",
			expected: "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/book.m4b",
		},
		{
			name:     "path with spaces",
			input:    "W:/itunes/iTunes Media/Audiobooks/Brandon Sanderson/01 Prologue.mp3",
			expected: "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Brandon Sanderson/01 Prologue.mp3",
		},
		{
			name:     "path with special characters",
			input:    "W:/itunes/iTunes Media/Audiobooks/O'Brien/book (1).m4b",
			expected: "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/O'Brien/book (1).m4b",
		},
		{
			name:     "no matching mapping",
			input:    "C:/other/path/book.m4b",
			expected: "C:/other/path/book.m4b",
		},
		{
			name:     "exact prefix match, no trailing path",
			input:    "W:/itunes/iTunes Media",
			expected: "/mnt/bigdata/books/itunes/iTunes Media",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := opts.RemapPath(tt.input)
			assert.Equal(t, tt.expected, result, "path remapping failed")
		})
	}
}

func TestRemapPath_EmptyMappings(t *testing.T) {
	opts := &ImportOptions{PathMappings: nil}
	input := "W:/itunes/iTunes Media/book.m4b"
	assert.Equal(t, input, opts.RemapPath(input), "empty mappings should return path unchanged")
}

func TestRemapPath_FirstMatchWins(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "W:/itunes/iTunes Media/Audiobooks", To: "/audiobooks"},
			{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
		},
	}

	got := opts.RemapPath("W:/itunes/iTunes Media/Audiobooks/Author/book.m4b")
	assert.Equal(t, "/audiobooks/Author/book.m4b", got, "first matching rule should win")
}

func TestRemapPath_URLEncodedMapping(t *testing.T) {
	// If the mapping's From includes URL-encoded segments, it must match literally.
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "file://localhost/W:/itunes/iTunes%20Media", To: "file://localhost/mnt/bigdata/books/itunes/iTunes Media"},
		},
	}

	input := "file://localhost/W:/itunes/iTunes%20Media/Audiobooks/The%20Hobbit.m4b"
	got := opts.RemapPath(input)
	assert.Equal(t, "file://localhost/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/The%20Hobbit.m4b", got)
}

func TestRemapPath_EmptyFromSkipped(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "", To: "/should/not/match"},
		},
	}
	input := "W:/itunes/file.m4b"
	assert.Equal(t, input, opts.RemapPath(input), "empty From should never match")
}

func TestRemapPath_EmptyToSkipped(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "W:/itunes", To: ""},
		},
	}
	input := "W:/itunes/file.m4b"
	assert.Equal(t, input, opts.RemapPath(input), "empty To should never match")
}

// ---------------------------------------------------------------------------
// ReverseRemapPath tests (standalone function)
// ---------------------------------------------------------------------------

func TestReverseRemapPath_LinuxToWindows(t *testing.T) {
	mappings := []PathMapping{
		{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic linux path",
			input:    "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/book.m4b",
			expected: "W:/itunes/iTunes Media/Audiobooks/Author/book.m4b",
		},
		{
			name:     "path with spaces",
			input:    "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author Name/book title.m4b",
			expected: "W:/itunes/iTunes Media/Audiobooks/Author Name/book title.m4b",
		},
		{
			name:     "no matching mapping",
			input:    "/home/user/other/book.m4b",
			expected: "/home/user/other/book.m4b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReverseRemapPath(tt.input, mappings)
			assert.Equal(t, tt.expected, result, "reverse path remapping failed")
		})
	}
}

func TestReverseRemapPath_EmptyMappings(t *testing.T) {
	input := "/mnt/bigdata/books/itunes/iTunes Media/book.m4b"
	assert.Equal(t, input, ReverseRemapPath(input, nil))
}

func TestReverseRemapPath_FirstMatchWins(t *testing.T) {
	mappings := []PathMapping{
		{From: "W:/narrow", To: "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks"},
		{From: "W:/broad", To: "/mnt/bigdata/books/itunes/iTunes Media"},
	}

	got := ReverseRemapPath("/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/file.m4b", mappings)
	assert.Equal(t, "W:/narrow/file.m4b", got, "first matching To prefix should win")
}

// ---------------------------------------------------------------------------
// Round-trip tests
// ---------------------------------------------------------------------------

func TestPathRoundTrip(t *testing.T) {
	mappings := []PathMapping{
		{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
	}
	opts := &ImportOptions{PathMappings: mappings}

	localPaths := []string{
		"/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author/book.m4b",
		"/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Multi Word Author/Long Book Title.mp3",
		"/mnt/bigdata/books/itunes/iTunes Media/Music/Artist/song.mp3",
	}

	for _, localPath := range localPaths {
		t.Run(localPath, func(t *testing.T) {
			windowsPath := ReverseRemapPath(localPath, mappings)
			require.NotEqual(t, localPath, windowsPath, "reverse remap should change the path")

			roundTripped := opts.RemapPath(windowsPath)
			assert.Equal(t, localPath, roundTripped, "round trip should return original path")
		})
	}
}

func TestPathRoundTrip_WindowsToLocal(t *testing.T) {
	mappings := []PathMapping{
		{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
	}
	opts := &ImportOptions{PathMappings: mappings}

	winPaths := []string{
		"W:/itunes/iTunes Media/Audiobooks/Author/book.m4b",
		"W:\\itunes\\iTunes Media\\Audiobooks\\David Weber\\01 Off Armageddon Reef 001-153.m4b",
	}

	for _, winPath := range winPaths {
		t.Run(winPath, func(t *testing.T) {
			localPath := opts.RemapPath(winPath)
			require.NotEqual(t, winPath, localPath, "remap should produce a different path")

			// Reverse and re-remap should be stable
			restored := ReverseRemapPath(localPath, mappings)
			restoredLocal := opts.RemapPath(restored)
			assert.Equal(t, localPath, restoredLocal, "round-trip should be stable")
		})
	}
}

// ---------------------------------------------------------------------------
// DecodeLocation / EncodeLocation round-trip
// ---------------------------------------------------------------------------

func TestDecodeEncodeLocation_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"simple path", "/Users/test/Music/Book.m4b"},
		{"path with spaces", "/Users/test/Music/The Hobbit.m4b"},
		{"deep path", "/mnt/bigdata/books/itunes/iTunes Media/Audiobooks/Author Name/Book Title/01 Chapter.mp3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeLocation(tt.path)
			decoded, err := DecodeLocation(encoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.path, decoded, "DecodeLocation(EncodeLocation(x)) should return x")
		})
	}
}

func TestDecodeLocation_WindowsFileURL(t *testing.T) {
	input := "file://localhost/W:/itunes/iTunes%20Media/Audiobooks/The%20Hobbit.m4b"
	decoded, err := DecodeLocation(input)
	assert.NoError(t, err)
	assert.Contains(t, decoded, "W:/itunes/iTunes Media/Audiobooks/The Hobbit.m4b")
}

// ---------------------------------------------------------------------------
// extractPathPrefixes tests
// ---------------------------------------------------------------------------

func TestExtractPathPrefixes(t *testing.T) {
	locations := []string{
		"file://localhost/W:/itunes/iTunes%20Media/Audiobooks/Author/book.m4b",
		"file://localhost/W:/itunes/iTunes%20Media/Music/Artist/song.mp3",
		"file://localhost/C:/other/Music/file.mp3",
		"https://podcast.example.com/feed.xml", // should be skipped
	}

	prefixes := extractPathPrefixes(locations)

	assert.Contains(t, prefixes, "file://localhost/W:/itunes/iTunes%20Media")
	assert.Contains(t, prefixes, "file://localhost/C:/other/Music")
	assert.Len(t, prefixes, 2, "should have 2 unique prefixes (HTTP skipped, W: de-duped)")
}

func TestExtractPathPrefixes_Empty(t *testing.T) {
	assert.Empty(t, extractPathPrefixes(nil))
	assert.Empty(t, extractPathPrefixes([]string{}))
}

func TestMultipleMappings(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
			{From: "D:/audiobooks", To: "/mnt/storage/audiobooks"},
		},
	}

	result1 := opts.RemapPath("W:/itunes/iTunes Media/book.m4b")
	assert.Equal(t, "/mnt/bigdata/books/itunes/iTunes Media/book.m4b", result1)

	result2 := opts.RemapPath("D:/audiobooks/author/book.mp3")
	assert.Equal(t, "/mnt/storage/audiobooks/author/book.mp3", result2)
}

func TestCaseInsensitivePaths(t *testing.T) {
	opts := &ImportOptions{
		PathMappings: []PathMapping{
			{From: "W:/itunes/iTunes Media", To: "/mnt/bigdata/books/itunes/iTunes Media"},
		},
	}

	// RemapPath is case-sensitive (uses HasPrefix). This documents current behavior.
	result := opts.RemapPath("w:/ITUNES/itunes media/Audiobooks/book.m4b")
	t.Logf("Case-insensitive remap result: %s (unchanged=%v)", result, result == "w:/ITUNES/itunes media/Audiobooks/book.m4b")
}
