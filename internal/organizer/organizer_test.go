// file: internal/organizer/organizer_test.go
// version: 1.1.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid filename", "My Audiobook", "My Audiobook"},
		{"invalid chars", "Book:Title?", "Book_Title_"},
		{"multiple spaces", "Book  Title", "Book Title"},
		{"long filename", string(make([]byte, 250)), string(make([]byte, 200))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if tt.name == "long filename" {
				if len(result) != 200 {
					t.Errorf("expected length 200, got %d", len(result))
				}
			} else if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExpandPattern(t *testing.T) {
	org := &Organizer{
		config: &config.Config{
			FolderNamingPattern: "{title}",
			FileNamingPattern:   "{title}",
		},
	}

	book := &database.Book{
		Title:    "The Hobbit",
		Narrator: stringPtr("Rob Inglis"),
	}

	result := org.expandPattern("{title}", book)
	expected := "The Hobbit"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Test with narrator
	result = org.expandPattern("{title} - {narrator}", book)
	expected = "The Hobbit - Rob Inglis"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRemoveEmptySegment(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		placeholder string
		expected    string
	}{
		{
			"remove dash segment",
			"Title - {narrator}",
			"{narrator}",
			"Title",
		},
		{
			"remove parentheses",
			"Title ({series})",
			"{series}",
			"Title",
		},
		{
			"keep filled segment",
			"Title - Narrator",
			"{empty}",
			"Title - Narrator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeEmptySegment(tt.pattern, tt.placeholder)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerateTargetPath(t *testing.T) {
	tmpDir := t.TempDir()

	org := &Organizer{
		config: &config.Config{
			RootDir:             tmpDir,
			FolderNamingPattern: "books",
			FileNamingPattern:   "{title}",
		},
	}

	book := &database.Book{
		Title:    "The Hobbit",
		FilePath: "/source/hobbit.m4b",
	}

	targetPath, err := org.generateTargetPath(book)
	if err != nil {
		t.Fatalf("failed to generate target path: %v", err)
	}

	expected := filepath.Join(tmpDir, "books", "The Hobbit.m4b")
	if targetPath != expected {
		t.Errorf("expected %q, got %q", expected, targetPath)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy file
	org := &Organizer{config: &config.Config{}}
	dstPath := filepath.Join(tmpDir, "dest.txt")
	if err := org.copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	// Verify
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(dstContent) != string(content) {
		t.Errorf("content mismatch: expected %q, got %q", content, dstContent)
	}
}

func TestHardlinkFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Create hardlink
	org := &Organizer{config: &config.Config{}}
	dstPath := filepath.Join(tmpDir, "hardlink.txt")
	if err := org.hardlinkFile(srcPath, dstPath); err != nil {
		t.Skipf("hardlink not supported on this system: %v", err)
	}

	// Verify hardlink exists
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("hardlink was not created")
	}
}

func TestSymlinkFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Create symlink
	org := &Organizer{config: &config.Config{}}
	dstPath := filepath.Join(tmpDir, "symlink.txt")
	if err := org.symlinkFile(srcPath, dstPath); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}

	// Verify symlink exists
	if _, err := os.Lstat(dstPath); os.IsNotExist(err) {
		t.Error("symlink was not created")
	}
}
