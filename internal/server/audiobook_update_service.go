// file: internal/server/audiobook_update_service.go
// version: 1.0.0
// guid: b2c3d4e5-f6g7-h8i9-j0k1-l2m3n4o5p6q7

package server

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type AudiobookUpdateService struct {
	db database.Store
}

func NewAudiobookUpdateService(db database.Store) *AudiobookUpdateService {
	return &AudiobookUpdateService{db: db}
}

// ValidateRequest checks if the update request has required fields
func (aus *AudiobookUpdateService) ValidateRequest(id string, payload map[string]interface{}) (map[string]interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("audiobook ID is required")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}
	return payload, nil
}

// ExtractStringField extracts a string value from payload
func (aus *AudiobookUpdateService) ExtractStringField(payload map[string]interface{}, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (aus *AudiobookUpdateService) ExtractIntField(payload map[string]interface{}, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	// JSON unmarshals numbers as float64
	f, ok := val.(float64)
	return int(f), ok
}

// ExtractBoolField extracts a bool value from payload
func (aus *AudiobookUpdateService) ExtractBoolField(payload map[string]interface{}, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractOverrides extracts and marshals the overrides map from payload
func (aus *AudiobookUpdateService) ExtractOverrides(payload map[string]interface{}) (map[string]interface{}, bool) {
	val, ok := payload["overrides"]
	if !ok {
		return nil, false
	}

	overridesMap, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}

	return overridesMap, true
}

// ApplyUpdatesToBook applies field updates to a book struct
func (aus *AudiobookUpdateService) ApplyUpdatesToBook(book *database.Book, updates map[string]interface{}) {
	if title, ok := aus.ExtractStringField(updates, "title"); ok {
		book.Title = title
	}
	if authorID, ok := aus.ExtractIntField(updates, "author_id"); ok {
		book.AuthorID = &authorID
	}
	if seriesID, ok := aus.ExtractIntField(updates, "series_id"); ok {
		book.SeriesID = &seriesID
	}
	if narrator, ok := aus.ExtractStringField(updates, "narrator"); ok {
		book.Narrator = &narrator
	}
	if publisher, ok := aus.ExtractStringField(updates, "publisher"); ok {
		book.Publisher = &publisher
	}
	if language, ok := aus.ExtractStringField(updates, "language"); ok {
		book.Language = &language
	}
	if year, ok := aus.ExtractIntField(updates, "audiobook_release_year"); ok {
		book.AudiobookReleaseYear = &year
	}
	if isbn10, ok := aus.ExtractStringField(updates, "isbn10"); ok {
		book.ISBN10 = &isbn10
	}
	if isbn13, ok := aus.ExtractStringField(updates, "isbn13"); ok {
		book.ISBN13 = &isbn13
	}
}

// UpdateAudiobook is the main business logic method
func (aus *AudiobookUpdateService) UpdateAudiobook(id string, payload map[string]interface{}) (*database.Book, error) {
	// Validate request
	_, err := aus.ValidateRequest(id, payload)
	if err != nil {
		return nil, err
	}

	// Get the book from database
	book, err := aus.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// Apply updates
	aus.ApplyUpdatesToBook(book, payload)

	// Persist to database
	updated, err := aus.db.UpdateBook(id, book)
	if err != nil {
		return nil, fmt.Errorf("failed to update audiobook: %w", err)
	}

	return updated, nil
}
