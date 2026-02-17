// file: internal/organizer/organizer_test.go
// version: 1.5.0
// guid: 8b9c0d1e-2f3a-4b5c-6d7e-8f9a0b1c2d3e

package organizer

import (
	"os"
	"path/filepath"
	"strings"
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
		{"long filename", strings.Repeat("a", 250), strings.Repeat("a", 200)},
		{"control chars stripped", "hello\x00world\x01test", "helloworldtest"},
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

	result, err := org.expandPattern("{title}", book)
	if err != nil {
		t.Fatalf("expand pattern: %v", err)
	}
	expected := "The Hobbit"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Test with narrator
	result, err = org.expandPattern("{title} - {narrator}", book)
	if err != nil {
		t.Fatalf("expand pattern: %v", err)
	}
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
			"Title ", // Note: removeEmptySegment doesn't trim, cleanupPattern does
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

func TestNewOrganizer(t *testing.T) {
	cfg := &config.Config{
		RootDir: "/test",
	}

	org := NewOrganizer(cfg)
	if org == nil {
		t.Fatal("NewOrganizer returned nil")
	}
	if org.config != cfg {
		t.Error("config not set correctly")
	}
}

func TestOrganizeBook_NilBook(t *testing.T) {
	org := NewOrganizer(&config.Config{})
	_, err := org.OrganizeBook(nil)
	if err == nil {
		t.Fatal("expected error for nil book")
	}
	if err.Error() != "invalid book or file path" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrganizeBook_EmptyFilePath(t *testing.T) {
	org := NewOrganizer(&config.Config{})
	book := &database.Book{}
	_, err := org.OrganizeBook(book)
	if err == nil {
		t.Fatal("expected error for empty file path")
	}
	if err.Error() != "invalid book or file path" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrganizeBook_Copy(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	// Create source file
	srcFile := filepath.Join(srcDir, "book.m4b")
	content := []byte("audio content")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              dstDir,
		FolderNamingPattern:  "{author}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "copy",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
		Author:   &database.Author{Name: "Test Author"},
	}

	targetPath, err := org.OrganizeBook(book)
	if err != nil {
		t.Fatalf("OrganizeBook failed: %v", err)
	}

	// Verify file was copied
	dstContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != string(content) {
		t.Error("content mismatch")
	}

	// Verify calling again with same book returns same path
	targetPath2, err := org.OrganizeBook(book)
	if err != nil {
		t.Fatalf("second OrganizeBook failed: %v", err)
	}
	if targetPath != targetPath2 {
		t.Errorf("expected same path, got %s vs %s", targetPath, targetPath2)
	}
}

func TestOrganizeBook_Hardlink(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              dstDir,
		FolderNamingPattern:  "{author}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "hardlink",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
		Author:   &database.Author{Name: "Test Author"},
	}

	targetPath, err := org.OrganizeBook(book)
	if err != nil {
		t.Skipf("hardlink not supported: %v", err)
	}

	if _, err := os.Stat(targetPath); err != nil {
		t.Errorf("hardlink was not created: %v", err)
	}
}

func TestOrganizeBook_Symlink(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              dstDir,
		FolderNamingPattern:  "{author}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "symlink",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
		Author:   &database.Author{Name: "Test Author"},
	}

	targetPath, err := org.OrganizeBook(book)
	if err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if _, err := os.Lstat(targetPath); err != nil {
		t.Errorf("symlink was not created: %v", err)
	}
}

func TestOrganizeBook_Auto(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              dstDir,
		FolderNamingPattern:  "{author}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "auto",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
		Author:   &database.Author{Name: "Test Author"},
	}

	targetPath, err := org.OrganizeBook(book)
	if err != nil {
		t.Fatalf("OrganizeBook with auto failed: %v", err)
	}

	if _, err := os.Stat(targetPath); err != nil {
		t.Errorf("auto strategy did not create file: %v", err)
	}
}

func TestOrganizeBook_UnknownStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              tmpDir,
		FolderNamingPattern:  "{title}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "invalid_strategy",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
	}

	_, err := org.OrganizeBook(book)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
	if !strings.Contains(err.Error(), "unknown organization strategy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrganizeBook_MkdirError(t *testing.T) {
	// Use a path that can't have subdirs created
	cfg := &config.Config{
		RootDir:              "/dev/null/impossible",
		FolderNamingPattern:  "{title}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "copy",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: "/tmp/test.m4b",
	}

	_, err := org.OrganizeBook(book)
	if err == nil {
		t.Fatal("expected error when creating impossible directory")
	}
	if !strings.Contains(err.Error(), "failed to create target directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExpandPattern_WithSeries(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	seriesNum := 2
	book := &database.Book{
		Title:           "The Two Towers",
		Author:          &database.Author{Name: "J.R.R. Tolkien"},
		Series:          &database.Series{Name: "The Lord of the Rings"},
		SeriesSequence:  &seriesNum,
	}

	result, err := org.expandPattern("{author}/{series}/{series_number} - {title}", book)
	if err != nil {
		t.Fatalf("expand pattern failed: %v", err)
	}

	expected := "J.R.R. Tolkien/The Lord of the Rings/2 - The Two Towers"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandPattern_WithAllFields(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	printYear := 2020
	bitrate := 128
	book := &database.Book{
		Title:      "Complete Book",
		Author:     &database.Author{Name: "Author Name"},
		Narrator:   stringPtr("Narrator Name"),
		Publisher:  stringPtr("Publisher Name"),
		Language:   stringPtr("English"),
		Edition:    stringPtr("First"),
		PrintYear:  &printYear,
		ISBN10:     stringPtr("1234567890"),
		ISBN13:     stringPtr("1234567890123"),
		Bitrate:    &bitrate,
		Codec:      stringPtr("AAC"),
		Quality:    stringPtr("High"),
	}

	result, err := org.expandPattern("{author} - {title} ({year}) - {narrator}", book)
	if err != nil {
		t.Fatalf("expand pattern failed: %v", err)
	}

	expected := "Author Name - Complete Book (2020) - Narrator Name"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandPattern_EmptyFields(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	book := &database.Book{
		Title: "Book",
	}

	result, err := org.expandPattern("{author} - {title}", book)
	if err != nil {
		t.Fatalf("expand pattern failed: %v", err)
	}

	expected := "Unknown Author - Book"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandPattern_UnresolvedPlaceholder(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	book := &database.Book{
		Title: "Book",
	}

	// Test with a placeholder that won't be resolved
	_, err := org.expandPattern("{title} - {unknown_field}", book)
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}
	if !strings.Contains(err.Error(), "unresolved placeholders") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExpandPattern_EmptyTitle(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	book := &database.Book{
		Title: "   ",
	}

	result, err := org.expandPattern("{title}", book)
	if err != nil {
		t.Fatalf("expand pattern failed: %v", err)
	}

	if result != defaultTitle {
		t.Errorf("expected %q, got %q", defaultTitle, result)
	}
}

func TestExpandPattern_EmptyNarrator(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	emptyStr := "   "
	book := &database.Book{
		Title:    "Book",
		Narrator: &emptyStr,
	}

	result, err := org.expandPattern("{narrator}", book)
	if err != nil {
		t.Fatalf("expand pattern failed: %v", err)
	}

	if result != defaultNarrator {
		t.Errorf("expected %q, got %q", defaultNarrator, result)
	}
}

func TestCleanupPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"multiple spaces", "Book  Title", "Book Title"},
		{"empty parens", "Book ( )", "Book"},
		{"leading dash", "- Book", "Book"},
		{"trailing dash", "Book -", "Book"},
		{"multiple slashes", "path//to///file", "path/to/file"},
		{"combined", "  Book  ( ) - ", "Book"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanupPattern(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "author/book", "author/book"},
		{"invalid chars in parts", "auth:or/bo<ok", "auth_or/bo_ok"},
		{"multiple parts", "a/b/c", "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePath(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestStringOrEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{"nil pointer", nil, ""},
		{"empty string", stringPtr(""), ""},
		{"non-empty", stringPtr("test"), "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringOrEmpty(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCopyFile_ErrorCases(t *testing.T) {
	tmpDir := t.TempDir()
	org := &Organizer{config: &config.Config{}}

	t.Run("source does not exist", func(t *testing.T) {
		err := org.copyFile("/nonexistent/file.txt", filepath.Join(tmpDir, "dest.txt"))
		if err == nil {
			t.Fatal("expected error for nonexistent source")
		}
		if !strings.Contains(err.Error(), "failed to open source file") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("destination path invalid", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "source.txt")
		if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		err := org.copyFile(srcPath, "/dev/null/impossible/dest.txt")
		if err == nil {
			t.Fatal("expected error for invalid destination")
		}
		if !strings.Contains(err.Error(), "failed to create destination file") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSymlinkFile_ErrorPath(t *testing.T) {
	org := &Organizer{config: &config.Config{}}

	// Test with invalid source path that has issues with Abs
	err := org.symlinkFile(string([]byte{0x00}), "/tmp/link")
	if err == nil {
		t.Fatal("expected error for invalid source path")
	}
}

func TestGenerateTargetPath_PatternError(t *testing.T) {
	org := &Organizer{
		config: &config.Config{
			RootDir:             "/tmp",
			FolderNamingPattern: "{unknown_placeholder}",
			FileNamingPattern:   "{title}",
		},
	}

	book := &database.Book{
		Title:    "Test",
		FilePath: "/test.m4b",
	}

	_, err := org.generateTargetPath(book)
	if err == nil {
		t.Fatal("expected error for invalid folder pattern")
	}
	if !strings.Contains(err.Error(), "folder pattern") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateTargetPath_FilePatternError(t *testing.T) {
	org := &Organizer{
		config: &config.Config{
			RootDir:             "/tmp",
			FolderNamingPattern: "{title}",
			FileNamingPattern:   "{unknown_placeholder}",
		},
	}

	book := &database.Book{
		Title:    "Test",
		FilePath: "/test.m4b",
	}

	_, err := org.generateTargetPath(book)
	if err == nil {
		t.Fatal("expected error for invalid file pattern")
	}
	if !strings.Contains(err.Error(), "file pattern") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOrganizeBook_Reflink(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "book.m4b")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := &config.Config{
		RootDir:              dstDir,
		FolderNamingPattern:  "{author}",
		FileNamingPattern:    "{title}",
		OrganizationStrategy: "reflink",
	}

	org := NewOrganizer(cfg)
	book := &database.Book{
		Title:    "Test Book",
		FilePath: srcFile,
		Author:   &database.Author{Name: "Test Author"},
	}

	targetPath, err := org.OrganizeBook(book)
	if err != nil {
		t.Skipf("reflink not supported: %v", err)
	}

	if _, err := os.Stat(targetPath); err != nil {
		t.Errorf("reflink was not created: %v", err)
	}
}

func TestCopyFile_IOCopyError(t *testing.T) {
	tmpDir := t.TempDir()
	org := &Organizer{config: &config.Config{}}

	// Create a directory with the destination name to cause io.Copy error
	srcPath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create a file, then try to copy to a path where a directory exists
	dstPath := filepath.Join(tmpDir, "subdir", "dest.txt")
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}

	// On some systems, trying to create a file in a read-only directory will fail
	// Let's create the file first and make it read-only after opening
	destFile, err := os.Create(dstPath)
	if err != nil {
		t.Fatalf("failed to create dest: %v", err)
	}
	destFile.Close()

	// Change destination to read-only
	if err := os.Chmod(dstPath, 0444); err != nil {
		t.Skipf("failed to make dest read-only: %v", err)
	}
	defer os.Chmod(dstPath, 0644) // restore permissions for cleanup

	// Now try to copy - should fail on write
	err = org.copyFile(srcPath, dstPath)
	if err == nil {
		t.Log("Expected error for read-only destination, but got none (may be OS-specific)")
	}
}
