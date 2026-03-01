// file: internal/server/metadata_fetch_service.go
// version: 4.3.0
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
			// Score all results and pick the best; reject if below quality threshold.
			scored := bestTitleMatch(results, searchTitle, book.Title)
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
		// Stamp last_written_at after successful write-back.
		if err := mfs.db.SetLastWrittenAt(book.ID, time.Now()); err != nil {
			log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", book.ID, err)
		}
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
	for _, w := range strings.Fields(strings.ToLower(s)) {
		// Strip leading/trailing punctuation (apostrophes, commas, etc.)
		w = strings.Trim(w, ".,;:!?\"'()")
		if len(w) > 2 && !scoreTitleStop[w] {
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

// WriteBackMetadataForBook reads current DB metadata for the book, resolves authors and
// narrators, writes comprehensive tags to all active audio file segments, and records a
// history entry. It is called by POST /api/v1/audiobooks/:id/write-back.
func (mfs *MetadataFetchService) WriteBackMetadataForBook(id string) (int, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return 0, fmt.Errorf("audiobook not found: %s", id)
	}

	// --- Resolve author names ---
	var authorNames []string
	bookAuthors, err := mfs.db.GetBookAuthors(id)
	if err == nil && len(bookAuthors) > 0 {
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
	artistStr := strings.Join(authorNames, " & ")

	// --- Resolve narrator names ---
	var narratorNames []string
	bookNarrators, err := mfs.db.GetBookNarrators(id)
	if err == nil && len(bookNarrators) > 0 {
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
	}

	opConfig := fileops.OperationConfig{VerifyChecksums: true}

	// --- Collect active segments ---
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, segErr := mfs.db.ListBookSegments(bookNumericID)
	if segErr != nil {
		segments = nil
	}
	// Filter to active only
	var activeSegments []database.BookSegment
	for _, seg := range segments {
		if seg.Active {
			activeSegments = append(activeSegments, seg)
		}
	}

	totalTracks := len(activeSegments)
	writtenCount := 0

	if totalTracks > 1 {
		// Multi-file: write to each segment with per-track title and numbering
		digits := len(fmt.Sprintf("%d", totalTracks))
		trackFmt := fmt.Sprintf("%%0%dd", digits)
		for i, seg := range activeSegments {
			trackNum := i + 1
			segTitle := fmt.Sprintf(trackFmt+" - %s", trackNum, book.Title)
			trackStr := fmt.Sprintf("%d/%d", trackNum, totalTracks)
			tagMap := mfs.buildTagMap(book.Title, segTitle, artistStr, narratorStr, year, trackStr)
			if err := metadata.WriteMetadataToFile(seg.FilePath, tagMap, opConfig); err != nil {
				log.Printf("[WARN] write-back failed for segment %s: %v", seg.FilePath, err)
			} else {
				writtenCount++
			}
		}
	} else {
		// Single-file or no segments: write to book.FilePath
		tagMap := mfs.buildTagMap(book.Title, book.Title, artistStr, narratorStr, year, "")
		if err := metadata.WriteMetadataToFile(book.FilePath, tagMap, opConfig); err != nil {
			log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
		} else {
			writtenCount++
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

	// Stamp last_written_at
	if writtenCount > 0 {
		if err := mfs.db.SetLastWrittenAt(book.ID, now); err != nil {
			log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", book.ID, err)
		}
	}

	return writtenCount, nil
}

// buildTagMap constructs the tag map shared by all write-back paths.
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
