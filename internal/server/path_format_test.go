// file: internal/server/path_format_test.go
// version: 1.0.0
// guid: b8c4d2e3-f5a6-7890-bcde-f01234567890

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatSegmentTitle(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		title    string
		track    int
		total    int
		expected string
	}{
		{"default format", "{title} - {track}_{total_tracks}", "Leviathan Falls", 15, 51, "Leviathan Falls - 15_51"},
		{"of format", "{title} - {track} of {total_tracks}", "Leviathan Falls", 15, 51, "Leviathan Falls - 15 of 51"},
		{"part format", "{title} - Part {track}", "Leviathan Falls", 15, 51, "Leviathan Falls - Part 15"},
		{"zero-padded", "{track:02d} - {title}", "Leviathan Falls", 3, 51, "03 - Leviathan Falls"},
		{"three-digit pad", "{track:03d} - {title}", "Leviathan Falls", 3, 200, "003 - Leviathan Falls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSegmentTitle(tt.format, tt.title, tt.track, tt.total)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatPath_BasicTemplate(t *testing.T) {
	vars := FormatVars{
		Author:      "James S.A. Corey",
		Title:       "Leviathan Falls",
		Series:      "The Expanse",
		SeriesPos:   "9",
		Track:       1,
		TotalTracks: 51,
		Ext:         "mp3",
	}
	result := FormatPath("{author}/{series_prefix}{title}/{track_title}.{ext}", vars)
	require.Equal(t, "James S.A. Corey/The Expanse 9 - Leviathan Falls/Leviathan Falls - 1_51.mp3", result)
}

func TestFormatPath_NoSeries(t *testing.T) {
	vars := FormatVars{
		Author:      "Author Name",
		Title:       "Book Title",
		Track:       1,
		TotalTracks: 10,
		Ext:         "m4b",
	}
	result := FormatPath("{author}/{series_prefix}{title}/{track_title}.{ext}", vars)
	require.Equal(t, "Author Name/Book Title/Book Title - 1_10.m4b", result)
}

func TestFormatPath_EmptyVariablesCollapse(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Ext:    "m4b",
	}
	result := FormatPath("{author}/{series_prefix}{title}.{lang}.{ext}", vars)
	require.Equal(t, "Author/Title.m4b", result)
}

func TestFormatPath_LanguageSuffix(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Lang:   "de",
		Ext:    "m4b",
	}
	result := FormatPath("{author}/{title}.{lang}.{ext}", vars)
	require.Equal(t, "Author/Title.de.m4b", result)
}

func TestFormatPath_WithYear(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Year:   2021,
		Ext:    "m4b",
	}
	result := FormatPath("{author}/{title} ({year}).{ext}", vars)
	require.Equal(t, "Author/Title (2021).m4b", result)
}

func TestFormatPath_NoYear(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Ext:    "m4b",
	}
	result := FormatPath("{author}/{title} ({year}).{ext}", vars)
	require.Equal(t, "Author/Title ().m4b", result)
}

func TestSanitizePathComponent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"James S.A. Corey", "James S.A. Corey"},
		{"Title: Subtitle", "Title - Subtitle"},
		{"No/Slash/Here", "No Slash Here"},
		{"What?", "What"},
		{"File*Name", "FileName"},
		{"  extra  spaces  ", "extra spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, sanitizePathComponent(tt.input))
		})
	}
}

func TestCollapseEmptySegments(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a//b", "a/b"},
		{"a..b", "a.b"},
		{"a/./b", "a/b"},
		{"a/b/", "a/b"},
		{"/a/b", "a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, collapseEmptySegments(tt.input))
		})
	}
}
