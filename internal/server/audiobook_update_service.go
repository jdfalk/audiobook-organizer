// file: internal/server/audiobook_update_service.go
// version: 1.1.1
// guid: b2c3d4e5-f6g7-h8i9-j0k1-l2m3n4o5p6q7

package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type AudiobookUpdateService struct {
	db               database.Store
	audiobookService *AudiobookService
}

func NewAudiobookUpdateService(db database.Store) *AudiobookUpdateService {
	return &AudiobookUpdateService{
		db:               db,
		audiobookService: NewAudiobookService(db),
	}
}

// ValidateRequest checks if the update request has required fields
func (aus *AudiobookUpdateService) ValidateRequest(id string, payload map[string]any) (map[string]any, error) {
	if id == "" {
		return nil, fmt.Errorf("audiobook ID is required")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}
	return payload, nil
}

// ExtractStringField extracts a string value from payload
func (aus *AudiobookUpdateService) ExtractStringField(payload map[string]any, key string) (string, bool) {
	val, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// ExtractIntField extracts an int value from payload (handling JSON float64)
func (aus *AudiobookUpdateService) ExtractIntField(payload map[string]any, key string) (int, bool) {
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	// JSON unmarshals numbers as float64
	f, ok := val.(float64)
	return int(f), ok
}

// ExtractBoolField extracts a bool value from payload
func (aus *AudiobookUpdateService) ExtractBoolField(payload map[string]any, key string) (bool, bool) {
	val, ok := payload[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// ExtractOverrides extracts and marshals the overrides map from payload
func (aus *AudiobookUpdateService) ExtractOverrides(payload map[string]any) (map[string]any, bool) {
	val, ok := payload["overrides"]
	if !ok {
		return nil, false
	}

	overridesMap, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}

	return overridesMap, true
}

// ApplyUpdatesToBook applies field updates to a book struct
func (aus *AudiobookUpdateService) ApplyUpdatesToBook(book *database.Book, updates map[string]any) {
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
func (aus *AudiobookUpdateService) UpdateAudiobook(id string, payload map[string]any) (*database.Book, error) {
	if id == "" {
		return nil, fmt.Errorf("audiobook ID is required")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("no updates provided")
	}
	if aus.audiobookService == nil {
		return nil, fmt.Errorf("audiobook service not initialized")
	}

	currentBook, err := aus.db.GetBookByID(id)
	if err != nil || currentBook == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	bookCopy := *currentBook
	updates := &AudiobookUpdate{Book: &bookCopy}

	if title, ok := aus.ExtractStringField(payload, "title"); ok {
		updates.Title = title
	}
	if authorID, ok := aus.ExtractIntField(payload, "author_id"); ok {
		updates.AuthorID = &authorID
	}
	if seriesID, ok := aus.ExtractIntField(payload, "series_id"); ok {
		updates.SeriesID = &seriesID
	}
	if authorName, ok := aus.ExtractStringField(payload, "author_name"); ok {
		updates.AuthorName = &authorName
	}
	if seriesName, ok := aus.ExtractStringField(payload, "series_name"); ok {
		updates.SeriesName = &seriesName
	}
	if format, ok := aus.ExtractStringField(payload, "format"); ok {
		updates.Format = format
	}
	if filePath, ok := aus.ExtractStringField(payload, "file_path"); ok {
		updates.FilePath = filePath
	}
	if narrator, ok := aus.ExtractStringField(payload, "narrator"); ok {
		updates.Narrator = &narrator
	}
	if publisher, ok := aus.ExtractStringField(payload, "publisher"); ok {
		updates.Publisher = &publisher
	}
	if language, ok := aus.ExtractStringField(payload, "language"); ok {
		updates.Language = &language
	}
	if year, ok := aus.ExtractIntField(payload, "audiobook_release_year"); ok {
		updates.AudiobookReleaseYear = &year
	}
	if isbn10, ok := aus.ExtractStringField(payload, "isbn10"); ok {
		updates.ISBN10 = &isbn10
	}
	if isbn13, ok := aus.ExtractStringField(payload, "isbn13"); ok {
		updates.ISBN13 = &isbn13
	}

	if overridesMap, ok := aus.ExtractOverrides(payload); ok {
		updates.Overrides = make(map[string]OverridePayload)
		for key, value := range overridesMap {
			overrideValue, ok := value.(map[string]any)
			if !ok {
				continue
			}

			override := OverridePayload{}
			if val, ok := overrideValue["value"]; ok {
				if valBytes, err := json.Marshal(val); err == nil {
					override.Value = valBytes
				}
			}
			if locked, ok := overrideValue["locked"].(bool); ok {
				override.Locked = &locked
			}
			if fetchedVal, ok := overrideValue["fetched_value"]; ok {
				if fetchedBytes, err := json.Marshal(fetchedVal); err == nil {
					override.FetchedValue = fetchedBytes
				}
			}
			if clear, ok := overrideValue["clear"].(bool); ok {
				override.Clear = clear
			}
			updates.Overrides[key] = override
		}
	}

	if unlockOverridesRaw, ok := payload["unlock_overrides"].([]any); ok {
		updates.UnlockOverrides = make([]string, 0, len(unlockOverridesRaw))
		for _, value := range unlockOverridesRaw {
			if str, ok := value.(string); ok {
				updates.UnlockOverrides = append(updates.UnlockOverrides, str)
			}
		}
	}

	rawPayload := make(map[string]json.RawMessage, len(payload))
	for key, value := range payload {
		rawValue, err := json.Marshal(value)
		if err != nil {
			continue
		}
		rawPayload[key] = rawValue
	}

	req := &UpdateAudiobookRequest{
		Updates:    updates,
		RawPayload: rawPayload,
	}

	return aus.audiobookService.UpdateAudiobook(context.Background(), id, req)
}
