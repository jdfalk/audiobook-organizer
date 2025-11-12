// file: internal/metadata/taglib_support.go
// version: 1.1.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

//go:build taglib
// +build taglib

// TagLib native writer support (optional via build tag 'taglib'). Default build without tag excludes this file.

package metadata

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/sentriz/go-taglib/taglib"
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
	f, err := taglib.Read(abs)
	if err != nil {
		return fmt.Errorf("taglib read failed: %w", err)
	}
	defer f.Close()

	if title, ok := metadata["title"].(string); ok && title != "" {
		f.SetTitle(title)
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		f.SetArtist(artist)
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		f.SetAlbum(album)
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		f.SetGenre(genre)
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		f.SetYear(year)
	}

	if err := f.Save(); err != nil {
		// Restore on failure
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("taglib save failed and restore failed: save=%w restore=%v", err, restoreErr)
		}
		return fmt.Errorf("taglib save failed (restored): %w", err)
	}
	return nil
}
