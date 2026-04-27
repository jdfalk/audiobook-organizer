// file: internal/tagger/embed_cover_test.go
// version: 2.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package tagger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbedCoverArt_EmptyAudioPath(t *testing.T) {
	t.Parallel()
	if err := EmbedCoverArt("", "/some/cover.jpg"); err == nil {
		t.Error("expected error for empty audio path")
	}
}

func TestEmbedCoverArt_EmptyCoverPath(t *testing.T) {
	t.Parallel()
	if err := EmbedCoverArt("/some/audio.mp3", ""); err == nil {
		t.Error("expected error for empty cover path")
	}
}

func TestEmbedCoverArt_MissingAudioFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	coverPath := filepath.Join(dir, "cover.jpg")
	os.WriteFile(coverPath, []byte("fake"), 0644) //nolint:errcheck
	err := EmbedCoverArt("/nonexistent/audio.mp3", coverPath)
	if err == nil {
		t.Error("expected error for missing audio file")
	}
}

func TestEmbedCoverArt_MissingCoverFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.mp3")
	os.WriteFile(audioPath, []byte("fake audio"), 0644) //nolint:errcheck
	err := EmbedCoverArt(audioPath, "/nonexistent/cover.jpg")
	if err == nil {
		t.Error("expected error for missing cover file")
	}
}
