// file: internal/tagger/embed_cover.go
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package tagger

import (
	"fmt"
	"os"

	"go.senan.xyz/taglib"
)

// EmbedCoverArt embeds a cover image into an audio file using TagLib.
// Supports MP3, M4A, M4B, AAC, OGG, and FLAC — no external tools required.
func EmbedCoverArt(audioPath string, coverPath string) error {
	if audioPath == "" {
		return fmt.Errorf("empty audio path")
	}
	if coverPath == "" {
		return fmt.Errorf("empty cover path")
	}
	if _, err := os.Stat(audioPath); err != nil {
		return fmt.Errorf("audio file not found: %w", err)
	}
	data, err := os.ReadFile(coverPath)
	if err != nil {
		return fmt.Errorf("cover file not found: %w", err)
	}
	if err := taglib.WriteImage(audioPath, data); err != nil {
		return fmt.Errorf("embed cover art in %s: %w", audioPath, err)
	}
	return nil
}
