// file: internal/server/audiobook_service.go
// version: 1.2.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// AudiobookService handles all audiobook business logic
type AudiobookService struct {
	store database.Store
}

// NewAudiobookService creates a new AudiobookService instance
func NewAudiobookService(store database.Store) *AudiobookService {
	return &AudiobookService{store: store}
}

// AudiobooksListResponse represents the response for listing audiobooks
type AudiobooksListResponse struct {
	Items  []AudiobookDetail `json:"items"`
	Count  int               `json:"count"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// AudiobookDetail extends database.Book with author and series names for response
type AudiobookDetail struct {
	*database.Book
	AuthorName *string `json:"author_name,omitempty"`
	SeriesName *string `json:"series_name,omitempty"`
}

// DuplicatesResult represents the result of duplicate detection
type DuplicatesResult struct {
	Groups         [][]database.Book `json:"groups"`
	GroupCount     int               `json:"group_count"`
	DuplicateCount int               `json:"duplicate_count"`
}

// SoftDeletedBooksResponse represents the response for listing soft-deleted books
type SoftDeletedBooksResponse struct {
	Items  []database.Book `json:"items"`
	Count  int             `json:"count"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// PurgeResult represents the result of purging soft-deleted books
type PurgeResult struct {
	Attempted    int      `json:"attempted"`
	Purged       int      `json:"purged"`
	FilesDeleted int      `json:"files_deleted"`
	Errors       []string `json:"errors"`
}

// AudiobookUpdate represents a partial update to an audiobook
type AudiobookUpdate struct {
	*database.Book
	AuthorName      *string                    `json:"author_name,omitempty"`
	SeriesName      *string                    `json:"series_name,omitempty"`
	Overrides       map[string]OverridePayload `json:"overrides,omitempty"`
	UnlockOverrides []string                   `json:"unlock_overrides,omitempty"`
}

// OverridePayload represents metadata override information
type OverridePayload struct {
	Value        json.RawMessage `json:"value"`
	Locked       *bool           `json:"locked,omitempty"`
	FetchedValue json.RawMessage `json:"fetched_value,omitempty"`
	Clear        bool            `json:"clear,omitempty"`
}

// GetAudiobooks retrieves audiobooks with optional filtering
// Supports search, author_id, and series_id filters
func (svc *AudiobookService) GetAudiobooks(ctx context.Context, limit int, offset int, search string, authorID *int, seriesID *int) ([]database.Book, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Normalize limit and offset
	if limit <= 0 || limit > 100000 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Initialize as empty slice to ensure we return [] instead of null
	books := []database.Book{}
	var err error

	// Apply filters in order of precedence
	if search != "" {
		books, err = svc.store.SearchBooks(search, limit, offset)
	} else if authorID != nil {
		books, err = svc.store.GetBooksByAuthorID(*authorID)
	} else if seriesID != nil {
		books, err = svc.store.GetBooksBySeriesID(*seriesID)
	}

	// Fall back to generic list only when no filter was applied
	if search == "" && authorID == nil && seriesID == nil {
		books, err = svc.store.GetAllBooks(limit, offset)
	}

	if err != nil {
		return nil, err
	}

	// Ensure we never return null - always return empty array
	if books == nil {
		books = []database.Book{}
	}

	return books, nil
}

// EnrichAudiobooksWithNames adds author and series names to audiobook details
func (svc *AudiobookService) EnrichAudiobooksWithNames(books []database.Book) []AudiobookDetail {
	enrichedBooks := make([]AudiobookDetail, 0, len(books))
	for _, book := range books {
		authorName, seriesName := resolveAuthorAndSeriesNames(&book)
		detail := AudiobookDetail{Book: &book}
		if authorName != "" {
			detail.AuthorName = &authorName
		}
		if seriesName != "" {
			detail.SeriesName = &seriesName
		}
		enrichedBooks = append(enrichedBooks, detail)
	}
	return enrichedBooks
}

// GetAudiobook retrieves a single audiobook by ID with full metadata provenance
func (svc *AudiobookService) GetAudiobook(ctx context.Context, id string) (*database.Book, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	book, err := svc.store.GetBookByID(id)
	if err != nil {
		return nil, err
	}
	if book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// Load metadata state and extract file metadata
	state, err := loadMetadataState(book.ID)
	if err != nil {
		log.Printf("[ERROR] GetAudiobook: failed to load metadata state for %s: %v", book.ID, err)
		// Don't fail the entire request, just use empty state
		state = map[string]metadataFieldState{}
	}

	authorName, seriesName := resolveAuthorAndSeriesNames(book)

	var meta metadata.Metadata
	if book.FilePath != "" {
		if m, err := metadata.ExtractMetadata(book.FilePath); err == nil {
			meta = m
		} else {
			log.Printf("[WARN] GetAudiobook: failed to extract metadata for %s: %v", book.FilePath, err)
		}
	}

	// Build metadata provenance
	book.MetadataProvenance = buildMetadataProvenance(book, state, meta, authorName, seriesName)
	now := time.Now().UTC()
	book.MetadataProvenanceAt = &now

	return book, nil
}

// GetAudiobookTags retrieves metadata tags and media info for an audiobook
func (svc *AudiobookService) GetAudiobookTags(ctx context.Context, id string) (map[string]any, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	book, err := svc.store.GetBookByID(id)
	if err != nil {
		return nil, err
	}
	if book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	state, err := loadMetadataState(book.ID)
	if err != nil {
		log.Printf("[ERROR] GetAudiobookTags: failed to load metadata state for %s: %v", book.ID, err)
		state = map[string]metadataFieldState{}
	}

	authorName, seriesName := resolveAuthorAndSeriesNames(book)

	response := map[string]any{
		"media_info": map[string]any{
			"codec":       stringVal(book.Codec),
			"bitrate":     intVal(book.Bitrate),
			"sample_rate": intVal(book.SampleRate),
			"channels":    intVal(book.Channels),
			"bit_depth":   intVal(book.BitDepth),
			"quality":     stringVal(book.Quality),
			"duration":    intVal(book.Duration),
		},
		"tags": map[string]database.MetadataProvenanceEntry{},
	}

	var meta metadata.Metadata
	if book.FilePath != "" {
		if m, err := metadata.ExtractMetadata(book.FilePath); err == nil {
			meta = m
		} else {
			log.Printf("[WARN] GetAudiobookTags: failed to extract metadata for %s: %v", book.FilePath, err)
		}
	}

	tags := buildMetadataProvenance(book, state, meta, authorName, seriesName)
	response["tags"] = tags

	return response, nil
}

// GetDuplicateBooks retrieves all duplicate book groups
func (svc *AudiobookService) GetDuplicateBooks(ctx context.Context) (*DuplicatesResult, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	duplicateGroups, err := svc.store.GetDuplicateBooks()
	if err != nil {
		return nil, err
	}

	// Ensure we never return null - always return empty array
	if duplicateGroups == nil {
		duplicateGroups = [][]database.Book{}
	}

	// Calculate total duplicates count (sum of all books in duplicate groups minus the count of groups)
	totalDuplicates := 0
	for _, group := range duplicateGroups {
		totalDuplicates += len(group) - 1 // Count extras in each group
	}

	return &DuplicatesResult{
		Groups:         duplicateGroups,
		GroupCount:     len(duplicateGroups),
		DuplicateCount: totalDuplicates,
	}, nil
}

// GetSoftDeletedBooks retrieves soft-deleted audiobooks with optional age filter
func (svc *AudiobookService) GetSoftDeletedBooks(ctx context.Context, limit int, offset int, olderThanDays *int) ([]database.Book, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Normalize limit and offset
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var cutoff *time.Time
	if olderThanDays != nil && *olderThanDays > 0 {
		ts := time.Now().AddDate(0, 0, -*olderThanDays)
		cutoff = &ts
	}

	books, err := svc.store.ListSoftDeletedBooks(limit, offset, cutoff)
	if err != nil {
		return nil, err
	}

	if books == nil {
		books = []database.Book{}
	}

	return books, nil
}

// PurgeSoftDeletedBooks permanently deletes soft-deleted audiobooks
func (svc *AudiobookService) PurgeSoftDeletedBooks(ctx context.Context, deleteFiles bool, olderThanDays *int) (*PurgeResult, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var cutoff *time.Time
	if olderThanDays != nil && *olderThanDays > 0 {
		ts := time.Now().AddDate(0, 0, -*olderThanDays)
		cutoff = &ts
	}

	books, err := svc.store.ListSoftDeletedBooks(1_000_000, 0, cutoff)
	if err != nil {
		return nil, err
	}

	result := &PurgeResult{
		Attempted: len(books),
		Errors:    []string{},
	}

	for _, book := range books {
		// Delete associated files if requested
		if deleteFiles && book.FilePath != "" {
			if err := os.Remove(book.FilePath); err != nil && !os.IsNotExist(err) {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to delete file: %v", book.ID, err))
			} else if err == nil {
				result.FilesDeleted++
			}
		}

		// Delete from database
		if err := svc.store.DeleteBook(book.ID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", book.ID, err))
			continue
		}
		result.Purged++
	}

	return result, nil
}

// RestoreAudiobook restores a soft-deleted audiobook
func (svc *AudiobookService) RestoreAudiobook(ctx context.Context, id string) (*database.Book, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	book, err := svc.store.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// Restore to imported state so the UI can re-process if needed
	book.MarkedForDeletion = boolPtr(false)
	book.MarkedForDeletionAt = nil
	book.LibraryState = stringPtr("imported")

	updated, err := svc.store.UpdateBook(id, book)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// CountAudiobooks returns the total count of audiobooks
func (svc *AudiobookService) CountAudiobooks(ctx context.Context) (int, error) {
	if svc.store == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	count, err := svc.store.CountBooks()
	if err != nil {
		return 0, err
	}
	return count, nil
}

// UpdateAudiobookRequest represents parameters for updating an audiobook
type UpdateAudiobookRequest struct {
	Updates             *AudiobookUpdate
	RawPayload          map[string]json.RawMessage
	ResolvingAuthorName string
	ResolvingSeriesName string
}

// UpdateAudiobook updates an audiobook with new metadata and handles overrides
func (svc *AudiobookService) UpdateAudiobook(ctx context.Context, id string, req *UpdateAudiobookRequest) (*database.Book, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	currentBook, err := svc.store.GetBookByID(id)
	if err != nil {
		return nil, err
	}
	if currentBook == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	now := time.Now()

	// Apply updates from req.Updates to currentBook first
	// This ensures extractors can read the updated fields
	if req.Updates.Title != "" {
		currentBook.Title = req.Updates.Title
	}
	if req.Updates.Format != "" {
		currentBook.Format = req.Updates.Format
	}
	if req.Updates.FilePath != "" {
		currentBook.FilePath = req.Updates.FilePath
	}
	if req.Updates.Narrator != nil {
		currentBook.Narrator = req.Updates.Narrator
	}
	if req.Updates.Publisher != nil {
		currentBook.Publisher = req.Updates.Publisher
	}
	if req.Updates.Language != nil {
		currentBook.Language = req.Updates.Language
	}
	if req.Updates.AudiobookReleaseYear != nil {
		currentBook.AudiobookReleaseYear = req.Updates.AudiobookReleaseYear
	}
	if req.Updates.ISBN10 != nil {
		currentBook.ISBN10 = req.Updates.ISBN10
	}
	if req.Updates.ISBN13 != nil {
		currentBook.ISBN13 = req.Updates.ISBN13
	}
	if req.Updates.AuthorID != nil {
		currentBook.AuthorID = req.Updates.AuthorID
	}
	if req.Updates.SeriesID != nil {
		currentBook.SeriesID = req.Updates.SeriesID
	}

	payload := &AudiobookUpdate{
		Book: currentBook,
	}

	// Load and process metadata state
	state, err := loadMetadataState(id)
	if err != nil {
		log.Printf("[ERROR] UpdateAudiobook: failed to load metadata state: %v", err)
		return nil, fmt.Errorf("failed to load metadata state")
	}
	if state == nil {
		state = map[string]metadataFieldState{}
	}

	// Process overrides
	for field, override := range req.Updates.Overrides {
		entry := state[field]
		if override.Clear {
			entry.OverrideValue = nil
			entry.OverrideLocked = false
			entry.UpdatedAt = now
		} else {
			if len(override.Value) > 0 {
				val := decodeRawValue(override.Value)
				entry.OverrideValue = val
				entry.OverrideLocked = override.Locked == nil || *override.Locked
				entry.UpdatedAt = now
				applyOverrideToPayload(payload, field, val)
			} else if override.Locked != nil {
				entry.OverrideLocked = *override.Locked
				entry.UpdatedAt = now
			}
			if len(override.FetchedValue) > 0 {
				entry.FetchedValue = decodeRawValue(override.FetchedValue)
				if entry.UpdatedAt.IsZero() {
					entry.UpdatedAt = now
				}
			}
		}
		state[field] = entry
	}

	// Resolve author by name or ID
	var resolvedAuthorName string
	if req.Updates.AuthorName != nil {
		name := strings.TrimSpace(*req.Updates.AuthorName)
		if name != "" {
			author, err := svc.store.GetAuthorByName(name)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve author")
			}
			if author == nil {
				author, err = svc.store.CreateAuthor(name)
				if err != nil {
					return nil, fmt.Errorf("failed to create author")
				}
			}
			payload.AuthorID = &author.ID
			resolvedAuthorName = author.Name
		} else {
			payload.AuthorID = nil
		}
	} else if payload.AuthorID != nil {
		if author, err := svc.store.GetAuthorByID(*payload.AuthorID); err == nil && author != nil {
			resolvedAuthorName = author.Name
		}
	}

	// Resolve series by name or ID
	var resolvedSeriesName string
	if req.Updates.SeriesName != nil {
		name := strings.TrimSpace(*req.Updates.SeriesName)
		if name != "" {
			series, err := svc.store.GetSeriesByName(name, payload.AuthorID)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve series")
			}
			if series == nil {
				series, err = svc.store.CreateSeries(name, payload.AuthorID)
				if err != nil {
					return nil, fmt.Errorf("failed to create series")
				}
			}
			payload.SeriesID = &series.ID
			resolvedSeriesName = series.Name
		} else {
			payload.SeriesID = nil
		}
	} else if payload.SeriesID != nil {
		if series, err := svc.store.GetSeriesByID(*payload.SeriesID); err == nil && series != nil {
			resolvedSeriesName = series.Name
		}
	}

	// Process direct field updates (non-override)
	fieldExtractors := map[string]func() (any, bool){
		"title": func() (any, bool) {
			return payload.Title, true
		},
		"author_name": func() (any, bool) {
			if resolvedAuthorName == "" {
				return nil, false
			}
			return resolvedAuthorName, true
		},
		"series_name": func() (any, bool) {
			if resolvedSeriesName == "" {
				return nil, false
			}
			return resolvedSeriesName, true
		},
		"narrator": func() (any, bool) {
			if payload.Narrator == nil {
				return nil, false
			}
			return *payload.Narrator, true
		},
		"publisher": func() (any, bool) {
			if payload.Publisher == nil {
				return nil, false
			}
			return *payload.Publisher, true
		},
		"language": func() (any, bool) {
			if payload.Language == nil {
				return nil, false
			}
			return *payload.Language, true
		},
		"audiobook_release_year": func() (any, bool) {
			if payload.AudiobookReleaseYear == nil {
				return nil, false
			}
			return *payload.AudiobookReleaseYear, true
		},
		"isbn10": func() (any, bool) {
			if payload.ISBN10 == nil {
				return nil, false
			}
			return *payload.ISBN10, true
		},
		"isbn13": func() (any, bool) {
			if payload.ISBN13 == nil {
				return nil, false
			}
			return *payload.ISBN13, true
		},
	}

	for field, extractor := range fieldExtractors {
		if _, ok := req.RawPayload[field]; !ok {
			log.Printf("[DEBUG] UpdateAudiobook: field %s not in RawPayload", field)
			continue
		}
		if _, hasOverride := req.Updates.Overrides[field]; hasOverride {
			log.Printf("[DEBUG] UpdateAudiobook: field %s has explicit override", field)
			continue
		}
		if value, ok := extractor(); ok {
			log.Printf("[DEBUG] UpdateAudiobook: creating state for field %s with value %v", field, value)
			entry := state[field]
			entry.OverrideValue = value
			entry.OverrideLocked = true
			entry.UpdatedAt = now
			state[field] = entry
		} else {
			log.Printf("[DEBUG] UpdateAudiobook: extractor for field %s returned false/nil", field)
		}
	}

	// Process unlock overrides
	for _, field := range req.Updates.UnlockOverrides {
		entry := state[field]
		entry.OverrideLocked = false
		entry.UpdatedAt = now
		state[field] = entry
	}

	// Save to database
	updatedBook, err := svc.store.UpdateBook(id, payload.Book)
	if err != nil {
		return nil, err
	}

	// Save metadata state
	if err := saveMetadataState(id, state); err != nil {
		log.Printf("[ERROR] UpdateAudiobook: failed to save metadata state: %v", err)
		return nil, fmt.Errorf("failed to persist metadata state")
	}

	// Enrich response with resolved names
	if resolvedAuthorName != "" && updatedBook.AuthorID != nil {
		updatedBook.Author = &database.Author{ID: *updatedBook.AuthorID, Name: resolvedAuthorName}
	}
	if resolvedSeriesName != "" && updatedBook.SeriesID != nil {
		updatedBook.Series = &database.Series{ID: *updatedBook.SeriesID, Name: resolvedSeriesName, AuthorID: payload.AuthorID}
	}

	return updatedBook, nil
}

// DeleteAudiobookOptions contains options for deleting an audiobook
type DeleteAudiobookOptions struct {
	SoftDelete bool
	BlockHash  bool
}

// DeleteAudiobook deletes an audiobook (soft or hard delete)
func (svc *AudiobookService) DeleteAudiobook(ctx context.Context, id string, opts *DeleteAudiobookOptions) (map[string]any, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if opts == nil {
		opts = &DeleteAudiobookOptions{}
	}

	// Get the book first to access its hash
	book, err := svc.store.GetBookByID(id)
	if err != nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	// If soft delete requested, mark for deletion instead of hard delete
	if opts.SoftDelete {
		if (book.MarkedForDeletion != nil && *book.MarkedForDeletion) ||
			(book.LibraryState != nil && strings.EqualFold(*book.LibraryState, "deleted")) {
			return nil, fmt.Errorf("audiobook already soft deleted")
		}

		now := time.Now()
		book.MarkedForDeletion = boolPtr(true)
		book.MarkedForDeletionAt = &now
		book.LibraryState = stringPtr("deleted")

		if _, err := svc.store.UpdateBook(id, book); err != nil {
			return nil, err
		}

		// Optionally block the hash
		blocked := false
		if opts.BlockHash && book.FileHash != nil && *book.FileHash != "" {
			if err := svc.store.AddBlockedHash(*book.FileHash, "User deleted - soft delete"); err != nil {
				log.Printf("Warning: failed to block hash during soft delete: %v", err)
			} else {
				blocked = true
			}
		}

		return map[string]any{
			"message":     "audiobook soft deleted",
			"blocked":     blocked,
			"soft_delete": true,
		}, nil
	}

	// Hard delete path
	// Optionally block the hash before deleting
	blocked := false
	if opts.BlockHash && book.FileHash != nil && *book.FileHash != "" {
		if err := svc.store.AddBlockedHash(*book.FileHash, "User deleted - prevent reimport"); err != nil {
			log.Printf("Warning: failed to block hash before delete: %v", err)
			// Continue with delete even if blocking fails
		} else {
			blocked = true
		}
	}

	if err := svc.store.DeleteBook(id); err != nil {
		if err.Error() == "book not found" {
			return nil, fmt.Errorf("audiobook not found")
		}
		return nil, err
	}

	return map[string]any{
		"message": "audiobook deleted",
		"blocked": blocked,
	}, nil
}

// applyOverrideToPayload applies an override value to the update payload
func applyOverrideToPayload(payload *AudiobookUpdate, field string, value any) {
	switch field {
	case "title":
		if v, ok := value.(string); ok {
			payload.Title = v
		}
	case "author_name":
		if v, ok := value.(string); ok {
			payload.AuthorName = &v
		}
	case "series_name":
		if v, ok := value.(string); ok {
			payload.SeriesName = &v
		}
	case "narrator":
		if v, ok := value.(string); ok {
			payload.Narrator = stringPtr(v)
		}
	case "publisher":
		if v, ok := value.(string); ok {
			payload.Publisher = stringPtr(v)
		}
	case "language":
		if v, ok := value.(string); ok {
			payload.Language = stringPtr(v)
		}
	case "audiobook_release_year":
		switch v := value.(type) {
		case float64:
			year := int(v)
			payload.AudiobookReleaseYear = &year
		case int:
			year := v
			payload.AudiobookReleaseYear = &year
		}
	case "isbn10":
		if v, ok := value.(string); ok {
			payload.ISBN10 = stringPtr(v)
		}
	case "isbn13":
		if v, ok := value.(string); ok {
			payload.ISBN13 = stringPtr(v)
		}
	}
}
