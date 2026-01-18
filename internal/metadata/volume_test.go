// file: internal/metadata/volume_test.go
// version: 1.1.0
// guid: 5c4d3e2f-1a0b-9c8d-7e6f-5a4b3c2d1e0f

package metadata

import "testing"

func TestDetectVolumeNumberRomanNumerals(t *testing.T) {
	if got := DetectVolumeNumber("Vol. IV"); got != 4 {
		t.Fatalf("expected 4, got %d", got)
	}
	if got := DetectVolumeNumber("Book xi"); got != 11 {
		t.Fatalf("expected 11, got %d", got)
	}
}

func TestExtractSeriesFromVolumeString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expSeries   string
		expPosition int
	}{
		{
			name:        "CommaVolSuffix",
			input:       "My Quiet Blacksmith Life in Another World, Vol. 01 (Audiobook)",
			expSeries:   "My Quiet Blacksmith Life in Another World",
			expPosition: 1,
		},
		{
			name:        "HyphenVolume",
			input:       "Reborn as a Space Mercenary - Volume 2",
			expSeries:   "Reborn as a Space Mercenary",
			expPosition: 2,
		},
		{
			name:        "HashNumber",
			input:       "Ascendance of a Bookworm #3",
			expSeries:   "Ascendance of a Bookworm",
			expPosition: 3,
		},
		{
			name:        "NoMatch",
			input:       "Standalone Novel",
			expSeries:   "",
			expPosition: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			series, idx := extractSeriesFromVolumeString(tc.input)
			if series != tc.expSeries {
				t.Fatalf("expected series %q, got %q", tc.expSeries, series)
			}
			if idx != tc.expPosition {
				t.Fatalf("expected index %d, got %d", tc.expPosition, idx)
			}
		})
	}
}

func TestDetectVolumeNumberArabic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"VolWithLeadingZero", "Vol. 01", 1},
		{"VolumeNoDot", "Volume 2", 2},
		{"BookPrefix", "Book 3", 3},
		{"BkPrefix", "Bk. 4", 4},
		{"PartPrefix", "Part 5", 5},
		{"HashPrefix", "Series #6", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectVolumeNumber(tt.input); got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}
