// file: internal/server/file_pipeline.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// FileRenameEntry represents a planned file rename operation.
type FileRenameEntry struct {
	SegmentID  string `json:"segment_id"`
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
}

// FilePipelineResult holds the results of a file pipeline operation.
type FilePipelineResult struct {
	Entries []FileRenameEntry `json:"entries"`
	Renamed int               `json:"renamed"`
	Errors  []string          `json:"errors,omitempty"`
}

// ComputeTargetPaths computes the target file paths for all segments of a book
// using the path format template and format variables.
func ComputeTargetPaths(rootDir, pathFormat, segTitleFormat string, book *database.Book, segments []database.BookSegment, vars FormatVars) []FileRenameEntry {
	if rootDir == "" || len(segments) == 0 {
		return nil
	}

	// Sort segments by track number then filepath
	sorted := make([]database.BookSegment, len(segments))
	copy(sorted, segments)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].TrackNumber
		tj := sorted[j].TrackNumber
		if ti != nil && tj != nil {
			if *ti != *tj {
				return *ti < *tj
			}
		} else if ti != nil {
			return true
		} else if tj != nil {
			return false
		}
		return sorted[i].FilePath < sorted[j].FilePath
	})

	totalTracks := len(sorted)
	var entries []FileRenameEntry

	for i, seg := range sorted {
		if !seg.Active {
			continue
		}

		trackNum := i + 1
		if seg.TrackNumber != nil {
			trackNum = *seg.TrackNumber
		}

		ext := strings.TrimPrefix(filepath.Ext(seg.FilePath), ".")
		if ext == "" {
			ext = seg.Format
		}

		segVars := vars
		segVars.Track = trackNum
		segVars.TotalTracks = totalTracks
		segVars.Ext = ext

		// Compute segment title
		if segTitleFormat == "" {
			segTitleFormat = DefaultSegmentTitleFormat
		}
		segVars.TrackTitle = FormatSegmentTitle(segTitleFormat, vars.Title, trackNum, totalTracks)

		if pathFormat == "" {
			pathFormat = DefaultPathFormat
		}
		relPath := FormatPath(pathFormat, segVars)
		targetPath := filepath.Join(rootDir, relPath)

		if targetPath != seg.FilePath {
			entries = append(entries, FileRenameEntry{
				SegmentID:  seg.ID,
				SourcePath: seg.FilePath,
				TargetPath: targetPath,
			})
		}
	}

	return entries
}

// RenameFiles performs atomic file renames using a temp intermediate step
// to avoid conflicts when source and target overlap.
func RenameFiles(entries []FileRenameEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Phase 1: rename source -> temp
	type tempEntry struct {
		TempPath string
		Entry    FileRenameEntry
	}
	var temps []tempEntry

	for _, entry := range entries {
		// Ensure target directory exists
		targetDir := filepath.Dir(entry.TargetPath)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("create target dir %s: %w", targetDir, err)
		}

		// Check source exists
		if _, err := os.Stat(entry.SourcePath); os.IsNotExist(err) {
			return fmt.Errorf("source file does not exist: %s", entry.SourcePath)
		}

		tempPath := entry.TargetPath + ".tmp-rename"
		if err := os.Rename(entry.SourcePath, tempPath); err != nil {
			// Rollback temps already moved
			for _, t := range temps {
				_ = os.Rename(t.TempPath, t.Entry.SourcePath)
			}
			return fmt.Errorf("rename %s -> temp: %w", entry.SourcePath, err)
		}
		temps = append(temps, tempEntry{TempPath: tempPath, Entry: entry})
	}

	// Phase 2: rename temp -> final
	for _, t := range temps {
		if err := os.Rename(t.TempPath, t.Entry.TargetPath); err != nil {
			return fmt.Errorf("rename temp -> %s: %w", t.Entry.TargetPath, err)
		}
	}

	return nil
}
