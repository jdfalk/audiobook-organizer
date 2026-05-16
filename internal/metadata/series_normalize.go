// file: internal/metadata/series_normalize.go
// version: 1.1.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package metadata

import (
	"regexp"
	"strings"
)

var (
	reDashPositionTitle = regexp.MustCompile(`^(.+?)\s+-\s+(\d+)\s+-\s+.+$`)
	reTrailingDigit     = regexp.MustCompile(`^(.+?)\s+-\s+(\d{1,2})$`)
	reTrailingOrdinal   = regexp.MustCompile(`(?i)^(.+?)\s+(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)$`)
)

var ordinalToDigit = map[string]string{
	"one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
	"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
	"eleven": "11", "twelve": "12", "thirteen": "13", "fourteen": "14",
	"fifteen": "15", "sixteen": "16", "seventeen": "17", "eighteen": "18",
	"nineteen": "19", "twenty": "20",
}

// StripSeriesContamination removes title and position info that has been incorrectly
// embedded in a series name. Returns the cleaned series name, the extracted position
// (empty if none), and flagForReview=true when the series name equals the book title
// and no structural pattern was matched (needs human review).
//
// Rules applied in order, stopping at first match:
//
//  1. Dash-embedded: "Series - 1 - Title" → series="Series", pos="1"
//  2. Trailing 1-2 digit number: "Series 2" or "Series - 2" → series="Series", pos="2"
//  3. Trailing ordinal word (one–twenty): "Series One" → series="Series", pos="1"
//  4. Series equals title → flagForReview=true, series unchanged
func StripSeriesContamination(name, title string) (series, position string, flagForReview bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}

	if m := reDashPositionTitle.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2], false
	}

	if m := reTrailingDigit.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2], false
	}

	if m := reTrailingOrdinal.FindStringSubmatch(name); m != nil {
		pos := ordinalToDigit[strings.ToLower(m[2])]
		return strings.TrimSpace(m[1]), pos, false
	}

	if title != "" && strings.EqualFold(name, strings.TrimSpace(title)) {
		return name, "", true
	}

	return name, "", false
}
