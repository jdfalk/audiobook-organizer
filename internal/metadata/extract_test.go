// file: internal/metadata/extract_test.go
// version: 1.2.0
// guid: 3e2f1a6b-7c8d-4e5f-9a0b-1c2d3e4f5a6b

package metadata

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	taglib "go.senan.xyz/taglib"
)

func copyFixture(t *testing.T, name string) string {
	t.Helper()

	fixturePath := filepath.Join("..", "..", "testdata", "fixtures", name)
	if _, err := os.Stat(fixturePath); err != nil {
		t.Skipf("fixture missing: %v", err)
	}

	dstPath := filepath.Join(t.TempDir(), name)
	src, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		t.Fatalf("create temp fixture: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	return dstPath
}

func TestExtractMetadata_UsesAlbumArtistForAuthor(t *testing.T) {
	// Arrange
	testFile := copyFixture(t, "test_sample.mp3")
	tags := map[string][]string{
		"TITLE":            {"The Title"},
		"ALBUM":            {"The Album"},
		taglib.AlbumArtist: {"Author Example"},
		taglib.Artist:      {"Narrator Example"},
	}

	if err := taglib.WriteTags(testFile, tags, 0); err != nil {
		t.Fatalf("write tags: %v", err)
	}

	// Act
	meta, err := ExtractMetadata(testFile)

	// Assert
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	if meta.Title != "The Title" {
		t.Fatalf("expected title %q, got %q", "The Title", meta.Title)
	}
	if meta.Album != "The Album" {
		t.Fatalf("expected album %q, got %q", "The Album", meta.Album)
	}
	if meta.Artist != "Author Example" {
		t.Fatalf("expected author %q, got %q", "Author Example", meta.Artist)
	}
	if meta.Narrator != "Narrator Example" {
		t.Fatalf("expected narrator %q, got %q", "Narrator Example", meta.Narrator)
	}
	if meta.UsedFilenameFallback {
		t.Fatalf("expected UsedFilenameFallback=false, got true")
	}
}

func TestExtractMetadata_ComposerOverridesAlbumArtist(t *testing.T) {
	// Arrange
	testFile := copyFixture(t, "test_sample.mp3")
	tags := map[string][]string{
		"TITLE":            {"Composer Title"},
		taglib.Composer:    {"Composer Author"},
		taglib.AlbumArtist: {"Album Artist Author"},
		taglib.Artist:      {"Narrator Example"},
		taglib.Performer:   {"Performer Narrator"},
	}

	if err := taglib.WriteTags(testFile, tags, 0); err != nil {
		t.Fatalf("write tags: %v", err)
	}

	// Act
	meta, err := ExtractMetadata(testFile)

	// Assert
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	if meta.Artist != "Composer Author" {
		t.Fatalf("expected author %q, got %q", "Composer Author", meta.Artist)
	}
	if meta.Narrator != "Performer Narrator" {
		t.Fatalf("expected narrator %q, got %q", "Performer Narrator", meta.Narrator)
	}
	if meta.UsedFilenameFallback {
		t.Fatalf("expected UsedFilenameFallback=false, got true")
	}
}

func TestExtractMetadata_FallbackFlagOnReadError(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Author Name - Book Title.mp3")
	if err := os.WriteFile(filePath, []byte("not audio"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Act
	meta, err := ExtractMetadata(filePath)

	// Assert
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	if !meta.UsedFilenameFallback {
		t.Fatalf("expected UsedFilenameFallback=true, got false")
	}
	if meta.Title != "Book Title" {
		t.Fatalf("expected title %q, got %q", "Book Title", meta.Title)
	}
	if meta.Artist != "Author Name" {
		t.Fatalf("expected author %q, got %q", "Author Name", meta.Artist)
	}
}

func TestExtractMetadata_DirectoryPath(t *testing.T) {
	// Create a directory named like an audiobook
	dir := filepath.Join(t.TempDir(), "Author Name - Book Title")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}

	// Should not error — should fall back to filename parsing
	meta, err := ExtractMetadata(dir)
	if err != nil {
		t.Fatalf("expected no error for directory, got: %v", err)
	}
	if !meta.UsedFilenameFallback {
		t.Fatal("expected UsedFilenameFallback to be true for directory")
	}
}
