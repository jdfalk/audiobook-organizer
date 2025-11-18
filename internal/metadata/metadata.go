// file: internal/metadata/metadata.go
// version: 1.4.0
// guid: 9d0e1f2a-3b4c-5d6e-7f8a-9b0c1d2e3f4a

package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

	// Remove leading track/chapter numbers (e.g., "01 - Title" or "001 Title")
	parts := strings.Split(filename, " ")
	if len(parts) > 0 {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			filename = strings.Join(parts[1:], " ")
		}
	}
	filename = strings.TrimSpace(filename)

	// Remove chapter info from end (e.g., "Title-10 Chapter 10" -> "Title")
	re := regexp.MustCompile(`(?i)[-_]\d+\s+Chapter\s+\d+$`)
	filename = re.ReplaceAllString(filename, "")

	// Try underscore separator first (for Author_Title patterns)
	if strings.Contains(filename, "_") && !strings.Contains(filename, " - ") {
		parts := strings.SplitN(filename, "_", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if looksLikePersonName(right) && !looksLikePersonName(left) {
				metadata.Title = left
				metadata.Artist = right
				return metadata
			} else if looksLikePersonName(left) && !looksLikePersonName(right) {
				metadata.Title = right
				metadata.Artist = left
				return metadata
			}
		}
	}

	// Try to parse "Title - Author" or "Author - Title" patterns
	if strings.Contains(filename, " - ") {
		title, author := parseFilenameForAuthor(filename)
		if author != "" {
			metadata.Title = title
			metadata.Artist = author
		} else {
			// Fallback to old behavior if we can't determine author
			parts := strings.Split(filename, " - ")
			if len(parts) >= 2 {
				metadata.Series = parts[0]
				metadata.Title = parts[len(parts)-1]
			} else {
				metadata.Title = filename
			}
		}
	} else {
		metadata.Title = filename
	}

	// If we still don't have an artist, try to get from parent directory
	if metadata.Artist == "" {
		metadata.Artist = extractAuthorFromDirectory(filePath)
	}

	return metadata
}

// extractAuthorFromDirectory extracts author from directory name with validation
func extractAuthorFromDirectory(filePath string) string {
	dir := filepath.Dir(filePath)
	dirName := filepath.Base(dir)

	// Skip common non-author directory names
	skipDirs := map[string]bool{
		"books": true, "audiobooks": true, "newbooks": true, "downloads": true,
		"media": true, "audio": true, "library": true, "collection": true,
		"bt": true, "incomplete": true, "data": true,
	}

	if skipDirs[strings.ToLower(dirName)] {
		return ""
	}

	// Handle complex directory patterns like "Author, Co-Author - translator - Title"
	if strings.Contains(dirName, " - translator - ") || strings.Contains(dirName, " - narrated by - ") {
		re := regexp.MustCompile(`^([^-]+)\s*-\s*(?:translator|narrated by)\s*-`)
		matches := re.FindStringSubmatch(dirName)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Extract author from "Author - Title" directory pattern
	if strings.Contains(dirName, " - ") {
		parts := strings.SplitN(dirName, " - ", 2)
		if len(parts) > 0 {
			author := strings.TrimSpace(parts[0])
			if isValidAuthor(author) {
				return author
			}
		}
	}

	// Use directory name if it's valid
	if isValidAuthor(dirName) {
		return dirName
	}

	return ""
}

// isValidAuthor checks if extracted author string is valid
func isValidAuthor(author string) bool {
	if author == "" {
		return false
	}

	author = strings.ToLower(author)

	// Skip invalid patterns
	if strings.HasPrefix(author, "book") || strings.HasPrefix(author, "chapter") ||
		strings.HasPrefix(author, "part") || strings.HasPrefix(author, "vol") ||
		strings.HasPrefix(author, "volume") || strings.HasPrefix(author, "disc") {
		return false
	}

	// Skip purely numeric (like "01", "02")
	if _, err := strconv.Atoi(author); err == nil {
		return false
	}

	// Skip chapter patterns
	if strings.HasPrefix(author, "chapter ") {
		return false
	}

	return true
} // parseFilenameForAuthor attempts to intelligently parse title and author from filename
// Handles patterns like "Title - Author" or "Author - Title"
// Returns (title, author) where author is empty string if pattern not detected
func parseFilenameForAuthor(filename string) (string, string) {
	parts := strings.Split(filename, " - ")
	if len(parts) != 2 {
		return "", "" // Not a simple two-part pattern
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	// Heuristic: check if right side looks like an author name
	rightIsName := looksLikePersonName(right)
	leftIsName := looksLikePersonName(left)

	if rightIsName && !leftIsName {
		// Pattern: "Title - Author"
		return left, right
	} else if leftIsName && !rightIsName {
		// Pattern: "Author - Title"
		return right, left
	} else if rightIsName {
		// Both could be names, prefer "Title - Author" pattern
		return left, right
	}

	// Couldn't determine, return empty author
	return "", ""
}

// looksLikePersonName checks if a string looks like a person's name
// Looks for patterns like "John Smith", "J. Smith", "J. K. Rowling"
func looksLikePersonName(s string) bool {
	if !isValidAuthor(s) {
		return false
	}

	// Check for initials like "J. K. Rowling" or "J.K. Rowling"
	if strings.Contains(s, ".") {
		// Count uppercase letters and periods
		uppers := 0
		for _, r := range s {
			if r >= 'A' && r <= 'Z' {
				uppers++
			}
		}
		if uppers >= 2 {
			return true
		}
	}

	// Check for multi-word names with proper capitalization
	words := strings.Fields(s)
	if len(words) >= 2 && len(words) <= 4 {
		// Check if all words start with uppercase
		allProperCase := true
		for _, word := range words {
			if len(word) == 0 || (word[0] < 'A' || word[0] > 'Z') {
				allProperCase = false
				break
			}
		}
		if allProperCase {
			return true
		}
	}

	// Check for "FirstName LastName" pattern (at least one space, proper case)
	if len(words) >= 2 {
		// First word starts with capital
		if len(words[0]) > 0 && words[0][0] >= 'A' && words[0][0] <= 'Z' {
			// Second word starts with capital
			if len(words[1]) > 0 && words[1][0] >= 'A' && words[1][0] <= 'Z' {
				return true
			}
		}
	}

	return false
}
