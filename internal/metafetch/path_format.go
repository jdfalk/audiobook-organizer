// file: internal/metafetch/path_format.go
// version: 1.1.0
// guid: a7b3c1d2-e4f5-6789-abcd-ef0123456789

package metafetch

import (
	"fmt"
	"regexp"
	"strings"
)

// FormatVars holds all variables available for path/title formatting.
type FormatVars struct {
	Author      string
	Title       string
	Series      string
	SeriesPos   string
	Year        int
	Narrator    string
	Lang        string // ISO 639-1 (en, de, fr)
	Track       int
	TotalTracks int
	TrackTitle  string // pre-computed segment title
	Ext         string
}

var formatVarPattern = regexp.MustCompile(`\{(\w+)(?::([^}]+))?\}`)

const (
	DefaultPathFormat         = "{author}/{series_prefix}{title}/{track_title}.{ext}"
	DefaultSegmentTitleFormat = "{title} - {track}_{total_tracks}"
)

// FormatSegmentTitle formats a per-segment title using the template.
// For single-file books (totalTracks == 1), returns just the title without numbering.
func FormatSegmentTitle(format string, title string, track, totalTracks int) string {
	if totalTracks <= 1 {
		return title
	}
	result := format
	result = strings.ReplaceAll(result, "{title}", title)
	result = strings.ReplaceAll(result, "{total_tracks}", fmt.Sprintf("%d", totalTracks))

	// Handle {track} with optional format spec like {track:02d}
	result = formatVarPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := formatVarPattern.FindStringSubmatch(match)
		name := parts[1]
		spec := parts[2]
		if name == "track" {
			if spec != "" {
				return fmt.Sprintf("%"+spec, track)
			}
			return fmt.Sprintf("%d", track)
		}
		return match
	})
	return result
}

// FormatPath formats a full file path using the path_format template.
func FormatPath(format string, vars FormatVars) string {
	trackTitle := vars.TrackTitle
	if trackTitle == "" && vars.Track > 0 {
		trackTitle = FormatSegmentTitle(DefaultSegmentTitleFormat, vars.Title, vars.Track, vars.TotalTracks)
	}

	seriesPrefix := ""
	if vars.Series != "" {
		seriesPrefix = vars.Series
		if vars.SeriesPos != "" {
			seriesPrefix += " " + vars.SeriesPos
		}
		seriesPrefix += " - "
	}

	yearStr := ""
	if vars.Year > 0 {
		yearStr = fmt.Sprintf("%d", vars.Year)
	}

	result := format
	result = strings.ReplaceAll(result, "{author}", vars.Author)
	result = strings.ReplaceAll(result, "{title}", vars.Title)
	result = strings.ReplaceAll(result, "{series}", vars.Series)
	result = strings.ReplaceAll(result, "{series_position}", vars.SeriesPos)
	result = strings.ReplaceAll(result, "{series_prefix}", seriesPrefix)
	result = strings.ReplaceAll(result, "{year}", yearStr)
	result = strings.ReplaceAll(result, "{narrator}", vars.Narrator)
	result = strings.ReplaceAll(result, "{lang}", vars.Lang)
	result = strings.ReplaceAll(result, "{track_title}", trackTitle)
	result = strings.ReplaceAll(result, "{ext}", vars.Ext)

	// Handle {track} and {total_tracks} with optional format specs
	result = formatVarPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := formatVarPattern.FindStringSubmatch(match)
		name := parts[1]
		spec := parts[2]
		switch name {
		case "track":
			if spec != "" {
				return fmt.Sprintf("%"+spec, vars.Track)
			}
			return fmt.Sprintf("%d", vars.Track)
		case "total_tracks":
			return fmt.Sprintf("%d", vars.TotalTracks)
		}
		return match
	})

	result = collapseEmptySegments(result)

	// Sanitize each path component
	parts := strings.Split(result, "/")
	for i, part := range parts {
		parts[i] = sanitizePathComponent(part)
	}
	result = strings.Join(parts, "/")

	// Final cleanup of empty segments after sanitization
	for strings.Contains(result, "//") {
		result = strings.ReplaceAll(result, "//", "/")
	}
	result = strings.Trim(result, "/")

	return result
}

// collapseEmptySegments cleans up paths with empty variable substitutions.
func collapseEmptySegments(path string) string {
	for strings.Contains(path, "..") {
		path = strings.ReplaceAll(path, "..", ".")
	}
	path = strings.ReplaceAll(path, "./", "/")
	path = strings.ReplaceAll(path, "/.", "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	path = strings.Trim(path, "/.")
	return path
}

// sanitizePathComponent removes filesystem-unsafe characters from a path component.
func sanitizePathComponent(s string) string {
	replacer := strings.NewReplacer(
		"/", " ",
		"\\", " ",
		":", " -",
		"*", "",
		"?", "",
		"\"", "'",
		"<", "",
		">", "",
		"|", " -",
		"[", "",
		"]", "",
	)
	s = replacer.Replace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
