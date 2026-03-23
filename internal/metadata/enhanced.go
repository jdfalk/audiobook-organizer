// file: internal/metadata/enhanced.go
// version: 1.6.0
// guid: 7e8d9c0b-1a2f-3e4d-5c6b-7a8d9c0b1a2f

package metadata

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
)

// MetadataUpdate represents a metadata update operation
type MetadataUpdate struct {
	BookID   string                 `json:"book_id" binding:"required"`
	Updates  map[string]interface{} `json:"updates" binding:"required"`
	Validate bool                   `json:"validate"`
}

// MetadataHistory represents a historical metadata change
type MetadataHistory struct {
	ID        int       `json:"id"`
	BookID    string    `json:"book_id"`
	Field     string    `json:"field"`
	OldValue  string    `json:"old_value"`
	NewValue  string    `json:"new_value"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

// ValidationRule defines a validation constraint
type ValidationRule struct {
	Field           string
	Required        bool
	MinLength       int
	MaxLength       int
	AllowedValues   []string
	CustomValidator func(interface{}) error
}

// ErrTaglibUnavailable is returned when native taglib writing is not compiled in.
var ErrTaglibUnavailable = errors.New("taglib native writer unavailable")

// DefaultValidationRules returns default validation rules for audiobook metadata
func DefaultValidationRules() map[string]ValidationRule {
	return map[string]ValidationRule{
		"title": {
			Field:     "title",
			Required:  true,
			MinLength: 1,
			MaxLength: 500,
		},
		"author": {
			Field:     "author",
			Required:  false,
			MinLength: 0,
			MaxLength: 200,
		},
		"series": {
			Field:     "series",
			Required:  false,
			MinLength: 0,
			MaxLength: 200,
		},
		"narrator": {
			Field:     "narrator",
			Required:  false,
			MinLength: 0,
			MaxLength: 200,
		},
		"format": {
			Field:         "format",
			Required:      false,
			AllowedValues: []string{"m4b", "mp3", "m4a", "aac", "ogg", "flac", "wma"},
		},
		"publishDate": {
			Field:    "publishDate",
			Required: false,
			CustomValidator: func(v interface{}) error {
				str, ok := v.(string)
				if !ok {
					return fmt.Errorf("publishDate must be a string")
				}
				// Try parsing as date (YYYY-MM-DD format)
				_, err := time.Parse("2006-01-02", str)
				if err != nil {
					return fmt.Errorf("publishDate must be in YYYY-MM-DD format")
				}
				return nil
			},
		},
	}
}

// ValidateMetadata validates metadata updates against rules
func ValidateMetadata(updates map[string]interface{}, rules map[string]ValidationRule) []error {
	var errors []error

	for field, value := range updates {
		rule, exists := rules[field]
		if !exists {
			continue // No validation rule for this field
		}

		// Check required
		if rule.Required && (value == nil || value == "") {
			errors = append(errors, fmt.Errorf("field %s is required", field))
			continue
		}

		// Skip further validation if value is nil/empty and not required
		if value == nil || value == "" {
			continue
		}

		// Convert to string for validation
		strValue := fmt.Sprintf("%v", value)

		// Check length constraints
		if rule.MinLength > 0 && len(strValue) < rule.MinLength {
			errors = append(errors, fmt.Errorf("field %s must be at least %d characters", field, rule.MinLength))
		}
		if rule.MaxLength > 0 && len(strValue) > rule.MaxLength {
			errors = append(errors, fmt.Errorf("field %s must be at most %d characters", field, rule.MaxLength))
		}

		// Check allowed values
		if len(rule.AllowedValues) > 0 {
			valid := false
			for _, allowed := range rule.AllowedValues {
				if strings.EqualFold(strValue, allowed) {
					valid = true
					break
				}
			}
			if !valid {
				errors = append(errors, fmt.Errorf("field %s must be one of: %v", field, rule.AllowedValues))
			}
		}

		// Custom validator
		if rule.CustomValidator != nil {
			if err := rule.CustomValidator(value); err != nil {
				errors = append(errors, fmt.Errorf("field %s validation failed: %w", field, err))
			}
		}
	}

	return errors
}

// BatchUpdateMetadata applies metadata updates to multiple books with validation
func BatchUpdateMetadata(updates []MetadataUpdate, store database.Store, validate bool) ([]error, int) {
	var errors []error
	successCount := 0
	rules := DefaultValidationRules()

	for i, update := range updates {
		// Validate if requested
		if validate || update.Validate {
			validationErrors := ValidateMetadata(update.Updates, rules)
			if len(validationErrors) > 0 {
				errors = append(errors, fmt.Errorf("update %d (book %s): %v", i, update.BookID, validationErrors))
				continue
			}
		}

		// Get current book
		book, err := store.GetBookByID(update.BookID)
		if err != nil {
			errors = append(errors, fmt.Errorf("update %d: failed to get book %s: %w", i, update.BookID, err))
			continue
		}

		// Apply updates
		if title, ok := update.Updates["title"].(string); ok {
			book.Title = title
		}
		if _, ok := update.Updates["author"].(string); ok {
			// TODO: Resolve author name to ID and update book.AuthorID
			// For now, skip author updates pending author resolution implementation
		}
		if _, ok := update.Updates["series"].(string); ok {
			// TODO: Resolve series name to ID and update book.SeriesID
			// For now, skip series updates pending series resolution implementation
		}
		if format, ok := update.Updates["format"].(string); ok {
			book.Format = format
		}

		// Update in database
		_, err = store.UpdateBook(update.BookID, book)
		if err != nil {
			errors = append(errors, fmt.Errorf("update %d: failed to update book %s: %w", i, update.BookID, err))
			continue
		}

		successCount++
	}

	return errors, successCount
}

// WriteMetadataToFile safely writes metadata to an audiobook file
// Prefers native TagLib writer when built with 'taglib'; falls back to external CLI tools if unavailable or failed.
// Uses backup/rollback strategy via fileops.SafeCopy for all paths.
func WriteMetadataToFile(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// Attempt native writer first if compiled in
	if taglibAvailable {
		if err := writeMetadataWithTaglib(filePath, metadata, config); err == nil {
			// For M4B/M4A: taglib handles standard atoms but silently drops
			// custom/freeform tags. Use ffmpeg to write those separately.
			ext := strings.ToLower(filepath.Ext(filePath))
			if ext == ".m4b" || ext == ".m4a" {
				if err := writeM4BCustomTagsWithFFmpeg(filePath, metadata); err != nil {
					log.Printf("[WARN] ffmpeg custom tag write failed for %s: %v", filePath, err)
				}
			}
			return nil
		}
		// Native failed; continue with CLI fallback
	}
	// Determine file type
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".m4b", ".m4a":
		return writeM4BMetadata(filePath, metadata, config)
	case ".mp3":
		return writeMP3Metadata(filePath, metadata, config)
	case ".flac":
		return writeFLACMetadata(filePath, metadata, config)
	default:
		return fmt.Errorf("unsupported file format: %s", ext)
	}
}

// writeM4BCustomTagsWithFFmpeg writes custom/freeform tags to M4B files using ffmpeg.
// TagLib handles standard MP4 atoms but silently drops custom tags.
// ffmpeg can write arbitrary metadata including custom fields.
func writeM4BCustomTagsWithFFmpeg(filePath string, metadata map[string]interface{}) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}

	// Only write tags that taglib can't handle for MP4
	customTags := map[string]string{}

	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		customTags["NARRATOR"] = narrator
	}
	if lang, ok := metadata["language"].(string); ok && lang != "" {
		customTags["LANGUAGE"] = strings.ToLower(lang)
	}
	if pub, ok := metadata["publisher"].(string); ok && pub != "" {
		customTags["PUBLISHER"] = pub
	}
	if isbn10, ok := metadata["isbn10"].(string); ok && isbn10 != "" {
		customTags["ISBN10"] = isbn10
	}
	if isbn13, ok := metadata["isbn13"].(string); ok && isbn13 != "" {
		customTags["ISBN13"] = isbn13
	}
	if series, ok := metadata["series"].(string); ok && series != "" {
		customTags["SERIES"] = series
	}
	if si, ok := metadata["series_index"].(int); ok && si > 0 {
		customTags["SERIES_INDEX"] = fmt.Sprintf("%d", si)
	}
	if asin, ok := metadata["asin"].(string); ok && asin != "" {
		customTags["ASIN"] = asin
	}
	customTags["AUDIOBOOK_ORGANIZER_VERSION"] = CustomTagVersion

	if len(customTags) == 0 {
		return nil
	}

	// Build ffmpeg command: copy all streams, add metadata
	tmpPath := filePath + ".tmp-ffmeta"
	args := []string{"-y", "-i", filePath}

	// Preserve all existing streams and chapters
	args = append(args, "-map", "0:a")
	args = append(args, "-map_chapters", "0")
	args = append(args, "-map_metadata", "0")
	args = append(args, "-c", "copy") // No re-encoding

	for k, v := range customTags {
		args = append(args, "-metadata", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, tmpPath)

	cmd := exec.Command(ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output[:min(len(output), 200)]))
	}

	// Atomic replace
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename failed: %w", err)
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// writeM4BMetadata writes metadata to M4B/M4A files using AtomicParsley.
// This is the CLI fallback path; the native taglib writer is preferred and
// handles all fields. AtomicParsley supports standard MP4 atoms and custom
// reverse-DNS atoms (--rDNSatom) for extended metadata.
func writeM4BMetadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// Check if AtomicParsley is available
	if _, err := exec.LookPath("AtomicParsley"); err != nil {
		return fmt.Errorf("AtomicParsley not found in PATH (install: brew install atomicparsley): %w", err)
	}

	log.Printf("[WARN] writeM4BMetadata: using AtomicParsley CLI fallback for %s; native taglib writer is preferred for full tag support", filePath)

	// Create backup using safe copy with config
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	defer func() {
		// Clean up backup unless PreserveOriginal is set
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	// Build AtomicParsley command with metadata updates
	args := []string{filePath, "--overWrite"}

	// --- Standard MP4 atoms ---
	if title, ok := metadata["title"].(string); ok && title != "" {
		args = append(args, "--title", title)
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		args = append(args, "--artist", artist)
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		args = append(args, "--album", album)
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		args = append(args, "--genre", genre)
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--year", fmt.Sprintf("%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--tracknum", track)
	}
	if desc, ok := metadata["description"].(string); ok && desc != "" {
		args = append(args, "--description", desc)
	}
	// --composer maps to ©wrt; use it for narrator (matches tag priority: composer = narrator)
	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		args = append(args, "--composer", narrator)
	}
	// --grouping maps to ©grp; use for series name
	if series, ok := metadata["series"].(string); ok && series != "" {
		args = append(args, "--grouping", series)
	}

	// --- Reverse-DNS atoms for fields without standard AtomicParsley flags ---
	// These are stored as ----:domain:name atoms and can be read back by taglib.
	const rdnsDomain = "audiobook-organizer"

	rdnsPairs := [][2]string{
		{"NARRATOR", "narrator"},
		{"LANGUAGE", "language"},
		{"PUBLISHER", "publisher"},
		{"SERIES", "series"},
		{"ISBN10", "isbn10"},
		{"ISBN13", "isbn13"},
	}
	for _, pair := range rdnsPairs {
		if val, ok := metadata[pair[1]].(string); ok && val != "" {
			args = append(args, "--rDNSatom", val, "name="+pair[0], "domain="+rdnsDomain)
		}
	}
	if si, ok := metadata["series_index"].(int); ok && si > 0 {
		args = append(args, "--rDNSatom", fmt.Sprintf("%d", si), "name=SERIES_INDEX", "domain="+rdnsDomain)
	}
	// Also try string form of series_index (some callers pass it as string)
	if si, ok := metadata["series_index"].(string); ok && si != "" {
		args = append(args, "--rDNSatom", si, "name=SERIES_INDEX", "domain="+rdnsDomain)
	}

	// --- Custom AUDIOBOOK_ORGANIZER_* reverse-DNS atoms ---
	customPairs := [][2]string{
		{TagBookID, "book_id"}, {TagISBN10, "isbn10"}, {TagISBN13, "isbn13"},
		{TagASIN, "asin"}, {TagOpenLibrary, "open_library_id"},
		{TagHardcover, "hardcover_id"}, {TagGoogleBooks, "google_books_id"},
		{TagEdition, "edition"}, {TagPrintYear, "print_year"},
	}
	for _, pair := range customPairs {
		if val, ok := metadata[pair[1]].(string); ok && val != "" {
			args = append(args, "--rDNSatom", val, "name="+pair[0], "domain="+rdnsDomain)
		}
	}
	args = append(args, "--rDNSatom", CustomTagVersion, "name="+TagVersion, "domain="+rdnsDomain)

	cmd := exec.Command("AtomicParsley", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Restore from backup on failure
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("tag write failed and restore failed: write=%w, restore=%v, output=%s", err, restoreErr, output)
		}
		return fmt.Errorf("tag write failed (restored from backup): %w, output: %s", err, output)
	}
	return nil
}

// writeMP3Metadata writes metadata to MP3 files using eyeD3
func writeMP3Metadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// Check if eyeD3 is available
	if _, err := exec.LookPath("eyeD3"); err != nil {
		return fmt.Errorf("eyeD3 not found in PATH (install: pip install eyeD3): %w", err)
	}

	// Create backup
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	defer func() {
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	// Build eyeD3 command
	args := []string{}
	if title, ok := metadata["title"].(string); ok && title != "" {
		args = append(args, "--title", title)
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		args = append(args, "--artist", artist)
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		args = append(args, "--album", album)
	}
	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		// Store narrator in a custom TXXX frame
		args = append(args, "--user-text-frame=NARRATOR:"+narrator)
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		args = append(args, "--genre", genre)
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--release-year", fmt.Sprintf("%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--track-num", track)
	}
	// Write custom AUDIOBOOK_ORGANIZER_* TXXX frames
	customPairs := [][2]string{
		{TagBookID, "book_id"}, {TagISBN10, "isbn10"}, {TagISBN13, "isbn13"},
		{TagASIN, "asin"}, {TagOpenLibrary, "open_library_id"},
		{TagHardcover, "hardcover_id"}, {TagGoogleBooks, "google_books_id"},
		{TagEdition, "edition"}, {TagPrintYear, "print_year"},
	}
	for _, pair := range customPairs {
		if val, ok := metadata[pair[1]].(string); ok && val != "" {
			args = append(args, "--user-text-frame="+pair[0]+":"+val)
		}
	}
	args = append(args, "--user-text-frame="+TagVersion+":"+CustomTagVersion)
	args = append(args, filePath)

	cmd := exec.Command("eyeD3", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Restore from backup on failure
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("tag write failed and restore failed: write=%w, restore=%v, output=%s", err, restoreErr, output)
		}
		return fmt.Errorf("tag write failed (restored from backup): %w, output: %s", err, output)
	}
	return nil
}

// writeFLACMetadata writes metadata to FLAC files using metaflac
func writeFLACMetadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// Check if metaflac is available
	if _, err := exec.LookPath("metaflac"); err != nil {
		return fmt.Errorf("metaflac not found in PATH (install: brew install flac): %w", err)
	}

	// Create backup
	backupPath := filePath + ".backup"
	if err := fileops.SafeCopy(filePath, backupPath, config); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	defer func() {
		if !config.PreserveOriginal {
			_ = os.Remove(backupPath)
		}
	}()

	// Build metaflac command (remove old tags first, then set new)
	removeArgs := []string{"--remove-tag=TITLE", "--remove-tag=ARTIST", "--remove-tag=ALBUM", "--remove-tag=GENRE", "--remove-tag=DATE", "--remove-tag=NARRATOR", filePath}
	if err := exec.Command("metaflac", removeArgs...).Run(); err != nil {
		// Non-fatal if tags don't exist
	}

	// Set new tags
	var args []string
	if title, ok := metadata["title"].(string); ok && title != "" {
		args = append(args, "--set-tag=TITLE="+title)
	}
	if artist, ok := metadata["artist"].(string); ok && artist != "" {
		args = append(args, "--set-tag=ARTIST="+artist)
	}
	if album, ok := metadata["album"].(string); ok && album != "" {
		args = append(args, "--set-tag=ALBUM="+album)
	}
	if narrator, ok := metadata["narrator"].(string); ok && narrator != "" {
		args = append(args, "--set-tag=NARRATOR="+narrator)
	}
	if genre, ok := metadata["genre"].(string); ok && genre != "" {
		args = append(args, "--set-tag=GENRE="+genre)
	}
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, fmt.Sprintf("--set-tag=DATE=%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--set-tag=TRACKNUMBER="+track)
	}
	// Write custom AUDIOBOOK_ORGANIZER_* Vorbis comments
	customPairs := [][2]string{
		{TagBookID, "book_id"}, {TagISBN10, "isbn10"}, {TagISBN13, "isbn13"},
		{TagASIN, "asin"}, {TagOpenLibrary, "open_library_id"},
		{TagHardcover, "hardcover_id"}, {TagGoogleBooks, "google_books_id"},
		{TagEdition, "edition"}, {TagPrintYear, "print_year"},
	}
	for _, pair := range customPairs {
		if val, ok := metadata[pair[1]].(string); ok && val != "" {
			args = append(args, "--set-tag="+pair[0]+"="+val)
		}
	}
	args = append(args, "--set-tag="+TagVersion+"="+CustomTagVersion)
	args = append(args, filePath)

	cmd := exec.Command("metaflac", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Restore from backup on failure
		if restoreErr := fileops.SafeCopy(backupPath, filePath, config); restoreErr != nil {
			return fmt.Errorf("tag write failed and restore failed: write=%w, restore=%v, output=%s", err, restoreErr, output)
		}
		return fmt.Errorf("tag write failed (restored from backup): %w, output: %s", err, output)
	}
	return nil
}

// RecordMetadataChange records a metadata change in history
// This would typically be stored in the database
func RecordMetadataChange(bookID string, field, oldValue, newValue, updatedBy string) *MetadataHistory {
	return &MetadataHistory{
		BookID:    bookID,
		Field:     field,
		OldValue:  oldValue,
		NewValue:  newValue,
		UpdatedAt: time.Now(),
		UpdatedBy: updatedBy,
	}
}

// GetMetadataHistory retrieves metadata change history for a book
// This is a placeholder for future database implementation
func GetMetadataHistory(bookID string, store database.Store) ([]MetadataHistory, error) {
	// TODO: Implement metadata history storage and retrieval in database
	return nil, fmt.Errorf("metadata history not yet implemented in database")
}

// ExportMetadata exports book metadata to a structured format
func ExportMetadata(books []database.Book) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	bookData := make([]map[string]interface{}, 0, len(books))
	for _, book := range books {
		bookData = append(bookData, map[string]interface{}{
			"id":              book.ID,
			"title":           book.Title,
			"author_id":       book.AuthorID,
			"series_id":       book.SeriesID,
			"series_sequence": book.SeriesSequence,
			"format":          book.Format,
			"file_path":       book.FilePath,
			"duration":        book.Duration,
		})
	}

	result["books"] = bookData
	result["count"] = len(books)
	result["exported_at"] = time.Now().Format(time.RFC3339)

	return result, nil
}

// ImportMetadata imports book metadata from a structured format
func ImportMetadata(data map[string]interface{}, store database.Store, validate bool) (int, []error) {
	var errors []error
	importCount := 0

	booksData, ok := data["books"].([]interface{})
	if !ok {
		return 0, []error{fmt.Errorf("invalid data format: books field missing or invalid")}
	}

	for i, bookInterface := range booksData {
		bookData, ok := bookInterface.(map[string]interface{})
		if !ok {
			errors = append(errors, fmt.Errorf("book %d: invalid book data format", i))
			continue
		}

		// Validate if requested
		if validate {
			validationErrors := ValidateMetadata(bookData, DefaultValidationRules())
			if len(validationErrors) > 0 {
				errors = append(errors, fmt.Errorf("book %d: validation failed: %v", i, validationErrors))
				continue
			}
		}

		// Create book object
		duration := getIntField(bookData, "duration")
		book := &database.Book{
			Title:          getStringField(bookData, "title"),
			Format:         getStringField(bookData, "format"),
			FilePath:       getStringField(bookData, "file_path"),
			Duration:       &duration,
			AuthorID:       getIntPtrField(bookData, "author_id"),
			SeriesID:       getIntPtrField(bookData, "series_id"),
			SeriesSequence: getIntPtrField(bookData, "series_sequence"),
		}

		// Create or update book
		_, err := store.CreateBook(book)
		if err != nil {
			errors = append(errors, fmt.Errorf("book %d: failed to import: %w", i, err))
			continue
		}

		importCount++
	}

	return importCount, errors
}

// Helper functions for type-safe field extraction
func getStringField(data map[string]interface{}, field string) string {
	if val, ok := data[field].(string); ok {
		return val
	}
	return ""
}

func getIntField(data map[string]interface{}, field string) int {
	if val, ok := data[field].(float64); ok {
		return int(val)
	}
	if val, ok := data[field].(int); ok {
		return val
	}
	return 0
}

func getIntPtrField(data map[string]interface{}, field string) *int {
	if val, ok := data[field].(float64); ok {
		intVal := int(val)
		return &intVal
	}
	if val, ok := data[field].(int); ok {
		return &val
	}
	return nil
}
