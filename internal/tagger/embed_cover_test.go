// file: internal/tagger/embed_cover_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package tagger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbedCoverArt_EmptyPaths(t *testing.T) {
	t.Parallel()
	if err := EmbedCoverArt("", "/some/cover.jpg"); err == nil {
		t.Error("expected error for empty audio path")
	}
	if err := EmbedCoverArt("/some/audio.mp3", ""); err == nil {
		t.Error("expected error for empty cover path")
	}
}

func TestEmbedCoverArt_MissingFiles(t *testing.T) {
	t.Parallel()
	err := EmbedCoverArt("/nonexistent/audio.mp3", "/nonexistent/cover.jpg")
	if err == nil {
		t.Error("expected error for missing audio file")
	}
}

func TestEmbedCoverArt_UnsupportedFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.wav")
	coverPath := filepath.Join(dir, "cover.jpg")
	os.WriteFile(audioPath, []byte("fake"), 0644)
	os.WriteFile(coverPath, []byte("fake"), 0644)

	err := EmbedCoverArt(audioPath, coverPath)
	if err == nil {
		t.Error("expected error for unsupported format .wav")
	}
	if err != nil && !containsStr(err.Error(), "unsupported audio format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmbedCoverArt_MissingFFmpeg(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.mp3")
	coverPath := filepath.Join(dir, "cover.jpg")
	os.WriteFile(audioPath, []byte("fake audio"), 0644)
	os.WriteFile(coverPath, []byte("fake image"), 0644)

	// Temporarily clear PATH so ffmpeg won't be found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	err := EmbedCoverArt(audioPath, coverPath)
	if err == nil {
		t.Error("expected error when ffmpeg is missing")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestEmbedCoverArt_MissingMetaflac(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.flac")
	coverPath := filepath.Join(dir, "cover.jpg")
	os.WriteFile(audioPath, []byte("fake audio"), 0644)
	os.WriteFile(coverPath, []byte("fake image"), 0644)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	err := EmbedCoverArt(audioPath, coverPath)
	if err == nil {
		t.Error("expected error when metaflac is missing")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestFindTool_NotFound(t *testing.T) {
	t.Parallel()
	_, err := findTool("definitely_not_a_real_tool_abc123")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestEmbedCoverArt_AllFormats_MissingTool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	coverPath := filepath.Join(dir, "cover.jpg")
	os.WriteFile(coverPath, []byte("fake"), 0644)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	formats := []string{".mp3", ".m4b", ".m4a", ".aac", ".ogg", ".flac"}
	for _, ext := range formats {
		t.Run(ext, func(t *testing.T) {
			audioPath := filepath.Join(dir, "test"+ext)
			os.WriteFile(audioPath, []byte("fake"), 0644)
			err := EmbedCoverArt(audioPath, coverPath)
			if err == nil {
				t.Errorf("expected error for %s with no tools", ext)
			}
		})
	}
}

// contains checks if substr is in s (avoids importing strings in test).
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
