// file: internal/server/metadata_handlers.go
// version: 1.0.0
// guid: 0299d0b0-b697-4386-a1ca-47c8bcc390de
//
// Metadata HTTP handlers split out of server.go: per-book fetch/
// search/apply/revert/no-match, bulk fetch and bulk writeback, the
// field-enumeration endpoint used by the editor, and the copy-on-
// write version list and prune endpoints.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	ulid "github.com/oklog/ulid/v2"
)

// batchUpdateMetadata handles batch metadata updates with validation
func (s *Server) batchUpdateMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Updates  []metadata.MetadataUpdate `json:"updates" binding:"required"`
		Validate bool                      `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	errors, successCount := metadata.BatchUpdateMetadata(req.Updates, database.GlobalStore, req.Validate)

	response := gin.H{
		"success_count": successCount,
		"total_count":   len(req.Updates),
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// validateMetadata validates metadata updates without applying them
func (s *Server) validateMetadata(c *gin.Context) {
	var req struct {
		Updates map[string]any `json:"updates" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rules := metadata.DefaultValidationRules()
	errors := metadata.ValidateMetadata(req.Updates, rules)

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":  false,
			"errors": errorMessages,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"valid":   true,
			"message": "metadata is valid",
		})
	}
}

// exportMetadata exports all audiobook metadata
func (s *Server) exportMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all books
	books, err := database.GlobalStore.GetAllBooks(0, 0) // No limit/offset
	if err != nil {
		internalError(c, "failed to get audiobooks", err)
		return
	}

	// Export metadata
	exportData, err := metadata.ExportMetadata(books)
	if err != nil {
		internalError(c, "failed to export metadata", err)
		return
	}

	c.JSON(http.StatusOK, exportData)
}

// importMetadata imports audiobook metadata
func (s *Server) importMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Data     map[string]any `json:"data" binding:"required"`
		Validate bool           `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	importCount, errors := metadata.ImportMetadata(req.Data, database.GlobalStore, req.Validate)

	response := gin.H{
		"import_count": importCount,
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		c.JSON(http.StatusPartialContent, response)
	} else {
		c.JSON(http.StatusOK, response)
	}
}

// searchMetadata searches external metadata sources
func (s *Server) searchMetadata(c *gin.Context) {
	title := c.Query("title")
	author := c.Query("author")

	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title parameter required"})
		return
	}

	// Use Open Library for now
	client := metadata.NewOpenLibraryClient()

	var results []metadata.BookMetadata
	var err error

	if author != "" {
		results, err = client.SearchByTitleAndAuthor(title, author)
	} else {
		results, err = client.SearchByTitle(title)
	}

	if err != nil {
		internalError(c, "metadata search failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"source":  "Open Library",
	})
}

// fetchAudiobookMetadata fetches and applies metadata to an audiobook
func (s *Server) fetchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	resp, err := s.metadataFetchService.FetchMetadataForBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Enqueue for iTunes auto write-back if metadata was updated
	if GlobalWriteBackBatcher != nil {
		GlobalWriteBackBatcher.Enqueue(id)
	}

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := database.GlobalStore.GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponse(enrichedBook),
		"source":  resp.Source,
	})
}

// searchAudiobookMetadata handles POST /api/v1/audiobooks/:id/search-metadata.
func (s *Server) searchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Query     string `json:"query"`
		Author    string `json:"author"`
		Narrator  string `json:"narrator"`
		Series    string `json:"series"`
		UseRerank bool   `json:"use_rerank"`
	}
	_ = c.ShouldBindJSON(&body)

	// Cache metadata search results for 60s — external API calls are expensive.
	// use_rerank is part of the cache key so a rerank result and a non-rerank
	// result for the same search don't clobber each other.
	cacheKey := fmt.Sprintf("meta_search:%s:%s:%s:%s:%s:%t",
		id, body.Query, body.Author, body.Narrator, body.Series, body.UseRerank)
	if cached, ok := s.listCache.Get(cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	resp, err := s.metadataFetchService.SearchMetadataForBookWithOptions(
		id, body.Query, body.Author, body.Narrator, body.Series,
		SearchOptions{UseRerank: body.UseRerank},
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	// Cache as gin.H wrapper
	respH := gin.H{"results": resp.Results, "query": resp.Query, "sources_tried": resp.SourcesTried, "sources_failed": resp.SourcesFailed}
	s.listCache.Set(cacheKey, respH)
	c.JSON(http.StatusOK, resp)
}

// applyAudiobookMetadata handles POST /api/v1/audiobooks/:id/apply-metadata.
func (s *Server) applyAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Candidate MetadataCandidate `json:"candidate"`
		Fields    []string          `json:"fields"`
		WriteBack *bool             `json:"write_back"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := s.metadataFetchService.ApplyMetadataCandidate(id, body.Candidate, body.Fields)
	if err != nil {
		internalError(c, "failed to apply metadata", err)
		return
	}

	// Kick off slow file I/O (cover embed, tags, rename) in background.
	// Cover download is already done inline so the response has the URL.
	shouldWriteBack := body.WriteBack == nil || *body.WriteBack
	if pool := GetGlobalFileIOPool(); pool != nil {
		bookID := id
		mfs := s.metadataFetchService
		pool.Submit(bookID, func() {
			mfs.ApplyMetadataFileIO(bookID)
			if shouldWriteBack {
				if _, wbErr := mfs.WriteBackMetadataForBook(bookID); wbErr != nil {
					log.Printf("[WARN] background write-back for %s: %v", bookID, wbErr)
				}
				if GlobalWriteBackBatcher != nil {
					GlobalWriteBackBatcher.Enqueue(bookID)
				}
			}
		})
	}

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := database.GlobalStore.GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	c.JSON(http.StatusOK, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponse(enrichedBook),
		"source":  resp.Source,
	})
}

// markAudiobookNoMatch handles POST /api/v1/audiobooks/:id/mark-no-match.
func (s *Server) markAudiobookNoMatch(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if err := s.metadataFetchService.MarkNoMatch(id); err != nil {
		internalError(c, "failed to mark no match", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Book marked as no match"})
}

// revertAudiobookMetadata handles POST /api/v1/audiobooks/:id/revert-metadata.
// It restores a book to a previous CoW version snapshot via the store layer.
func (s *Server) revertAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Timestamp string `json:"timestamp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Timestamp == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "timestamp is required"})
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, body.Timestamp)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timestamp format, use RFC3339Nano"})
		return
	}
	book, err := database.GlobalStore.RevertBookToVersion(id, ts)
	if err != nil {
		internalError(c, "failed to revert metadata", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Book reverted to version", "book": book})
}

// listBookCOWVersions handles GET /api/v1/audiobooks/:id/cow-versions.
// Returns copy-on-write version snapshots from the store layer.
func (s *Server) listBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 50
	if q := c.Query("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			limit = v
		}
	}
	versions, err := database.GlobalStore.GetBookVersions(id, limit)
	if err != nil {
		internalError(c, "failed to list versions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// pruneBookCOWVersions handles POST /api/v1/audiobooks/:id/cow-versions/prune.
// Prunes old copy-on-write version snapshots, keeping the most recent N.
func (s *Server) pruneBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		KeepCount int `json:"keep_count"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KeepCount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keep_count must be a positive integer"})
		return
	}
	pruned, err := database.GlobalStore.PruneBookVersions(id, body.KeepCount)
	if err != nil {
		internalError(c, "failed to prune versions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pruned": pruned})
}

// writeBackAudiobookMetadata handles POST /api/v1/audiobooks/:id/write-back.
// It writes current DB metadata to audio files AND renames files if AutoRenameOnApply is enabled.
func (s *Server) writeBackAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	// Parse optional segment filter and rename flag from request body
	var body struct {
		SegmentIDs []string `json:"segment_ids"`
		Rename     *bool    `json:"rename"`
	}
	_ = c.ShouldBindJSON(&body)

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Step 1: Rename files if requested or AutoRenameOnApply is on
	renamed := 0
	doRename := (body.Rename != nil && *body.Rename) || config.AppConfig.AutoRenameOnApply
	if doRename && len(body.SegmentIDs) == 0 {
		if err := s.metadataFetchService.RunApplyPipelineRenameOnly(id, book); err != nil {
			log.Printf("[WARN] rename failed for book %s: %v", id, err)
		} else {
			renamed = 1
			// Re-fetch book after rename since file_path may have changed
			book, _ = database.GlobalStore.GetBookByID(id)
		}
	}

	// Step 2: Write tags to files
	var writtenCount int
	if len(body.SegmentIDs) > 0 {
		writtenCount, err = s.metadataFetchService.WriteBackMetadataForBook(id, body.SegmentIDs)
	} else {
		writtenCount, err = s.metadataFetchService.WriteBackMetadataForBook(id)
	}
	if err != nil {
		internalError(c, "failed to write back metadata", err)
		return
	}

	msg := fmt.Sprintf("metadata written to %d file(s)", writtenCount)
	if writtenCount == 0 {
		msg = "no files needed tag updates (tags already match DB values)"
	}
	if renamed > 0 {
		msg += ", files renamed"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       msg,
		"written_count": writtenCount,
		"renamed":       renamed > 0,
	})
}

// bulkFetchMetadata fetches external metadata for multiple audiobooks and applies
// fields only when they are missing and not manually overridden or locked.
func (s *Server) bulkFetchMetadata(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req bulkFetchMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	onlyMissing := true
	if req.OnlyMissing != nil {
		onlyMissing = *req.OnlyMissing
	}

	sourceChain := s.metadataFetchService.BuildSourceChain()
	if len(sourceChain) == 0 {
		// Fallback to Audible if no sources configured (best for audiobooks)
		sourceChain = []metadata.MetadataSource{metadata.NewAudibleClient()}
	}
	results := make([]bulkFetchMetadataResult, 0, len(req.BookIDs))
	updatedCount := 0

	for _, bookID := range req.BookIDs {
		result := bulkFetchMetadataResult{
			BookID: bookID,
			Status: "skipped",
		}

		book, err := database.GlobalStore.GetBookByID(bookID)
		if err != nil || book == nil {
			result.Status = "not_found"
			result.Message = "audiobook not found"
			results = append(results, result)
			continue
		}

		if strings.TrimSpace(book.Title) == "" {
			result.Message = "missing title"
			results = append(results, result)
			continue
		}

		state, err := loadMetadataState(bookID)
		if err != nil {
			result.Status = "error"
			result.Message = "failed to load metadata state"
			results = append(results, result)
			continue
		}
		if state == nil {
			state = map[string]metadataFieldState{}
		}

		// Resolve current author for post-search verification (NOT for search query)
		currentAuthor := ""
		if book.Author != nil {
			currentAuthor = book.Author.Name
		} else if book.AuthorID != nil {
			if author, err := database.GlobalStore.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				currentAuthor = author.Name
			}
		}

		// Clean title: strip track number prefixes like "01 - ", chapter markers, etc.
		searchTitle := stripChapterFromTitle(book.Title)

		// Search using both title and author (like the manual search dialog does)
		// for better match quality. Author is used as a filter, not as the primary query.
		var metaResults []metadata.BookMetadata
		var sourceName string
		for _, src := range sourceChain {
			// If we have an author, try title+author search first for more precise results
			if currentAuthor != "" {
				metaResults, err = src.SearchByTitleAndAuthor(searchTitle, currentAuthor)
				if err == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			// Fall back to title-only search
			metaResults, err = src.SearchByTitle(searchTitle)
			if err == nil && len(metaResults) > 0 {
				sourceName = src.Name()
				break
			}
			// Try original title if stripped version returned nothing
			if searchTitle != book.Title {
				if currentAuthor != "" {
					metaResults, err = src.SearchByTitleAndAuthor(book.Title, currentAuthor)
					if err == nil && len(metaResults) > 0 {
						sourceName = src.Name()
						break
					}
				}
				metaResults, err = src.SearchByTitle(book.Title)
				if err == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			log.Printf("[DEBUG] bulkFetchMetadata: source %s returned no results for %q, trying next", src.Name(), searchTitle)
		}
		if len(metaResults) == 0 {
			result.Status = "not_found"
			result.Message = "no metadata found from any source"
			results = append(results, result)
			continue
		}

		// Pick best match: prefer result whose author matches current author if known
		meta := metaResults[0]
		if currentAuthor != "" && len(metaResults) > 1 {
			lowerAuthor := strings.ToLower(currentAuthor)
			for _, r := range metaResults {
				if strings.EqualFold(r.Author, currentAuthor) || strings.Contains(strings.ToLower(r.Author), lowerAuthor) {
					meta = r
					break
				}
			}
		}
		fetchedValues := map[string]any{}
		appliedFields := []string{}
		fetchedFields := []string{}

		addFetched := func(field string, value any) {
			fetchedValues[field] = value
			fetchedFields = append(fetchedFields, field)
		}

		shouldApply := func(field string, hasValue bool) bool {
			entry := state[field]
			if entry.OverrideLocked || entry.OverrideValue != nil {
				return false
			}
			if onlyMissing && hasValue {
				return false
			}
			return true
		}

		hasBookValue := func(field string) bool {
			switch field {
			case "title":
				return strings.TrimSpace(book.Title) != ""
			case "author_name":
				return book.AuthorID != nil || book.Author != nil
			case "publisher":
				return book.Publisher != nil && strings.TrimSpace(*book.Publisher) != ""
			case "language":
				return book.Language != nil && strings.TrimSpace(*book.Language) != ""
			case "audiobook_release_year":
				return book.AudiobookReleaseYear != nil && *book.AudiobookReleaseYear != 0
			case "isbn10":
				return book.ISBN10 != nil && strings.TrimSpace(*book.ISBN10) != ""
			case "isbn13":
				return book.ISBN13 != nil && strings.TrimSpace(*book.ISBN13) != ""
			default:
				return false
			}
		}

		didUpdate := false

		if meta.Title != "" && !isGarbageValue(meta.Title) {
			addFetched("title", meta.Title)
			if shouldApply("title", hasBookValue("title")) {
				book.Title = meta.Title
				appliedFields = append(appliedFields, "title")
				didUpdate = true
			}
		}

		if meta.Author != "" && !isGarbageValue(meta.Author) {
			addFetched("author_name", meta.Author)
			if shouldApply("author_name", hasBookValue("author_name")) {
				author, err := database.GlobalStore.GetAuthorByName(meta.Author)
				if err != nil {
					result.Status = "error"
					result.Message = "failed to resolve author"
					results = append(results, result)
					continue
				}
				if author == nil {
					author, err = database.GlobalStore.CreateAuthor(meta.Author)
					if err != nil {
						result.Status = "error"
						result.Message = "failed to create author"
						results = append(results, result)
						continue
					}
				}
				book.AuthorID = &author.ID
				appliedFields = append(appliedFields, "author_name")
				didUpdate = true
			}
		}

		if meta.Publisher != "" && !isGarbageValue(meta.Publisher) {
			addFetched("publisher", meta.Publisher)
			if shouldApply("publisher", hasBookValue("publisher")) {
				book.Publisher = stringPtr(meta.Publisher)
				appliedFields = append(appliedFields, "publisher")
				didUpdate = true
			}
		}

		if meta.Language != "" && !isGarbageValue(meta.Language) {
			addFetched("language", meta.Language)
			if shouldApply("language", hasBookValue("language")) {
				book.Language = stringPtr(meta.Language)
				appliedFields = append(appliedFields, "language")
				didUpdate = true
			}
		}

		if meta.PublishYear != 0 {
			addFetched("audiobook_release_year", meta.PublishYear)
			if shouldApply("audiobook_release_year", hasBookValue("audiobook_release_year")) {
				year := meta.PublishYear
				book.AudiobookReleaseYear = &year
				appliedFields = append(appliedFields, "audiobook_release_year")
				didUpdate = true
			}
		}

		if meta.ISBN != "" {
			if len(meta.ISBN) == 10 {
				addFetched("isbn10", meta.ISBN)
				if shouldApply("isbn10", hasBookValue("isbn10")) {
					book.ISBN10 = stringPtr(meta.ISBN)
					appliedFields = append(appliedFields, "isbn10")
					didUpdate = true
				}
			} else {
				addFetched("isbn13", meta.ISBN)
				if shouldApply("isbn13", hasBookValue("isbn13")) {
					book.ISBN13 = stringPtr(meta.ISBN)
					appliedFields = append(appliedFields, "isbn13")
					didUpdate = true
				}
			}
		}

		if len(fetchedValues) > 0 {
			if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
				log.Printf("[WARN] bulkFetchMetadata: failed to persist fetched metadata state for %s: %v", bookID, err)
			}
		}

		if didUpdate {
			// Record change history before applying
			s.metadataFetchService.recordChangeHistory(book, meta, sourceName)

			if _, err := database.GlobalStore.UpdateBook(bookID, book); err != nil {
				result.Status = "error"
				result.Message = fmt.Sprintf("failed to update book: %v", err)
				results = append(results, result)
				continue
			}
			updatedCount++
			result.Status = "updated"

			// System tag the source and language so the review UI
			// and future upgrade jobs know where this came from.
			s.metadataFetchService.applyMetadataSystemTags(bookID, sourceName, meta.Language)
		} else if len(fetchedValues) > 0 {
			result.Status = "fetched"
		}

		result.AppliedFields = appliedFields
		result.FetchedFields = fetchedFields
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"updated_count": updatedCount,
		"total_count":   len(req.BookIDs),
		"results":       results,
	})
}

// handleBulkWriteBack handles POST /api/v1/audiobooks/bulk-write-back.
// It creates an async operation that writes metadata tags and renames files
// for all books matching the provided filters (or all organized/imported books).
func (s *Server) handleBulkWriteBack(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		Filter struct {
			LibraryState *string `json:"library_state"`
			AuthorID     *int    `json:"author_id"`
			SeriesID     *int    `json:"series_id"`
		} `json:"filter"`
		DryRun bool `json:"dry_run"`
		Rename bool `json:"rename"`
	}
	_ = c.ShouldBindJSON(&req)

	store := database.GlobalStore

	// Gather matching books based on filters
	var books []database.Book
	var err error

	if req.Filter.AuthorID != nil {
		books, err = store.GetBooksByAuthorID(*req.Filter.AuthorID)
	} else if req.Filter.SeriesID != nil {
		books, err = store.GetBooksBySeriesID(*req.Filter.SeriesID)
	} else {
		// Get all books, then filter by library_state
		books, err = store.GetAllBooks(1_000_000, 0)
	}
	if err != nil {
		internalError(c, "failed to query books", err)
		return
	}

	// Filter by library_state if specified, otherwise default to organized+imported
	targetStates := map[string]bool{"organized": true, "imported": true}
	if req.Filter.LibraryState != nil && *req.Filter.LibraryState != "" {
		targetStates = map[string]bool{*req.Filter.LibraryState: true}
	}

	var filtered []database.Book
	for _, book := range books {
		// Skip soft-deleted books
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			continue
		}
		// Skip books with empty file paths
		if book.FilePath == "" {
			continue
		}
		// Skip protected paths
		if isProtectedPath(book.FilePath) {
			continue
		}
		// Filter by library state (only when not filtering by author/series exclusively)
		if book.LibraryState != nil {
			if !targetStates[*book.LibraryState] {
				continue
			}
		} else if req.Filter.AuthorID == nil && req.Filter.SeriesID == nil {
			// No library_state set and no author/series filter: skip
			continue
		}
		filtered = append(filtered, book)
	}

	estimatedBooks := len(filtered)

	// Dry run: just return the count
	if req.DryRun {
		c.JSON(http.StatusOK, gin.H{
			"estimated_books": estimatedBooks,
			"dry_run":         true,
		})
		return
	}

	if estimatedBooks == 0 {
		c.JSON(http.StatusOK, gin.H{
			"estimated_books": 0,
			"message":         "no books match the given filters",
		})
		return
	}

	// Create the operation
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "bulk_write_back", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	doRename := req.Rename
	mfs := s.metadataFetchService
	bookIDs := make([]string, len(filtered))
	for i, b := range filtered {
		bookIDs[i] = b.ID
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		total := len(bookIDs)
		written := 0
		failed := 0

		for i, bookID := range bookIDs {
			if progress.IsCanceled() {
				msg := fmt.Sprintf("canceled after %d/%d books (%d written, %d failed)", i, total, written, failed)
				_ = progress.Log("info", msg, nil)
				return nil
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				msg := fmt.Sprintf("context canceled after %d/%d books (%d written, %d failed)", i, total, written, failed)
				_ = progress.Log("info", msg, nil)
				return ctx.Err()
			default:
			}

			book, err := store.GetBookByID(bookID)
			if err != nil || book == nil {
				failed++
				detail := fmt.Sprintf("book %s: not found", bookID)
				_ = progress.Log("warn", detail, nil)
				_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (skipped: not found)", i+1, total))
				continue
			}

			// Skip protected paths (re-check in case data changed)
			if isProtectedPath(book.FilePath) {
				detail := fmt.Sprintf("book %s: skipping protected path %s", bookID, book.FilePath)
				_ = progress.Log("info", detail, nil)
				_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (skipped: protected)", i+1, total))
				continue
			}

			// Step 1: Rename if requested
			if doRename {
				if renameErr := mfs.RunApplyPipelineRenameOnly(bookID, book); renameErr != nil {
					detail := fmt.Sprintf("book %s: rename failed: %v", bookID, renameErr)
					_ = progress.Log("warn", detail, nil)
				}
			}

			// Step 2: Write tags
			count, writeErr := mfs.WriteBackMetadataForBook(bookID)
			if writeErr != nil {
				failed++
				detail := fmt.Sprintf("book %s: write-back failed: %v", bookID, writeErr)
				_ = progress.Log("warn", detail, nil)
			} else {
				written++
				if count > 0 {
					detail := fmt.Sprintf("book %s: wrote %d file(s)", bookID, count)
					_ = progress.Log("debug", detail, nil)
				}
			}

			_ = progress.UpdateProgress(i+1, total, fmt.Sprintf("processing %d/%d (%d written, %d failed)", i+1, total, written, failed))
		}

		summary := fmt.Sprintf("bulk write-back complete: %d written, %d failed out of %d", written, failed, total)
		_ = progress.Log("info", summary, nil)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "bulk_write_back", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id":    op.ID,
		"estimated_books": estimatedBooks,
	})
}

func (s *Server) batchWriteBackAudiobooks(c *gin.Context) {
	var req struct {
		BookIDs  []string `json:"book_ids"`
		Rename   bool     `json:"rename"`
		Organize bool     `json:"organize"`
		Force    bool     `json:"force"` // skip change detection, rewrite everything
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	store := database.GlobalStore
	doOrganize := req.Organize || req.Rename

	// Create a supervisor operation for tracking
	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "batch_save_to_files", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	bookIDs := make([]string, len(req.BookIDs))
	copy(bookIDs, req.BookIDs)
	totalBooks := len(bookIDs)
	force := req.Force
	mfs := s.metadataFetchService
	orgSvc := s.organizeService

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, totalBooks, "starting save to files")

		written, organized, failed, skipped := 0, 0, 0, 0
		org := organizer.NewOrganizer(&config.AppConfig)
		log2 := logger.NewWithActivityLog("batch-write-back", store)

		for i, id := range bookIDs {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			book, err := store.GetBookByID(id)
			if err != nil || book == nil {
				failed++
				_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("book %s not found", id), nil)
				continue
			}

			// Skip if already written and metadata hasn't changed since last write
			if !force && book.LastWrittenAt != nil && !book.UpdatedAt.After(*book.LastWrittenAt) {
				skipped++
				_ = progress.UpdateProgress(i+1, totalBooks,
					fmt.Sprintf("processed %d/%d (skipped: %d — already up to date)", i+1, totalBooks, skipped))
				continue
			}

			// Write tags
			_, wbErr := mfs.WriteBackMetadataForBook(id)
			if wbErr != nil {
				failed++
				detail := wbErr.Error()
				_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("write-back failed for %s", book.Title), &detail)
				continue
			}
			written++
			// Stamp last_written_at on the book the user sees (may differ from library copy)
			_ = store.SetLastWrittenAt(id, time.Now())

			// Organize
			if doOrganize {
				book, _ = store.GetBookByID(id)
				if book != nil {
					oldPath := book.FilePath
					alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)
					var newPath string
					var orgErr error
					if alreadyInRoot {
						newPath, orgErr = orgSvc.reOrganizeInPlace(book, log2)
					} else {
						bookFiles, _ := store.GetBookFiles(id)
						isDir := len(bookFiles) > 1
						if !isDir {
							if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
								isDir = true
							}
						}
						if isDir {
							newPath, orgErr = orgSvc.organizeDirectoryBook(org, book, log2)
						} else {
							newPath, _, orgErr = org.OrganizeBook(book)
						}
					}
					if orgErr != nil {
						detail := orgErr.Error()
						_ = store.AddOperationLog(opID, "warn", fmt.Sprintf("organize failed for %s", book.Title), &detail)
					} else if newPath != "" && newPath != oldPath {
						organized++
					}
				}
			}

			// Enqueue ITL write-back
			if GlobalWriteBackBatcher != nil {
				GlobalWriteBackBatcher.Enqueue(id)
			}

			_ = progress.UpdateProgress(i+1, totalBooks,
				fmt.Sprintf("processed %d/%d (written: %d, organized: %d, failed: %d)",
					i+1, totalBooks, written, organized, failed))
		}

		_ = progress.UpdateProgress(totalBooks, totalBooks,
			fmt.Sprintf("complete: written %d, organized %d, skipped %d, failed %d", written, organized, skipped, failed))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "batch_save_to_files", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"message":      fmt.Sprintf("Save to files queued for %d books", totalBooks),
		"book_count":   totalBooks,
	})
}

// getMetadataFields returns available metadata fields with their types and validation rules
func (s *Server) getMetadataFields(c *gin.Context) {
	fields := []map[string]any{
		{
			"name":        "title",
			"type":        "string",
			"required":    true,
			"maxLength":   500,
			"description": "Book title",
		},
		{
			"name":        "author",
			"type":        "string",
			"required":    false,
			"description": "Author name",
		},
		{
			"name":        "narrator",
			"type":        "string",
			"required":    false,
			"description": "Narrator name",
		},
		{
			"name":        "publisher",
			"type":        "string",
			"required":    false,
			"description": "Publisher name",
		},
		{
			"name":        "publishDate",
			"type":        "integer",
			"required":    false,
			"min":         1000,
			"max":         9999,
			"description": "Publication year",
		},
		{
			"name":        "series",
			"type":        "string",
			"required":    false,
			"description": "Series name",
		},
		{
			"name":        "language",
			"type":        "string",
			"required":    false,
			"pattern":     "^[a-z]{2}$",
			"description": "ISO 639-1 language code (e.g., 'en', 'es')",
		},
		{
			"name":        "isbn10",
			"type":        "string",
			"required":    false,
			"pattern":     "^[0-9]{9}[0-9X]$",
			"description": "ISBN-10",
		},
		{
			"name":        "isbn13",
			"type":        "string",
			"required":    false,
			"pattern":     "^97[89][0-9]{10}$",
			"description": "ISBN-13",
		},
		{
			"name":        "series_sequence",
			"type":        "integer",
			"required":    false,
			"min":         1,
			"description": "Position in series",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"fields": fields,
	})
}
