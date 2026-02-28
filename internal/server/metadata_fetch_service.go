// file: internal/server/metadata_fetch_service.go
// version: 3.0.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
	"github.com/jdfalk/audiobook-organizer/internal/tagger"
)

type MetadataFetchService struct {
	db              database.Store
	olStore         *openlibrary.OLStore
	overrideSources []metadata.MetadataSource // for testing
}

func NewMetadataFetchService(db database.Store) *MetadataFetchService {
	return &MetadataFetchService{db: db}
}

// SetOLStore sets the Open Library dump store for local-first lookups.
func (mfs *MetadataFetchService) SetOLStore(store *openlibrary.OLStore) {
	mfs.olStore = store
}

type FetchMetadataResponse struct {
	Message      string
	Book         *database.Book
	Source       string
	FetchedCount int
}

// BuildSourceChain returns metadata sources ordered by config priority.
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
		switch src.ID {
		case "openlibrary":
			client := metadata.NewOpenLibraryClient()
			if mfs.olStore != nil {
				client.SetOLStore(mfs.olStore)
			}
			chain = append(chain, client)
		case "google-books":
			apiKey := config.AppConfig.GoogleBooksAPIKey
			if apiKey == "" {
				if k, ok := src.Credentials["apiKey"]; ok && k != "" {
					apiKey = k
				}
			}
			chain = append(chain, metadata.NewGoogleBooksClient(apiKey))
		case "audnexus":
			chain = append(chain, metadata.NewAudnexusClient())
		case "hardcover":
			token := config.AppConfig.HardcoverAPIToken
			if token == "" {
				// Also check credentials map from metadata source config
				if apiToken, ok := src.Credentials["api_token"]; ok && apiToken != "" {
					token = apiToken
				}
			}
			if token != "" {
				chain = append(chain, metadata.NewHardcoverClient(token))
			} else {
				log.Printf("[WARN] Hardcover source enabled but no API token configured")
			}
		default:
			log.Printf("[WARN] Unknown metadata source: %s", src.ID)
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

	// Resolve author name for fallback searches
	var authorName string
	if book.AuthorID != nil {
		author, authorErr := mfs.db.GetAuthorByID(*book.AuthorID)
		if authorErr == nil && author != nil {
			authorName = author.Name
		}
	}

	var lastErr error
	for _, src := range sources {
		results, searchErr := src.SearchByTitle(searchTitle)
		if searchErr != nil {
			log.Printf("[WARN] %s failed for %q: %v", src.Name(), searchTitle, searchErr)
			lastErr = searchErr
			continue
		}

		// Try original title if cleaned title returned nothing
		if len(results) == 0 && searchTitle != book.Title {
			results, searchErr = src.SearchByTitle(book.Title)
			if searchErr != nil {
				lastErr = searchErr
				continue
			}
		}

		// Try with author if we have one and still no results
		if len(results) == 0 && authorName != "" {
			results, searchErr = src.SearchByTitleAndAuthor(searchTitle, authorName)
			if searchErr != nil {
				lastErr = searchErr
				continue
			}
			if len(results) == 0 && searchTitle != book.Title {
				results, searchErr = src.SearchByTitleAndAuthor(book.Title, authorName)
				if searchErr != nil {
					lastErr = searchErr
					continue
				}
			}
		}

		// Step 4: Try with subtitle stripped (e.g. "Title: Subtitle" → "Title")
		if len(results) == 0 {
			strippedTitle := stripSubtitle(searchTitle)
			if strippedTitle != searchTitle && strippedTitle != book.Title {
				if authorName != "" {
					results, searchErr = src.SearchByTitleAndAuthor(strippedTitle, authorName)
				} else {
					results, searchErr = src.SearchByTitle(strippedTitle)
				}
				if searchErr != nil {
					lastErr = searchErr
					continue
				}
			}
		}

		// Step 5: Author-only search — pick best match from results
		if len(results) == 0 && authorName != "" {
			results, searchErr = src.SearchByTitle(authorName)
			if searchErr != nil {
				lastErr = searchErr
				continue
			}
			// Filter results to find best title match
			if len(results) > 0 {
				results = bestTitleMatch(results, searchTitle, book.Title)
			}
		}

		if len(results) == 0 {
			log.Printf("[DEBUG] %s returned 0 results for %q", src.Name(), searchTitle)
		}
		if len(results) > 0 {
			meta := results[0]

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
					// Embed cover art into audio file metadata if enabled
					if config.AppConfig.EmbedCoverArt && updatedBook != nil && updatedBook.FilePath != "" {
						if embedErr := tagger.EmbedCoverArt(updatedBook.FilePath, coverPath); embedErr != nil {
							log.Printf("[WARN] cover art embedding failed for %s: %v", updatedBook.FilePath, embedErr)
						} else {
							log.Printf("[INFO] cover art embedded into %s", updatedBook.FilePath)
						}
					}
				}
			}

			// Write metadata back to audio file(s) if enabled
			if config.AppConfig.WriteBackMetadata {
				mfs.writeBackMetadata(updatedBook, meta)
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

func (mfs *MetadataFetchService) applyMetadataToBook(book *database.Book, meta metadata.BookMetadata) {
	if meta.Title != "" && isBetterValue(book.Title, meta.Title) {
		book.Title = meta.Title
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

	// Apply author if fetched data is better
	if meta.Author != "" && !isGarbageValue(meta.Author) {
		// Author is handled via AuthorID resolution, but we record it for provenance
		// The actual AuthorID update happens in persistFetchedMetadata / resolve flow
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
	garbage := []string{"unknown", "narrator", "various", "n/a", "none", "null", "undefined", ""}
	for _, g := range garbage {
		if lower == g {
			return true
		}
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
	tagMap := make(map[string]interface{})
	if meta.Title != "" {
		tagMap["title"] = meta.Title
	}
	if meta.Author != "" {
		tagMap["artist"] = meta.Author
	}
	if meta.Publisher != "" {
		tagMap["publisher"] = meta.Publisher
	}
	if meta.PublishYear != 0 {
		tagMap["year"] = meta.PublishYear
	}
	if len(tagMap) == 0 {
		return
	}

	opConfig := fileops.OperationConfig{VerifyChecksums: true}

	// Write to primary file
	if err := metadata.WriteMetadataToFile(book.FilePath, tagMap, opConfig); err != nil {
		log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
	} else {
		log.Printf("[INFO] wrote metadata back to %s", book.FilePath)
	}

	// Write to each segment file for multi-file books
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, err := mfs.db.ListBookSegments(bookNumericID)
	if err != nil {
		return
	}
	for _, seg := range segments {
		if !seg.Active {
			continue
		}
		if err := metadata.WriteMetadataToFile(seg.FilePath, tagMap, opConfig); err != nil {
			log.Printf("[WARN] write-back failed for segment %s: %v", seg.FilePath, err)
		}
	}
}

// bestTitleMatch filters author-search results to find the best match for the
// book title. Returns a single-element slice with the best match, or nil if
// no result has any word overlap with the title.
func bestTitleMatch(results []metadata.BookMetadata, titles ...string) []metadata.BookMetadata {
	// Build set of significant words from all title variants
	titleWords := map[string]bool{}
	for _, t := range titles {
		for _, w := range strings.Fields(strings.ToLower(t)) {
			if len(w) > 2 { // skip short words like "a", "of", "the" noise
				titleWords[w] = true
			}
		}
	}

	bestIdx := -1
	bestScore := 0
	for i, r := range results {
		score := 0
		for _, w := range strings.Fields(strings.ToLower(r.Title)) {
			if titleWords[w] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 && bestScore > 0 {
		return []metadata.BookMetadata{results[bestIdx]}
	}
	return nil
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
	if len(fetchedValues) > 0 {
		if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
			log.Printf("[ERROR] FetchMetadataForBook: failed to persist fetched metadata state: %v", err)
		}
	}
}
