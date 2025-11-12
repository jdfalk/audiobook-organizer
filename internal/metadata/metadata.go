// file: internal/metadata/metadata.go
// version: 1.2.0
// guid: 9d0e1f2a-3b4c-5d6e-7f8a-9b0c1d2e3f4a

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
	// Extended fields (best-effort; may be empty)
	Narrator  string
	Language  string
	Publisher string
	ISBN10    string
	ISBN13    string
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

	// Try to extract extended fields from raw tag map using common frames/atoms
	// Note: Availability varies by format and tagging tool; this is best-effort.
	raw := m.Raw()
	// Language (ID3: TLAN)
	if v, ok := raw["TLAN"]; ok {
		if s, sok := v.(string); sok {
			metadata.Language = strings.TrimSpace(s)
		}
	}
	// Publisher (ID3: TPUB, MP4: ©pub)
	if v, ok := raw["TPUB"]; ok {
		if s, sok := v.(string); sok {
			metadata.Publisher = strings.TrimSpace(s)
		}
	} else if v, ok := raw["©pub"]; ok {
		if s, sok := v.(string); sok {
			metadata.Publisher = strings.TrimSpace(s)
		}
	}
	// Narrator (often custom TXXX frames; also MP4 freeform keys)
	for _, key := range []string{"TXXX:NARRATOR", "TXXX:Narrator", "NARRATOR", "Narrator", "©nrt"} {
		if v, ok := raw[key]; ok {
			if s, sok := v.(string); sok {
				metadata.Narrator = strings.TrimSpace(s)
				break
			}
		}
	}
	// ISBNs (custom frames)
	for k, target := range map[string]*string{
		"TXXX:ISBN":   &metadata.ISBN13,
		"TXXX:ISBN13": &metadata.ISBN13,
		"TXXX:ISBN10": &metadata.ISBN10,
		"ISBN":        &metadata.ISBN13,
	} {
		if v, ok := raw[k]; ok {
			if s, sok := v.(string); sok {
				*target = strings.TrimSpace(s)
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
