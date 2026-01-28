// file: internal/itunes/parser_test.go
// version: 1.0.0
// guid: ba52e249-9f83-4b59-9494-c68465e5d1f9

package itunes

import (
	"runtime"
	"testing"
)

// TestIsAudiobook verifies audiobook detection against common iTunes metadata.
func TestIsAudiobook(t *testing.T) {
	tests := []struct {
		name     string
		track    *Track
		expected bool
	}{
		{
			name:     "Kind is Audiobook",
			track:    &Track{Kind: "Audiobook"},
			expected: true,
		},
		{
			name:     "Kind is Spoken Word",
			track:    &Track{Kind: "Spoken Word"},
			expected: true,
		},
		{
			name:     "Genre contains audiobook",
			track:    &Track{Genre: "Audiobooks"},
			expected: true,
		},
		{
			name: "Location contains Audiobooks",
			track: &Track{
				Location: "file:///Users/username/Music/iTunes/Audiobooks/book.m4b",
			},
			expected: true,
		},
		{
			name: "Music track",
			track: &Track{
				Kind:  "MPEG audio file",
				Genre: "Rock",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAudiobook(tt.track)
			if result != tt.expected {
				t.Errorf("IsAudiobook() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestDecodeLocation confirms iTunes URL decoding handles common paths.
func TestDecodeLocation(t *testing.T) {
	tests := []struct {
		name     string
		location string
		expected string
		wantErr  bool
	}{
		{
			name:     "Standard macOS path",
			location: "file://localhost/Users/username/Music/iTunes/Audiobooks/Book.m4b",
			expected: "/Users/username/Music/iTunes/Audiobooks/Book.m4b",
			wantErr:  false,
		},
		{
			name:     "Path with spaces",
			location: "file://localhost/Users/username/Music/iTunes/Audiobooks/The%20Hobbit.m4b",
			expected: "/Users/username/Music/iTunes/Audiobooks/The Hobbit.m4b",
			wantErr:  false,
		},
		{
			name:     "Empty location",
			location: "",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeLocation(tt.location)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeLocation() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("DecodeLocation() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestEncodeLocation ensures file paths are encoded into iTunes file URLs.
func TestEncodeLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows paths are not supported in this repository")
	}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Standard path",
			path:     "/Users/username/Music/Book.m4b",
			expected: "file://localhost/Users/username/Music/Book.m4b",
		},
		{
			name:     "Path with spaces",
			path:     "/Users/username/Music/The Hobbit.m4b",
			expected: "file://localhost/Users/username/Music/The%20Hobbit.m4b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeLocation(tt.path)
			if result != tt.expected {
				t.Errorf("EncodeLocation() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestFindLibraryFile confirms the search helper returns or errors gracefully.
func TestFindLibraryFile(t *testing.T) {
	path, err := FindLibraryFile()
	if err != nil {
		t.Logf("No iTunes library found (expected on systems without iTunes): %v", err)
		return
	}
	if path == "" {
		t.Fatal("FindLibraryFile returned empty path without error")
	}
}
