// file: internal/server/audiobook_service.go
// version: 1.18.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/cache"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/mediainfo"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/search"
)

// audiobookStore is the narrow slice of database.Store that
// AudiobookService actually needs — both for its own method calls
// and for the helpers it forwards the store to (asExternalIDStore,
// NewMetadataStateService). Declared as a named composite so the
// service's dependency surface is inspectable in one place.
type audiobookStore interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.NarratorStore
	database.BookFileStore
	database.HashBlocklistStore
	database.TagStore
	// Transitively required — audiobook_service forwards svc.store to
	// NewMetadataStateService for change history tracking and to
	// asExternalIDStore for tombstone cleanup.
	database.MetadataStore
	database.UserPreferenceStore
}

// AudiobookService handles all audiobook business logic
type AudiobookService struct {
	store           audiobookStore
	bookCache       *cache.Cache[*database.Book]
	listCache       *cache.Cache[[]database.Book]
	activityService *activity.Service
	// searchIndex is the Bleve index for full-text search. When nil
	// the service falls back to the legacy Store.SearchBooks path.
	// Wired in by the Server after Bleve opens in Start(), which is
	// after NewAudiobookService runs in NewServer.
	searchIndex *search.BleveIndex
}

// SetActivityService wires the activity service for snapshot fallback in GetAudiobookTags.
func (svc *AudiobookService) SetActivityService(as *activity.Service) {
	svc.activityService = as
}

// SetSearchIndex wires the Bleve index for Bleve-backed search.
// Calling with nil reverts to the Store.SearchBooks fallback.
func (svc *AudiobookService) SetSearchIndex(idx *search.BleveIndex) {
	svc.searchIndex = idx
}

// NewAudiobookService creates a new AudiobookService instance
func NewAudiobookService(store audiobookStore) *AudiobookService {
	return &AudiobookService{
		store:     store,
		bookCache: cache.New[*database.Book](30 * time.Second),
		listCache: cache.New[[]database.Book](10 * time.Second),
	}
}

// InvalidateBookCaches clears all book-related caches. Called after any
// mutation (create, update, delete) to keep reads consistent.
//
// Order matters: invalidate bookCache first, then listCache. If we did it the
// other way around, a concurrent reader could miss the list cache (just
// cleared), re-fetch a fresh list from the DB, but still hit stale individual
// book entries that haven't been invalidated yet. By clearing individual books
// first, any concurrent reader that re-fetches the list will also get fresh
// individual books on subsequent lookups.
func (svc *AudiobookService) InvalidateBookCaches() {
	svc.bookCache.InvalidateAll()
	svc.listCache.InvalidateAll()
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

// FieldFilter represents a field-specific search filter from advanced search.
type FieldFilter struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Negated bool   `json:"negated"`
}

// ListFilters holds optional filters for listing audiobooks.
type ListFilters struct {
	IsPrimaryVersion *bool
	LibraryState     string
	Tag              string
	SortBy           string        // column sort key
	SortOrder        string        // "asc" or "desc"
	FieldFilters     []FieldFilter // advanced field-specific filters
}

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefInt safely dereferences a *int, returning 0 for nil.
func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// derefInt64 safely dereferences a *int64, returning 0 for nil.
func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

// cmpTime compares two *time.Time values, treating nil as zero time.
func cmpTime(a, b *time.Time) int {
	ta := time.Time{}
	tb := time.Time{}
	if a != nil {
		ta = *a
	}
	if b != nil {
		tb = *b
	}
	if ta.Before(tb) {
		return -1
	}
	if ta.After(tb) {
		return 1
	}
	return 0
}

// sortFieldMap maps sort keys to comparison functions.
// Each function returns <0 if a<b, 0 if equal, >0 if a>b.
var sortFieldMap = map[string]func(a, b *database.Book) int{
	"title": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	},
	"author": func(a, b *database.Book) int {
		an := ""
		bn := ""
		if a.Author != nil {
			an = a.Author.Name
		}
		if b.Author != nil {
			bn = b.Author.Name
		}
		return strings.Compare(strings.ToLower(an), strings.ToLower(bn))
	},
	"narrator": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Narrator)), strings.ToLower(derefStr(b.Narrator)))
	},
	"series": func(a, b *database.Book) int {
		an := ""
		bn := ""
		if a.Series != nil {
			an = a.Series.Name
		}
		if b.Series != nil {
			bn = b.Series.Name
		}
		return strings.Compare(strings.ToLower(an), strings.ToLower(bn))
	},
	"genre": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Genre)), strings.ToLower(derefStr(b.Genre)))
	},
	"year": func(a, b *database.Book) int {
		ay := derefInt(a.AudiobookReleaseYear)
		by := derefInt(b.AudiobookReleaseYear)
		if ay == 0 {
			ay = derefInt(a.PrintYear)
		}
		if by == 0 {
			by = derefInt(b.PrintYear)
		}
		return ay - by
	},
	"language": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Language)), strings.ToLower(derefStr(b.Language)))
	},
	"publisher": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Publisher)), strings.ToLower(derefStr(b.Publisher)))
	},
	"format": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(a.Format), strings.ToLower(b.Format))
	},
	"duration": func(a, b *database.Book) int {
		return derefInt(a.Duration) - derefInt(b.Duration)
	},
	"bitrate": func(a, b *database.Book) int {
		return derefInt(a.Bitrate) - derefInt(b.Bitrate)
	},
	"file_size": func(a, b *database.Book) int {
		diff := derefInt64(a.FileSize) - derefInt64(b.FileSize)
		if diff < 0 {
			return -1
		}
		if diff > 0 {
			return 1
		}
		return 0
	},
	"codec": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Codec)), strings.ToLower(derefStr(b.Codec)))
	},
	"created_at": func(a, b *database.Book) int {
		return cmpTime(a.CreatedAt, b.CreatedAt)
	},
	"updated_at": func(a, b *database.Book) int {
		return cmpTime(a.UpdatedAt, b.UpdatedAt)
	},
	"library_state": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.LibraryState)), strings.ToLower(derefStr(b.LibraryState)))
	},
	"quality": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Quality)), strings.ToLower(derefStr(b.Quality)))
	},
	"edition": func(a, b *database.Book) int {
		return strings.Compare(strings.ToLower(derefStr(a.Edition)), strings.ToLower(derefStr(b.Edition)))
	},
	// Aliases for frontend field names (e.g. SortField enum uses suffixed variants)
	"duration_seconds": func(a, b *database.Book) int {
		return derefInt(a.Duration) - derefInt(b.Duration)
	},
	"bitrate_kbps": func(a, b *database.Book) int {
		return derefInt(a.Bitrate) - derefInt(b.Bitrate)
	},
	"file_size_bytes": func(a, b *database.Book) int {
		diff := derefInt64(a.FileSize) - derefInt64(b.FileSize)
		if diff < 0 {
			return -1
		}
		if diff > 0 {
			return 1
		}
		return 0
	},
	"sample_rate_hz": func(a, b *database.Book) int {
		return derefInt(a.SampleRate) - derefInt(b.SampleRate)
	},
}

// applySorting sorts a slice of books in-place based on the filter's SortBy and SortOrder.
func applySorting(books []database.Book, f ListFilters) {
	if f.SortBy == "" {
		return
	}
	cmpFn, ok := sortFieldMap[f.SortBy]
	if !ok {
		return
	}
	sort.SliceStable(books, func(i, j int) bool {
		result := cmpFn(&books[i], &books[j])
		if result == 0 {
			// Tiebreaker: sort by ID for stable ordering
			result = strings.Compare(books[i].ID, books[j].ID)
		}
		if f.SortOrder == "desc" {
			return result > 0
		}
		return result < 0
	})
}

// matchesFieldFilters returns true if a book matches all the given field filters.
// All filters are ANDed: every filter must match for the book to be included.
func matchesFieldFilters(book database.Book, filters []FieldFilter) bool {
	for _, f := range filters {
		matches := fieldMatchesValue(book, f.Field, f.Value)
		if f.Negated && matches {
			return false // NOT filter: exclude if matches
		}
		if !f.Negated && !matches {
			return false // positive filter: exclude if doesn't match
		}
	}
	return true
}

// fieldMatchesValue checks whether a book's field value contains the search value
// (case-insensitive). Unknown fields return false.
func fieldMatchesValue(book database.Book, field, value string) bool {
	var bookValue string
	switch field {
	case "title":
		bookValue = book.Title
	case "author":
		if book.Author != nil {
			bookValue = book.Author.Name
		}
	case "narrator":
		bookValue = derefStr(book.Narrator)
	case "series":
		if book.Series != nil {
			bookValue = book.Series.Name
		}
	case "genre":
		bookValue = derefStr(book.Genre)
	case "language":
		bookValue = derefStr(book.Language)
	case "publisher":
		bookValue = derefStr(book.Publisher)
	case "edition":
		bookValue = derefStr(book.Edition)
	case "format":
		bookValue = book.Format
	case "codec":
		bookValue = derefStr(book.Codec)
	case "quality":
		bookValue = derefStr(book.Quality)
	case "library_state":
		bookValue = derefStr(book.LibraryState)
	case "description":
		bookValue = derefStr(book.Description)
	case "metadata_review_status", "review":
		bookValue = derefStr(book.MetadataReviewStatus)
	case "has_cover":
		if book.CoverURL != nil && *book.CoverURL != "" {
			bookValue = "yes"
		} else {
			bookValue = "no"
		}
	case "has_written":
		if book.LastWrittenAt != nil {
			bookValue = "yes"
		} else {
			bookValue = "no"
		}
	case "has_organized":
		if book.LastOrganizedAt != nil {
			bookValue = "yes"
		} else {
			bookValue = "no"
		}
	case "itunes_sync_status":
		bookValue = derefStr(book.ITunesSyncStatus)
	// Aliases for frontend field names
	case "duration_seconds":
		bookValue = fmt.Sprintf("%d", derefInt(book.Duration))
	case "bitrate_kbps":
		bookValue = fmt.Sprintf("%d", derefInt(book.Bitrate))
	case "file_size_bytes":
		bookValue = fmt.Sprintf("%d", derefInt64(book.FileSize))
	case "sample_rate_hz":
		bookValue = fmt.Sprintf("%d", derefInt(book.SampleRate))
	default:
		return false // unknown field
	}
	return strings.Contains(strings.ToLower(bookValue), strings.ToLower(value))
}

// GetAudiobooks retrieves audiobooks with optional filtering.
// Supports search, author_id, series_id, is_primary_version, and library_state filters.
func (svc *AudiobookService) GetAudiobooks(ctx context.Context, limit int, offset int, search string, authorID *int, seriesID *int, filters ...ListFilters) ([]database.Book, error) {
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

	var f ListFilters
	if len(filters) > 0 {
		f = filters[0]
	}
	hasSorting := f.SortBy != ""
	hasPostFilters := f.IsPrimaryVersion != nil || f.LibraryState != "" || f.Tag != "" || len(f.FieldFilters) > 0 || hasSorting

	// When post-filters are active, fetch all and filter in memory
	// (PebbleStore doesn't support query-level boolean/string filtering)
	storeLimit := limit
	storeOffset := offset
	if hasPostFilters {
		storeLimit = 0
		storeOffset = 0
	}

	// Initialize as empty slice to ensure we return [] instead of null
	books := []database.Book{}
	var err error

	// Apply filters in order of precedence
	if search != "" {
		if svc.searchIndex != nil {
			books, err = svc.searchWithBleve(search, limit, offset)
		} else {
			books, err = svc.store.SearchBooks(search, limit, offset)
		}
	} else if authorID != nil {
		books, err = svc.store.GetBooksByAuthorID(*authorID)
	} else if seriesID != nil {
		books, err = svc.store.GetBooksBySeriesID(*seriesID)
	}

	// Fall back to generic list only when no filter was applied
	if search == "" && authorID == nil && seriesID == nil {
		if !hasPostFilters {
			cacheKey := fmt.Sprintf("all:%d:%d", limit, offset)
			if cached, ok := svc.listCache.Get(cacheKey); ok {
				return cached, nil
			}
			books, err = svc.store.GetAllBooks(storeLimit, storeOffset)
			if err == nil && books != nil {
				svc.listCache.Set(cacheKey, books)
			}
		} else {
			books, err = svc.store.GetAllBooks(storeLimit, storeOffset)
		}
	}

	if err != nil {
		return nil, err
	}

	// Apply post-filters
	if hasPostFilters {
		// If tag filter is set, build a set of matching book IDs
		var tagBookIDs map[string]struct{}
		if f.Tag != "" {
			ids, tagErr := svc.store.GetBooksByTag(f.Tag)
			if tagErr != nil {
				return nil, tagErr
			}
			tagBookIDs = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				tagBookIDs[id] = struct{}{}
			}
		}

		filtered := make([]database.Book, 0, len(books))
		for _, b := range books {
			if f.Tag != "" {
				if _, ok := tagBookIDs[b.ID]; !ok {
					continue
				}
			}
			if f.IsPrimaryVersion != nil {
				bPrimary := b.IsPrimaryVersion != nil && *b.IsPrimaryVersion
				if *f.IsPrimaryVersion != bPrimary {
					continue
				}
			}
			if f.LibraryState != "" {
				bState := ""
				if b.LibraryState != nil {
					bState = *b.LibraryState
				}
				if bState != f.LibraryState {
					continue
				}
			}
			filtered = append(filtered, b)
		}

		// Apply field-specific filters (advanced search)
		if len(f.FieldFilters) > 0 {
			fieldFiltered := make([]database.Book, 0, len(filtered))
			for _, b := range filtered {
				if matchesFieldFilters(b, f.FieldFilters) {
					fieldFiltered = append(fieldFiltered, b)
				}
			}
			filtered = fieldFiltered
		}

		// Apply pagination after filtering
		if offset > 0 && offset < len(filtered) {
			filtered = filtered[offset:]
		} else if offset >= len(filtered) {
			filtered = nil
		}
		if limit > 0 && limit < len(filtered) {
			filtered = filtered[:limit]
		}
		books = filtered
	}

	// Apply sorting after all filtering but before returning
	applySorting(books, f)

	// Ensure we never return null - always return empty array
	if books == nil {
		books = []database.Book{}
	}

	return books, nil
}

// CountAudiobooksFiltered returns the count of audiobooks matching the given filters.
func (svc *AudiobookService) CountAudiobooksFiltered(ctx context.Context, filters ListFilters) (int, error) {
	if svc.store == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	books, err := svc.store.GetAllBooks(0, 0)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, b := range books {
		if filters.IsPrimaryVersion != nil {
			bPrimary := b.IsPrimaryVersion != nil && *b.IsPrimaryVersion
			if *filters.IsPrimaryVersion != bPrimary {
				continue
			}
		}
		if filters.LibraryState != "" {
			bState := ""
			if b.LibraryState != nil {
				bState = *b.LibraryState
			}
			if bState != filters.LibraryState {
				continue
			}
		}
		count++
	}
	return count, nil
}

// splitMultipleNames splits a name string on " & " to support multiple authors/narrators.
func splitMultipleNames(name string) []string {
	parts := strings.Split(name, " & ")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{name}
	}
	return result
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

	if cached, ok := svc.bookCache.Get(id); ok {
		return cached, nil
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

	meta := svc.extractBookFileMetadata(book, authorName)

	// Backfill duration (and other media info) from file if DB fields are missing
	if book.FilePath != "" && book.Duration == nil {
		if mi, miErr := mediainfo.Extract(book.FilePath); miErr == nil && mi.Duration > 0 {
			book.Duration = &mi.Duration
			if _, updErr := svc.store.UpdateBook(book.ID, book); updErr != nil {
				log.Printf("[WARN] GetAudiobook: failed to backfill duration for %s: %v", book.ID, updErr)
			}
		}
	}

	// Build metadata provenance
	book.MetadataProvenance = buildMetadataProvenance(book, state, meta, authorName, seriesName, nil)
	nowUTC := time.Now().UTC()
	book.MetadataProvenanceAt = &nowUTC

	svc.bookCache.Set(id, book)
	return book, nil
}

// GetAudiobookTags retrieves metadata tags and media info for an audiobook
func (svc *AudiobookService) GetAudiobookTags(ctx context.Context, id string, compareID string, snapshotTS string) (map[string]any, error) {
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

	meta := svc.extractBookFileMetadata(book, authorName)

	// Backfill empty media_info from file if DB fields are missing
	if book.FilePath != "" && (book.Codec == nil || book.Bitrate == nil || book.SampleRate == nil) {
		if mi, err := mediainfo.Extract(book.FilePath); err == nil {
			needsUpdate := false
			if book.Codec == nil && mi.Codec != "" {
				book.Codec = &mi.Codec
				needsUpdate = true
			}
			if book.Bitrate == nil && mi.Bitrate > 0 {
				book.Bitrate = &mi.Bitrate
				needsUpdate = true
			}
			if book.SampleRate == nil && mi.SampleRate > 0 {
				book.SampleRate = &mi.SampleRate
				needsUpdate = true
			}
			if book.Channels == nil && mi.Channels > 0 {
				book.Channels = &mi.Channels
				needsUpdate = true
			}
			if book.Duration == nil && mi.Duration > 0 {
				book.Duration = &mi.Duration
				needsUpdate = true
			}
			if needsUpdate {
				if _, err := svc.store.UpdateBook(book.ID, book); err != nil {
					log.Printf("[WARN] GetAudiobookTags: failed to backfill media info for %s: %v", book.ID, err)
				}
				response["media_info"] = map[string]any{
					"codec":       stringVal(book.Codec),
					"bitrate":     intVal(book.Bitrate),
					"sample_rate": intVal(book.SampleRate),
					"channels":    intVal(book.Channels),
					"bit_depth":   intVal(book.BitDepth),
					"quality":     stringVal(book.Quality),
					"duration":    intVal(book.Duration),
				}
			}
		}
	}

	// Load comparison metadata if compare_id is provided
	var comparisonValues map[string]any
	if snapshotTS != "" {
		ts, err := time.Parse(time.RFC3339Nano, snapshotTS)
		if err != nil {
			return nil, fmt.Errorf("invalid snapshot timestamp: %w", err)
		}
		snapshotBook, verErr := svc.store.GetBookAtVersion(id, ts)
		if verErr == nil && snapshotBook != nil {
			snapshotAuthorName, snapshotSeriesName := resolveAuthorAndSeriesNames(snapshotBook)
			comparisonValues = buildComparisonValuesFromBook(snapshotBook, snapshotAuthorName, snapshotSeriesName)
		} else {
			// Fallback: reconstruct "before" state from activity log old_values
			log.Printf("[DEBUG] GetAudiobookTags: GetBookAtVersion failed (%v), falling back to activity log for snapshot at %s", verErr, snapshotTS)
			if svc.activityService != nil {
				comparisonValues = buildComparisonValuesFromActivityLog(svc.activityService, id, ts)
			}
		}
	} else if compareID != "" {
		compBook, err := svc.store.GetBookByID(compareID)
		if err != nil {
			log.Printf("[WARN] GetAudiobookTags: failed to load comparison book %s: %v", compareID, err)
		} else if compBook != nil && compBook.FilePath != "" {
			if cm, err := metadata.ExtractMetadata(compBook.FilePath, nil); err == nil {
				comparisonValues = buildComparisonValuesFromMetadata(&cm)
			} else {
				log.Printf("[WARN] GetAudiobookTags: failed to extract comparison metadata for %s: %v", compBook.FilePath, err)
			}
		}
	}

	tags := buildMetadataProvenance(book, state, meta, authorName, seriesName, comparisonValues)
	response["tags"] = tags

	return response, nil
}

func (svc *AudiobookService) extractBookFileMetadata(book *database.Book, authorName string) metadata.Metadata {
	var meta metadata.Metadata
	if book == nil || book.FilePath == "" {
		return meta
	}

	m, err := metadata.ExtractMetadata(book.FilePath, nil)
	if err != nil {
		log.Printf("[WARN] audiobook_service: failed to extract metadata for %s: %v", book.FilePath, err)
		return meta
	}

	if m.OrganizerTagVersion == "" &&
		strings.TrimSpace(authorName) != "" &&
		strings.TrimSpace(m.Narrator) != "" &&
		strings.EqualFold(strings.TrimSpace(m.Artist), strings.TrimSpace(m.Narrator)) {
		m.Artist = authorName
		m.AuthorSource = "database author fallback"
	}

	return m
}

// GetDuplicateBooks retrieves all duplicate book groups
func (svc *AudiobookService) GetDuplicateBooks(ctx context.Context) (*DuplicatesResult, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get hash-based duplicates
	duplicateGroups, err := svc.store.GetDuplicateBooks()
	if err != nil {
		return nil, err
	}
	if duplicateGroups == nil {
		duplicateGroups = [][]database.Book{}
	}

	// Get folder-based duplicates (same title in same folder, e.g. M4B + MP3)
	folderGroups, err := svc.store.GetFolderDuplicates()
	if err != nil {
		log.Printf("[WARN] folder duplicate detection failed: %v", err)
	} else {
		// Merge folder groups, avoiding duplicate groups already found by hash
		seenBookIDs := map[string]bool{}
		for _, group := range duplicateGroups {
			for _, b := range group {
				seenBookIDs[b.ID] = true
			}
		}
		for _, group := range folderGroups {
			// Skip if all books in this group are already in hash-based groups
			allSeen := true
			for _, b := range group {
				if !seenBookIDs[b.ID] {
					allSeen = false
					break
				}
			}
			if !allSeen {
				duplicateGroups = append(duplicateGroups, group)
				for _, b := range group {
					seenBookIDs[b.ID] = true
				}
			}
		}
	}

	// Calculate total duplicates count
	totalDuplicates := 0
	for _, group := range duplicateGroups {
		totalDuplicates += len(group) - 1
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
	if limit <= 0 || limit > 10000 {
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
		// Tombstone external IDs so reimport is blocked
		if eidStore := asExternalIDStore(svc.store); eidStore != nil {
			extIDs, _ := eidStore.GetExternalIDsForBook(book.ID)
			for _, ext := range extIDs {
				_ = eidStore.TombstoneExternalID(ext.Source, ext.ExternalID)
			}
		}

		// Protect books with iTunes PIDs from import paths — these are the
		// canonical link to the iTunes library. Purging them would cause
		// reimport on the next iTunes sync.
		if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" &&
			book.ITunesImportSource != nil && *book.ITunesImportSource != "" {
			log.Printf("[DEBUG] purge: skipping %s (has iTunes PID %s)", book.ID, *book.ITunesPersistentID)
			continue
		}

		// Step 1: Create tombstone (snapshot of book for rollback)
		if err := svc.store.CreateBookTombstone(&book); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to create tombstone: %v", book.ID, err))
			continue
		}

		// Step 2: Delete from database (book record gone, tombstone preserved)
		if err := svc.store.DeleteBook(book.ID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to delete DB record: %v", book.ID, err))
			// Tombstone exists but book still exists — sweeper will clean up tombstone
			continue
		}

		// Step 3: Delete file if requested (only from organizer root, never from protected/import paths)
		if deleteFiles && book.FilePath != "" {
			if isProtectedPath(book.FilePath) {
				log.Printf("[DEBUG] purge: skipping file deletion for %s — protected path: %s", book.ID, book.FilePath)
			} else {
				info, statErr := os.Stat(book.FilePath)
				if statErr == nil && info.IsDir() {
					// Directory-based book: remove all book files then the directory
					if bookFiles, bfErr := svc.store.GetBookFiles(book.ID); bfErr == nil {
						for _, bf := range bookFiles {
							if bf.FilePath != "" && !isProtectedPath(bf.FilePath) {
								if rmErr := os.Remove(bf.FilePath); rmErr != nil && !os.IsNotExist(rmErr) {
									result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to delete book file %s: %v", book.ID, bf.FilePath, rmErr))
								}
							}
						}
					}
					// Remove the directory if it is now empty
					if entries, rdErr := os.ReadDir(book.FilePath); rdErr == nil && len(entries) == 0 {
						if rmErr := os.Remove(book.FilePath); rmErr != nil && !os.IsNotExist(rmErr) {
							result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to remove empty dir %s: %v", book.ID, book.FilePath, rmErr))
						} else if rmErr == nil {
							result.FilesDeleted++
							// Also clean up empty parent dirs up to RootDir
							if config.AppConfig.RootDir != "" {
								parentDir := filepath.Dir(book.FilePath)
								for parentDir != config.AppConfig.RootDir &&
									strings.HasPrefix(parentDir, config.AppConfig.RootDir) &&
									parentDir != "/" {
									pe, peErr := os.ReadDir(parentDir)
									if peErr != nil || len(pe) > 0 {
										break
									}
									if os.Remove(parentDir) != nil {
										break
									}
									parentDir = filepath.Dir(parentDir)
								}
							}
						}
					}
				} else if statErr == nil {
					// Single-file book
					if err := os.Remove(book.FilePath); err != nil && !os.IsNotExist(err) {
						result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to delete file (tombstone preserved): %v", book.ID, err))
						// DB record gone, file still exists, tombstone preserved for sweeper
					} else if err == nil {
						result.FilesDeleted++
						// Clean up empty parent dirs up to RootDir
						if config.AppConfig.RootDir != "" {
							parentDir := filepath.Dir(book.FilePath)
							for parentDir != config.AppConfig.RootDir &&
								strings.HasPrefix(parentDir, config.AppConfig.RootDir) &&
								parentDir != "/" {
								pe, peErr := os.ReadDir(parentDir)
								if peErr != nil || len(pe) > 0 {
									break
								}
								if os.Remove(parentDir) != nil {
									break
								}
								parentDir = filepath.Dir(parentDir)
							}
						}
					}
				}
				// If statErr is os.IsNotExist, file is already gone — that's fine
			}
		}

		// Step 4: Clean up tombstone (best-effort — sweeper handles failures)
		_ = svc.store.DeleteBookTombstone(book.ID)

		result.Purged++
	}

	if result.Purged > 0 {
		svc.InvalidateBookCaches()
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

	svc.InvalidateBookCaches()
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
	if req.Updates.FilePath != "" && req.Updates.FilePath != currentBook.FilePath {
		// Validate new file exists before accepting path change
		if _, err := os.Stat(req.Updates.FilePath); err != nil {
			return nil, fmt.Errorf("file does not exist at new path: %s", req.Updates.FilePath)
		}
		log.Printf("[INFO] audiobook_service: FilePath changed for %s: %s → %s", id, currentBook.FilePath, req.Updates.FilePath)
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

	// Create a MetadataStateService for recording change history.
	mss := NewMetadataStateService(svc.store)

	// Process overrides
	for field, override := range req.Updates.Overrides {
		entry := state[field]
		oldOverrideValue := entry.OverrideValue
		if override.Clear {
			entry.OverrideValue = nil
			entry.OverrideLocked = false
			entry.UpdatedAt = now
			// Record history for clearing an override.
			if fmt.Sprintf("%v", oldOverrideValue) != fmt.Sprintf("%v", nil) {
				mss.recordChange(id, field, "override", "user_edit", oldOverrideValue, nil)
			}
		} else {
			if len(override.Value) > 0 {
				val := decodeRawValue(override.Value)
				entry.OverrideValue = val
				entry.OverrideLocked = override.Locked == nil || *override.Locked
				entry.UpdatedAt = now
				applyOverrideToPayload(payload, field, val)
				// Record history for setting an override.
				if fmt.Sprintf("%v", oldOverrideValue) != fmt.Sprintf("%v", val) {
					mss.recordChange(id, field, "override", "user_edit", oldOverrideValue, val)
				}
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

	// Resolve author by name or ID — auto-split on " & " for multiple authors
	var resolvedAuthorName string
	if req.Updates.AuthorName != nil {
		name := strings.TrimSpace(*req.Updates.AuthorName)
		if name != "" {
			// Split on " & " to support multiple authors
			authorNames := splitMultipleNames(name)
			var bookAuthors []database.BookAuthor
			var primaryAuthorID int
			for i, aName := range authorNames {
				aName = strings.TrimSpace(aName)
				if aName == "" {
					continue
				}
				normalizedName := dedup.NormalizeAuthorName(aName)
				author, err := svc.store.GetAuthorByName(normalizedName)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve author")
				}
				if author == nil {
					author, err = svc.store.CreateAuthor(normalizedName)
					if err != nil {
						return nil, fmt.Errorf("failed to create author")
					}
				}
				role := "author"
				if i > 0 {
					role = "co-author"
				}
				bookAuthors = append(bookAuthors, database.BookAuthor{
					BookID: id, AuthorID: author.ID, Role: role, Position: i,
				})
				if i == 0 {
					primaryAuthorID = author.ID
				}
			}
			// Set primary author on the book for backward compat
			payload.AuthorID = &primaryAuthorID
			resolvedAuthorName = name // Keep the combined name for display
			// Save multiple authors to join table
			if len(bookAuthors) > 0 {
				if err := svc.store.SetBookAuthors(id, bookAuthors); err != nil {
					log.Printf("[WARN] failed to set book authors: %v", err)
				}
			}
		} else {
			payload.AuthorID = nil
		}
	} else if payload.AuthorID != nil {
		if author, err := svc.store.GetAuthorByID(*payload.AuthorID); err == nil && author != nil {
			resolvedAuthorName = author.Name
		}
	}

	// Resolve narrator — auto-split on " & " for multiple narrators
	if req.Updates.Narrator != nil {
		narStr := strings.TrimSpace(*req.Updates.Narrator)
		if narStr != "" {
			narratorNames := splitMultipleNames(narStr)
			var bookNarrators []database.BookNarrator
			for i, nName := range narratorNames {
				nName = strings.TrimSpace(nName)
				if nName == "" {
					continue
				}
				narrator, err := svc.store.GetNarratorByName(nName)
				if err != nil || narrator == nil {
					narrator, err = svc.store.CreateNarrator(nName)
					if err != nil {
						log.Printf("[WARN] failed to create narrator %q: %v", nName, err)
						continue
					}
				}
				role := "narrator"
				if i > 0 {
					role = "co-narrator"
				}
				bookNarrators = append(bookNarrators, database.BookNarrator{
					BookID: id, NarratorID: narrator.ID, Role: role, Position: i,
				})
			}
			if len(bookNarrators) > 0 {
				if err := svc.store.SetBookNarrators(id, bookNarrators); err != nil {
					log.Printf("[WARN] failed to set book narrators: %v", err)
				}
			}
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
			oldValue := entry.OverrideValue

			entry.OverrideValue = value
			entry.OverrideLocked = true
			entry.UpdatedAt = now
			state[field] = entry

			// Record history only when the value actually changed.
			if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", value) {
				mss.recordChange(id, field, "override", "user_edit", oldValue, value)
			}
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

	svc.InvalidateBookCaches()

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

		svc.InvalidateBookCaches()
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

	svc.InvalidateBookCaches()
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
	case "asin":
		if v, ok := value.(string); ok {
			payload.ASIN = stringPtr(v)
		}
	}
}

// --- User tag service methods ---

// ListAllUserTags returns all unique user tags with usage counts.
func (svc *AudiobookService) ListAllUserTags() ([]database.TagWithCount, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return svc.store.ListAllTags()
}

// GetBookUserTags returns all user tags for a specific book.
func (svc *AudiobookService) GetBookUserTags(bookID string) ([]string, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return svc.store.GetBookTags(bookID)
}

// SetBookUserTags replaces all user tags on a book and returns the new set.
func (svc *AudiobookService) SetBookUserTags(bookID string, tags []string) ([]string, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := svc.store.SetBookTags(bookID, tags); err != nil {
		return nil, err
	}
	svc.InvalidateBookCaches()
	return svc.store.GetBookTags(bookID)
}

// AddBookUserTag adds a single user tag to a book and returns all tags.
func (svc *AudiobookService) AddBookUserTag(bookID, tag string) ([]string, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := svc.store.AddBookTag(bookID, tag); err != nil {
		return nil, err
	}
	svc.InvalidateBookCaches()
	return svc.store.GetBookTags(bookID)
}

// RemoveBookUserTag removes a single user tag from a book and returns remaining tags.
func (svc *AudiobookService) RemoveBookUserTag(bookID, tag string) ([]string, error) {
	if svc.store == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := svc.store.RemoveBookTag(bookID, tag); err != nil {
		return nil, err
	}
	svc.InvalidateBookCaches()
	return svc.store.GetBookTags(bookID)
}

// BatchUpdateUserTags applies tag additions and removals to multiple books.
// Returns the number of books successfully updated.
func (svc *AudiobookService) BatchUpdateUserTags(bookIDs []string, addTags []string, removeTags []string) (int, error) {
	if svc.store == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	updated := 0
	for _, bookID := range bookIDs {
		for _, tag := range addTags {
			if err := svc.store.AddBookTag(bookID, tag); err != nil {
				log.Printf("[WARN] BatchUpdateUserTags: failed to add tag %q to book %s: %v", tag, bookID, err)
				continue
			}
		}
		for _, tag := range removeTags {
			if err := svc.store.RemoveBookTag(bookID, tag); err != nil {
				log.Printf("[WARN] BatchUpdateUserTags: failed to remove tag %q from book %s: %v", tag, bookID, err)
				continue
			}
		}
		updated++
	}
	if updated > 0 {
		svc.InvalidateBookCaches()
	}
	return updated, nil
}

// searchWithBleve parses the query via the DSL, translates to a
// Bleve native query, and returns the matching books. Per-user
// filters produced by the translator (read_status / progress_pct /
// last_played) are currently dropped here — the library-list route
// doesn't carry user state. Spec 3.6 will wire them back in at the
// handler layer once the user context is plumbed.
//
// Falls back to an empty slice (not nil) on zero matches so callers
// get consistent JSON shape.
func (svc *AudiobookService) searchWithBleve(query string, limit, offset int) ([]database.Book, error) {
	ast, err := search.ParseQuery(query)
	if err != nil {
		// Parser failure: fall back to the substring search path so
		// users still see results for simple queries the DSL parser
		// rejects (e.g. punctuation-heavy book titles).
		return svc.store.SearchBooks(query, limit, offset)
	}
	bleveQ, _, err := search.Translate(ast)
	if err != nil {
		return svc.store.SearchBooks(query, limit, offset)
	}
	hits, _, err := svc.searchIndex.SearchNative(bleveQ, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("bleve search: %w", err)
	}
	books := make([]database.Book, 0, len(hits))
	for _, h := range hits {
		b, _ := svc.store.GetBookByID(h.BookID)
		if b != nil {
			books = append(books, *b)
		}
	}
	return books, nil
}
