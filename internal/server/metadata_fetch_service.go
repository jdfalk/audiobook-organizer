// file: internal/server/metadata_fetch_service.go
// version: 4.38.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
)

type MetadataFetchService struct {
	db              database.Store
	olStore         *openlibrary.OLStore
	overrideSources []metadata.MetadataSource // for testing
	isbnEnrichment  *ISBNEnrichmentService
	activityService *ActivityService
}

// SetActivityService sets the activity service for dual-writing to the unified activity log.
func (mfs *MetadataFetchService) SetActivityService(svc *ActivityService) {
	mfs.activityService = svc
}

func NewMetadataFetchService(db database.Store) *MetadataFetchService {
	return &MetadataFetchService{db: db}
}

// SetOLStore sets the Open Library dump store for local-first lookups.
func (mfs *MetadataFetchService) SetOLStore(store *openlibrary.OLStore) {
	mfs.olStore = store
}

// SetISBNEnrichment sets the ISBN enrichment service for background ISBN/ASIN lookups.
func (mfs *MetadataFetchService) SetISBNEnrichment(svc *ISBNEnrichmentService) {
	mfs.isbnEnrichment = svc
}

// queueISBNEnrichment starts a background goroutine to enrich ISBN/ASIN for a book
// if the book is missing those identifiers.
func (mfs *MetadataFetchService) queueISBNEnrichment(id string, book *database.Book) {
	if mfs.isbnEnrichment == nil {
		return
	}
	needsISBN := (book.ISBN10 == nil || *book.ISBN10 == "") && (book.ISBN13 == nil || *book.ISBN13 == "")
	needsASIN := book.ASIN == nil || *book.ASIN == ""
	if !needsISBN && !needsASIN {
		return
	}
	go func(bid string) {
		found, err := mfs.isbnEnrichment.EnrichBookISBN(bid)
		if err != nil {
			log.Printf("[WARN] ISBN enrichment failed for %s: %v", bid, err)
		} else if found {
			log.Printf("[INFO] ISBN enrichment succeeded for %s", bid)
		}
	}(id)
}

type FetchMetadataResponse struct {
	Message         string
	Book            *database.Book
	Source          string
	FetchedCount    int
	PendingCoverURL string // set by ApplyMetadataCandidate for background download
}

// MetadataCandidate represents a single search result for manual metadata matching.
type MetadataCandidate struct {
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	Narrator       string  `json:"narrator,omitempty"`
	Series         string  `json:"series,omitempty"`
	SeriesPosition string  `json:"series_position,omitempty"`
	Year           int     `json:"year,omitempty"`
	Publisher      string  `json:"publisher,omitempty"`
	ISBN           string  `json:"isbn,omitempty"`
	ASIN           string  `json:"asin,omitempty"`
	CoverURL       string  `json:"cover_url,omitempty"`
	Description    string  `json:"description,omitempty"`
	Language       string  `json:"language,omitempty"`
	Source         string  `json:"source"`
	Score          float64 `json:"score"`
}

// SearchMetadataResponse is returned by SearchMetadataForBook.
type SearchMetadataResponse struct {
	Results       []MetadataCandidate `json:"results"`
	Query         string              `json:"query"`
	SourcesTried  []string            `json:"sources_tried"`
	SourcesFailed map[string]string   `json:"sources_failed,omitempty"`
}

// BuildSourceChain returns metadata sources ordered by config priority.
// Each source is wrapped with a circuit breaker that opens after 5 consecutive
// failures and retries after 30 seconds.
func (mfs *MetadataFetchService) BuildSourceChain() []metadata.MetadataSource {
	// Copy and sort by priority
	sources := make([]config.MetadataSource, len(config.AppConfig.MetadataSources))
	copy(sources, config.AppConfig.MetadataSources)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Priority < sources[j].Priority
	})

	var chain []metadata.MetadataSource
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		var rawSource metadata.MetadataSource
		switch src.ID {
		case "openlibrary":
			client := metadata.NewOpenLibraryClient()
			if mfs.olStore != nil {
				client.SetOLStore(mfs.olStore)
			}
			rawSource = client
		case "google-books":
			apiKey := config.AppConfig.GoogleBooksAPIKey
			if apiKey == "" {
				if k, ok := src.Credentials["apiKey"]; ok && k != "" {
					apiKey = k
				}
			}
			rawSource = metadata.NewGoogleBooksClient(apiKey)
		case "audible":
			rawSource = metadata.NewAudibleClient()
		case "audnexus":
			rawSource = metadata.NewAudnexusClient()
		case "hardcover":
			token := config.AppConfig.HardcoverAPIToken
			if token == "" {
				// Also check credentials map from metadata source config
				if apiToken, ok := src.Credentials["api_token"]; ok && apiToken != "" {
					token = apiToken
				} else if apiKey, ok := src.Credentials["apiKey"]; ok && apiKey != "" {
					token = apiKey
				}
			}
			if token != "" {
				rawSource = metadata.NewHardcoverClient(token)
			} else {
				log.Printf("[WARN] Hardcover source enabled but no API token configured")
			}
		case "wikipedia":
			rawSource = metadata.NewWikipediaClient()
		default:
			log.Printf("[WARN] Unknown metadata source: %s", src.ID)
		}
		if rawSource != nil {
			chain = append(chain, metadata.NewProtectedSource(rawSource, 5, 30*time.Second))
		}
	}
	return chain
}

// FetchMetadataForBook fetches and applies metadata for a single audiobook,
// trying each configured source in priority order until one succeeds.
func (mfs *MetadataFetchService) FetchMetadataForBook(id string) (*FetchMetadataResponse, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	if book.MetadataReviewStatus != nil && *book.MetadataReviewStatus == "no_match" {
		return nil, fmt.Errorf("book %q is marked as no-match; use search-metadata to re-evaluate", book.Title)
	}

	var sources []metadata.MetadataSource
	if len(mfs.overrideSources) > 0 {
		sources = mfs.overrideSources
	} else {
		sources = mfs.BuildSourceChain()
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no metadata sources enabled")
	}

	searchTitle := stripChapterFromTitle(book.Title)

	// Resolve current author and narrator for search refinement and scoring
	currentAuthor := ""
	if book.Author != nil {
		currentAuthor = book.Author.Name
	} else if book.AuthorID != nil {
		if author, aErr := mfs.db.GetAuthorByID(*book.AuthorID); aErr == nil && author != nil {
			currentAuthor = author.Name
		}
	}
	if isGarbageValue(currentAuthor) {
		currentAuthor = ""
	}
	currentNarrator := ""
	if book.Narrator != nil && *book.Narrator != "" && !isGarbageValue(*book.Narrator) {
		currentNarrator = *book.Narrator
	}

	var lastErr error
	for _, src := range sources {
		var results []metadata.BookMetadata
		var searchErr error

		// Try title+author search first for better match quality
		if currentAuthor != "" {
			results, searchErr = src.SearchByTitleAndAuthor(searchTitle, currentAuthor)
			if searchErr != nil {
				log.Printf("[WARN] %s title+author search failed for %q by %q: %v", src.Name(), searchTitle, currentAuthor, searchErr)
			}
		}

		// Fall back to title-only search
		if len(results) == 0 {
			results, searchErr = src.SearchByTitle(searchTitle)
			if searchErr != nil {
				log.Printf("[WARN] %s failed for %q: %v", src.Name(), searchTitle, searchErr)
				lastErr = searchErr
			}
		}

		// Try original title if cleaned title returned nothing
		if len(results) == 0 && searchTitle != book.Title {
			results, searchErr = src.SearchByTitle(book.Title)
			if searchErr != nil {
				lastErr = searchErr
				continue
			}
		}

		// Try with subtitle stripped (e.g. "Title: Subtitle" → "Title")
		if len(results) == 0 {
			strippedTitle := stripSubtitle(searchTitle)
			if strippedTitle != searchTitle && strippedTitle != book.Title {
				results, searchErr = src.SearchByTitle(strippedTitle)
				if searchErr != nil {
					lastErr = searchErr
					continue
				}
			}
		}

		if len(results) == 0 {
			log.Printf("[DEBUG] %s returned 0 results for %q", src.Name(), searchTitle)
		}
		if len(results) > 0 {
			// Score all results and pick the best; reject if below quality threshold.
			scored := bestTitleMatchWithContext(results, currentAuthor, currentNarrator, searchTitle, book.Title)
			if len(scored) == 0 {
				log.Printf("[DEBUG] %s: all %d results rejected by quality scorer for %q",
					src.Name(), len(results), searchTitle)
				continue // try next source
			}
			// Apply series position filter if the book's position is already known.
			if book.SeriesSequence != nil {
				scored = applySeriesPositionFilter(scored, *book.SeriesSequence)
				if len(scored) == 0 {
					log.Printf("[DEBUG] %s: best result rejected by series position filter for %q",
						src.Name(), searchTitle)
					continue
				}
			}
			meta := scored[0]

			// Parse series string if present (e.g. "(Long Earth 05) The Long Cosmos")
			parsedSeries, parsedPosition, parsedTitle := parseSeriesFromTitle(meta.Title)
			if parsedSeries == "" && meta.Series != "" {
				parsedSeries, parsedPosition, parsedTitle = parseSeriesFromTitle(meta.Series)
				if parsedTitle == "" {
					parsedTitle = meta.Title
				}
			}
			if parsedSeries != "" {
				meta.Series = parsedSeries
				meta.SeriesPosition = parsedPosition
				if parsedTitle != "" {
					meta.Title = parsedTitle
				}
			}

			// Safety: never apply empty/untitled metadata
			if meta.Title == "" || strings.ToLower(meta.Title) == "untitled" {
				meta.Title = book.Title // keep original
			}

			// Record history before applying changes
			mfs.recordChangeHistory(book, meta, src.Name())

			// Apply metadata with downgrade protection
			mfs.applyMetadataToBook(book, meta)

			updatedBook, updateErr := mfs.db.UpdateBook(id, book)
			if updateErr != nil {
				return nil, fmt.Errorf("failed to update book: %w", updateErr)
			}

			mfs.persistFetchedMetadata(id, meta)

			// Download cover art locally if we got a cover URL
			if meta.CoverURL != "" && config.AppConfig.RootDir != "" {
				coverPath, coverErr := metadata.DownloadCoverArt(meta.CoverURL, config.AppConfig.RootDir, id)
				if coverErr != nil {
					log.Printf("[WARN] cover art download failed for %s: %v", id, coverErr)
				} else {
					log.Printf("[INFO] cover art saved to %s", coverPath)
					// Update book's cover_url to the local path for serving
					localCoverURL := "/api/v1/covers/local/" + filepath.Base(coverPath)
					if updatedBook != nil {
						updatedBook.CoverURL = &localCoverURL
						// Write the full book back — UpdateBook does full column
						// replacement, so passing only CoverURL would wipe everything.
						mfs.db.UpdateBook(id, updatedBook)
					}
					// Embed cover art into all audio files for this book
					if updatedBook != nil {
						mfs.embedCoverInBookFiles(updatedBook, coverPath)
					}
				}
			}

			// Write metadata back to audio file(s) if enabled
			if config.AppConfig.WriteBackMetadata {
				mfs.writeBackMetadata(updatedBook, meta)
			}

			// Queue background ISBN/ASIN enrichment if identifiers are missing
			if updatedBook != nil {
				mfs.queueISBNEnrichment(id, updatedBook)
			}

			return &FetchMetadataResponse{
				Message: "metadata fetched and applied",
				Book:    updatedBook,
				Source:  src.Name(),
			}, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no metadata found from any source (last error: %v)", lastErr)
	}
	return nil, fmt.Errorf("no metadata found for '%s' from any source", book.Title)
}

// FetchMetadataForBookByTitle searches metadata sources using only the book's title,
// suppressing the author name. This is useful when the current author is a production
// company and we want to discover the real author from external sources.
func (mfs *MetadataFetchService) FetchMetadataForBookByTitle(id string) (*FetchMetadataResponse, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	var sources []metadata.MetadataSource
	if len(mfs.overrideSources) > 0 {
		sources = mfs.overrideSources
	} else {
		sources = mfs.BuildSourceChain()
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no metadata sources enabled")
	}

	searchTitle := stripChapterFromTitle(book.Title)

	// Resolve narrator for scoring (author intentionally suppressed in this path)
	titleOnlyNarrator := ""
	if book.Narrator != nil && *book.Narrator != "" && !isGarbageValue(*book.Narrator) {
		titleOnlyNarrator = *book.Narrator
	}

	var lastErr error
	for _, src := range sources {
		results, searchErr := src.SearchByTitle(searchTitle)
		if searchErr != nil {
			lastErr = searchErr
			continue
		}
		if len(results) == 0 && searchTitle != book.Title {
			results, searchErr = src.SearchByTitle(book.Title)
			if searchErr != nil {
				lastErr = searchErr
				continue
			}
		}
		if len(results) == 0 {
			strippedTitle := stripSubtitle(searchTitle)
			if strippedTitle != searchTitle {
				results, searchErr = src.SearchByTitle(strippedTitle)
				if searchErr != nil {
					lastErr = searchErr
					continue
				}
			}
		}
		if len(results) == 0 {
			continue
		}

		scored := bestTitleMatchWithContext(results, "", titleOnlyNarrator, searchTitle, book.Title)
		if len(scored) == 0 {
			continue
		}
		meta := scored[0]

		parsedSeries, parsedPosition, parsedTitle := parseSeriesFromTitle(meta.Title)
		if parsedSeries == "" && meta.Series != "" {
			parsedSeries, parsedPosition, parsedTitle = parseSeriesFromTitle(meta.Series)
			if parsedTitle == "" {
				parsedTitle = meta.Title
			}
		}
		if parsedSeries != "" {
			meta.Series = parsedSeries
			meta.SeriesPosition = parsedPosition
			if parsedTitle != "" {
				meta.Title = parsedTitle
			}
		}

		mfs.recordChangeHistory(book, meta, src.Name())
		mfs.applyMetadataToBook(book, meta)

		updatedBook, updateErr := mfs.db.UpdateBook(id, book)
		if updateErr != nil {
			return nil, fmt.Errorf("failed to update book: %w", updateErr)
		}

		mfs.persistFetchedMetadata(id, meta)

		return &FetchMetadataResponse{
			Message: "metadata fetched by title only",
			Book:    updatedBook,
			Source:  src.Name(),
		}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no metadata found from any source (last error: %v)", lastErr)
	}
	return nil, fmt.Errorf("no metadata found for '%s' from any source (title-only search)", book.Title)
}

func (mfs *MetadataFetchService) applyMetadataToBook(book *database.Book, meta metadata.BookMetadata) {
	originalTitle := book.Title
	if meta.Title != "" && meta.Title != "Untitled" && isBetterValue(book.Title, meta.Title) {
		// Don't replace a real title with something shorter/worse
		if book.Title != "" && !isGarbageValue(book.Title) && len(meta.Title) < 3 {
			// Skip very short replacement titles
		} else {
			book.Title = meta.Title
		}
	}
	// Final safety: never leave title empty if it was set before
	if book.Title == "" && originalTitle != "" {
		book.Title = originalTitle
		log.Printf("[WARN] applyMetadataToBook: prevented title from being cleared for book %s", book.ID)
	}
	if meta.Publisher != "" && isBetterStringPtr(book.Publisher, meta.Publisher) {
		book.Publisher = stringPtr(meta.Publisher)
	}
	if meta.Language != "" && isBetterStringPtr(book.Language, meta.Language) {
		book.Language = stringPtr(meta.Language)
	}
	if meta.PublishYear != 0 {
		book.AudiobookReleaseYear = intPtrHelper(meta.PublishYear)
	}
	if meta.CoverURL != "" {
		book.CoverURL = stringPtr(meta.CoverURL)
	}
	if meta.Narrator != "" && !isGarbageValue(meta.Narrator) && isBetterStringPtr(book.Narrator, meta.Narrator) {
		book.Narrator = stringPtr(meta.Narrator)
	}

	// Apply author if fetched data is better — resolve to AuthorID and
	// replace the book_authors join table so stale associations are removed.
	extractedAuthor := meta.Author
	if extractedAuthor != "" && !isGarbageValue(extractedAuthor) {
		// Guard: if extracted artist matches the book's narrator (not the author),
		// the tag has narrator in the artist field — keep the DB author.
		if book.AuthorID != nil && book.Narrator != nil {
			if existingAuthor, aErr := mfs.db.GetAuthorByID(*book.AuthorID); aErr == nil && existingAuthor != nil {
				if strings.EqualFold(extractedAuthor, *book.Narrator) && !strings.EqualFold(extractedAuthor, existingAuthor.Name) {
					log.Printf("[INFO] applyMetadataToBook: extracted artist %q matches narrator %q but not author %q for book %s — skipping author update",
						extractedAuthor, *book.Narrator, existingAuthor.Name, book.ID)
					extractedAuthor = ""
				} else if !strings.EqualFold(extractedAuthor, existingAuthor.Name) && !strings.EqualFold(extractedAuthor, *book.Narrator) {
					// Extracted artist doesn't match either stored author or narrator — log mismatch for review
					log.Printf("[WARN] applyMetadataToBook: extracted artist %q matches neither author %q nor narrator %q for book %s",
						extractedAuthor, existingAuthor.Name, *book.Narrator, book.ID)
				}
			}
		}
	}
	if extractedAuthor != "" && !isGarbageValue(extractedAuthor) {
		author, err := mfs.db.GetAuthorByName(extractedAuthor)
		if err == nil && author == nil {
			author, err = mfs.db.CreateAuthor(extractedAuthor)
		}
		if err == nil && author != nil {
			book.AuthorID = &author.ID
			_ = mfs.db.SetBookAuthors(book.ID, []database.BookAuthor{
				{BookID: book.ID, AuthorID: author.ID, Role: "author", Position: 0},
			})
		}
	}

	// Apply ISBN/ASIN
	if meta.ISBN != "" {
		if len(meta.ISBN) == 10 {
			book.ISBN10 = stringPtr(meta.ISBN)
		} else {
			book.ISBN13 = stringPtr(meta.ISBN)
		}
	}
	if meta.ASIN != "" {
		book.ASIN = stringPtr(meta.ASIN)
	}
	if meta.Description != "" {
		book.Description = stringPtr(meta.Description)
	}
	if meta.Genre != "" {
		book.Genre = stringPtr(meta.Genre)
	}

	// Apply series info if available
	if meta.Series != "" && !isGarbageValue(meta.Series) {
		series, err := mfs.db.GetSeriesByName(meta.Series, book.AuthorID)
		if err == nil && series == nil {
			series, err = mfs.db.CreateSeries(meta.Series, book.AuthorID)
		}
		if err == nil && series != nil {
			book.SeriesID = &series.ID
		}
		if meta.SeriesPosition != "" {
			if pos, err := strconv.Atoi(meta.SeriesPosition); err == nil {
				book.SeriesSequence = &pos
			}
		}
	}
}

// isGarbageValue returns true if a string value is effectively useless metadata.
func isGarbageValue(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	garbage := []string{"unknown", "narrator", "various", "n/a", "none", "null", "undefined", "",
		"test", "untitled", "no title", "no author", "various authors", "various artists"}
	for _, g := range garbage {
		if lower == g {
			return true
		}
	}
	// Reject HTML fragments or error messages that may leak from Wikipedia/API errors
	if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") ||
		strings.Contains(lower, "403 forbidden") || strings.Contains(lower, "error") {
		return true
	}
	return false
}

// isBetterValue returns true if newVal should replace oldVal.
// Never replaces a good value with garbage.
func isBetterValue(oldVal, newVal string) bool {
	if isGarbageValue(newVal) {
		return false
	}
	if isGarbageValue(oldVal) {
		return true
	}
	// Both are real values; allow the update (fetched data may be more accurate)
	return true
}

// isBetterStringPtr returns true if newVal should replace the existing *string.
func isBetterStringPtr(oldPtr *string, newVal string) bool {
	if isGarbageValue(newVal) {
		return false
	}
	if oldPtr == nil || isGarbageValue(*oldPtr) {
		return true
	}
	// Both are real values; allow the update
	return true
}

// recordChangeHistory records metadata changes before they are applied.
func (mfs *MetadataFetchService) recordChangeHistory(book *database.Book, meta metadata.BookMetadata, sourceName string) {
	now := time.Now()

	// Resolve current author name for history
	var currentAuthor string
	if book.AuthorID != nil {
		if author, err := mfs.db.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
			currentAuthor = author.Name
		}
	}

	// Resolve current series name for history
	var currentSeries string
	if book.SeriesID != nil {
		if series, err := mfs.db.GetSeriesByID(*book.SeriesID); err == nil && series != nil {
			currentSeries = series.Name
		}
	}

	changes := []struct {
		field    string
		oldVal   string
		newVal   string
	}{
		{"title", book.Title, meta.Title},
		{"author_name", currentAuthor, meta.Author},
		{"narrator", derefString(book.Narrator), meta.Narrator},
		{"publisher", derefString(book.Publisher), meta.Publisher},
		{"language", derefString(book.Language), meta.Language},
		{"series", currentSeries, meta.Series},
		{"series_position", derefIntAsString(book.SeriesSequence), meta.SeriesPosition},
		{"cover_url", derefString(book.CoverURL), meta.CoverURL},
	}

	if meta.PublishYear != 0 {
		changes = append(changes, struct {
			field  string
			oldVal string
			newVal string
		}{"audiobook_release_year", derefIntAsString(book.AudiobookReleaseYear), strconv.Itoa(meta.PublishYear)})
	}

	for _, c := range changes {
		if c.newVal == "" || c.newVal == c.oldVal {
			continue
		}
		oldJSON := jsonEncodeString(c.oldVal)
		newJSON := jsonEncodeString(c.newVal)
		record := &database.MetadataChangeRecord{
			BookID:        book.ID,
			Field:         c.field,
			PreviousValue: &oldJSON,
			NewValue:      &newJSON,
			ChangeType:    "fetched",
			Source:        sourceName,
			ChangedAt:     now,
		}
		if err := mfs.db.RecordMetadataChange(record); err != nil {
			log.Printf("[WARN] failed to record metadata change for %s.%s: %v", book.ID, c.field, err)
		}
		// Dual-write to unified activity log
		if mfs.activityService != nil {
			_ = mfs.activityService.Record(database.ActivityEntry{
				Tier:    "change",
				Type:    "metadata_apply",
				Level:   "info",
				Source:  "background",
				BookID:  book.ID,
				Summary: fmt.Sprintf("Applied %s: %s → %s", c.field, truncateActivity(c.oldVal, 50), truncateActivity(c.newVal, 50)),
				Details: map[string]any{"field": c.field, "old_value": c.oldVal, "new_value": c.newVal, "source": sourceName},
			})
		}
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefIntAsString(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func jsonEncodeString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// parseSeriesFromTitle extracts series name, position, and title from strings like:
//   - "(Long Earth 05) The Long Cosmos" -> series="Long Earth", pos="5", title="The Long Cosmos"
//   - "(Series Name 3) Title" -> series="Series Name", pos="3", title="Title"
//   - "Long Earth 05 - The Long Cosmos" -> series="Long Earth", pos="5", title="The Long Cosmos"
func parseSeriesFromTitle(s string) (series, position, title string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", ""
	}

	// Pattern 1: "(Series Name NN) Title"
	parenRe := regexp.MustCompile(`^\((.+?)\s+(\d+)\)\s*(.*)$`)
	if m := parenRe.FindStringSubmatch(s); m != nil {
		pos := strings.TrimLeft(m[2], "0")
		if pos == "" {
			pos = "0"
		}
		return strings.TrimSpace(m[1]), pos, strings.TrimSpace(m[3])
	}

	// Pattern 2: "(Series Name #NN) Title"
	parenHashRe := regexp.MustCompile(`^\((.+?)\s+#(\d+)\)\s*(.*)$`)
	if m := parenHashRe.FindStringSubmatch(s); m != nil {
		pos := strings.TrimLeft(m[2], "0")
		if pos == "" {
			pos = "0"
		}
		return strings.TrimSpace(m[1]), pos, strings.TrimSpace(m[3])
	}

	// Pattern 3: "Series Name, Book NN" (no title extraction)
	commaBookRe := regexp.MustCompile(`^(.+?),\s*[Bb]ook\s+(\d+)$`)
	if m := commaBookRe.FindStringSubmatch(s); m != nil {
		pos := strings.TrimLeft(m[2], "0")
		if pos == "" {
			pos = "0"
		}
		return strings.TrimSpace(m[1]), pos, ""
	}

	return "", "", ""
}

// writeBackMetadata writes enriched metadata back to audio file(s).
func (mfs *MetadataFetchService) writeBackMetadata(book *database.Book, meta metadata.BookMetadata) {
	// --- Resolve author names (same logic as WriteBackMetadataForBook) ---
	var authorNames []string
	if bookAuthors, err := mfs.db.GetBookAuthors(book.ID); err == nil && len(bookAuthors) > 0 {
		for _, ba := range bookAuthors {
			if author, aerr := mfs.db.GetAuthorByID(ba.AuthorID); aerr == nil && author != nil {
				authorNames = append(authorNames, author.Name)
			}
		}
	} else if book.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			authorNames = append(authorNames, author.Name)
		}
	}
	if len(authorNames) == 0 && meta.Author != "" {
		authorNames = append(authorNames, meta.Author)
	}
	artistStr := strings.Join(authorNames, ", ")

	// --- Resolve narrator names ---
	var narratorNames []string
	if bookNarrators, err := mfs.db.GetBookNarrators(book.ID); err == nil && len(bookNarrators) > 0 {
		for _, bn := range bookNarrators {
			if narrator, nerr := mfs.db.GetNarratorByID(bn.NarratorID); nerr == nil && narrator != nil {
				narratorNames = append(narratorNames, narrator.Name)
			}
		}
	} else if book.Narrator != nil && *book.Narrator != "" {
		narratorNames = append(narratorNames, *book.Narrator)
	}
	narratorStr := strings.Join(narratorNames, " & ")

	// --- Determine year ---
	year := 0
	if book.AudiobookReleaseYear != nil && *book.AudiobookReleaseYear > 0 {
		year = *book.AudiobookReleaseYear
	} else if book.PrintYear != nil && *book.PrintYear > 0 {
		year = *book.PrintYear
	} else if meta.PublishYear > 0 {
		year = meta.PublishYear
	}

	bookTitle := meta.Title
	if bookTitle == "" {
		bookTitle = book.Title
	}

	opConfig := fileops.OperationConfig{VerifyChecksums: true}

	// CRITICAL: Never write metadata to files in protected paths (import paths,
	// iTunes Media folders). Only write to files in our organized library.
	if isProtectedPath(book.FilePath) {
		log.Printf("[INFO] skipping write-back for protected path: %s", book.FilePath)
		return
	}

	// Collect active book files for multi-file books
	bookFiles, bfErr := mfs.db.GetBookFiles(book.ID)
	var activeFiles []database.BookFile
	if bfErr == nil {
		for _, bf := range bookFiles {
			if !bf.Missing {
				activeFiles = append(activeFiles, bf)
			}
		}
	}

	totalTracks := len(activeFiles)

	if totalTracks > 1 {
		// Multi-file: write to each file with per-track title and numbering
		digits := len(fmt.Sprintf("%d", totalTracks))
		trackFmt := fmt.Sprintf("%%0%dd", digits)
		for i, bf := range activeFiles {
			trackNum := i + 1
			segTitle := fmt.Sprintf(trackFmt+" - %s", trackNum, bookTitle)
			trackStr := fmt.Sprintf("%d/%d", trackNum, totalTracks)
			tagMap := mfs.buildFullTagMap(book, bookTitle, segTitle, artistStr, narratorStr, year, trackStr)
			tagMap = filterUnchangedTags(bf.FilePath, tagMap)
			if len(tagMap) == 0 {
				continue
			}
			if isProtectedPath(bf.FilePath) {
				log.Printf("[INFO] skipping write-back for protected file: %s", bf.FilePath)
				continue
			}
			backupFileBeforeWrite(bf.FilePath)
			if err := metadata.WriteMetadataToFile(bf.FilePath, tagMap, opConfig); err != nil {
				log.Printf("[WARN] write-back failed for file %s: %v", bf.FilePath, err)
			}
		}
	} else {
		// Single-file or no segments: write to book.FilePath.
		// If book.FilePath is a directory (multi-file book with no segment records),
		// glob for audio files inside and write to each one individually.
		tagMap := mfs.buildFullTagMap(book, bookTitle, bookTitle, artistStr, narratorStr, year, "")
		log.Printf("[DEBUG] write-back: full tag map has %d entries for %s", len(tagMap), book.FilePath)
		for k, v := range tagMap {
			log.Printf("[DEBUG] write-back:   %s = %v", k, v)
		}

		dirFiles := audioFilesInDir(book.FilePath)
		if len(dirFiles) > 0 {
			// book.FilePath is a directory — write to each audio file found inside.
			log.Printf("[INFO] write-back: %s is a directory; writing to %d audio file(s) inside", book.FilePath, len(dirFiles))
			wroteAny := false
			for _, f := range dirFiles {
				fm := filterUnchangedTags(f, tagMap)
				if len(fm) == 0 {
					log.Printf("[DEBUG] write-back: all tags match, skipping %s", f)
					continue
				}
				backupFileBeforeWrite(f)
				if err := metadata.WriteMetadataToFile(f, fm, opConfig); err != nil {
					log.Printf("[WARN] write-back failed for %s: %v", f, err)
				} else {
					log.Printf("[INFO] wrote metadata back to %s", f)
					wroteAny = true
				}
			}
			if wroteAny {
				if err := mfs.db.SetLastWrittenAt(book.ID, time.Now()); err != nil {
					log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", book.ID, err)
				}
				_ = mfs.db.MarkNeedsRescan(book.ID)
			}
		} else {
			tagMap = filterUnchangedTags(book.FilePath, tagMap)
			log.Printf("[DEBUG] write-back: after filter, %d entries remain", len(tagMap))
			if len(tagMap) == 0 {
				log.Printf("[DEBUG] write-back: all tags match, skipping write for %s", book.FilePath)
				return
			}
			backupFileBeforeWrite(book.FilePath)
			if err := metadata.WriteMetadataToFile(book.FilePath, tagMap, opConfig); err != nil {
				log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
			} else {
				log.Printf("[INFO] wrote metadata back to %s", book.FilePath)
				// Stamp last_written_at after successful write-back.
				if err := mfs.db.SetLastWrittenAt(book.ID, time.Now()); err != nil {
					log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", book.ID, err)
				}
				// Flag for rescan so the next incremental scan re-reads the updated tags.
				_ = mfs.db.MarkNeedsRescan(book.ID)
			}
		}
	}
}

// scoreTitleStop is the set of common English stop-words excluded from scoring.
var scoreTitleStop = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "are": true, "was": true, "were": true,
	"been": true, "have": true, "has": true, "had": true, "not": true,
	"but": true, "its": true, "our": true, "your": true, "their": true,
	"all": true, "any": true, "can": true, "will": true, "may": true,
	"into": true,
}

// compilationRe detects "N books" patterns like "5 books" or "10 books".
var compilationRe = regexp.MustCompile(`\b\d+\s+books\b`)

// compilationPhrases is the list of lowercased substrings that mark a
// result as a compilation/box-set rather than a single title.
var compilationPhrases = []string{
	"box set", "boxset", "box-set",
	"collection",
	"complete series", "complete collection",
	"books set", "book set",
	"omnibus",
	"anthology",
	"compendium",
	"series collection", "series set",
}

// significantWords returns the deduplicated set of words longer than 2 chars
// that are not stop-words, all lowercased.
func significantWords(s string) map[string]bool {
	words := map[string]bool{}
	var allWords []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		// Strip leading/trailing punctuation (apostrophes, commas, etc.)
		w = strings.Trim(w, ".,;:!?\"'()")
		if w == "" {
			continue
		}
		allWords = append(allWords, w)
		if len(w) > 2 && !scoreTitleStop[w] {
			words[w] = true
		}
	}
	// If all words were filtered out (e.g. title is "14", "IT", "Us"),
	// include them all so scoring can still work.
	if len(words) == 0 {
		for _, w := range allWords {
			words[w] = true
		}
	}
	return words
}

// isCompilation returns true when the title appears to be a box-set,
// collection, omnibus, anthology, or other multi-title compilation.
func isCompilation(title string) bool {
	lower := strings.ToLower(title)
	for _, phrase := range compilationPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return compilationRe.MatchString(lower)
}

// scoreOneResult computes a quality score in [0, ~1.15] for a single result
// against a set of search-title significant words.
func scoreOneResult(r metadata.BookMetadata, searchWords map[string]bool) float64 {
	resultWords := significantWords(r.Title)

	if len(searchWords) == 0 || len(resultWords) == 0 {
		return 0
	}

	// Recall: how many search words appear in the result?
	recallHits := 0
	for w := range searchWords {
		if resultWords[w] {
			recallHits++
		}
	}
	recall := float64(recallHits) / float64(len(searchWords))

	// Precision: how many result words appear in the search?
	precHits := 0
	for w := range resultWords {
		if searchWords[w] {
			precHits++
		}
	}
	precision := float64(precHits) / float64(len(resultWords))

	// F1
	var f1 float64
	if recall+precision > 0 {
		f1 = 2 * recall * precision / (recall + precision)
	}

	// Compilation penalty
	if isCompilation(r.Title) {
		f1 *= 0.15
	}

	// Length penalty: penalise results that are much longer than the search
	nSearch := float64(len(searchWords))
	nResult := float64(len(resultWords))
	if nResult > 1.5*nSearch {
		f1 *= (1.5 * nSearch) / nResult
	}

	// Rich-metadata bonus (capped at +0.15)
	bonus := 0.0
	if r.Description != "" {
		bonus += 0.05
	}
	if r.CoverURL != "" {
		bonus += 0.05
	}
	if r.Narrator != "" {
		bonus += 0.05
	}
	if r.ISBN != "" {
		bonus += 0.05
	}
	if bonus > 0.15 {
		bonus = 0.15
	}

	return f1 + bonus
}

// applySeriesPositionFilter rejects the top result if it claims a different
// series position than the book's known position. If the result has no
// SeriesPosition or the book has no known position, results pass through.
func applySeriesPositionFilter(
	results []metadata.BookMetadata,
	knownPosition int,
) []metadata.BookMetadata {
	if len(results) == 0 || knownPosition <= 0 {
		return results
	}
	wantPos := strconv.Itoa(knownPosition)
	best := results[0]
	if best.SeriesPosition != "" && best.SeriesPosition != wantPos {
		log.Printf("[DEBUG] scorer: rejecting result %q (series position %q != expected %q)",
			best.Title, best.SeriesPosition, wantPos)
		return nil
	}
	return results
}

// bestTitleMatch filters results to find the single best match for the given
// title variants using precision+recall+penalty scoring.
//
// It replaces the old recall-only word-overlap function. A result must score
// at least 0.35 to be returned; if none qualify, nil is returned so the
// caller can fall through to the next source or report "no metadata found".
func bestTitleMatch(results []metadata.BookMetadata, titles ...string) []metadata.BookMetadata {
	return bestTitleMatchWithContext(results, "", "", titles...)
}

func bestTitleMatchWithContext(results []metadata.BookMetadata, bookAuthor, bookNarrator string, titles ...string) []metadata.BookMetadata {
	const minScore = 0.35

	// Union of significant words from all title variants.
	searchWords := map[string]bool{}
	for _, t := range titles {
		for w := range significantWords(t) {
			searchWords[w] = true
		}
	}

	bestIdx := -1
	bestScore := 0.0
	for i, r := range results {
		score := scoreOneResult(r, searchWords)

		// Author-based scoring: boost matches, penalize mismatches or missing
		if bookAuthor != "" {
			if r.Author != "" {
				rAuthorLower := strings.ToLower(r.Author)
				bAuthorLower := strings.ToLower(bookAuthor)
				if strings.Contains(rAuthorLower, bAuthorLower) || strings.Contains(bAuthorLower, rAuthorLower) {
					score *= 1.5
				} else {
					score *= 0.7
				}
			} else {
				score *= 0.75
			}
		}

		// Narrator-based scoring: boost matches as secondary tiebreaker
		if bookNarrator != "" && r.Narrator != "" {
			rNarrLower := strings.ToLower(r.Narrator)
			bNarrLower := strings.ToLower(bookNarrator)
			if strings.Contains(rNarrLower, bNarrLower) || strings.Contains(bNarrLower, rNarrLower) {
				score *= 1.3
			}
		}

		// Audiobook-specific: boost results with narrator, penalize without
		if r.Narrator != "" {
			score *= 1.15
		} else {
			score *= 0.85
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore >= minScore {
		return []metadata.BookMetadata{results[bestIdx]}
	}
	return nil
}

// syncMetadataToLibraryCopy copies metadata fields from the original book to
// the library copy so that both DB records stay in sync. This is needed because
// ApplyMetadataCandidate only updates the original book's DB record, leaving
// the library copy with stale metadata.
func (mfs *MetadataFetchService) syncMetadataToLibraryCopy(original, libCopy *database.Book) {
	// Sync display/metadata fields — preserve library copy's file/path/version fields
	libCopy.Title = original.Title
	libCopy.AuthorID = original.AuthorID
	libCopy.Narrator = original.Narrator
	libCopy.SeriesID = original.SeriesID
	libCopy.SeriesSequence = original.SeriesSequence
	libCopy.Publisher = original.Publisher
	libCopy.Language = original.Language
	libCopy.Description = original.Description
	libCopy.AudiobookReleaseYear = original.AudiobookReleaseYear
	libCopy.PrintYear = original.PrintYear
	libCopy.ISBN10 = original.ISBN10
	libCopy.ISBN13 = original.ISBN13
	libCopy.ASIN = original.ASIN
	libCopy.Edition = original.Edition
	libCopy.Genre = original.Genre
	libCopy.OpenLibraryID = original.OpenLibraryID
	libCopy.HardcoverID = original.HardcoverID
	libCopy.GoogleBooksID = original.GoogleBooksID
	libCopy.CoverURL = original.CoverURL
	libCopy.MetadataReviewStatus = original.MetadataReviewStatus

	if _, err := mfs.db.UpdateBook(libCopy.ID, libCopy); err != nil {
		log.Printf("[WARN] failed to sync metadata to library copy %s: %v", libCopy.ID, err)
	} else {
		log.Printf("[INFO] synced metadata from %s to library copy %s", original.ID, libCopy.ID)
	}

	// Also sync author associations
	if authors, err := mfs.db.GetBookAuthors(original.ID); err == nil && len(authors) > 0 {
		var newAuthors []database.BookAuthor
		for _, ba := range authors {
			newAuthors = append(newAuthors, database.BookAuthor{
				BookID: libCopy.ID, AuthorID: ba.AuthorID, Role: ba.Role, Position: ba.Position,
			})
		}
		_ = mfs.db.SetBookAuthors(libCopy.ID, newAuthors)
	}

	// Sync narrator associations
	if narrators, err := mfs.db.GetBookNarrators(original.ID); err == nil && len(narrators) > 0 {
		var newNarrators []database.BookNarrator
		for _, bn := range narrators {
			newNarrators = append(newNarrators, database.BookNarrator{
				BookID: libCopy.ID, NarratorID: bn.NarratorID, Role: bn.Role, Position: bn.Position,
			})
		}
		_ = mfs.db.SetBookNarrators(libCopy.ID, newNarrators)
	}
}

// ensureLibraryCopy returns a book record with files in the library folder.
// If the book is already in the library, returns it as-is. If the book is in a
// protected path (iTunes/import), looks for an existing library version or
// organizes (hard-links) the file(s) to the library and creates a new version record.
// For multi-file books, all segments are also organized and recreated.
func (mfs *MetadataFetchService) ensureLibraryCopy(book *database.Book) *database.Book {
	if config.AppConfig.RootDir == "" {
		return book // no library configured
	}
	if strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
		return book // already in library
	}
	if !isProtectedPath(book.FilePath) {
		return book // not protected, safe to modify
	}

	// Check for existing library version in the same version group
	if book.VersionGroupID != nil && *book.VersionGroupID != "" {
		siblings, err := mfs.db.GetBooksByVersionGroup(*book.VersionGroupID)
		if err == nil {
			for i := range siblings {
				if siblings[i].ID != book.ID && strings.HasPrefix(siblings[i].FilePath, config.AppConfig.RootDir) {
					log.Printf("[INFO] using existing library copy %s for protected book %s", siblings[i].ID, book.ID)
					return &siblings[i]
				}
			}
		}
	}

	// Collect file paths for multi-file books
	bookFiles, bfErr := mfs.db.GetBookFiles(book.ID)
	var activeFiles []database.BookFile
	if bfErr == nil {
		for _, bf := range bookFiles {
			if !bf.Missing {
				activeFiles = append(activeFiles, bf)
			}
		}
	}

	org := organizer.NewOrganizer(&config.AppConfig)
	var newBookPath string
	var pathMap map[string]string

	if len(activeFiles) > 1 {
		// Multi-file: organize all book files to library directory
		filePaths := make([]string, len(activeFiles))
		for i, bf := range activeFiles {
			filePaths[i] = bf.FilePath
		}
		targetDir, pm, err := org.OrganizeBookDirectory(book, filePaths)
		if err != nil {
			log.Printf("[WARN] failed to create library copy for multi-file book %s: %v", book.ID, err)
			return nil
		}
		pathMap = pm
		// Use the directory as the book's primary path
		newBookPath = targetDir
	} else {
		// Single-file: organize just the book file
		p, _, err := org.OrganizeBook(book)
		if err != nil {
			log.Printf("[WARN] failed to create library copy for %s: %v", book.ID, err)
			return nil
		}
		newBookPath = p
	}

	// Create version-linked record for the library copy
	isPrimary := true
	isNotPrimary := false
	organizedState := "organized"
	versionGroupID := ""
	if book.VersionGroupID != nil && *book.VersionGroupID != "" {
		versionGroupID = *book.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
	}

	newBook := *book
	newBook.ID = ulid.Make().String()
	newBook.FilePath = newBookPath
	newBook.LibraryState = &organizedState
	newBook.VersionGroupID = &versionGroupID
	newBook.IsPrimaryVersion = &isPrimary

	created, err := mfs.db.CreateBook(&newBook)
	if err != nil {
		log.Printf("[WARN] failed to create library book record for %s: %v", book.ID, err)
		return nil
	}

	// Copy book_authors to the new record
	if authors, err := mfs.db.GetBookAuthors(book.ID); err == nil && len(authors) > 0 {
		var newAuthors []database.BookAuthor
		for _, ba := range authors {
			newAuthors = append(newAuthors, database.BookAuthor{
				BookID: created.ID, AuthorID: ba.AuthorID, Role: ba.Role, Position: ba.Position,
			})
		}
		_ = mfs.db.SetBookAuthors(created.ID, newAuthors)
	}

	// Copy book files with updated file paths for multi-file books
	if len(activeFiles) > 1 && pathMap != nil {
		for _, bf := range activeFiles {
			newBF := bf
			newBF.ID = ulid.Make().String()
			newBF.BookID = created.ID
			if newPath, ok := pathMap[bf.FilePath]; ok {
				newBF.FilePath = newPath
				newBF.ITunesPath = computeITunesPath(newPath)
			}
			if err := mfs.db.CreateBookFile(&newBF); err != nil {
				log.Printf("[WARN] failed to copy book_file %s for library book %s: %v", bf.ID, created.ID, err)
			}
		}
	}

	// Demote original to non-primary
	book.VersionGroupID = &versionGroupID
	book.IsPrimaryVersion = &isNotPrimary
	_, _ = mfs.db.UpdateBook(book.ID, book)

	log.Printf("[INFO] created library copy %s -> %s for protected book %s (%d file(s))", newBookPath, created.ID, book.ID, len(activeFiles))
	return created
}

// embedCoverInBookFiles embeds cover art into all audio files for a book.
// Always overwrites existing cover art. Before overwriting, extracts the old
// cover and saves it as a timestamped version in covers/history/ so it can be
// restored later via the changelog.
func (mfs *MetadataFetchService) embedCoverInBookFiles(book *database.Book, coverPath string) {
	if book == nil || book.FilePath == "" || coverPath == "" {
		return
	}

	audioExts := map[string]bool{
		".mp3": true, ".m4b": true, ".m4a": true, ".aac": true,
		".ogg": true, ".flac": true,
	}

	// If book is in a protected path, get or create a library copy
	if isProtectedPath(book.FilePath) {
		libCopy := mfs.ensureLibraryCopy(book)
		if libCopy == nil {
			log.Printf("[WARN] cannot embed cover: no library copy for protected book %s", book.ID)
			return
		}
		book = libCopy
	}

	// collectFiles gathers all audio files that need cover embedding
	var files []string
	ext := strings.ToLower(filepath.Ext(book.FilePath))
	if audioExts[ext] {
		files = append(files, book.FilePath)
	} else {
		// Multi-file book
		bookFiles, err := mfs.db.GetBookFiles(book.ID)
		if err != nil {
			log.Printf("[WARN] failed to list book files for cover embedding on book %s: %v", book.ID, err)
			return
		}
		for _, bf := range bookFiles {
			if bf.Missing {
				continue
			}
			if isProtectedPath(bf.FilePath) {
				continue
			}
			bfExt := strings.ToLower(filepath.Ext(bf.FilePath))
			if audioExts[bfExt] {
				files = append(files, bf.FilePath)
			}
		}
	}

	if len(files) == 0 {
		return
	}

	// Archive the old cover from the first file before overwriting
	mfs.archiveExistingCover(book.ID, files[0])

	// Embed new cover into all files (always overwrite)
	embedded := 0
	for _, f := range files {
		if err := tagger.EmbedCoverArt(f, coverPath); err != nil {
			log.Printf("[WARN] cover art embedding failed for %s: %v", f, err)
		} else {
			embedded++
		}
	}
	if embedded > 0 {
		log.Printf("[INFO] cover art embedded into %d file(s) for book %s", embedded, book.ID)
	}
}

// archiveExistingCover extracts the current embedded cover art from an audio
// file and saves it as a timestamped version in covers/history/{bookID}/ so it
// can be restored later. Records a metadata change for changelog tracking.
func (mfs *MetadataFetchService) archiveExistingCover(bookID string, audioFilePath string) {
	data, mimeType, err := metadata.ExtractCoverArtBytes(audioFilePath)
	if err != nil || len(data) == 0 {
		return // no existing cover to archive
	}

	// Determine extension from MIME type
	ext := ".jpg"
	switch {
	case strings.Contains(mimeType, "png"):
		ext = ".png"
	case strings.Contains(mimeType, "webp"):
		ext = ".webp"
	case strings.Contains(mimeType, "gif"):
		ext = ".gif"
	}

	// Hash the cover data for deduplication
	coverHash := fmt.Sprintf("%x", sha256.Sum256(data))

	// Check if we already have this exact image archived (by hash)
	dedupDir := filepath.Join(config.AppConfig.RootDir, "covers", "dedup")
	if err := os.MkdirAll(dedupDir, 0775); err != nil {
		log.Printf("[WARN] failed to create cover dedup dir: %v", err)
		return
	}

	dedupPath := filepath.Join(dedupDir, coverHash+ext)
	if _, err := os.Stat(dedupPath); err != nil {
		// New unique image — save to dedup store
		if err := os.WriteFile(dedupPath, data, 0664); err != nil {
			log.Printf("[WARN] failed to write dedup cover for %s: %v", bookID, err)
			return
		}
	}

	// Create a history entry that references the dedup hash instead of storing a copy
	historyDir := filepath.Join(config.AppConfig.RootDir, "covers", "history", bookID)
	if err := os.MkdirAll(historyDir, 0775); err != nil {
		log.Printf("[WARN] failed to create cover history dir: %v", err)
		return
	}

	ts := time.Now().Format("20060102-150405")
	// History entry is a symlink to the dedup store to avoid duplicate storage
	archivePath := filepath.Join(historyDir, ts+ext)
	if err := os.Symlink(dedupPath, archivePath); err != nil {
		// Symlink failed (cross-device, Windows, etc.) — fall back to hardlink or copy
		if err := os.Link(dedupPath, archivePath); err != nil {
			// Hardlink also failed — just copy
			if err := os.WriteFile(archivePath, data, 0664); err != nil {
				log.Printf("[WARN] failed to archive old cover for %s: %v", bookID, err)
				return
			}
		}
	}
	log.Printf("[INFO] archived old cover art: %s (hash=%s)", archivePath, coverHash[:12])

	// Record in metadata change history so it appears in the changelog
	now := time.Now()
	summaryJSON := jsonEncodeString(fmt.Sprintf("cover_art: archived previous cover to %s", filepath.Base(archivePath)))
	record := &database.MetadataChangeRecord{
		BookID:     bookID,
		Field:      "cover_art",
		NewValue:   &summaryJSON,
		ChangeType: "cover-archive",
		Source:     "system",
		ChangedAt:  now,
	}
	if err := mfs.db.RecordMetadataChange(record); err != nil {
		log.Printf("[WARN] failed to record cover archive history for %s: %v", bookID, err)
	}
	// Dual-write to unified activity log
	if mfs.activityService != nil {
		_ = mfs.activityService.Record(database.ActivityEntry{
			Tier:    "change",
			Type:    "metadata_apply",
			Level:   "info",
			Source:  "background",
			BookID:  bookID,
			Summary: fmt.Sprintf("Archived cover art to %s", filepath.Base(archivePath)),
		})
	}
}

func (mfs *MetadataFetchService) persistFetchedMetadata(bookID string, meta metadata.BookMetadata) {
	fetchedValues := map[string]any{}
	if meta.Title != "" {
		fetchedValues["title"] = meta.Title
	}
	if meta.Publisher != "" {
		fetchedValues["publisher"] = meta.Publisher
	}
	if meta.Language != "" {
		fetchedValues["language"] = meta.Language
	}
	if meta.PublishYear != 0 {
		fetchedValues["audiobook_release_year"] = meta.PublishYear
	}
	if meta.CoverURL != "" {
		fetchedValues["cover_url"] = meta.CoverURL
	}
	if meta.Author != "" {
		fetchedValues["author_name"] = meta.Author
	}
	if meta.ISBN != "" {
		if len(meta.ISBN) == 10 {
			fetchedValues["isbn10"] = meta.ISBN
		} else {
			fetchedValues["isbn13"] = meta.ISBN
		}
	}
	if meta.ASIN != "" {
		fetchedValues["asin"] = meta.ASIN
	}
	if len(fetchedValues) > 0 {
		if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
			log.Printf("[ERROR] FetchMetadataForBook: failed to persist fetched metadata state: %v", err)
		}
	}
}

// SearchMetadataForBook searches all configured metadata sources and returns
// scored candidates for manual matching.
func (mfs *MetadataFetchService) SearchMetadataForBook(id string, query string, authorHint ...string) (*SearchMetadataResponse, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	searchTitle := query
	if searchTitle == "" {
		searchTitle = book.Title
	}
	searchTitle = stripChapterFromTitle(searchTitle)

	// If title is effectively empty but we have author/narrator hints,
	// use the author name as search query to get results
	if strings.TrimSpace(searchTitle) == "" || searchTitle == "-" {
		if len(authorHint) > 0 && authorHint[0] != "" {
			searchTitle = authorHint[0]
		} else if book.AuthorID != nil {
			if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
				searchTitle = author.Name
			}
		}
	}

	var sources []metadata.MetadataSource
	if len(mfs.overrideSources) > 0 {
		sources = mfs.overrideSources
	} else {
		sources = mfs.BuildSourceChain()
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no metadata sources enabled")
	}

	// Extract author, narrator, and series hints from variadic parameter
	searchAuthor := ""
	if len(authorHint) > 0 && authorHint[0] != "" {
		searchAuthor = strings.TrimSpace(authorHint[0])
	}
	searchNarrator := ""
	if len(authorHint) > 1 && authorHint[1] != "" {
		searchNarrator = strings.TrimSpace(authorHint[1])
	}
	searchSeries := ""
	if len(authorHint) > 2 && authorHint[2] != "" {
		searchSeries = strings.TrimSpace(authorHint[2])
	}

	// Always resolve the book's own author and narrator for scoring tiebreaks,
	// even when no explicit hints were provided in the search request
	bookAuthor := searchAuthor
	if bookAuthor == "" && book.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			bookAuthor = author.Name
		}
	}
	if isGarbageValue(bookAuthor) {
		bookAuthor = ""
	}
	bookNarrator := searchNarrator
	if bookNarrator == "" && book.Narrator != nil && *book.Narrator != "" {
		bookNarrator = *book.Narrator
	}
	if isGarbageValue(bookNarrator) {
		bookNarrator = ""
	}

	searchWords := significantWords(searchTitle)
	if book.Title != searchTitle {
		for w := range significantWords(book.Title) {
			searchWords[w] = true
		}
	}

	// Dedupe by lowercase title+author
	seen := map[string]bool{}
	var candidates []MetadataCandidate
	var sourcesTried []string
	sourcesFailed := map[string]string{}

	for _, src := range sources {
		var allResults []metadata.BookMetadata
		var lastErr error
		sourcesTried = append(sourcesTried, src.Name())

		// If author hint provided, use title+author search for better results
		if searchAuthor != "" {
			if results, serr := src.SearchByTitleAndAuthor(searchTitle, searchAuthor); serr == nil {
				allResults = append(allResults, results...)
			} else {
				lastErr = serr
				log.Printf("[DEBUG] metadata-search: %s SearchByTitleAndAuthor(%q, %q) error: %v", src.Name(), searchTitle, searchAuthor, serr)
			}
		}

		// Narrator-as-author fallback: author/narrator fields are frequently
		// swapped in audiobook metadata. Try searching with the narrator as
		// author to catch these cases.
		if bookNarrator != "" && bookNarrator != searchAuthor {
			if results, serr := src.SearchByTitleAndAuthor(searchTitle, bookNarrator); serr == nil {
				allResults = append(allResults, results...)
			} else {
				log.Printf("[DEBUG] metadata-search: %s narrator-as-author fallback(%q, %q) error: %v", src.Name(), searchTitle, bookNarrator, serr)
			}
		}

		// Always also search by title only to get broader results
		if results, serr := src.SearchByTitle(searchTitle); serr == nil {
			allResults = append(allResults, results...)
		} else {
			lastErr = serr
			log.Printf("[DEBUG] metadata-search: %s SearchByTitle(%q) error: %v", src.Name(), searchTitle, serr)
		}
		// SearchByTitle with original title if different
		if searchTitle != book.Title {
			if results, serr := src.SearchByTitle(book.Title); serr == nil {
				allResults = append(allResults, results...)
			} else {
				lastErr = serr
			}
		}

		// If all calls failed (no results and there was an error), record it
		if len(allResults) == 0 && lastErr != nil {
			sourcesFailed[src.Name()] = lastErr.Error()
		}

		log.Printf("[DEBUG] metadata-search: %s returned %d raw results for %q", src.Name(), len(allResults), searchTitle)

		for _, r := range allResults {
			key := strings.ToLower(r.Title + "|" + r.Author)
			if seen[key] {
				continue
			}
			seen[key] = true

			score := scoreOneResult(r, searchWords)
			if score <= 0 {
				log.Printf("[DEBUG] metadata-search: score=0 for %q by %q from %s", r.Title, r.Author, src.Name())
				continue
			}

			// Author-based scoring: boost matches, penalize mismatches or missing
			if bookAuthor != "" {
				if r.Author != "" {
					rAuthorLower := strings.ToLower(r.Author)
					bAuthorLower := strings.ToLower(bookAuthor)
					if strings.Contains(rAuthorLower, bAuthorLower) || strings.Contains(bAuthorLower, rAuthorLower) {
						score *= 1.5 // Strong boost for author match
					} else {
						score *= 0.7 // Penalize non-matching authors
					}
				} else {
					score *= 0.75 // Penalize results missing author when we know the book's author
				}
			}

			// Narrator-based scoring: boost matches as secondary tiebreaker
			if bookNarrator != "" && r.Narrator != "" {
				rNarrLower := strings.ToLower(r.Narrator)
				bNarrLower := strings.ToLower(bookNarrator)
				if strings.Contains(rNarrLower, bNarrLower) || strings.Contains(bNarrLower, rNarrLower) {
					score *= 1.3 // Boost for narrator match
				}
			}

			// Series-based scoring: boost results in the matching series
			if searchSeries != "" && r.Series != "" {
				rSeriesLower := strings.ToLower(r.Series)
				sSeriesLower := strings.ToLower(searchSeries)
				if strings.Contains(rSeriesLower, sSeriesLower) || strings.Contains(sSeriesLower, rSeriesLower) {
					score *= 1.4 // Boost for series match
				}
			}

			// Audiobook-specific scoring: boost results with narrator info,
			// penalize sparse results from non-audiobook sources
			if r.Narrator != "" {
				score *= 1.15 // Results with narrator are more likely correct audiobook matches
			} else {
				score *= 0.85 // Penalize results without narrator info (likely non-audiobook sources)
			}

			candidates = append(candidates, MetadataCandidate{
				Title:          r.Title,
				Author:         r.Author,
				Narrator:       r.Narrator,
				Series:         r.Series,
				SeriesPosition: r.SeriesPosition,
				Year:           r.PublishYear,
				Publisher:      r.Publisher,
				ISBN:            r.ISBN,
				ASIN:           r.ASIN,
				CoverURL:       r.CoverURL,
				Description:    r.Description,
				Language:        r.Language,
				Source:          src.Name(),
				Score:          score,
			})
		}
	}

	// Try ASIN lookup: either the whole query is an ASIN, or extract one from the query
	asinToLookup := ""
	if looksLikeASIN(searchTitle) {
		asinToLookup = searchTitle
	} else {
		asinToLookup = extractASIN(searchTitle)
	}
	if asinToLookup != "" {
		// Try Audible API first (more complete), fall back to Audnexus
		audibleClient := metadata.NewAudibleClient()
		result, err := audibleClient.LookupByASIN(asinToLookup)
		if err != nil || result == nil {
			log.Printf("[DEBUG] metadata-search: Audible API lookup for %q failed, trying Audnexus: %v", asinToLookup, err)
			audnexus := metadata.NewAudnexusClient()
			result, err = audnexus.LookupByASIN(asinToLookup)
		}
		if err == nil && result != nil {
			key := strings.ToLower(result.Title + "|" + result.Author)
			if !seen[key] {
				score := scoreOneResult(*result, searchWords)
				if score <= 0 {
					score = 1.0 // Direct ASIN match always scores high
				}
				candidates = append(candidates, MetadataCandidate{
					Title:          result.Title,
					Author:         result.Author,
					Narrator:       result.Narrator,
					Series:         result.Series,
					SeriesPosition: result.SeriesPosition,
					Year:           result.PublishYear,
					Publisher:      result.Publisher,
					ISBN:            result.ISBN,
					ASIN:           result.ASIN,
					CoverURL:       result.CoverURL,
					Description:    result.Description,
					Language:        result.Language,
					Source:          "Audnexus (Audible)",
					Score:          score,
				})
			}
		} else {
			log.Printf("[DEBUG] metadata-search: ASIN lookup for %q failed: %v", asinToLookup, err)
		}
	}

	// Filter out results without cover images — they're typically low-quality
	// entries that clutter the results. Keep them only if ALL results lack covers.
	var withCover []MetadataCandidate
	for _, c := range candidates {
		if c.CoverURL != "" {
			withCover = append(withCover, c)
		}
	}
	if len(withCover) > 0 {
		candidates = withCover
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Cap at 50 to support large series
	if len(candidates) > 50 {
		candidates = candidates[:50]
	}

	log.Printf("[DEBUG] metadata-search: returning %d candidates for %q (search words: %v)", len(candidates), searchTitle, searchWords)

	return &SearchMetadataResponse{
		Results:       candidates,
		Query:         searchTitle,
		SourcesTried:  sourcesTried,
		SourcesFailed: sourcesFailed,
	}, nil
}

// looksLikeASIN checks if a string looks like an Amazon ASIN (10 alphanumeric chars, typically starts with B0).
func looksLikeASIN(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 10 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}

// extractASIN finds an ASIN-like pattern (B0 followed by 8 alphanumeric chars) anywhere in the string.
func extractASIN(s string) string {
	s = strings.TrimSpace(s)
	// Split on whitespace and check each token
	for _, word := range strings.Fields(s) {
		word = strings.Trim(word, ",.;:!?()[]{}\"'")
		if looksLikeASIN(word) {
			return word
		}
	}
	return ""
}

// ApplyMetadataCandidate applies a user-selected metadata candidate to a book.
// If fields is non-empty, only the listed fields are applied.
func (mfs *MetadataFetchService) ApplyMetadataCandidate(id string, candidate MetadataCandidate, fields []string) (*FetchMetadataResponse, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return nil, fmt.Errorf("audiobook not found")
	}

	meta := metadata.BookMetadata{
		Title:          candidate.Title,
		Author:         candidate.Author,
		Narrator:       candidate.Narrator,
		Series:         candidate.Series,
		SeriesPosition: candidate.SeriesPosition,
		PublishYear:    candidate.Year,
		Publisher:      candidate.Publisher,
		ISBN:           candidate.ISBN,
		CoverURL:       candidate.CoverURL,
		Description:    candidate.Description,
		Language:        candidate.Language,
	}

	// If fields list is non-empty, zero out fields NOT in the list
	if len(fields) > 0 {
		allowed := map[string]bool{}
		for _, f := range fields {
			allowed[f] = true
		}
		if !allowed["title"] {
			meta.Title = ""
		}
		if !allowed["author"] {
			meta.Author = ""
		}
		if !allowed["narrator"] {
			meta.Narrator = ""
		}
		if !allowed["series"] {
			meta.Series = ""
			meta.SeriesPosition = ""
		}
		if !allowed["year"] {
			meta.PublishYear = 0
		}
		if !allowed["publisher"] {
			meta.Publisher = ""
		}
		if !allowed["isbn"] {
			meta.ISBN = ""
		}
		if !allowed["cover_url"] {
			meta.CoverURL = ""
		}
		if !allowed["description"] {
			meta.Description = ""
		}
		if !allowed["language"] {
			meta.Language = ""
		}
	}

	// Record history BEFORE applying changes so old values are correct
	mfs.recordChangeHistory(book, meta, candidate.Source)

	mfs.applyMetadataToBook(book, meta)

	// Set review status to matched
	matched := "matched"
	book.MetadataReviewStatus = &matched

	updatedBook, updateErr := mfs.db.UpdateBook(id, book)
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update book: %w", updateErr)
	}

	// Persist fetched values for provenance tracking
	mfs.persistFetchedMetadata(id, meta)

	// Generate segment titles (fast, DB-only)
	if err := mfs.generateSegmentTitles(id, updatedBook.Title); err != nil {
		log.Printf("[WARN] generate segment titles failed for %s: %v", id, err)
	}

	// Download cover art (fast network fetch + file write — keep inline so
	// the response includes the updated cover_url for the UI).
	if meta.CoverURL != "" && config.AppConfig.RootDir != "" {
		coverPath, coverErr := metadata.DownloadCoverArt(meta.CoverURL, config.AppConfig.RootDir, id)
		if coverErr != nil {
			log.Printf("[WARN] cover art download failed for %s: %v", id, coverErr)
		} else {
			log.Printf("[INFO] cover art saved to %s", coverPath)
			localCoverURL := "/api/v1/covers/local/" + filepath.Base(coverPath)
			if updatedBook != nil {
				updatedBook.CoverURL = &localCoverURL
				mfs.db.UpdateBook(id, updatedBook)
			}
		}
	}

	// Queue background ISBN/ASIN enrichment if identifiers are missing
	if updatedBook != nil {
		mfs.queueISBNEnrichment(id, updatedBook)
	}

	return &FetchMetadataResponse{
		Message: "metadata candidate applied",
		Book:    updatedBook,
		Source:  candidate.Source,
	}, nil
}

// ApplyMetadataFileIO runs the slow file operations after metadata is applied:
// cover embed, tag write-back, file rename. Cover download is done inline
// in ApplyMetadataCandidate so the response includes the updated cover URL.
// Designed to run in a background goroutine.
func (mfs *MetadataFetchService) ApplyMetadataFileIO(id string) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return
	}

	// Embed cover art into audio files (slow: ffmpeg)
	if config.AppConfig.RootDir != "" {
		mfs.embedCoverInBookFiles(book, metadata.CoverPathForBook(config.AppConfig.RootDir, id))
	}

	// Run file rename + tag write pipeline
	if config.AppConfig.AutoRenameOnApply || config.AppConfig.AutoWriteTagsOnApply {
		if err := mfs.runApplyPipeline(id, book); err != nil {
			log.Printf("[WARN] apply pipeline failed for %s: %v", id, err)
		}
	}
}

// MarkNoMatch marks a book as having no metadata match.
func (mfs *MetadataFetchService) MarkNoMatch(id string) error {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return fmt.Errorf("audiobook not found")
	}

	status := "no_match"
	book.MetadataReviewStatus = &status
	_, err = mfs.db.UpdateBook(id, book)
	return err
}

// WriteBackMetadataForBook reads current DB metadata for the book, resolves authors and
// narrators, writes comprehensive tags to all active audio file segments, and records a
// history entry. It is called by POST /api/v1/audiobooks/:id/write-back.
func (mfs *MetadataFetchService) WriteBackMetadataForBook(id string, segmentFilter ...[]string) (int, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return 0, fmt.Errorf("audiobook not found: %s", id)
	}

	// If book is in a protected path, write to the library copy instead.
	// Keep a reference to the original book so we can use its (freshly-updated)
	// metadata for building the tag map, rather than the library copy's stale data.
	originalBook := book
	originalID := id
	if isProtectedPath(book.FilePath) {
		libCopy := mfs.ensureLibraryCopy(book)
		if libCopy == nil {
			return 0, fmt.Errorf("cannot write back: no library copy for protected book %s", id)
		}
		// Sync metadata from the original book to the library copy so both
		// DB records stay in sync and the tag map uses current data.
		mfs.syncMetadataToLibraryCopy(originalBook, libCopy)
		book = libCopy
		id = libCopy.ID
	}

	// --- Resolve author names ---
	// Use the original book's ID for author/narrator lookup since that's where
	// ApplyMetadataCandidate stores the updated associations.
	var authorNames []string
	bookAuthors, err := mfs.db.GetBookAuthors(originalID)
	if err == nil && len(bookAuthors) > 0 {
		for _, ba := range bookAuthors {
			if author, aerr := mfs.db.GetAuthorByID(ba.AuthorID); aerr == nil && author != nil {
				authorNames = append(authorNames, author.Name)
			}
		}
	} else if originalBook.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*originalBook.AuthorID); aerr == nil && author != nil {
			authorNames = append(authorNames, author.Name)
		}
	}
	artistStr := strings.Join(authorNames, ", ")

	// --- Resolve narrator names ---
	var narratorNames []string
	bookNarrators, err := mfs.db.GetBookNarrators(originalID)
	if err == nil && len(bookNarrators) > 0 {
		for _, bn := range bookNarrators {
			if narrator, nerr := mfs.db.GetNarratorByID(bn.NarratorID); nerr == nil && narrator != nil {
				narratorNames = append(narratorNames, narrator.Name)
			}
		}
	} else if originalBook.Narrator != nil && *originalBook.Narrator != "" {
		narratorNames = append(narratorNames, *originalBook.Narrator)
	}
	narratorStr := strings.Join(narratorNames, " & ")

	// --- Determine year ---
	// Use original book's year since it has the freshly-applied metadata
	year := 0
	if originalBook.AudiobookReleaseYear != nil && *originalBook.AudiobookReleaseYear > 0 {
		year = *originalBook.AudiobookReleaseYear
	} else if originalBook.PrintYear != nil && *originalBook.PrintYear > 0 {
		year = *originalBook.PrintYear
	}

	opConfig := fileops.OperationConfig{VerifyChecksums: true}

	// --- Collect active book files ---
	bookFiles, bfErr := mfs.db.GetBookFiles(book.ID)
	if bfErr != nil {
		bookFiles = nil
	}
	// Filter to non-missing only
	var activeFiles []database.BookFile
	for _, bf := range bookFiles {
		if !bf.Missing {
			activeFiles = append(activeFiles, bf)
		}
	}

	// Apply optional segment/file filter
	if len(segmentFilter) > 0 && len(segmentFilter[0]) > 0 {
		filterSet := make(map[string]struct{}, len(segmentFilter[0]))
		for _, sid := range segmentFilter[0] {
			filterSet[sid] = struct{}{}
		}
		var filtered []database.BookFile
		for _, bf := range activeFiles {
			if _, ok := filterSet[bf.ID]; ok {
				filtered = append(filtered, bf)
			}
		}
		activeFiles = filtered
	}

	totalTracks := len(activeFiles)
	writtenCount := 0
	skippedProtected := 0

	// Embed cover art BEFORE writing tags — ffmpeg's cover embed (-map_metadata 0)
	// does not preserve freeform iTunes atoms, so we embed first and write tags last.
	if config.AppConfig.RootDir != "" {
		mfs.embedCoverInBookFiles(book, metadata.CoverPathForBook(config.AppConfig.RootDir, book.ID))
	}

	// Use the original book's title for tag content (it has freshly-applied metadata)
	bookTitle := originalBook.Title
	if totalTracks > 1 {
		// Multi-file: write to each file with per-track title and numbering
		digits := len(fmt.Sprintf("%d", totalTracks))
		trackFmt := fmt.Sprintf("%%0%dd", digits)
		for i, bf := range activeFiles {
			trackNum := i + 1
			segTitle := fmt.Sprintf(trackFmt+" - %s", trackNum, bookTitle)
			trackStr := fmt.Sprintf("%d/%d", trackNum, totalTracks)
			tagMap := mfs.buildFullTagMap(book, bookTitle, segTitle, artistStr, narratorStr, year, trackStr)
			tagMap = filterUnchangedTags(bf.FilePath, tagMap)
			if len(tagMap) == 0 {
				log.Printf("[DEBUG] write-back: file %s tags already match, skipping", bf.FilePath)
				continue
			}
			if isProtectedPath(bf.FilePath) {
				log.Printf("[DEBUG] skipping write-back for protected file: %s", bf.FilePath)
				skippedProtected++
				continue
			}
			backupFileBeforeWrite(bf.FilePath)
			if err := metadata.WriteMetadataToFile(bf.FilePath, tagMap, opConfig); err != nil {
				log.Printf("[WARN] write-back failed for file %s: %v", bf.FilePath, err)
			} else {
				writtenCount++
			}
		}
	} else {
		// Single-file or no files: write to book.FilePath.
		// If book.FilePath is a directory (multi-file book with no file records),
		// glob for audio files inside and write to each one individually.
		if isProtectedPath(book.FilePath) {
			log.Printf("[DEBUG] skipping write-back for protected path: %s", book.FilePath)
			skippedProtected++
		} else {
			fullTagMap := mfs.buildFullTagMap(book, bookTitle, bookTitle, artistStr, narratorStr, year, "")
			// Always write all tags — taglib fork handles custom MP4 atoms natively.
			// The filter previously skipped writes when standard tags matched,
			// but custom tags (NARRATOR, SERIES, etc.) need writing too.
			dirFiles := audioFilesInDir(book.FilePath)
			if len(dirFiles) > 0 {
				// book.FilePath is a directory — write to each audio file found inside.
				log.Printf("[INFO] write-back: %s is a directory; writing to %d audio file(s) inside", book.FilePath, len(dirFiles))
				for _, f := range dirFiles {
					if isProtectedPath(f) {
						log.Printf("[DEBUG] skipping write-back for protected file: %s", f)
						skippedProtected++
						continue
					}
					backupFileBeforeWrite(f)
					if err := metadata.WriteMetadataToFile(f, fullTagMap, opConfig); err != nil {
						log.Printf("[WARN] write-back failed for %s: %v", f, err)
					} else {
						log.Printf("[INFO] wrote metadata back to %s", f)
						writtenCount++
					}
				}
			} else {
				backupFileBeforeWrite(book.FilePath)
				if err := metadata.WriteMetadataToFile(book.FilePath, fullTagMap, opConfig); err != nil {
					log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
				} else {
					writtenCount++
				}
			}
		}
	}

	// --- Write to version-linked copies in the library folder ---
	if book.VersionGroupID != nil && *book.VersionGroupID != "" && config.AppConfig.RootDir != "" {
		siblings, sibErr := mfs.db.GetBooksByVersionGroup(*book.VersionGroupID)
		if sibErr == nil {
			for _, sib := range siblings {
				if sib.ID == book.ID {
					continue // already written above
				}
				if !strings.HasPrefix(sib.FilePath, config.AppConfig.RootDir) {
					continue // only write to library copies, leave import copies alone
				}
				if isProtectedPath(sib.FilePath) {
					continue
				}
				tagMap := mfs.buildTagMap(bookTitle, bookTitle, artistStr, narratorStr, year, "")
				tagMap = filterUnchangedTags(sib.FilePath, tagMap)
				if len(tagMap) == 0 {
					continue // tags already match, nothing to write
				}
				backupFileBeforeWrite(sib.FilePath)
				if err := metadata.WriteMetadataToFile(sib.FilePath, tagMap, opConfig); err != nil {
					log.Printf("[WARN] write-back failed for version-linked %s: %v", sib.FilePath, err)
				} else {
					writtenCount++
					log.Printf("[INFO] wrote metadata to version-linked copy: %s", sib.FilePath)
				}
			}
		}
	}

	// --- Record history entry ---
	now := time.Now()
	summaryVal := fmt.Sprintf("%q (wrote %d file(s))", book.Title, writtenCount)
	summaryJSON := jsonEncodeString(summaryVal)
	record := &database.MetadataChangeRecord{
		BookID:     book.ID,
		Field:      "write_back",
		NewValue:   &summaryJSON,
		ChangeType: "write-back",
		Source:     "manual",
		ChangedAt:  now,
	}
	if err := mfs.db.RecordMetadataChange(record); err != nil {
		log.Printf("[WARN] failed to record write-back history for %s: %v", book.ID, err)
	}
	// Dual-write to unified activity log (Task 16: tag_write)
	if mfs.activityService != nil && writtenCount > 0 {
		_ = mfs.activityService.Record(database.ActivityEntry{
			Tier:    "change",
			Type:    "tag_write",
			Level:   "info",
			Source:  "background",
			BookID:  book.ID,
			Summary: fmt.Sprintf("Wrote tags to %d file(s) for %s", writtenCount, book.Title),
		})
	}

	// Cover art already embedded above (before tag write) to prevent
	// ffmpeg's -map_metadata from clobbering freeform iTunes atoms.

	// Stamp last_written_at
	if writtenCount > 0 {
		if err := mfs.db.SetLastWrittenAt(book.ID, now); err != nil {
			log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", book.ID, err)
		}
		// Flag for rescan so the next incremental scan re-reads the updated tags.
		_ = mfs.db.MarkNeedsRescan(book.ID)
	}

	if skippedProtected > 0 {
		log.Printf("[INFO] write-back for book %s: wrote %d file(s), skipped %d protected path(s)", book.ID, writtenCount-skippedProtected, skippedProtected)
	}

	return writtenCount, nil
}

// audioFilesInDir returns the audio files found directly inside dir.
// It globs for common audiobook extensions. Returns nil if dir is not a
// directory or contains no matching files.
var audioExtensions = []string{"*.m4b", "*.m4a", "*.mp3", "*.flac", "*.ogg", "*.opus", "*.wma", "*.aac"}

func audioFilesInDir(dir string) []string {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	var files []string
	for _, ext := range audioExtensions {
		matches, err := filepath.Glob(filepath.Join(dir, ext))
		if err == nil {
			files = append(files, matches...)
		}
	}
	return files
}

// backupFileBeforeWrite creates a timestamped .bak copy of a file before writing tags.
// Failures are logged but non-fatal — the write-back proceeds regardless.
func backupFileBeforeWrite(filePath string) {
	if filePath == "" {
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		return
	}
	backupPath := filePath + ".bak-" + time.Now().Format("20060102-150405")
	// Use hardlink — same data, no disk space cost. Falls back to copy if
	// hardlink fails (cross-device, unsupported filesystem).
	if err := os.Link(filePath, backupPath); err != nil {
		// Hardlink failed — fall back to copy
		if err := fileops.SafeCopy(filePath, backupPath, fileops.OperationConfig{}); err != nil {
			log.Printf("[WARN] backup before tag write failed: %s: %v", filePath, err)
			return
		}
	}
	log.Printf("[DEBUG] backup before tag write: %s", backupPath)
}

// buildTagMap constructs the tag map shared by all write-back paths.
// Includes all available metadata fields — standard and custom tags.
func (mfs *MetadataFetchService) buildTagMap(
	albumTitle, trackTitle, artist, narrator string, year int, track string,
) map[string]interface{} {
	tagMap := make(map[string]interface{})
	tagMap["title"] = trackTitle
	tagMap["album"] = albumTitle
	if artist != "" {
		tagMap["artist"] = artist
	}
	if narrator != "" {
		tagMap["narrator"] = narrator
	}
	if year > 0 {
		tagMap["year"] = year
	}
	tagMap["genre"] = "Audiobook"
	if track != "" {
		tagMap["track"] = track
	}
	return tagMap
}

// buildFullTagMap constructs a tag map with ALL available metadata from the book record,
// including custom tags for fields that don't have standard audio tag equivalents.
func (mfs *MetadataFetchService) buildFullTagMap(
	book *database.Book, albumTitle, trackTitle, artist, narrator string, year int, track string,
) map[string]interface{} {
	tagMap := mfs.buildTagMap(albumTitle, trackTitle, artist, narrator, year, track)

	// Add fields that have standard or custom tag equivalents
	if book.Language != nil && *book.Language != "" {
		tagMap["language"] = *book.Language
	}
	if book.Publisher != nil && *book.Publisher != "" {
		tagMap["publisher"] = *book.Publisher
	}
	if book.Description != nil && *book.Description != "" {
		tagMap["description"] = *book.Description
	}
	if book.ISBN10 != nil && *book.ISBN10 != "" {
		tagMap["isbn10"] = *book.ISBN10
	}
	if book.ISBN13 != nil && *book.ISBN13 != "" {
		tagMap["isbn13"] = *book.ISBN13
	}
	if book.ASIN != nil && *book.ASIN != "" {
		tagMap["asin"] = *book.ASIN
	}

	// Series info as custom tags
	if book.SeriesID != nil {
		if series, err := mfs.db.GetSeriesByID(*book.SeriesID); err == nil && series != nil {
			tagMap["series"] = series.Name
		}
	}
	if book.SeriesSequence != nil {
		tagMap["series_index"] = *book.SeriesSequence
	}

	// External provider IDs (written as AUDIOBOOK_ORGANIZER_* custom tags)
	tagMap["book_id"] = book.ID
	if book.OpenLibraryID != nil && *book.OpenLibraryID != "" {
		tagMap["open_library_id"] = *book.OpenLibraryID
	}
	if book.HardcoverID != nil && *book.HardcoverID != "" {
		tagMap["hardcover_id"] = *book.HardcoverID
	}
	if book.GoogleBooksID != nil && *book.GoogleBooksID != "" {
		tagMap["google_books_id"] = *book.GoogleBooksID
	}

	// Edition and print year
	if book.Edition != nil && *book.Edition != "" {
		tagMap["edition"] = *book.Edition
	}
	if book.PrintYear != nil && *book.PrintYear > 0 {
		tagMap["print_year"] = fmt.Sprintf("%d", *book.PrintYear)
	}

	return tagMap
}

// filterUnchangedTags reads the current tags from filePath and removes any
// entries from tagMap whose values already match, so only changed fields are
// written back to the file.
func filterUnchangedTags(filePath string, tagMap map[string]interface{}) map[string]interface{} {
	current, err := metadata.ExtractMetadata(filePath, nil)
	if err != nil {
		// Can't read current tags — write everything to be safe
		return tagMap
	}

	currentVals := map[string]string{
		"title":          current.Title,
		"album":          current.Album,
		"artist":         current.Artist,
		"narrator":       current.Narrator,
		"genre":          current.Genre,
		"year":           fmt.Sprintf("%d", current.Year),
		"language":       current.Language,
		"series":         current.Series,
		"asin":           current.ASIN,
		"description":    current.Comments, // description is stored in comments field
		"edition":        current.Edition,
		"print_year":     current.PrintYear,
		"book_id":        current.BookOrganizerID,
		"open_library_id": current.OpenLibraryID,
		"hardcover_id":   current.HardcoverID,
		"google_books_id": current.GoogleBooksID,
	}
	if current.Publisher != "" {
		currentVals["publisher"] = current.Publisher
	}
	if current.SeriesIndex > 0 {
		currentVals["series_index"] = fmt.Sprintf("%d", int(current.SeriesIndex))
	}
	if current.ISBN10 != "" {
		currentVals["isbn10"] = current.ISBN10
	}
	if current.ISBN13 != "" {
		currentVals["isbn13"] = current.ISBN13
	}

	filtered := make(map[string]interface{}, len(tagMap))
	for k, v := range tagMap {
		cur, ok := currentVals[k]
		if !ok {
			// Unknown field (e.g. "track") — always write
			filtered[k] = v
			continue
		}
		newStr := fmt.Sprintf("%v", v)
		if newStr != cur {
			filtered[k] = v
		}
	}

	if len(filtered) == 0 {
		return filtered
	}
	return filtered
}

// generateSegmentTitles computes and persists file titles for all book files of a book.
func (mfs *MetadataFetchService) generateSegmentTitles(bookID string, bookTitle string) error {
	bookFiles, err := mfs.db.GetBookFiles(bookID)
	if err != nil {
		return fmt.Errorf("list book files: %w", err)
	}
	if len(bookFiles) == 0 {
		return nil
	}

	// Sort by track number (0 last), then filepath
	sort.Slice(bookFiles, func(i, j int) bool {
		ti := bookFiles[i].TrackNumber
		tj := bookFiles[j].TrackNumber
		if ti != 0 && tj != 0 {
			if ti != tj {
				return ti < tj
			}
		} else if ti != 0 {
			return true
		} else if tj != 0 {
			return false
		}
		return bookFiles[i].FilePath < bookFiles[j].FilePath
	})

	totalTracks := len(bookFiles)

	// Determine segment title format from config
	segTitleFormat := config.AppConfig.SegmentTitleFormat
	if segTitleFormat == "" {
		segTitleFormat = DefaultSegmentTitleFormat
	}

	for i := range bookFiles {
		// Auto-assign track numbers if zero
		if bookFiles[i].TrackNumber == 0 {
			bookFiles[i].TrackNumber = i + 1
		}
		bookFiles[i].TrackCount = totalTracks

		// Compute file title
		title := FormatSegmentTitle(segTitleFormat, bookTitle, bookFiles[i].TrackNumber, totalTracks)
		bookFiles[i].Title = title

		if err := mfs.db.UpdateBookFile(bookFiles[i].ID, &bookFiles[i]); err != nil {
			log.Printf("[WARN] failed to update book file title for %s: %v", bookFiles[i].ID, err)
		}
	}

	return nil
}

// runApplyPipeline runs the file rename pipeline after metadata is applied.
// For protected books (iTunes/import paths), it operates on the library copy
// instead of the original to avoid moving source files.
func (mfs *MetadataFetchService) runApplyPipeline(id string, book *database.Book) error {
	// If the book is in a protected path, run the pipeline on the library copy instead
	if isProtectedPath(book.FilePath) {
		libCopy := mfs.ensureLibraryCopy(book)
		if libCopy == nil {
			log.Printf("[WARN] runApplyPipeline: no library copy for protected book %s, skipping", id)
			return nil
		}
		log.Printf("[INFO] runApplyPipeline: using library copy %s for protected book %s", libCopy.ID, id)
		id = libCopy.ID
		book = libCopy
	}

	bookFiles, err := mfs.db.GetBookFiles(id)
	if err != nil {
		return fmt.Errorf("list book files: %w", err)
	}
	if len(bookFiles) == 0 {
		return nil
	}

	// Resolve author name
	var authorName string
	if book.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			authorName = author.Name
		}
	}

	// Resolve series name and position
	var seriesName, seriesPos string
	if book.SeriesID != nil {
		if series, serr := mfs.db.GetSeriesByID(*book.SeriesID); serr == nil && series != nil {
			seriesName = series.Name
		}
		if book.SeriesSequence != nil {
			seriesPos = strconv.Itoa(*book.SeriesSequence)
		}
	}

	year := 0
	if book.AudiobookReleaseYear != nil {
		year = *book.AudiobookReleaseYear
	}

	vars := FormatVars{
		Author:    authorName,
		Title:     book.Title,
		Series:    seriesName,
		SeriesPos: seriesPos,
		Year:      year,
		Narrator:  derefString(book.Narrator),
		Lang:      derefString(book.Language),
	}

	pathFormat := config.AppConfig.PathFormat
	if pathFormat == "" {
		pathFormat = DefaultPathFormat
	}
	segTitleFormat := config.AppConfig.SegmentTitleFormat
	if segTitleFormat == "" {
		segTitleFormat = DefaultSegmentTitleFormat
	}

	entries := ComputeTargetPaths(config.AppConfig.RootDir, pathFormat, segTitleFormat, book, bookFiles, vars)

	if config.AppConfig.AutoRenameOnApply {
		renameResult, err := RenameFiles(entries)
		if err != nil {
			return fmt.Errorf("rename files: %w", err)
		}
		if len(renameResult.Skipped) > 0 {
			log.Printf("[WARN] %d files skipped (source missing) during rename", len(renameResult.Skipped))
		}

		// Update book file records with new paths (only for succeeded renames)
		bfMap := make(map[string]*database.BookFile, len(bookFiles))
		for i := range bookFiles {
			bfMap[bookFiles[i].ID] = &bookFiles[i]
		}
		for _, entry := range renameResult.Succeeded {
			if bf, ok := bfMap[entry.SegmentID]; ok {
				bf.FilePath = entry.TargetPath
				bf.ITunesPath = computeITunesPath(entry.TargetPath)
				if err := mfs.db.UpdateBookFile(bf.ID, bf); err != nil {
					log.Printf("[WARN] failed to update book_file path for %s: %v", bf.ID, err)
				}
			}
			// Record path change for each successful rename
			if entry.SourcePath != entry.TargetPath {
				_ = mfs.db.RecordPathChange(&database.BookPathChange{
					BookID:     id,
					OldPath:    entry.SourcePath,
					NewPath:    entry.TargetPath,
					ChangeType: "rename",
				})
				// Dual-write to unified activity log
				if mfs.activityService != nil {
					_ = mfs.activityService.Record(database.ActivityEntry{
						Tier:    "change",
						Type:    "rename",
						Level:   "info",
						Source:  "background",
						BookID:  id,
						Summary: fmt.Sprintf("Moved: %s → %s", filepath.Base(entry.SourcePath), filepath.Base(entry.TargetPath)),
						Details: map[string]any{"old_path": entry.SourcePath, "new_path": entry.TargetPath},
					})
				}
			}
		}

		// Update the book's file_path to match the new segment directory.
		// For multi-file books, file_path is the parent directory of the segments.
		if len(renameResult.Succeeded) > 0 {
			newBookPath := filepath.Dir(renameResult.Succeeded[0].TargetPath)
			if newBookPath != book.FilePath {
				book.FilePath = newBookPath
				if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
					book.ITunesPath = &itunesPath
				}
				if _, err := mfs.db.UpdateBook(id, book); err != nil {
					log.Printf("[WARN] failed to update book path for %s: %v", id, err)
				} else {
					log.Printf("[INFO] updated book path for %s: %s", id, newBookPath)
				}
			}
		}
	}

	// Always ensure itunes_path is set if a mapping exists (for already-organized books)
	if book.ITunesPath == nil || *book.ITunesPath == "" {
		if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
			book.ITunesPath = &itunesPath
			if _, err := mfs.db.UpdateBook(id, book); err != nil {
				log.Printf("[WARN] failed to update itunes_path for %s: %v", id, err)
			}
		}
	}

	// Write metadata tags to audio files
	if config.AppConfig.AutoWriteTagsOnApply {
		if written, err := mfs.WriteBackMetadataForBook(id); err != nil {
			log.Printf("[WARN] tag writing failed for book %s: %v", id, err)
		} else {
			log.Printf("[INFO] wrote metadata tags to %d file(s) for book %s", written, id)
		}
	}

	return nil
}

// RunApplyPipelineRenameOnly runs only the rename portion of the apply pipeline.
// Used by the "Save to Files" button to rename files without re-writing tags (tags are written separately).
func (mfs *MetadataFetchService) RunApplyPipelineRenameOnly(id string, book *database.Book) error {
	// If the book is in a protected path, run on library copy
	if isProtectedPath(book.FilePath) {
		libCopy := mfs.ensureLibraryCopy(book)
		if libCopy == nil {
			return fmt.Errorf("no library copy for protected book %s", id)
		}
		id = libCopy.ID
		book = libCopy
	}

	bookFiles, err := mfs.db.GetBookFiles(id)
	if err != nil {
		return fmt.Errorf("list book files: %w", err)
	}

	// For single-file books with no book files, create a virtual entry from book.FilePath
	if len(bookFiles) == 0 && book.FilePath != "" {
		ext := strings.TrimPrefix(filepath.Ext(book.FilePath), ".")
		if ext != "" {
			// This is a file, not a directory — create a virtual book file entry
			bookFiles = []database.BookFile{{
				ID:       "virtual-" + id,
				BookID:   id,
				FilePath: book.FilePath,
				Format:   ext,
			}}
		}
	}
	if len(bookFiles) == 0 {
		return nil
	}

	var authorName string
	if book.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			authorName = author.Name
		}
	}
	var seriesName, seriesPos string
	if book.SeriesID != nil {
		if series, serr := mfs.db.GetSeriesByID(*book.SeriesID); serr == nil && series != nil {
			seriesName = series.Name
		}
		if book.SeriesSequence != nil {
			seriesPos = strconv.Itoa(*book.SeriesSequence)
		}
	}
	year := 0
	if book.AudiobookReleaseYear != nil {
		year = *book.AudiobookReleaseYear
	}

	vars := FormatVars{
		Author:    authorName,
		Title:     book.Title,
		Series:    seriesName,
		SeriesPos: seriesPos,
		Year:      year,
		Narrator:  derefString(book.Narrator),
		Lang:      derefString(book.Language),
	}

	pathFormat := config.AppConfig.PathFormat
	if pathFormat == "" {
		pathFormat = DefaultPathFormat
	}
	segTitleFormat := config.AppConfig.SegmentTitleFormat
	if segTitleFormat == "" {
		segTitleFormat = DefaultSegmentTitleFormat
	}

	entries := ComputeTargetPaths(config.AppConfig.RootDir, pathFormat, segTitleFormat, book, bookFiles, vars)

	renameResult, err := RenameFiles(entries)
	if err != nil {
		return fmt.Errorf("rename files: %w", err)
	}

	// Update book file records with new paths
	bfMap := make(map[string]*database.BookFile, len(bookFiles))
	for i := range bookFiles {
		bfMap[bookFiles[i].ID] = &bookFiles[i]
	}
	for _, entry := range renameResult.Succeeded {
		if strings.HasPrefix(entry.SegmentID, "virtual-") {
			// Virtual entry = single-file book. Update book.FilePath directly to the new file path.
			book.FilePath = entry.TargetPath
			if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
				book.ITunesPath = &itunesPath
			}
			if _, err := mfs.db.UpdateBook(id, book); err != nil {
				log.Printf("[WARN] failed to update book path for %s: %v", id, err)
			} else {
				log.Printf("[INFO] renamed single-file book %s: %s", id, entry.TargetPath)
			}
		} else if bf, ok := bfMap[entry.SegmentID]; ok {
			bf.FilePath = entry.TargetPath
			bf.ITunesPath = computeITunesPath(entry.TargetPath)
			if err := mfs.db.UpdateBookFile(bf.ID, bf); err != nil {
				log.Printf("[WARN] failed to update book_file path for %s: %v", bf.ID, err)
			}
		}
		// Record path change for each successful rename
		if entry.SourcePath != entry.TargetPath {
			_ = mfs.db.RecordPathChange(&database.BookPathChange{
				BookID:     id,
				OldPath:    entry.SourcePath,
				NewPath:    entry.TargetPath,
				ChangeType: "rename",
			})
			// Dual-write to unified activity log
			if mfs.activityService != nil {
				_ = mfs.activityService.Record(database.ActivityEntry{
					Tier:    "change",
					Type:    "rename",
					Level:   "info",
					Source:  "background",
					BookID:  id,
					Summary: fmt.Sprintf("Moved: %s → %s", filepath.Base(entry.SourcePath), filepath.Base(entry.TargetPath)),
					Details: map[string]any{"old_path": entry.SourcePath, "new_path": entry.TargetPath},
				})
			}
		}
	}

	// Update book file_path for multi-segment books (directory path)
	if len(renameResult.Succeeded) > 0 && !strings.HasPrefix(renameResult.Succeeded[0].SegmentID, "virtual-") {
		newBookPath := filepath.Dir(renameResult.Succeeded[0].TargetPath)
		if newBookPath != book.FilePath {
			book.FilePath = newBookPath
			if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
				book.ITunesPath = &itunesPath
			}
			if _, err := mfs.db.UpdateBook(id, book); err != nil {
				log.Printf("[WARN] failed to update book path for %s: %v", id, err)
			} else {
				log.Printf("[INFO] renamed book files for %s: %s", id, newBookPath)
			}
		}
	}

	// Always ensure itunes_path is set if a mapping exists (for already-organized books)
	if book.ITunesPath == nil || *book.ITunesPath == "" {
		if itunesPath := computeITunesPath(book.FilePath); itunesPath != "" {
			book.ITunesPath = &itunesPath
			if _, err := mfs.db.UpdateBook(id, book); err != nil {
				log.Printf("[WARN] failed to update itunes_path for %s: %v", id, err)
			}
		}
	}

	// Clean up empty directories left after rename
	for _, entry := range renameResult.Succeeded {
		oldDir := filepath.Dir(entry.SourcePath)
		if oldDir != filepath.Dir(entry.TargetPath) {
			removeEmptyDirs(oldDir, config.AppConfig.RootDir)
		}
	}

	return nil
}

// truncateActivity shortens s to maxLen runes, appending "..." if truncated.
func truncateActivity(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// computeITunesPath converts a local file path to an iTunes file:// URL
// using the configured path mappings (m.To = Linux prefix, m.From = Windows prefix).
// Returns an empty string if no mapping matches.
func computeITunesPath(localPath string) string {
	for _, m := range config.AppConfig.ITunesPathMappings {
		if m.To != "" && m.From != "" && strings.HasPrefix(localPath, m.To) {
			remainder := localPath[len(m.To):]
			windowsPath := m.From + remainder
			encoded := url.PathEscape(windowsPath)
			encoded = strings.ReplaceAll(encoded, "%2F", "/")
			encoded = strings.ReplaceAll(encoded, "%3A", ":")
			return "file://localhost/" + encoded
		}
	}
	return ""
}

// removeEmptyDirs removes empty directories walking up from dir until reaching stopAt.
func removeEmptyDirs(dir, stopAt string) {
	for dir != stopAt && dir != "/" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
		log.Printf("[INFO] removed empty directory: %s", dir)
		dir = filepath.Dir(dir)
	}
}
