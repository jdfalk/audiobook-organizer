// file: internal/server/metadata_fetch_service.go
// version: 2.7.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"fmt"
	"hash/crc32"
	"log"
	"sort"
	"strings"

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
	if meta.Title != "" {
		book.Title = meta.Title
	}
	if meta.Publisher != "" {
		book.Publisher = stringPtr(meta.Publisher)
	}
	if meta.Language != "" {
		book.Language = stringPtr(meta.Language)
	}
	if meta.PublishYear != 0 {
		book.AudiobookReleaseYear = intPtrHelper(meta.PublishYear)
	}
	if meta.CoverURL != "" {
		book.CoverURL = stringPtr(meta.CoverURL)
	}
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
