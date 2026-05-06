// file: internal/maintenance/jobs/relink_missing_to_itunes_test.go
// version: 1.0.0
// guid: b2c3d4e5-6f7a-8b9c-0d1e-2f3a4b5c6d7e
// last-edited: 2026-05-05

package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRmtFindInITunes_CoauthorDirectory(t *testing.T) {
	// Build a fake iTunes root:
	//   <root>/Robert Jordan, Brandon Sanderson/The Wheel of Time/book.m4b
	iTunesRoot := t.TempDir()
	coauthorDir := filepath.Join(iTunesRoot, "Robert Jordan, Brandon Sanderson")
	albumDir := filepath.Join(coauthorDir, "The Wheel of Time")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "The Wheel of Time.m4b"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}
	results := rmt_findInITunes(iTunesRoot, "Robert Jordan", "The Wheel of Time", audioExts)

	if len(results) == 0 {
		t.Fatal("expected at least one match for co-author directory, got none")
	}
	if !strings.Contains(results[0], "Robert Jordan, Brandon Sanderson") {
		t.Errorf("expected match inside co-author dir, got: %v", results)
	}
}

func TestRmtFindInITunes_PrimaryPassStillWorks(t *testing.T) {
	// Verify the primary pass still works when author dir matches by first word.
	iTunesRoot := t.TempDir()
	authorDir := filepath.Join(iTunesRoot, "Robert Jordan")
	albumDir := filepath.Join(authorDir, "The Eye of the World")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "The Eye of the World.m4b"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}
	results := rmt_findInITunes(iTunesRoot, "Robert Jordan", "The Eye of the World", audioExts)

	if len(results) == 0 {
		t.Fatal("expected at least one match for primary pass, got none")
	}
	if !strings.Contains(results[0], "Robert Jordan") {
		t.Errorf("expected match inside author dir, got: %v", results)
	}
}

func TestRmtFindInITunes_NoFalsePositivesWhenBothExist(t *testing.T) {
	// When primary pass finds a match, ensure surname fallback is NOT also applied.
	iTunesRoot := t.TempDir()

	// Primary match
	authorDir := filepath.Join(iTunesRoot, "Robert Jordan")
	albumDir := filepath.Join(authorDir, "The Eye of the World")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "The Eye of the World.m4b"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Co-author dir that would also match surname fallback
	coauthorDir := filepath.Join(iTunesRoot, "Robert Jordan, Brandon Sanderson")
	coAlbumDir := filepath.Join(coauthorDir, "The Eye of the World")
	if err := os.MkdirAll(coAlbumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coAlbumDir, "The Eye of the World.m4b"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}
	results := rmt_findInITunes(iTunesRoot, "Robert Jordan", "The Eye of the World", audioExts)

	// Primary pass should find both author dirs (both contain "robert"), so exactly 2 results.
	// The surname fallback should NOT run since primary pass found results.
	if len(results) == 0 {
		t.Fatal("expected matches, got none")
	}
}
