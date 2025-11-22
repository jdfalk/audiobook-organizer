// file: internal/metadata/volume.go
// version: 1.0.0
// guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e

package metadata

import (
	"regexp"
	"strconv"
)

var volumePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bvol(?:ume)?\.?\s*(\d+)`),
	regexp.MustCompile(`(?i)\bbook\.?\s*(\d+)`),
	regexp.MustCompile(`(?i)\bbk\.?\s*(\d+)`),
	regexp.MustCompile(`(?i)\bpart\.?\s*(\d+)`),
	regexp.MustCompile(`(?i)#\s*(\d+)`),
}

// DetectVolumeNumber returns the first volume number found in a string.
// It understands patterns like "Vol. 01", "Volume 1", "Book 2", "Bk. 3", and "#4".
func DetectVolumeNumber(text string) int {
	for _, pattern := range volumePatterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) > 1 {
			if value, err := strconv.Atoi(matches[1]); err == nil {
				return value
			}
		}
	}
	return 0
}
