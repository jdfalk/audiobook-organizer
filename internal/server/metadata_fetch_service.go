// file: internal/server/metadata_fetch_service.go
// version: 4.52.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"context"
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

	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
)

type MetadataFetchService struct {
	db               database.Store
	olStore          *openlibrary.OLStore
	overrideSources  []metadata.MetadataSource // for testing
	isbnEnrichment   *ISBNEnrichmentService
	activityService  *activity.Service
	dedupEngine      *dedup.Engine
	metadataScorer   ai.MetadataCandidateScorer // optional; nil = fallback to F1
	llmScorer        ai.MetadataCandidateScorer // optional; nil = no LLM rerank tier
	writeBackBatcher *WriteBackBatcher
}

// SetActivityService sets the activity service for dual-writing to the unified activity log.
func (mfs *MetadataFetchService) SetActivityService(svc *activity.Service) {
	mfs.activityService = svc
}

// SetWriteBackBatcher sets the iTunes write-back batcher.
func (mfs *MetadataFetchService) SetWriteBackBatcher(b *WriteBackBatcher) {
	mfs.writeBackBatcher = b
}

func NewMetadataFetchService(db database.Store) *MetadataFetchService {
	return &MetadataFetchService{db: db}
}

// SetOLStore sets the Open Library dump store for local-first lookups.
func (mfs *MetadataFetchService) SetOLStore(store *openlibrary.OLStore) {
	mfs.olStore = store
}

// SetDedupEngine sets the dedup engine for post-apply dedup checks.
func (mfs *MetadataFetchService) SetDedupEngine(engine *dedup.Engine) {
	mfs.dedupEngine = engine
}

// SetMetadataScorer injects the pluggable metadata candidate scorer. A nil
// scorer (or a scorer that returns errors at runtime) makes the search
// pipeline fall back to the pre-existing significantWords F1 path, so this
// method is safe to leave unset.
func (mfs *MetadataFetchService) SetMetadataScorer(scorer ai.MetadataCandidateScorer) {
	mfs.metadataScorer = scorer
}

// SetMetadataLLMScorer injects the LLM rerank scorer. A nil scorer or a
// scorer that returns errors at runtime makes the rerank pass a no-op, so
// this method is safe to leave unset.
func (mfs *MetadataFetchService) SetMetadataLLMScorer(scorer ai.MetadataCandidateScorer) {
	mfs.llmScorer = scorer
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

// SearchOptions carries optional per-request flags for SearchMetadataForBook.
// Adding a new option never breaks existing callers — they can keep using the
// zero-value or the simpler variadic signature.
type SearchOptions struct {
	// UseRerank asks the LLM rerank tier to run on the top candidates (if
	// MetadataLLMScoringEnabled is true on the server). When false, only
	// the base scorer tier runs.
	UseRerank bool
}

// BuildSourceChain returns metadata sources ordered by config priority.
// Each source is wrapped with a circuit breaker that opens after 5 consecutive
// failures and retries after 30 seconds.
// buildSearchContext gathers the richer context fields from a Book
// that metadata.ContextualSearch implementations can use to do better
// than plain title+author lookups. Empty fields are left empty so
// sources see "" instead of a garbage placeholder.
func buildSearchContext(book *database.Book, searchTitle, author, narrator string) *metadata.SearchContext {
	ctx := &metadata.SearchContext{
		Title:    searchTitle,
		Author:   author,
		Narrator: narrator,
	}
	if book != nil {
		if book.ISBN10 != nil {
			ctx.ISBN10 = *book.ISBN10
		}
		if book.ISBN13 != nil {
			ctx.ISBN13 = *book.ISBN13
		}
		if book.ASIN != nil {
			ctx.ASIN = *book.ASIN
		}
		// buildSearchContext is a top-level helper; use the package
		// global here until the function is refactored into a method.
		if book.SeriesID != nil && database.GetGlobalStore() != nil {
			if series, err := database.GetGlobalStore().GetSeriesByID(*book.SeriesID); err == nil && series != nil {
				ctx.Series = series.Name
			}
		}
	}
	return ctx
}

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

		// Try the ContextualSearch path first if the source implements
		// it. This hands richer context (ASIN, ISBN, narrator) to
		// sources that can use it — Audnexus uses the ASIN for a direct
		// lookup that works when title search can't, Hardcover uses
		// the ISBN for a more precise match than the fuzzy GraphQL
		// search. Sources that don't implement the interface just
		// fall through to the title/author path below.
		if ctxSearch, ok := src.(metadata.ContextualSearch); ok {
			ctx := buildSearchContext(book, searchTitle, currentAuthor, currentNarrator)
			results, searchErr = ctxSearch.SearchByContext(ctx)
			if searchErr != nil {
				log.Printf("[WARN] %s context search failed for %q: %v", src.Name(), book.Title, searchErr)
				// Context search failure is non-fatal — fall through
				// to the regular title/author path in case that works.
			}
		}

		// Try title+author search first for better match quality
		if len(results) == 0 && currentAuthor != "" {
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
			scored := mfs.bestTitleMatchForBook(book, results, currentAuthor, currentNarrator, searchTitle, book.Title)
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
			normalizeMetaSeries(&meta)

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

		scored := mfs.bestTitleMatchForBook(book, results, "", titleOnlyNarrator, searchTitle, book.Title)
		if len(scored) == 0 {
			continue
		}
		meta := scored[0]
		normalizeMetaSeries(&meta)

		mfs.recordChangeHistory(book, meta, src.Name())
		mfs.applyMetadataToBook(book, meta)

		updatedBook, updateErr := mfs.db.UpdateBook(id, book)
		if updateErr != nil {
			return nil, fmt.Errorf("failed to update book: %w", updateErr)
		}

		mfs.persistFetchedMetadata(id, meta)

		// Mirror of ApplyMetadataCandidate: tag the book with the
		// source and language so downstream filters (review dialog,
		// upgrade jobs) have provenance to key on.
		mfs.applyMetadataSystemTags(id, src.Name(), meta.Language)

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
		field  string
		oldVal string
		newVal string
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

// normalizeMetaSeries splits an embedded "Series Name, Book N" pattern
// out of meta.Title or meta.Series into separate Series + SeriesPosition
// fields. Audible/Audnexus sometimes return the series name with the
// book number baked in (e.g. "Mistborn, Book 3") instead of using their
// own Sequence field, which leaves us with a series row named
// "Mistborn, Book 3" if we apply the candidate as-is.
//
// Safe to call multiple times: a no-match leaves meta untouched, and an
// already-split series field will not match Pattern 3.
func normalizeMetaSeries(meta *metadata.BookMetadata) {
	parsedSeries, parsedPosition, parsedTitle := parseSeriesFromTitle(meta.Title)
	if parsedSeries == "" && meta.Series != "" {
		parsedSeries, parsedPosition, parsedTitle = parseSeriesFromTitle(meta.Series)
		if parsedTitle == "" {
			parsedTitle = meta.Title
		}
	}
	if parsedSeries == "" {
		return
	}
	meta.Series = parsedSeries
	if parsedPosition != "" {
		meta.SeriesPosition = parsedPosition
	}
	if parsedTitle != "" {
		meta.Title = parsedTitle
	}
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

// extractTrailingNumber pulls a trailing number from a title, handling patterns
// like "Series Name 8", "Series Name, Book 3", "Title #12", "Title (Volume 5)".
// Returns the number as a string, or "" if none found.
var trailingNumberRe = regexp.MustCompile(
	`(?i)(?:,?\s*(?:book|volume|vol\.?|part|pt\.?|#)\s*)?(\d+(?:\.\d+)?)\s*(?:\(.*\))?\s*$`)

func extractTrailingNumber(title string) string {
	// Strip common suffixes that aren't numbers
	clean := regexp.MustCompile(`(?i)\s*\((un)?abridged\)\s*$`).ReplaceAllString(title, "")
	clean = regexp.MustCompile(`\s*\[.*?\]\s*$`).ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)

	m := trailingNumberRe.FindStringSubmatch(clean)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// normalizeSeriesNumber extracts the numeric portion from a series position
// string like "8", "8.0", "Book 8", "#8".
var seriesNumRe = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

func normalizeSeriesNumber(pos string) string {
	m := seriesNumRe.FindStringSubmatch(pos)
	if len(m) >= 2 {
		// Normalize "8.0" → "8"
		if strings.HasSuffix(m[1], ".0") {
			return strings.TrimSuffix(m[1], ".0")
		}
		return m[1]
	}
	return ""
}

// computeF1Base returns just the F1 token-overlap portion of the score, with
// no penalties or bonuses applied. It's the "base score" contribution from
// the significantWords pathway, extracted so alternative scorers (embedding,
// LLM, reranker) can supply their own base score and reuse the shared
// non-base adjustment function.
func computeF1Base(r metadata.BookMetadata, searchWords map[string]bool) float64 {
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

	if recall+precision == 0 {
		return 0
	}
	return 2 * recall * precision / (recall + precision)
}

// applyNonBaseAdjustments applies the compilation penalty, length penalty,
// and rich-metadata bonus to a base score. These adjustments are meaningful
// regardless of which scorer tier produced the base score and are applied
// identically on every path.
//
// baseWordCount is the number of significant words in the search title —
// used for the length penalty. Pass 0 to disable the length penalty (e.g.
// when the length ratio is meaningless for a non-token-overlap scorer).
func applyNonBaseAdjustments(baseScore float64, r metadata.BookMetadata, baseWordCount int) float64 {
	score := baseScore

	// Compilation penalty
	if isCompilation(r.Title) {
		score *= 0.15
	}

	// Length penalty: penalise results that are much longer than the search.
	// Only applies when baseWordCount > 0 (the F1 path).
	if baseWordCount > 0 {
		resultWords := significantWords(r.Title)
		nSearch := float64(baseWordCount)
		nResult := float64(len(resultWords))
		if nResult > 1.5*nSearch {
			score *= (1.5 * nSearch) / nResult
		}
	}

	// Rich-metadata bonus (capped at +0.15, additive)
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

	return score + bonus
}

// pickBestMatchFromScored takes pre-computed base scores from any tier and
// returns the single best-matching result above the tier-appropriate
// threshold, applying the full stack of author/narrator/audiobook bonus
// multipliers. It's shared between the F1-only package-level
// bestTitleMatchWithContext and the scorer-backed bestTitleMatchForBook
// method, so the bonus logic lives in one place.
//
// baseScores must be aligned to results (same length, same order).
// baseTier drives the minimum score threshold and the length-penalty
// behavior inside applyNonBaseAdjustments: "f1" uses the historical 0.35
// threshold and applies the length penalty; other tiers (e.g. "embedding")
// use MetadataEmbeddingBestMatchMin (default 0.70) and disable the length
// penalty since their base scores have no token-overlap ratio.
//
// For the F1 tier we preserve the historical "skip bonuses when base==0"
// behavior of scoreOneResult: a result whose F1 base is zero contributes a
// final score of zero, so it can never win regardless of rich-metadata
// bonuses or author/narrator multipliers. This keeps the package-level
// bestTitleMatchWithContext bit-for-bit equivalent to its pre-refactor
// implementation, which the existing test suite locks in.
func pickBestMatchFromScored(
	results []metadata.BookMetadata,
	baseScores []float64,
	baseTier string,
	searchWords map[string]bool,
	bookAuthor, bookNarrator string,
) []metadata.BookMetadata {
	const f1MinScore = 0.35

	minScore := f1MinScore
	if baseTier != "f1" {
		minScore = config.AppConfig.MetadataEmbeddingBestMatchMin
	}

	bestIdx := -1
	bestScore := 0.0
	for i, r := range results {
		baseScore := baseScores[i]

		var score float64
		if baseTier == "f1" {
			// Preserve scoreOneResult's early-return-on-zero behavior so the
			// F1 path stays bit-for-bit identical to the pre-refactor code.
			if baseScore == 0 {
				continue
			}
			score = applyNonBaseAdjustments(baseScore, r, len(searchWords))
		} else {
			// Non-F1 tiers (embedding, etc.) skip the length penalty by
			// passing baseWordCount=0; the cosine-based base has no
			// token-overlap ratio for the penalty to be meaningful.
			score = applyNonBaseAdjustments(baseScore, r, 0)
		}

		// Author-based scoring: boost matches, penalize mismatches or missing.
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

		// Narrator-based scoring: boost matches as secondary tiebreaker.
		if bookNarrator != "" && r.Narrator != "" {
			rNarrLower := strings.ToLower(r.Narrator)
			bNarrLower := strings.ToLower(bookNarrator)
			if strings.Contains(rNarrLower, bNarrLower) || strings.Contains(bNarrLower, rNarrLower) {
				score *= 1.3
			}
		}

		// Audiobook-specific: boost results with narrator, penalize without.
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

// scoreOneResult computes a quality score in [0, ~1.15] for a single result
// against a set of search-title significant words. It preserves the
// pre-refactor signature and behavior, composing computeF1Base and
// applyNonBaseAdjustments. Existing callers are unchanged.
func scoreOneResult(r metadata.BookMetadata, searchWords map[string]bool) float64 {
	base := computeF1Base(r, searchWords)
	if base == 0 {
		return 0 // preserve original early-return behavior (skips bonus)
	}
	return applyNonBaseAdjustments(base, r, len(searchWords))
}

// scoreBaseCandidates picks the highest-available base scorer tier and
// returns one base score per input result, aligned to input order, along
// with a short tier name for logs and UI badges ("embedding", "f1", ...).
//
// The fallback chain is:
//  1. If MetadataEmbeddingScoringEnabled AND a scorer is injected AND the
//     scorer succeeds → use those scores. Tier = scorer.Name().
//  2. Otherwise, compute F1 inline. Tier = "f1".
//
// Any scorer error is logged and falls through to the F1 tier. The search
// path must never fail because of a scorer problem — F1 is always reachable
// as a last resort since it only depends on the in-memory result data.
func (mfs *MetadataFetchService) scoreBaseCandidates(
	ctx context.Context,
	book *database.Book,
	results []metadata.BookMetadata,
	searchWords map[string]bool,
) ([]float64, string) {
	if config.AppConfig.MetadataEmbeddingScoringEnabled && mfs.metadataScorer != nil && len(results) > 0 {
		query := ai.Query{
			BookID:   book.ID,
			Title:    book.Title,
			Narrator: derefStr(book.Narrator),
		}
		if book.AuthorID != nil {
			if author, err := mfs.db.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				query.Author = author.Name
			}
		}

		cands := make([]ai.Candidate, len(results))
		for i, r := range results {
			cands[i] = ai.Candidate{
				Title:    r.Title,
				Author:   r.Author,
				Narrator: r.Narrator,
			}
		}

		scores, err := mfs.metadataScorer.Score(ctx, query, cands)
		if err == nil && len(scores) == len(results) {
			return scores, mfs.metadataScorer.Name()
		}
		if err != nil {
			log.Printf("[WARN] metadata-scorer %s failed, falling back to F1: %v",
				mfs.metadataScorer.Name(), err)
		} else {
			log.Printf("[WARN] metadata-scorer %s returned %d scores for %d results, falling back to F1",
				mfs.metadataScorer.Name(), len(scores), len(results))
		}
	}

	// F1 fallback tier.
	scores := make([]float64, len(results))
	for i, r := range results {
		scores[i] = computeF1Base(r, searchWords)
	}
	return scores, "f1"
}

// bestTitleMatchForBook is the scorer-aware sibling of
// bestTitleMatchWithContext. It routes through scoreBaseCandidates so
// callers that have a *database.Book in hand (e.g. the automatic metadata
// fetch paths) get embedding-based scoring when available, falling back
// silently to the F1 path when the scorer is disabled or errors.
//
// The package-level bestTitleMatch[WithContext] functions still exist and
// still use F1 — they're kept for the test suite and for code paths that
// don't have a Book in scope. This method is the preferred entry point
// for production call sites that do.
func (mfs *MetadataFetchService) bestTitleMatchForBook(
	book *database.Book,
	results []metadata.BookMetadata,
	bookAuthor, bookNarrator string,
	titles ...string,
) []metadata.BookMetadata {
	// Union of significant words from all title variants. Needed by both
	// the F1 fallback path (via scoreBaseCandidates) and by
	// pickBestMatchFromScored for the length penalty.
	searchWords := map[string]bool{}
	for _, t := range titles {
		for w := range significantWords(t) {
			searchWords[w] = true
		}
	}

	baseScores, baseTier := mfs.scoreBaseCandidates(context.Background(), book, results, searchWords)
	return pickBestMatchFromScored(results, baseScores, baseTier, searchWords, bookAuthor, bookNarrator)
}

// rerankTopK asks the LLM scorer to re-judge the ambiguous top candidates
// after the base scorer has produced initial rankings. "Ambiguous" means
// candidates whose Score lands within MetadataLLMRerankEpsilon of the best
// candidate's Score. At most MetadataLLMRerankTopK candidates are sent to
// the LLM, even if more fall inside the epsilon window, to cap per-search
// cost.
//
// On success, the returned slice is the same candidates with updated Score
// values for the top-K slots, re-sorted descending by Score. On any failure
// (LLM disabled, backend error, fewer than 2 ambiguous candidates to resolve)
// the input slice is returned unchanged so the search path degrades cleanly.
func (mfs *MetadataFetchService) rerankTopK(
	ctx context.Context,
	book *database.Book,
	candidates []MetadataCandidate,
) []MetadataCandidate {
	if len(candidates) < 2 || mfs.llmScorer == nil {
		return candidates
	}

	// Sort descending by current score so the "ambiguous top" is contiguous
	// at index 0.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	epsilon := config.AppConfig.MetadataLLMRerankEpsilon
	topK := config.AppConfig.MetadataLLMRerankTopK
	if topK <= 0 {
		topK = 5
	}

	bestScore := candidates[0].Score
	ambiguousEnd := 1
	for ambiguousEnd < len(candidates) && ambiguousEnd < topK {
		if bestScore-candidates[ambiguousEnd].Score > epsilon {
			break
		}
		ambiguousEnd++
	}
	if ambiguousEnd < 2 {
		// Only one candidate within epsilon — nothing to resolve.
		log.Printf("[DEBUG] metadata-search: rerank skipped — only 1 candidate within %.3f of best (%.3f)",
			epsilon, bestScore)
		return candidates
	}

	topCands := candidates[:ambiguousEnd]
	log.Printf("[DEBUG] metadata-search: rerank firing on top %d candidates (epsilon=%.3f, bestScore=%.3f)",
		len(topCands), epsilon, bestScore)

	// Resolve the book's author name for the query payload.
	authorName := ""
	if book.AuthorID != nil {
		if author, err := mfs.db.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
			authorName = author.Name
		}
	}
	query := ai.Query{
		BookID:   book.ID,
		Title:    book.Title,
		Author:   authorName,
		Narrator: derefStr(book.Narrator),
	}

	llmCands := make([]ai.Candidate, len(topCands))
	for i, c := range topCands {
		llmCands[i] = ai.Candidate{
			Title:    c.Title,
			Author:   c.Author,
			Narrator: c.Narrator,
		}
	}

	llmScores, err := mfs.llmScorer.Score(ctx, query, llmCands)
	if err != nil || len(llmScores) != len(topCands) {
		if err != nil {
			log.Printf("[WARN] metadata-search: rerank LLM call failed, keeping base scores: %v", err)
		} else {
			log.Printf("[WARN] metadata-search: rerank returned %d scores for %d candidates, keeping base scores",
				len(llmScores), len(topCands))
		}
		return candidates
	}

	// Replace top-K base scores with LLM scores directly — do not apply the
	// author/narrator/series bonus multipliers again. The LLM prompt already
	// sees those fields and judges them as part of its score; re-multiplying
	// would double-count the same evidence and distort the top-K's position
	// relative to the non-reranked tail.
	for i := range topCands {
		candidates[i].Score = llmScores[i]
	}

	// Resort the full list so the reranked top-K is in correct order against
	// the untouched tail.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
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
	// Union of significant words from all title variants.
	searchWords := map[string]bool{}
	for _, t := range titles {
		for w := range significantWords(t) {
			searchWords[w] = true
		}
	}

	// F1 base scores aligned to results — the helper applies bonuses,
	// multipliers, and the 0.35 threshold for the "f1" tier.
	baseScores := make([]float64, len(results))
	for i, r := range results {
		baseScores[i] = computeF1Base(r, searchWords)
	}

	return pickBestMatchFromScored(results, baseScores, "f1", searchWords, bookAuthor, bookNarrator)
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

	// Check if the new cover is different from what's already embedded.
	// Skip archive + embed if they match (same hash).
	newCoverData, _ := os.ReadFile(coverPath)
	if len(newCoverData) > 0 {
		newHash := fmt.Sprintf("%x", sha256.Sum256(newCoverData))[:12]
		existingData, _, _ := metadata.ExtractCoverArtBytes(files[0])
		if len(existingData) > 0 {
			existingHash := fmt.Sprintf("%x", sha256.Sum256(existingData))[:12]
			if newHash == existingHash {
				log.Printf("[DEBUG] cover art unchanged for book %s, skipping embed", book.ID)
				return
			}
		}
	}

	// Archive the old cover from the first file before overwriting
	mfs.archiveExistingCover(book.ID, files[0])

	// Embed new cover into all files
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
// SearchMetadataForBook is the backward-compatible variadic entry point.
// New callers should prefer SearchMetadataForBookWithOptions — the variadic
// author/narrator/series positioning is historical and easy to get wrong.
func (mfs *MetadataFetchService) SearchMetadataForBook(id string, query string, authorHint ...string) (*SearchMetadataResponse, error) {
	var author, narrator, series string
	if len(authorHint) > 0 {
		author = authorHint[0]
	}
	if len(authorHint) > 1 {
		narrator = authorHint[1]
	}
	if len(authorHint) > 2 {
		series = authorHint[2]
	}
	return mfs.SearchMetadataForBookWithOptions(id, query, author, narrator, series, SearchOptions{})
}

// SearchMetadataForBookWithOptions is the canonical search entry point. The
// old variadic signature wraps this and passes default options. All new call
// sites should use this method directly so they can pass SearchOptions fields
// (UseRerank etc.) explicitly.
func (mfs *MetadataFetchService) SearchMetadataForBookWithOptions(
	id, query, author, narrator, series string,
	opts SearchOptions,
) (*SearchMetadataResponse, error) {
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
		if author != "" {
			searchTitle = author
		} else if book.AuthorID != nil {
			if a, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && a != nil {
				searchTitle = a.Name
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

	// Normalize explicit author/narrator/series hints for downstream scoring.
	searchAuthor := strings.TrimSpace(author)
	searchNarrator := strings.TrimSpace(narrator)
	searchSeries := strings.TrimSpace(series)

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
		cacheHit := false

		// Check the metadata fetch cache before hitting the
		// external API. Cache key is (bookID, source name) —
		// on hit, we use the cached results as-is and skip the
		// Search* calls entirely. On miss we fall through to
		// the API path and write the result back at the end
		// of the per-source block.
		//
		// Added 2026-04-11 after the OpenAI quota incident
		// where re-fetching 8000 books hit every external API
		// 8000 times even for books we'd already matched with
		// high confidence.
		if cached, cerr := database.GetCachedMetadataFetch(mfs.db, id, src.Name()); cerr == nil && cached != nil {
			var cachedResults []metadata.BookMetadata
			if jerr := json.Unmarshal(cached.Results, &cachedResults); jerr == nil {
				allResults = cachedResults
				cacheHit = true
				log.Printf("[DEBUG] metadata-search: cache HIT for (%s, %s) — %d results, age=%s",
					id, src.Name(), len(cachedResults), time.Since(cached.CachedAt).Round(time.Second))
			}
		}

		if !cacheHit {
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

			// Write to cache on a successful non-empty fetch.
			// Empty and error cases are not cached so they can
			// be retried. Cache is best-effort — a Put failure
			// is logged but doesn't fail the outer search.
			if len(allResults) > 0 {
				if blob, merr := json.Marshal(allResults); merr == nil {
					if perr := database.PutCachedMetadataFetch(mfs.db, id, src.Name(), blob, 0); perr != nil {
						log.Printf("[WARN] metadata-search: cache put failed for (%s, %s): %v", id, src.Name(), perr)
					}
				}
			}
		}

		baseScores, baseTier := mfs.scoreBaseCandidates(context.Background(), book, allResults, searchWords)
		log.Printf("[DEBUG] metadata-search: scored %d results from %s with tier %s", len(allResults), src.Name(), baseTier)

		for i, r := range allResults {
			key := strings.ToLower(r.Title + "|" + r.Author)
			if seen[key] {
				continue
			}
			seen[key] = true

			baseScore := baseScores[i]

			// Apply non-base adjustments (compilation, length, rich metadata). For
			// non-F1 tiers, pass baseWordCount=0 so the length penalty is suppressed —
			// it's a token-overlap-specific signal that doesn't translate to semantic
			// embedding scores.
			baseWordCount := 0
			if baseTier == "f1" {
				baseWordCount = len(searchWords)
			}
			score := applyNonBaseAdjustments(baseScore, r, baseWordCount)

			// Tier-specific minimum on the adjusted score. F1 path filters at <= 0
			// (preserves original behavior); embedding path uses the configured
			// MetadataEmbeddingMinScore threshold.
			minScore := 0.0
			if baseTier == "embedding" {
				minScore = config.AppConfig.MetadataEmbeddingMinScore
			}
			if score <= minScore {
				log.Printf("[DEBUG] metadata-search: adjusted score=%.3f (tier=%s) below threshold for %q by %q from %s",
					score, baseTier, r.Title, r.Author, src.Name())
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
				ISBN:           r.ISBN,
				ASIN:           r.ASIN,
				CoverURL:       r.CoverURL,
				Description:    r.Description,
				Language:       r.Language,
				Source:         src.Name(),
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
					ISBN:           result.ISBN,
					ASIN:           result.ASIN,
					CoverURL:       result.CoverURL,
					Description:    result.Description,
					Language:       result.Language,
					Source:         "Audnexus (Audible)",
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

	// Series-number tiebreaker: if the original title contains a number that
	// was stripped for search (e.g. "We Hunt Monsters 8" → "We Hunt Monsters"),
	// boost candidates whose SeriesPosition or title number matches.
	originalTitle := query
	if originalTitle == "" {
		originalTitle = book.Title
	}
	if expectedNum := extractTrailingNumber(originalTitle); expectedNum != "" {
		for i := range candidates {
			c := &candidates[i]
			candidateNum := ""
			// Check SeriesPosition first (most reliable)
			if c.SeriesPosition != "" {
				candidateNum = normalizeSeriesNumber(c.SeriesPosition)
			}
			// Fall back to trailing number in candidate title
			if candidateNum == "" {
				candidateNum = extractTrailingNumber(c.Title)
			}
			if candidateNum == expectedNum {
				c.Score *= 2.0 // Strong boost for exact number match
			} else if candidateNum != "" && candidateNum != expectedNum {
				c.Score *= 0.5 // Penalize wrong number in same series
			}
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Cap at 50 to support large series
	if len(candidates) > 50 {
		candidates = candidates[:50]
	}

	// Optional LLM rerank pass on the top ambiguous candidates.
	if opts.UseRerank && mfs.llmScorer != nil && config.AppConfig.MetadataLLMScoringEnabled {
		candidates = mfs.rerankTopK(context.Background(), book, candidates)
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
		Language:       candidate.Language,
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

	// Strip embedded "Series Name, Book N" before persisting — protects
	// against Audible/Audnexus candidates where the book number is baked
	// into the series name. Same normalization the auto-fetch paths run.
	normalizeMetaSeries(&meta)

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

	// Tag the book with metadata:source:* and metadata:language:*
	// as system-applied provenance tags. Uses the singleton
	// helpers so a no-op re-apply of the same source/language is
	// a true no-op at the tag layer (no wasted writes). Done after
	// UpdateBook so a failed update never leaves stale tags behind.
	mfs.applyMetadataSystemTags(id, candidate.Source, meta.Language)

	// Invalidate the metadata fetch cache for this book. After a
	// successful apply the book's title/author may have changed,
	// which means any cached candidates from the per-source cache
	// were queried against stale search terms and should be
	// re-fetched next time the user asks.
	if cerr := database.InvalidateAllCachedMetadataFetchesForBook(mfs.db, id); cerr != nil {
		log.Printf("[WARN] metadata apply: failed to invalidate fetch cache for %s: %v", id, cerr)
	}

	return &FetchMetadataResponse{
		Message: "metadata candidate applied",
		Book:    updatedBook,
		Source:  candidate.Source,
	}, nil
}

// applyMetadataSystemTags writes the metadata:source:* and
// metadata:language:* system tags for a book. Logs but doesn't
// propagate errors — tagging is provenance metadata, not part
// of the apply transaction, so a tag write failure shouldn't
// fail the apply itself.
func (mfs *MetadataFetchService) applyMetadataSystemTags(bookID, sourceName, language string) {
	sourceTag := metadataSourceTag(sourceName)
	if sourceTag != "" {
		if err := database.EnsureSingletonBookTag(
			mfs.db, bookID, "metadata:source:", sourceTag, "system",
		); err != nil {
			log.Printf("[WARN] failed to tag book %s with %s: %v", bookID, sourceTag, err)
		}
	}
	langTag := metadataLanguageTag(language)
	if langTag != "" {
		if err := database.EnsureSingletonBookTag(
			mfs.db, bookID, "metadata:language:", langTag, "system",
		); err != nil {
			log.Printf("[WARN] failed to tag book %s with %s: %v", bookID, langTag, err)
		}
	}
}

// metadataSourceTag turns a human-readable source name from
// metadata.MetadataSource.Name() into a tag-safe slug under the
// metadata:source:* namespace. Returns "" for empty inputs so
// the caller can skip the tag write.
//
//	"Hardcover"          → "metadata:source:hardcover"
//	"Open Library"       → "metadata:source:open_library"
//	"Google Books"       → "metadata:source:google_books"
//	"Audnexus (Audible)" → "metadata:source:audnexus"
//	"Audible"            → "metadata:source:audible"
func metadataSourceTag(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Special case: drop the "(Audible)" parenthetical on Audnexus
	// so the tag cleanly identifies the source provider, not its
	// upstream. We still have metadata:source:audible for the
	// direct Audible path.
	if strings.HasPrefix(name, "Audnexus") {
		return "metadata:source:audnexus"
	}
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "_")
	slug = strings.ReplaceAll(slug, "(", "")
	slug = strings.ReplaceAll(slug, ")", "")
	slug = strings.ReplaceAll(slug, "-", "_")
	return "metadata:source:" + slug
}

// metadataLanguageTag turns a language string from a metadata
// source into a tag under the metadata:language:* namespace.
// Accepts ISO 639-1 codes ("en"), ISO 639-2 codes ("eng"), and
// full English names ("English"); normalizes to the 2-letter
// form where recognized and lowercases everything else. Returns
// "" for empty inputs so the caller can skip the tag write.
func metadataLanguageTag(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	// Short list of ISO 639-2 / English-name variants we see
	// across the real sources. Unknown languages fall through
	// to the lowercased input so we never drop data — worst
	// case the tag looks weird but it's still filterable.
	canonical := map[string]string{
		"english":    "en",
		"eng":        "en",
		"spanish":    "es",
		"spa":        "es",
		"french":     "fr",
		"fre":        "fr",
		"fra":        "fr",
		"german":     "de",
		"ger":        "de",
		"deu":        "de",
		"italian":    "it",
		"ita":        "it",
		"japanese":   "ja",
		"jpn":        "ja",
		"chinese":    "zh",
		"chi":        "zh",
		"zho":        "zh",
		"mandarin":   "zh",
		"portuguese": "pt",
		"por":        "pt",
		"russian":    "ru",
		"rus":        "ru",
		"dutch":      "nl",
		"nld":        "nl",
		"korean":     "ko",
		"kor":        "ko",
		"arabic":     "ar",
		"ara":        "ar",
	}
	if code, ok := canonical[lang]; ok {
		return "metadata:language:" + code
	}
	// Already a 2-letter code? Keep it.
	if len(lang) == 2 {
		return "metadata:language:" + lang
	}
	// Unknown — slugify and pass through.
	slug := strings.ReplaceAll(lang, " ", "_")
	return "metadata:language:" + slug
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
			// Filter out tags whose current on-disk value already
			// matches the DB state, so a re-run of bulk write-back
			// is near-free when nothing actually changed.
			// filterUnchangedTags now covers album_artist and
			// composer (both narrator-sourced in our convention),
			// so the filter correctly no-ops on unchanged books
			// instead of always-writing because of those keys.
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
					fm := filterUnchangedTags(f, fullTagMap)
					if len(fm) == 0 {
						log.Printf("[DEBUG] write-back: all tags match, skipping %s", f)
						continue
					}
					backupFileBeforeWrite(f)
					if err := metadata.WriteMetadataToFile(f, fm, opConfig); err != nil {
						log.Printf("[WARN] write-back failed for %s: %v", f, err)
					} else {
						log.Printf("[INFO] wrote metadata back to %s", f)
						writtenCount++
					}
				}
			} else {
				fm := filterUnchangedTags(book.FilePath, fullTagMap)
				if len(fm) == 0 {
					log.Printf("[DEBUG] write-back: all tags match, skipping %s", book.FilePath)
				} else {
					backupFileBeforeWrite(book.FilePath)
					if err := metadata.WriteMetadataToFile(book.FilePath, fm, opConfig); err != nil {
						log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
					} else {
						writtenCount++
					}
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

// backupFileBeforeWrite creates a timestamped .bak copy of a file before
// writing tags — IF the WriteBackupBeforeTagWrite config flag is enabled.
//
// Default is OFF. Historically this function ran unconditionally on every
// tag write and used os.Link (hardlink) for "no disk space cost". Two
// problems with that:
//
//  1. Tens of thousands of stale backup files accumulated across the
//     library (43K+ files, multi-TB apparent size in production) because
//     nothing ever cleaned them up.
//  2. Hardlinks don't actually preserve pre-write content when the
//     writer modifies the inode in place (which TagLib does for some
//     formats). The "backup" could be a hardlink to the same now-modified
//     data, providing false safety.
//
// The flag is opt-in. Users who turn it on should also run the
// cleanup-backups maintenance endpoint periodically to keep the library
// from growing unbounded.
//
// Failures are logged but non-fatal — the write-back proceeds regardless.
func backupFileBeforeWrite(filePath string) {
	if !config.AppConfig.WriteBackupBeforeTagWrite {
		return
	}
	if filePath == "" {
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		return
	}
	backupPath := filePath + ".bak-" + time.Now().Format("20060102-150405")
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
		"title": current.Title,
		"album": current.Album,
		"artist": current.Artist,
		// album_artist and composer both hold the narrator in our
		// audiobook tag convention (album_artist > artist > composer
		// is the read priority). RenameService writes them as two
		// separate keys, so filterUnchangedTags needs to know they
		// compare against current.Narrator too — otherwise every
		// organize pass sees album_artist/composer as "unknown
		// field → always write" and falls through to a real write,
		// which was the root cause of the "organize rewrites tags
		// every time even when unchanged" investigation.
		"album_artist":    current.Narrator,
		"composer":        current.Narrator,
		"narrator":        current.Narrator,
		"genre":           current.Genre,
		"year":            fmt.Sprintf("%d", current.Year),
		"language":        current.Language,
		"series":          current.Series,
		"asin":            current.ASIN,
		"description":     current.Comments, // description is stored in comments field
		"edition":         current.Edition,
		"print_year":      current.PrintYear,
		"book_id":         current.BookOrganizerID,
		"open_library_id": current.OpenLibraryID,
		"hardcover_id":    current.HardcoverID,
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

	if config.AppConfig.AutoRenameOnApply && !hasCheckpoint(mfs.db, id, phaseRename) {
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
		setCheckpoint(mfs.db, id, phaseRename)
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
	if config.AppConfig.AutoWriteTagsOnApply && !hasCheckpoint(mfs.db, id, phaseTags) {
		if written, err := mfs.WriteBackMetadataForBook(id); err != nil {
			log.Printf("[WARN] tag writing failed for book %s: %v", id, err)
		} else {
			log.Printf("[INFO] wrote metadata tags to %d file(s) for book %s", written, id)
			setCheckpoint(mfs.db, id, phaseTags)
		}
	}

	// Enqueue iTunes writeback so the batcher picks up both location
	// (if the file was renamed) and metadata changes. The apply
	// handler also enqueues after this returns; the batcher dedupes
	// on book ID so the duplicate is harmless.
	if mfs.writeBackBatcher != nil && !hasCheckpoint(mfs.db, id, phaseITunes) {
		mfs.writeBackBatcher.Enqueue(id)
		setCheckpoint(mfs.db, id, phaseITunes)
	}

	// All phases complete — clear checkpoints.
	clearCheckpoints(mfs.db, id)
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

	// Trigger dedup check after metadata apply
	if mfs.dedupEngine != nil {
		go func() {
			if _, err := mfs.dedupEngine.CheckBook(context.Background(), id); err != nil {
				log.Printf("[WARN] dedup re-check failed for book %s after metadata apply: %v", id, err)
			}
		}()
	}

	// Enqueue iTunes writeback so location changes from the rename
	// propagate to iTunes. Callers (bulk write-back) also enqueue,
	// the batcher dedupes.
	if mfs.writeBackBatcher != nil {
		mfs.writeBackBatcher.Enqueue(id)
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
