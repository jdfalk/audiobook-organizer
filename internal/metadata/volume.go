// file: internal/metadata/volume.go
// version: 1.1.0
// guid: 1b2c3d4e-5f6a-7b8c-9d0e-1f2a3b4c5d6e

package metadata

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	volumePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bvol(?:ume)?\.?\s*(\d+|[ivxlcdm]+)`),
		regexp.MustCompile(`(?i)\bbook\.?\s*(\d+|[ivxlcdm]+)`),
		regexp.MustCompile(`(?i)\bbk\.?\s*(\d+|[ivxlcdm]+)`),
		regexp.MustCompile(`(?i)\bpart\.?\s*(\d+|[ivxlcdm]+)`),
		regexp.MustCompile(`(?i)#\s*(\d+|[ivxlcdm]+)`),
	}
	seriesVolumeIndicatorRegex = regexp.MustCompile(`(?i)\b(vol(?:ume)?|book|bk|part)\b`)
	seriesHashVolumeRegex      = regexp.MustCompile(`#\s*(\d+|[ivxlcdm]+)\b`)
)

// DetectVolumeNumber returns the first volume number found in a string.
// It understands patterns like "Vol. 01", "Volume 1", "Book 2", "Bk. 3", and "#4".
func DetectVolumeNumber(text string) int {
	for _, pattern := range volumePatterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) > 1 {
			token := matches[1]
			if value, err := strconv.Atoi(token); err == nil {
				return value
			}
			if roman := romanToInt(token); roman > 0 {
				return roman
			}
		}
	}
	return 0
}

func extractSeriesFromVolumeString(value string) (string, int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", 0
	}

	if loc := seriesVolumeIndicatorRegex.FindStringIndex(trimmed); loc != nil && loc[0] > 0 {
		series := trimSeriesDelimiters(trimmed[:loc[0]])
		if series != "" {
			return series, DetectVolumeNumber(trimmed[loc[0]:])
		}
	}

	if loc := seriesHashVolumeRegex.FindStringIndex(trimmed); loc != nil && loc[0] > 0 {
		series := trimSeriesDelimiters(trimmed[:loc[0]])
		if series != "" {
			return series, DetectVolumeNumber(trimmed[loc[0]:])
		}
	}

	return "", 0
}

func trimSeriesDelimiters(value string) string {
	return strings.Trim(value, " 	-_,:;–—#[]()")
}

func romanToInt(token string) int {
	upper := strings.ToUpper(strings.TrimSpace(token))
	if upper == "" {
		return 0
	}
	values := map[rune]int{
		'I': 1,
		'V': 5,
		'X': 10,
		'L': 50,
		'C': 100,
		'D': 500,
		'M': 1000,
	}
	total := 0
	prev := 0
	for i := len(upper) - 1; i >= 0; i-- {
		ch := rune(upper[i])
		value, ok := values[ch]
		if !ok {
			return 0
		}
		if value < prev {
			total -= value
		} else {
			total += value
			prev = value
		}
	}
	return total
}
