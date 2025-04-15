package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// Metadata holds audio file metadata
type Metadata struct {
	Title       string
	Artist      string
	Album       string
	Genre       string
	Series      string
	SeriesIndex int
	Comments    string
	Year        int
}

// ExtractMetadata reads metadata from audio files
func ExtractMetadata(filePath string) (Metadata, error) {
	var metadata Metadata

	f, err := os.Open(filePath)
	if err != nil {
		return metadata, fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	// Extract metadata using tag library
	m, err := tag.ReadFrom(f)
	if err != nil {
		// If tag library fails, try to extract info from filename
		metadata = extractFromFilename(filePath)
		return metadata, nil
	}

	metadata.Title = m.Title()
	metadata.Artist = m.Artist()
	metadata.Album = m.Album()
	metadata.Genre = m.Genre()
	metadata.Year = m.Year()

	// Try to extract series information from various fields
	// Check album or grouping tag
	if grouping, ok := m.Raw()["TGID"]; ok && grouping != "" {
		metadata.Series = grouping.(string)
	} else if grouping, ok := m.Raw()["GRP1"]; ok && grouping != "" {
		metadata.Series = grouping.(string)
	} else if strings.Contains(metadata.Album, " - ") {
		// Sometimes series is part of the album name
		parts := strings.Split(metadata.Album, " - ")
		if len(parts) > 1 {
			metadata.Series = parts[0]
		}
	}

	// Check for comments that might include series info
	if m.Comment() != "" {
		metadata.Comments = m.Comment()
		// Try to extract series info from comments
		if metadata.Series == "" {
			seriesMatches := []string{"Series:", "Series :", "Part of:"}
			for _, match := range seriesMatches {
				if strings.Contains(metadata.Comments, match) {
					parts := strings.Split(metadata.Comments, match)
					if len(parts) > 1 {
						seriesPart := strings.TrimSpace(parts[1])
						endPos := strings.IndexAny(seriesPart, "\n\r.,;")
						if endPos > 0 {
							metadata.Series = seriesPart[:endPos]
						} else {
							metadata.Series = seriesPart
						}
						break
					}
				}
			}
		}
	}

	return metadata, nil
}

// extractFromFilename tries to extract metadata from filename when tags are unavailable
func extractFromFilename(filePath string) Metadata {
	var metadata Metadata

	filename := filepath.Base(filePath)
	// Remove extension
	filename = strings.TrimSuffix(filename, filepath.Ext(filename))

	// Check for common patterns
	// Pattern: "Series Name - Book Title"
	if strings.Contains(filename, " - ") {
		parts := strings.Split(filename, " - ")
		if len(parts) >= 2 {
			metadata.Series = parts[0]
			metadata.Title = parts[len(parts)-1]
		} else {
			metadata.Title = filename
		}
	} else {
		metadata.Title = filename
	}

	// Try to get artist from parent directory
	dir := filepath.Dir(filePath)
	dirName := filepath.Base(dir)
	metadata.Artist = dirName

	return metadata
}
