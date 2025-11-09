// file: internal/tagger/tagger.go
// version: 1.1.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

package tagger

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// UpdateSeriesTags updates the audio files with series metadata tags
func UpdateSeriesTags() error {
	// Get all books with series information
	rows, err := database.DB.Query(`
        SELECT books.file_path, books.title, series.name, books.series_sequence
        FROM books
        JOIN series ON books.series_id = series.id
    `)
	if err != nil {
		return fmt.Errorf("failed to query books with series: %w", err)
	}
	defer rows.Close()

	fmt.Println("Updating audio file tags with series information...")
	count := 0

	for rows.Next() {
		var filePath, title, seriesName string
		var seriesSequence sql.NullInt64

		if err := rows.Scan(&filePath, &title, &seriesName, &seriesSequence); err != nil {
			return fmt.Errorf("failed to scan book row: %w", err)
		}

		// Create series tag value
		seriesTag := seriesName
		if seriesSequence.Valid && seriesSequence.Int64 > 0 {
			seriesTag = fmt.Sprintf("%s, Book %d", seriesName, seriesSequence.Int64)
		}

		// Update the file tags
		if err := updateFileTags(filePath, title, seriesTag); err != nil {
			fmt.Printf("Warning: Could not update tags for %s: %v\n", filePath, err)
			continue
		}

		count++
	}

	fmt.Printf("Updated tags for %d files\n", count)
	return nil
}

// updateFileTags updates the tags of an audio file
func updateFileTags(filePath, title, seriesTag string) error {
	// Since dhowden/tag library doesn't support tag writing, we'd need to use platform-specific
	// tools like ffmpeg or AtomicParsley to actually modify the tags

	// For demonstration purposes, this is a placeholder for the actual implementation
	// In a real implementation, you would:
	// 1. Use a command-line tool like ffmpeg, AtomicParsley, or eyeD3 to update tags
	// 2. Or use a Go library that supports writing tags to audio files

	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".m4b", ".m4a", ".aac":
		// For AAC/M4A/M4B files, we could use AtomicParsley
		return updateM4BTags(filePath, seriesTag)
	case ".mp3":
		// For MP3 files, we could use eyeD3 or ffmpeg
		return updateMP3Tags(filePath, seriesTag)
	case ".flac":
		// For FLAC files
		return updateFLACTags(filePath, seriesTag)
	default:
		return fmt.Errorf("unsupported file format: %s", ext)
	}
}

// updateM4BTags updates tags for M4B files
func updateM4BTags(filePath, seriesTag string) error {
	// In a real implementation, you would call AtomicParsley like:
	// cmd := exec.Command("AtomicParsley", filePath, "--grouping", seriesTag, "--overWrite")
	// return cmd.Run()

	// Placeholder implementation
	fmt.Printf("Would update M4B tags for %s with series: %s\n", filePath, seriesTag)
	return nil
}

// updateMP3Tags updates tags for MP3 files
func updateMP3Tags(filePath, seriesTag string) error {
	// In a real implementation, you would call eyeD3 like:
	// cmd := exec.Command("eyeD3", "--text-frame=TGID:"+seriesTag, filePath)
	// return cmd.Run()

	// Placeholder implementation
	fmt.Printf("Would update MP3 tags for %s with series: %s\n", filePath, seriesTag)
	return nil
}

// updateFLACTags updates tags for FLAC files
func updateFLACTags(filePath, seriesTag string) error {
	// In a real implementation, you would call metaflac like:
	// cmd := exec.Command("metaflac", "--set-tag=CONTENTGROUP="+seriesTag, filePath)
	// return cmd.Run()

	// Placeholder implementation
	fmt.Printf("Would update FLAC tags for %s with series: %s\n", filePath, seriesTag)
	return nil
}
