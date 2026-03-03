// file: internal/metadata/filename_track.go
// version: 1.1.0
// guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d

package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// TrackInfo holds track/disk numbers extracted from a filename.
type TrackInfo struct {
	TrackNumber *int
	TotalTracks *int
	DiskNumber  *int
	TotalDisks  *int
}

// Patterns ordered by specificity (most specific first).
var trackPatterns = []*regexp.Regexp{
	// "Part 3 of 12", "Part 03 of 12"
	regexp.MustCompile(`(?i)part\s+(\d+)\s+of\s+(\d+)`),
	// "03 of 12", "3 of 12"
	regexp.MustCompile(`(?i)(\d+)\s+of\s+(\d+)`),
	// "Track 03", "Track03"
	regexp.MustCompile(`(?i)track\s*(\d+)`),
	// "Chapter 03", "Ch03"
	regexp.MustCompile(`(?i)(?:chapter|ch)\s*(\d+)`),
	// Leading number: "01 - Title.mp3", "01_Title.mp3"
	regexp.MustCompile(`^(\d{1,3})[\s_.\-]+`),
}

var diskPatterns = []*regexp.Regexp{
	// "Disk 2 of 3", "Disc 2 of 3"
	regexp.MustCompile(`(?i)dis[ck]\s+(\d+)\s+of\s+(\d+)`),
	// "Disk 2", "Disc02", "CD2"
	regexp.MustCompile(`(?i)(?:dis[ck]|cd)\s*(\d+)`),
}

// ExtractTrackInfoFromFilename parses track and disk numbers from a filename.
func ExtractTrackInfoFromFilename(filePath string) TrackInfo {
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	info := TrackInfo{}

	for _, pat := range trackPatterns {
		m := pat.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			info.TrackNumber = &n
		}
		if len(m) > 2 {
			if t, err := strconv.Atoi(m[2]); err == nil {
				info.TotalTracks = &t
			}
		}
		break
	}

	for _, pat := range diskPatterns {
		m := pat.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			info.DiskNumber = &n
		}
		if len(m) > 2 {
			if t, err := strconv.Atoi(m[2]); err == nil {
				info.TotalDisks = &t
			}
		}
		break
	}

	return info
}

// ExtractTrackInfoBatch parses track info from multiple filenames and extrapolates
// total tracks from the highest track number seen if not explicitly present.
func ExtractTrackInfoBatch(filePaths []string) []TrackInfo {
	results := make([]TrackInfo, len(filePaths))
	maxTrack := 0
	anyHasTotal := false

	for i, fp := range filePaths {
		results[i] = ExtractTrackInfoFromFilename(fp)
		if results[i].TrackNumber != nil && *results[i].TrackNumber > maxTrack {
			maxTrack = *results[i].TrackNumber
		}
		if results[i].TotalTracks != nil {
			anyHasTotal = true
		}
	}

	// If no file had an explicit total but we found track numbers, use file count
	if !anyHasTotal && maxTrack > 0 {
		total := len(filePaths)
		for i := range results {
			if results[i].TrackNumber != nil {
				results[i].TotalTracks = &total
			}
		}
	}

	return results
}
