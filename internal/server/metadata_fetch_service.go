// file: internal/server/metadata_fetch_service.go
// version: 2.2.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package server

import (
	"fmt"
	"hash/crc32"
	"log"
	"sort"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
)

type MetadataFetchService struct {
	db      database.Store
	olStore *openlibrary.OLStore
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
			chain = append(chain, metadata.NewGoogleBooksClient())
		case "audnexus":
			chain = append(chain, metadata.NewAudnexusClient())
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

	sources := mfs.BuildSourceChain()
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
