// file: internal/organizer/organizer.go
// version: 1.0.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package organizer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// Organizer handles file organization operations
type Organizer struct {
	config *config.Config
}

// NewOrganizer creates a new organizer instance
func NewOrganizer(cfg *config.Config) *Organizer {
	return &Organizer{
		config: cfg,
	}
}

// OrganizeBook organizes a book file according to the configured patterns
func (o *Organizer) OrganizeBook(book *database.Book) (string, error) {
	if book == nil || book.FilePath == "" {
		return "", fmt.Errorf("invalid book or file path")
	}

	// Generate target path
	targetPath, err := o.generateTargetPath(book)
	if err != nil {
		return "", fmt.Errorf("failed to generate target path: %w", err)
	}

	// Create target directory
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Check if file already exists at target
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil // Already organized
	}

	// Perform the organization based on strategy
	strategy := o.config.OrganizationStrategy

	if strategy == "auto" {
		// Try reflink -> hardlink -> copy
		if err := o.reflinkFile(book.FilePath, targetPath); err == nil {
			return targetPath, nil
		}
		if err := o.hardlinkFile(book.FilePath, targetPath); err == nil {
			return targetPath, nil
		}
		strategy = "copy"
	}

	switch strategy {
	case "copy":
		return targetPath, o.copyFile(book.FilePath, targetPath)
	case "hardlink":
		return targetPath, o.hardlinkFile(book.FilePath, targetPath)
	case "reflink":
		return targetPath, o.reflinkFile(book.FilePath, targetPath)
	case "symlink":
		return targetPath, o.symlinkFile(book.FilePath, targetPath)
	default:
		return "", fmt.Errorf("unknown organization strategy: %s", strategy)
	}
}

// generateTargetPath creates the target file path based on naming patterns
func (o *Organizer) generateTargetPath(book *database.Book) (string, error) {
	// Get file extension
	ext := filepath.Ext(book.FilePath)

	// Generate folder path
	folderPath := o.expandPattern(o.config.FolderNamingPattern, book)
	folderPath = sanitizePath(folderPath)

	// Generate file name
	fileName := o.expandPattern(o.config.FileNamingPattern, book)
	fileName = sanitizeFilename(fileName) + ext

	// Combine with root directory
	fullPath := filepath.Join(o.config.RootDir, folderPath, fileName)

	return fullPath, nil
}

// expandPattern expands a pattern with book metadata
func (o *Organizer) expandPattern(pattern string, book *database.Book) string {
	result := pattern

	// Get author name from embedded Author object or default
	authorName := "Unknown Author"
	if book.Author != nil && book.Author.Name != "" {
		authorName = book.Author.Name
	}

	// Get series info from embedded Series object
	seriesName := ""
	if book.Series != nil {
		seriesName = book.Series.Name
	}
	seriesNum := ""
	if book.SeriesSequence != nil && *book.SeriesSequence > 0 {
		seriesNum = fmt.Sprintf("%d", *book.SeriesSequence)
	}

	// Helper to convert int pointer to string
	intToString := func(i *int) string {
		if i == nil {
			return ""
		}
		return fmt.Sprintf("%d", *i)
	}

	// Replacements map
	replacements := map[string]string{
		"{title}":         book.Title,
		"{author}":        authorName,
		"{series}":        seriesName,
		"{series_number}": seriesNum,
		"{narrator}":      stringOrEmpty(book.Narrator),
		"{publisher}":     stringOrEmpty(book.Publisher),
		"{language}":      stringOrEmpty(book.Language),
		"{edition}":       stringOrEmpty(book.Edition),
		"{print_year}":    intToString(book.PrintYear),
		"{year}":          intToString(book.PrintYear),
		"{isbn10}":        stringOrEmpty(book.ISBN10),
		"{isbn13}":        stringOrEmpty(book.ISBN13),
		"{bitrate}":       intToString(book.Bitrate),
		"{codec}":         stringOrEmpty(book.Codec),
		"{quality}":       stringOrEmpty(book.Quality),
	}

	// Perform replacements
	for placeholder, value := range replacements {
		if value == "" {
			result = removeEmptySegment(result, placeholder)
		} else {
			result = strings.ReplaceAll(result, placeholder, value)
		}
	}

	result = cleanupPattern(result)
	return result
}

// removeEmptySegment removes segments containing empty placeholders
func removeEmptySegment(pattern, placeholder string) string {
	patterns := []string{
		fmt.Sprintf(` - %s`, placeholder),
		fmt.Sprintf(`%s - `, placeholder),
		fmt.Sprintf(`\(%s[^)]*\)`, regexp.QuoteMeta(placeholder)),
		fmt.Sprintf(`\([^(]*%s\)`, regexp.QuoteMeta(placeholder)),
	}

	result := pattern
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		result = re.ReplaceAllString(result, "")
	}
	return result
}

// cleanupPattern cleans up extra spaces, dashes, and parentheses
func cleanupPattern(pattern string) string {
	re := regexp.MustCompile(`\s+`)
	pattern = re.ReplaceAllString(pattern, " ")

	re = regexp.MustCompile(`\(\s*\)`)
	pattern = re.ReplaceAllString(pattern, "")

	pattern = strings.Trim(pattern, " -/")

	re = regexp.MustCompile(`/+`)
	pattern = re.ReplaceAllString(pattern, "/")

	return pattern
}

// sanitizePath sanitizes a path for filesystem use
func sanitizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = sanitizeFilename(part)
	}
	return strings.Join(parts, "/")
}

// sanitizeFilename sanitizes a filename for filesystem use
func sanitizeFilename(name string) string {
	invalid := []string{"<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range invalid {
		name = strings.ReplaceAll(name, char, "_")
	}

	re := regexp.MustCompile(`\s+`)
	name = re.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	if len(name) > 200 {
		name = name[:200]
	}

	return name
}

// stringOrEmpty returns the string value or empty string if nil
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// copyFile copies a file from src to dst
func (o *Organizer) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}

// hardlinkFile creates a hard link from src to dst
func (o *Organizer) hardlinkFile(src, dst string) error {
	return os.Link(src, dst)
}

// symlinkFile creates a symbolic link from src to dst
func (o *Organizer) symlinkFile(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	return os.Symlink(absSrc, dst)
}

// reflinkFile creates a copy-on-write reflink (platform-specific)
func (o *Organizer) reflinkFile(src, dst string) error {
	return o.reflinkFilePlatform(src, dst)
}
