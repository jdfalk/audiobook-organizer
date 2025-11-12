// file: internal/metadata/taglib_support.go
// version: 1.3.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

//go:build taglib
// +build taglib

// TagLib native writer support (optional via build tag 'taglib'). Default build without tag excludes this file.

package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	taglib "go.senan.xyz/taglib"
)

// taglibAvailable indicates native taglib path compiled in
var taglibAvailable = true

// writeMetadataWithTaglib performs native metadata writing using TagLib.
// Supports basic fields; extended custom fields still require CLI fallback.
func writeMetadataWithTaglib(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("taglib backup failed: %w", err)
	}
	defer func() {
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	abs, _ := filepath.Abs(filePath)

	// Build tag map (map[string][]string) according to README examples.
	// Use standard common tag names; TagLib accepts arbitrary keys.
	tags := make(map[string][]string)

	if title, ok := metadata["title"].(string); ok && title != "" {
		tags["TITLE"] = []string{title}
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		// Prefer ALBUMARTIST if we have a single artist (semantic for audiobooks narrator/author)
		tags[taglib.AlbumArtist] = []string{artist}
		tags["ARTIST"] = []string{artist}
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		tags[taglib.Album] = []string{album}
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		tags["GENRE"] = []string{genre}
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		tags["DATE"] = []string{fmt.Sprintf("%d", year)}
	}
	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		tags["NARRATOR"] = []string{narrator}
	}
	// Include language/publisher as custom tags if present
	if lang, ok := metadata["language"].(string); ok && lang != "" {
		tags["LANGUAGE"] = []string{strings.ToLower(lang)}
	}
	if pub, ok := metadata["publisher"].(string); ok && pub != "" {
		tags["PUBLISHER"] = []string{pub}
	}
	if isbn10, ok := metadata["isbn10"].(string); ok && isbn10 != "" {
		tags["ISBN10"] = []string{isbn10}
	}
	if isbn13, ok := metadata["isbn13"].(string); ok && isbn13 != "" {
		tags["ISBN13"] = []string{isbn13}
	}

	if len(tags) == 0 {
		return fmt.Errorf("no writable metadata supplied")
	}

	if err := taglib.WriteTags(abs, tags, 0); err != nil {
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("taglib write failed and restore failed: write=%w restore=%v", err, restoreErr)
		}
		return fmt.Errorf("taglib write failed (restored): %w", err)
	}
	return nil
}
