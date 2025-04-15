package matcher

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"
)

// Common series indicators in file names
var seriesPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(.*?)\s+-\s+(.+)`),                                 // "Series - Title"
	regexp.MustCompile(`(?i)(.+?)\s+(\d+)(?:\s*:|\s+-)\s+(.+)`),                // "Series 1: Title" or "Series 1 - Title"
	regexp.MustCompile(`(?i)(.+?)\s+Book\s+(\d+)(?:\s*:|\s+-)\s+(.+)`),         // "Series Book 1: Title"
	regexp.MustCompile(`(?i)(.+?)\s+#(\d+)(?:\s*:|\s+-)\s+(.+)`),               // "Series #1: Title"
	regexp.MustCompile(`(?i)(.+?)\s+Vol(\.|ume)?\s+(\d+)(?:\s*:|\s+-)\s+(.+)`), // "Series Vol. 1: Title"
}

// seriesWords are common words indicating a series
var seriesWords = []string{"trilogy", "series", "saga", "chronicles", "sequence"}

// IdentifySeries attempts to identify the series and position from title and filepath
func IdentifySeries(title, filePath string) (string, int) {
	if title == "" {
		// Try to extract from filename if title is empty
		title = filepath.Base(filePath)
		title = strings.TrimSuffix(title, filepath.Ext(title))
	}

	// First try pattern matching
	for _, pattern := range seriesPatterns {
		matches := pattern.FindStringSubmatch(title)
		if len(matches) >= 3 {
			series := strings.TrimSpace(matches[1])
			position := 0

			// Extract position number based on the specific pattern
			positionIdx := 2
			if len(matches) > 3 && strings.Contains(pattern.String(), "Vol") {
				positionIdx = 3
			}

			if posIdx, err := strconv.Atoi(matches[positionIdx]); err == nil {
				position = posIdx
			}

			return series, position
		}
	}

	// Next, look at the directory structure
	dirs := strings.Split(filepath.Dir(filePath), string(filepath.Separator))
	if len(dirs) >= 2 {
		// Check if parent directory might be a series name
		parentDir := dirs[len(dirs)-1]
		authorDir := ""
		if len(dirs) >= 3 {
			authorDir = dirs[len(dirs)-2]
		}

		// If parent directory is not the author name, it might be a series
		if parentDir != authorDir && !isSingleWord(parentDir) {
			// Check if any series keywords are present
			for _, word := range seriesWords {
				if strings.Contains(strings.ToLower(parentDir), word) {
					return parentDir, 0
				}
			}

			// Fuzzy check if title contains parent directory name or vice versa
			if fuzzy.Match(strings.ToLower(parentDir), strings.ToLower(title)) {
				return parentDir, 0
			}
		}
	}

	// Finally, look for series names in the title itself
	// Check for common series name patterns in the title
	colonParts := strings.Split(title, ": ")
	if len(colonParts) >= 2 {
		// "Series: Book Title" pattern
		return colonParts[0], 0
	}

	dashParts := strings.Split(title, " - ")
	if len(dashParts) >= 2 {
		// "Series - Book Title" pattern
		return dashParts[0], 0
	}

	// If no series identified, return empty
	return "", 0
}

// isSingleWord checks if a string consists of a single word
func isSingleWord(s string) bool {
	return len(strings.Fields(s)) == 1
}
