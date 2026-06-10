// file: internal/tagger/tagger.go
// version: 1.4.0
// guid: 3b4c5d6e-7f8a-9b0c-1d2e-3f4a5b6c7d8e

package tagger

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// UpdateSeriesTags updates the audio files with series metadata tags.
// NOTE: This function used the legacy global database.DB (SQLite) which was
// removed in fable5 TASK-022. Use the tag-writing pipeline via the Store API
// (server/handlers/tags.go) for production workflows.
func UpdateSeriesTags() error {
	return fmt.Errorf("UpdateSeriesTags: the legacy SQLite path was removed in fable5 T022; use the Store-backed tag-write pipeline instead")
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
	slog.Info("tagger would update M4B tags", "path", filePath, "series", seriesTag)
	return nil
}

// updateMP3Tags updates tags for MP3 files
func updateMP3Tags(filePath, seriesTag string) error {
	// In a real implementation, you would call eyeD3 like:
	// cmd := exec.Command("eyeD3", "--text-frame=TGID:"+seriesTag, filePath)
	// return cmd.Run()

	// Placeholder implementation
	slog.Info("tagger would update MP3 tags", "path", filePath, "series", seriesTag)
	return nil
}

// updateFLACTags updates tags for FLAC files
func updateFLACTags(filePath, seriesTag string) error {
	// In a real implementation, you would call metaflac like:
	// cmd := exec.Command("metaflac", "--set-tag=CONTENTGROUP="+seriesTag, filePath)
	// return cmd.Run()

	// Placeholder implementation
	slog.Info("tagger would update FLAC tags", "path", filePath, "series", seriesTag)
	return nil
}
