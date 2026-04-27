// file: internal/metadata/series_normalize_test.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package metadata

import (
	"testing"
)

func TestStripSeriesContamination(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		title          string
		wantSeries     string
		wantPosition   string
		wantFlagReview bool
	}{
		// Rule 1: dash-embedded position+title
		{
			name:         "dash embedded position and title",
			input:        "The Long Earth - 1 - The Long Earth",
			wantSeries:   "The Long Earth",
			wantPosition: "1",
		},
		{
			name:         "dash embedded with different title",
			input:        "My Long Series - 3 - The Third Book",
			wantSeries:   "My Long Series",
			wantPosition: "3",
		},
		// Rule 2: trailing digit
		{
			name:       "trailing digit with bare space not matched",
			input:      "The Long Earth 2",
			wantSeries: "The Long Earth 2",
		},
		{
			name:         "trailing digit with dash-space",
			input:        "The Long Earth - 2",
			wantSeries:   "The Long Earth",
			wantPosition: "2",
		},
		// Rule 3: trailing ordinal word
		{
			name:         "trailing ordinal One",
			input:        "The Long Earth One",
			wantSeries:   "The Long Earth",
			wantPosition: "1",
		},
		{
			name:         "trailing ordinal Two lowercase",
			input:        "the long earth two",
			wantSeries:   "the long earth",
			wantPosition: "2",
		},
		{
			name:         "trailing Twenty",
			input:        "My Series Twenty",
			wantSeries:   "My Series",
			wantPosition: "20",
		},
		// Rule 4: series equals title (no other pattern matched)
		{
			name:           "exact series==title with no other match",
			input:          "Just A Title",
			title:          "Just A Title",
			wantSeries:     "Just A Title",
			wantPosition:   "",
			wantFlagReview: true,
		},
		{
			name:           "series==title with whitespace on title triggers flag",
			input:          "Just A Title",
			title:          "  Just A Title  ",
			wantSeries:     "Just A Title",
			wantFlagReview: true,
		},
		// No-op cases
		{
			name:       "clean series name unchanged",
			input:      "The Expanse",
			wantSeries: "The Expanse",
		},
		{
			name:       "Discworld unchanged",
			input:      "Discworld",
			wantSeries: "Discworld",
		},
		{
			name:       "Babylon 5 not stripped",
			input:      "Babylon 5",
			wantSeries: "Babylon 5",
		},
		{
			name:       "World War 2 not stripped",
			input:      "World War 2",
			wantSeries: "World War 2",
		},
		{
			name:       "Section 31 not stripped",
			input:      "Section 31",
			wantSeries: "Section 31",
		},
		// Edge cases
		{
			name:       "ordinal Twenty-One not matched (out of range)",
			input:      "My Series Twenty-One",
			wantSeries: "My Series Twenty-One",
		},
		{
			name:       "word Someone not matched as ordinal",
			input:      "Someone",
			wantSeries: "Someone",
		},
		{
			name:       "empty name unchanged",
			input:      "",
			wantSeries: "",
		},
		{
			name:         "trailing digit 99 with dash-space matched",
			input:        "Big Series - 99",
			wantSeries:   "Big Series",
			wantPosition: "99",
		},
		{
			name:       "trailing 3-digit number not matched",
			input:      "Fahrenheit 451",
			wantSeries: "Fahrenheit 451",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSeries, gotPos, gotFlag := StripSeriesContamination(tt.input, tt.title)
			if gotSeries != tt.wantSeries {
				t.Errorf("series: got %q, want %q", gotSeries, tt.wantSeries)
			}
			if gotPos != tt.wantPosition {
				t.Errorf("position: got %q, want %q", gotPos, tt.wantPosition)
			}
			if gotFlag != tt.wantFlagReview {
				t.Errorf("flagForReview: got %v, want %v", gotFlag, tt.wantFlagReview)
			}
		})
	}
}
