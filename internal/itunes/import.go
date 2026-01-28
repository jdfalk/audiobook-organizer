package itunes

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/models"
)

// ImportMode specifies how audiobooks should be imported from iTunes
type ImportMode string

const (
	// ImportModeOrganized treats iTunes files as already organized (no file operations)
	ImportModeOrganized ImportMode = "organized"

	// ImportModeImport adds books to database but doesn't organize them yet
	ImportModeImport ImportMode = "import"

	// ImportModeOrganize imports and immediately triggers organization
	ImportModeOrganize ImportMode = "organize"
)

// ImportOptions configures how the iTunes import should behave
type ImportOptions struct {
	LibraryPath      string     // Path to iTunes Library.xml
	ImportMode       ImportMode // How to handle file organization
	PreserveLocation bool       // Keep files in iTunes location (don't move)
	ImportPlaylists  bool       // Import playlists as tags
	SkipDuplicates   bool       // Skip books already in library (by hash)
}

// ValidationResult contains the results of validating an iTunes import
type ValidationResult struct {
	TotalTracks      int               // Total tracks in iTunes library
	AudiobookTracks  int               // Tracks identified as audiobooks
	FilesFound       int               // Audiobook files that exist on disk
	FilesMissing     int               // Audiobook files that are missing
	MissingPaths     []string          // List of missing file paths
	DuplicateHashes  map[string][]string // hash -> list of titles
	EstimatedTime    string            // Estimated import time
}

// ImportResult contains the results of an iTunes import operation
type ImportResult struct {
	TotalProcessed int      // Total audiobooks processed
	Imported       int      // Successfully imported
	Skipped        int      // Skipped (duplicates)
	Failed         int      // Failed to import
	Errors         []string // Error messages
}

// ValidateImport validates an iTunes library file and checks file existence
func ValidateImport(opts ImportOptions) (*ValidationResult, error) {
	// Parse the iTunes library
	library, err := ParseLibrary(opts.LibraryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse iTunes library: %w", err)
	}

	result := &ValidationResult{
		TotalTracks:     len(library.Tracks),
		MissingPaths:    make([]string, 0),
		DuplicateHashes: make(map[string][]string),
	}

	// Check each track
	for _, track := range library.Tracks {
		if !IsAudiobook(track) {
			continue
		}
		result.AudiobookTracks++

		// Decode the location
		path, err := DecodeLocation(track.Location)
		if err != nil {
			result.MissingPaths = append(result.MissingPaths, track.Location)
			result.FilesMissing++
			continue
		}

		// Check if file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			result.FilesMissing++
			result.MissingPaths = append(result.MissingPaths, path)
		} else {
			result.FilesFound++

			// Check for duplicates by hash
			if opts.SkipDuplicates {
				hash, err := computeFileHash(path)
				if err == nil {
					if existing, ok := result.DuplicateHashes[hash]; ok {
						result.DuplicateHashes[hash] = append(existing, track.Name)
					} else {
						result.DuplicateHashes[hash] = []string{track.Name}
					}
				}
			}
		}
	}

	// Estimate import time (roughly 1 second per book for metadata extraction)
	seconds := result.FilesFound
	if seconds < 60 {
		result.EstimatedTime = fmt.Sprintf("%d seconds", seconds)
	} else if seconds < 3600 {
		result.EstimatedTime = fmt.Sprintf("%d minutes", seconds/60)
	} else {
		result.EstimatedTime = fmt.Sprintf("%d hours %d minutes", seconds/3600, (seconds%3600)/60)
	}

	return result, nil
}

// ConvertTrack converts an iTunes track to an audiobook model
func ConvertTrack(track *Track, opts ImportOptions) (*models.Audiobook, error) {
	// Decode file location
	filePath, err := DecodeLocation(track.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to decode location: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// Helper functions for pointer values
	stringPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }
	int64Ptr := func(i int64) *int64 { return &i }
	timePtr := func(t time.Time) *time.Time { return &t }

	// Convert duration from milliseconds to int (duration in seconds)
	var duration *int
	if track.TotalTime > 0 {
		seconds := int(track.TotalTime / 1000)
		duration = &seconds
	}

	// Determine audiobook release year
	var releaseYear *int
	if track.Year > 0 {
		releaseYear = &track.Year
	}

	// Create audiobook model
	book := &models.Audiobook{
		Title:                track.Name,
		FilePath:             filePath,
		Format:               strings.ToLower(filepath.Ext(filePath)[1:]), // e.g., "m4b", "mp3"
		Duration:             duration,
		AudiobookReleaseYear: releaseYear,

		// iTunes-specific fields
		ITunesPersistentID: stringPtr(track.PersistentID),
		ITunesDateAdded:    timePtr(track.DateAdded),
		ITunesPlayCount:    intPtr(track.PlayCount),
		ITunesRating:       intPtr(track.Rating),
		ITunesBookmark:     int64Ptr(track.Bookmark),
		ITunesImportSource: stringPtr(opts.LibraryPath),
	}

	// Convert iTunes play date (Unix timestamp) to time.Time
	if track.PlayDate > 0 {
		lastPlayed := time.Unix(track.PlayDate, 0)
		book.ITunesLastPlayed = &lastPlayed
	}

	// Extract narrator from Album Artist if different from Artist
	if track.AlbumArtist != "" && track.AlbumArtist != track.Artist {
		book.Narrator = stringPtr(track.AlbumArtist)
	}

	// Extract edition or publisher from comments
	if track.Comments != "" {
		book.Edition = stringPtr(track.Comments)
	}

	// Try to extract series information from Album field
	// Common patterns: "Series Name, Book X", "Series Name - Book X"
	if track.Album != "" {
		series, position := extractSeriesFromAlbum(track.Album)
		if series != "" {
			// Note: We store series as text here, need to create Series record later
			// and link via SeriesID in actual import process
			_ = series
			_ = position
		}
	}

	return book, nil
}

// extractSeriesFromAlbum attempts to extract series name and position from an album field
func extractSeriesFromAlbum(album string) (series string, position float64) {
	// Common patterns:
	// "Series Name, Book 1"
	// "Series Name - Book 1"
	// "Series Name: Book 1"

	// Try comma separator
	if parts := strings.Split(album, ","); len(parts) == 2 {
		series = strings.TrimSpace(parts[0])
		// Try to extract number from second part
		// This is simplified - in production you'd use regex
		return series, 0.0
	}

	// Try dash separator
	if parts := strings.Split(album, "-"); len(parts) == 2 {
		series = strings.TrimSpace(parts[0])
		return series, 0.0
	}

	// If no pattern matches, return the album as series name
	return album, 0.0
}

// computeFileHash computes SHA256 hash of a file
func computeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// ExtractPlaylistTags extracts playlist names as tags for a given track
func ExtractPlaylistTags(trackID int, playlists []*Playlist) []string {
	tags := make([]string, 0)

	for _, playlist := range playlists {
		// Skip built-in playlists (like "Music", "Movies", etc.)
		if isBuiltInPlaylist(playlist.Name) {
			continue
		}

		// Check if track is in this playlist
		for _, id := range playlist.TrackIDs {
			if id == trackID {
				// Add playlist name as tag (lowercase for consistency)
				tags = append(tags, strings.ToLower(playlist.Name))
				break
			}
		}
	}

	return tags
}

// isBuiltInPlaylist checks if a playlist name is a built-in iTunes playlist
func isBuiltInPlaylist(name string) bool {
	builtIn := []string{
		"Music",
		"Movies",
		"TV Shows",
		"Podcasts",
		"Audiobooks",
		"iTunes U",
		"Books",
		"Genius",
		"Recently Added",
		"Recently Played",
		"Top 25 Most Played",
	}

	for _, builtin := range builtIn {
		if name == builtin {
			return true
		}
	}

	return false
}
