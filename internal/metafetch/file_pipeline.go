// file: internal/metafetch/file_pipeline.go
// version: 1.2.0
// guid: b2c3d4e5-f6a7-8901-bcde-f01234567890

package metafetch

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

// ComputeTargetPaths computes the target file paths for all files of a book
// using the path format template and format variables.
func ComputeTargetPaths(rootDir, pathFormat, segTitleFormat string, book *database.Book, files []database.BookFile, vars FormatVars) []FileRenameEntry {
	if rootDir == "" || len(files) == 0 {
		return nil
	}

	// Sort files by track number then filepath
	sorted := make([]database.BookFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].TrackNumber
		tj := sorted[j].TrackNumber
		if ti != 0 && tj != 0 {
			if ti != tj {
				return ti < tj
			}
		} else if ti != 0 {
			return true
		} else if tj != 0 {
			return false
		}
		return sorted[i].FilePath < sorted[j].FilePath
	})

	totalTracks := len(sorted)
	var entries []FileRenameEntry

	for i, f := range sorted {
		if f.Missing {
			continue
		}

		trackNum := i + 1
		if f.TrackNumber != 0 {
			trackNum = f.TrackNumber
		}

		ext := strings.TrimPrefix(filepath.Ext(f.FilePath), ".")
		if ext == "" {
			ext = f.Format
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

		if targetPath != f.FilePath {
			entries = append(entries, FileRenameEntry{
				SegmentID:  f.ID,
				SourcePath: f.FilePath,
				TargetPath: targetPath,
			})
		}
	}

	return entries
}

// ComputeTargetPathsFromSegments is a backward-compatible wrapper that accepts
// BookSegment slices and converts them to BookFile before computing paths.
// Deprecated: callers should use ComputeTargetPaths with []BookFile directly.
func ComputeTargetPathsFromSegments(rootDir, pathFormat, segTitleFormat string, book *database.Book, segments []database.BookSegment, vars FormatVars) []FileRenameEntry {
	files := make([]database.BookFile, 0, len(segments))
	for _, seg := range segments {
		trackNum := 0
		if seg.TrackNumber != nil {
			trackNum = *seg.TrackNumber
		}
		trackCount := 0
		if seg.TotalTracks != nil {
			trackCount = *seg.TotalTracks
		}
		bf := database.BookFile{
			ID:          seg.ID,
			BookID:      fmt.Sprintf("%d", seg.BookID),
			FilePath:    seg.FilePath,
			Format:      seg.Format,
			FileSize:    seg.SizeBytes,
			Duration:    seg.DurationSec * 1000, // seconds to milliseconds
			TrackNumber: trackNum,
			TrackCount:  trackCount,
			Missing:     !seg.Active,
		}
		if seg.FileHash != nil {
			bf.FileHash = *seg.FileHash
		}
		if seg.SegmentTitle != nil {
			bf.Title = *seg.SegmentTitle
		}
		files = append(files, bf)
	}
	return ComputeTargetPaths(rootDir, pathFormat, segTitleFormat, book, files, vars)
}

// RenameResult holds the outcome of a rename operation.
type RenameResult struct {
	Succeeded []FileRenameEntry `json:"succeeded"`
	Skipped   []FileRenameEntry `json:"skipped"` // source not found
	Errors    []string          `json:"errors,omitempty"`
}

// RenameFiles performs atomic file renames using a temp intermediate step
// to avoid conflicts when source and target overlap.
// Missing source files are skipped (not fatal) and reported in the result.
func RenameFiles(entries []FileRenameEntry) (*RenameResult, error) {
	result := &RenameResult{}
	if len(entries) == 0 {
		return result, nil
	}

	// Pre-filter: skip entries where source doesn't exist
	var valid []FileRenameEntry
	for _, entry := range entries {
		if _, err := os.Stat(entry.SourcePath); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, entry)
			continue
		}
		valid = append(valid, entry)
	}

	if len(valid) == 0 {
		return result, nil
	}

	// Phase 1: rename source -> temp
	type tempEntry struct {
		TempPath string
		Entry    FileRenameEntry
	}
	var temps []tempEntry

	for _, entry := range valid {
		// Ensure target directory exists
		targetDir := filepath.Dir(entry.TargetPath)
		if err := os.MkdirAll(targetDir, 0o775); err != nil {
			return result, fmt.Errorf("create target dir %s: %w", targetDir, err)
		}

		tempPath := entry.TargetPath + ".tmp-rename"
		if err := os.Rename(entry.SourcePath, tempPath); err != nil {
			// Rollback temps already moved
			for _, t := range temps {
				_ = os.Rename(t.TempPath, t.Entry.SourcePath)
			}
			return result, fmt.Errorf("rename %s -> temp: %w", entry.SourcePath, err)
		}
		temps = append(temps, tempEntry{TempPath: tempPath, Entry: entry})
	}

	// Phase 2: rename temp -> final
	for _, t := range temps {
		if err := os.Rename(t.TempPath, t.Entry.TargetPath); err != nil {
			return result, fmt.Errorf("rename temp -> %s: %w", t.Entry.TargetPath, err)
		}
		result.Succeeded = append(result.Succeeded, t.Entry)
	}

	return result, nil
}

// RelocateRequest represents a request to relocate book files.
type RelocateRequest struct {
	SegmentID  string `json:"segment_id,omitempty"`
	NewPath    string `json:"new_path,omitempty"`
	FolderPath string `json:"folder_path,omitempty"`
}

// RelocateResult holds the outcome of a relocate operation.
type RelocateResult struct {
	Updated int      `json:"updated"`
	Errors  []string `json:"errors,omitempty"`
}
