// file: internal/metadata/enhanced.go
// version: 1.0.0
// guid: 9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d

package metadata

import (
	"fmt"
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
				errors = append(errors, fmt.Errorf("update %d (book %d): %v", i, update.BookID, validationErrors))
				continue
			}
		}

		// Get current book
		book, err := store.GetBookByID(update.BookID)
		if err != nil {
			errors = append(errors, fmt.Errorf("update %d: failed to get book %d: %w", i, update.BookID, err))
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
			errors = append(errors, fmt.Errorf("update %d: failed to update book %d: %w", i, update.BookID, err))
			continue
		}

		successCount++
	}

	return errors, successCount
}

// WriteMetadataToFile safely writes metadata to an audiobook file
func WriteMetadataToFile(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
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

// writeM4BMetadata writes metadata to M4B/M4A files using safe file operations
func writeM4BMetadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// TODO: Implement M4B metadata writing using a library like github.com/dhowden/tag
	// For now, return not implemented
	return fmt.Errorf("M4B metadata writing not yet implemented")
}

// writeMP3Metadata writes metadata to MP3 files using safe file operations
func writeMP3Metadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// TODO: Implement MP3 metadata writing using ID3 tags
	// For now, return not implemented
	return fmt.Errorf("MP3 metadata writing not yet implemented")
}

// writeFLACMetadata writes metadata to FLAC files using safe file operations
func writeFLACMetadata(filePath string, metadata map[string]interface{}, config fileops.OperationConfig) error {
	// TODO: Implement FLAC metadata writing using Vorbis comments
	// For now, return not implemented
	return fmt.Errorf("FLAC metadata writing not yet implemented")
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
