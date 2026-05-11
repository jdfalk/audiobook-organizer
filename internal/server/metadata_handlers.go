// file: internal/server/metadata_handlers.go
// version: 3.7.0
// guid: 0299d0b0-b697-4386-a1ca-47c8bcc390de
// last-edited: 2026-05-10
//
// Metadata HTTP handlers split out of server.go: per-book fetch/
// search/apply/revert/no-match, bulk fetch and bulk writeback, the
// field-enumeration endpoint used by the editor, and the copy-on-
// write version list and prune endpoints.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	ulid "github.com/oklog/ulid/v2"
)

// batchUpdateMetadata handles batch metadata updates with validation
func (s *Server) batchUpdateMetadata(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		Updates  []metadata.MetadataUpdate `json:"updates" binding:"required"`
		Validate bool                      `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	errors, successCount := metadata.BatchUpdateMetadata(req.Updates, s.Store(), req.Validate)

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
		httputil.RespondWithSuccess(c, 206, response)
	} else {
		httputil.RespondWithOK(c, response)
	}
}

// validateMetadata validates metadata updates without applying them
func (s *Server) validateMetadata(c *gin.Context) {
	var req struct {
		Updates map[string]any `json:"updates" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	rules := metadata.DefaultValidationRules()
	errors := metadata.ValidateMetadata(req.Updates, rules)

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		httputil.RespondWithBadRequest(c, fmt.Sprintf("validation errors: %v", errorMessages))
	} else {
		httputil.RespondWithOK(c, gin.H{
			"valid":   true,
			"message": "metadata is valid",
		})
	}
}

// exportMetadata exports all audiobook metadata
func (s *Server) exportMetadata(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	// Get all books
	books, err := s.Store().GetAllBooks(0, 0) // No limit/offset
	if err != nil {
		httputil.InternalError(c, "failed to get audiobooks", err)
		return
	}

	// Export metadata
	exportData, err := metadata.ExportMetadata(books)
	if err != nil {
		httputil.InternalError(c, "failed to export metadata", err)
		return
	}

	httputil.RespondWithOK(c, exportData)
}

// importMetadata imports audiobook metadata
func (s *Server) importMetadata(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		Data     map[string]any `json:"data" binding:"required"`
		Validate bool           `json:"validate"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	importCount, errors := metadata.ImportMetadata(req.Data, s.Store(), req.Validate)

	response := gin.H{
		"import_count": importCount,
	}

	if len(errors) > 0 {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			errorMessages[i] = err.Error()
		}
		response["errors"] = errorMessages
		httputil.RespondWithSuccess(c, 206, response)
	} else {
		httputil.RespondWithOK(c, response)
	}
}

// searchMetadata searches external metadata sources
func (s *Server) searchMetadata(c *gin.Context) {
	title := c.Query("title")
	author := c.Query("author")

	if title == "" {
		httputil.RespondWithBadRequest(c, "title parameter required")
		return
	}

	// Use Open Library for now
	client := metadata.NewOpenLibraryClient()

	var results []metadata.BookMetadata
	var err error

	ctx := c.Request.Context()
	if author != "" {
		results, err = client.SearchByTitleAndAuthor(ctx, title, author)
	} else {
		results, err = client.SearchByTitle(ctx, title)
	}

	if err != nil {
		httputil.InternalError(c, "metadata search failed", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{
		"results": results,
		"source":  "Open Library",
	})
}

// fetchAudiobookMetadata fetches and applies metadata to an audiobook
func (s *Server) fetchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")

	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	resp, err := s.metadataFetchService.FetchMetadataForBook(id)
	if err != nil {
		httputil.RespondWithError(c, 404, err.Error(), "NOT_FOUND")
		return
	}

	// Enqueue for iTunes auto write-back if metadata was updated
	if s.writeBackBatcher != nil {
		s.writeBackBatcher.Enqueue(id)
	}

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := s.Store().GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	httputil.RespondWithOK(c, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponseSingle(enrichedBook),
		"source":  resp.Source,
	})
}

// searchAudiobookMetadata handles POST /api/v1/audiobooks/:id/search-metadata.
func (s *Server) searchAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
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
		httputil.RespondWithOK(c, cached)
		return
	}

	resp, err := s.metadataFetchService.SearchMetadataForBookWithOptions(
		id, body.Query, body.Author, body.Narrator, body.Series,
		metafetch.SearchOptions{UseRerank: body.UseRerank},
	)
	if err != nil {
		httputil.RespondWithError(c, 404, err.Error(), "NOT_FOUND")
		return
	}
	// Cache as gin.H wrapper
	respH := gin.H{"results": resp.Results, "query": resp.Query, "sources_tried": resp.SourcesTried, "sources_failed": resp.SourcesFailed}
	s.listCache.Set(cacheKey, respH)
	httputil.RespondWithOK(c, resp)
}

// applyAudiobookMetadata handles POST /api/v1/audiobooks/:id/apply-metadata.
func (s *Server) applyAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	var body struct {
		Candidate metafetch.MetadataCandidate `json:"candidate"`
		Fields    []string                    `json:"fields"`
		WriteBack *bool                       `json:"write_back"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}
	resp, err := s.metadataFetchService.ApplyMetadataCandidate(id, body.Candidate, body.Fields)
	if err != nil {
		httputil.InternalError(c, "failed to apply metadata", err)
		return
	}

	// Kick off slow file I/O (cover embed, tags, rename) in background.
	// Cover download is already done inline so the response has the URL.
	shouldWriteBack := body.WriteBack == nil || *body.WriteBack

	// Enqueue in the write-back batcher immediately (before pool submission)
	// so the batcher picks up the metadata change even if the background
	// file-IO job panics on a malformed audio file. The DB metadata is
	// already updated at this point, so early enqueueing is correct.
	if shouldWriteBack && s.writeBackBatcher != nil {
		s.writeBackBatcher.Enqueue(id)
	}

	if pool := s.fileIOPool; pool != nil {
		bookID := id
		mfs := s.metadataFetchService
		pool.Submit(bookID, func() {
			mfs.ApplyMetadataFileIO(bookID)
			if shouldWriteBack {
				if _, wbErr := mfs.WriteBackMetadataForBook(bookID); wbErr != nil {
					log.Printf("[WARN] background write-back for %s: %v", bookID, wbErr)
				}
			}
		})
	}

	s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventMetadataApplied, id, map[string]any{
		"source":  resp.Source,
		"message": resp.Message,
	}))

	// Re-fetch to get fully enriched book with author/series/narrator names
	enrichedBook := resp.Book
	if fresh, err := s.Store().GetBookByID(id); err == nil && fresh != nil {
		enrichedBook = fresh
	}
	httputil.RespondWithOK(c, gin.H{
		"message": resp.Message,
		"book":    enrichBookForResponseSingle(enrichedBook),
		"source":  resp.Source,
	})
}

// markAudiobookNoMatch handles POST /api/v1/audiobooks/:id/mark-no-match.
func (s *Server) markAudiobookNoMatch(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if err := s.metadataFetchService.MarkNoMatch(id); err != nil {
		httputil.InternalError(c, "failed to mark no match", err)
		return
	}
	rejection := database.MetadataRejection{
		ID:              ulid.Make().String(),
		BookID:          id,
		Source:          "user",
		RejectionReason: "user_rejected",
		RejectedAt:      time.Now(),
	}
	if rerr := s.Store().AddMetadataRejection(rejection); rerr != nil {
		log.Printf("[WARN] markAudiobookNoMatch: could not record rejection for %s: %v", id, rerr)
	}
	httputil.RespondWithOK(c, gin.H{"message": "Book marked as no match"})
}

// handleGetMetadataRejections handles GET /api/v1/audiobooks/:id/metadata-rejections.
func (s *Server) handleGetMetadataRejections(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	rejections, err := s.Store().GetMetadataRejections(id)
	if err != nil {
		httputil.InternalError(c, "failed to get metadata rejections", err)
		return
	}
	if rejections == nil {
		rejections = []database.MetadataRejection{}
	}
	httputil.RespondWithOK(c, gin.H{"rejections": rejections})
}

// revertAudiobookMetadata handles POST /api/v1/audiobooks/:id/revert-metadata.
// It restores a book to a previous CoW version snapshot via the store layer.
func (s *Server) revertAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	var body struct {
		Timestamp string `json:"timestamp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Timestamp == "" {
		httputil.RespondWithBadRequest(c, "timestamp is required")
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, body.Timestamp)
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid timestamp format, use RFC3339Nano")
		return
	}
	book, err := s.Store().RevertBookToVersion(id, ts)
	if err != nil {
		httputil.InternalError(c, "failed to revert metadata", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "Book reverted to version", "book": book})
}

// listBookCOWVersions handles GET /api/v1/audiobooks/:id/cow-versions.
// Returns copy-on-write version snapshots from the store layer.
func (s *Server) listBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	p := httputil.ParsePaginationParams(c)
	limit := p.Limit
	versions, err := s.Store().GetBookSnapshots(id, limit)
	if err != nil {
		httputil.InternalError(c, "failed to list versions", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"versions": versions})
}

// pruneBookCOWVersions handles POST /api/v1/audiobooks/:id/cow-versions/prune.
// Prunes old copy-on-write version snapshots, keeping the most recent N.
func (s *Server) pruneBookCOWVersions(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	var body struct {
		KeepCount int `json:"keep_count"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.KeepCount <= 0 {
		httputil.RespondWithBadRequest(c, "keep_count must be a positive integer")
		return
	}
	pruned, err := s.Store().PruneBookSnapshots(id, body.KeepCount)
	if err != nil {
		httputil.InternalError(c, "failed to prune versions", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"pruned": pruned})
}

// writeBackAudiobookMetadata handles POST /api/v1/audiobooks/:id/write-back.
// It writes current DB metadata to audio files AND renames files if AutoRenameOnApply is enabled.
func (s *Server) writeBackAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "book id is required")
		return
	}

	// Parse optional segment filter and rename flag from request body
	var body struct {
		SegmentIDs []string `json:"segment_ids"`
		Rename     *bool    `json:"rename"`
	}
	_ = c.ShouldBindJSON(&body)

	book, err := s.Store().GetBookByID(id)
	if err != nil || book == nil {
		httputil.RespondWithNotFound(c, "audiobook", "")
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
			book, _ = s.Store().GetBookByID(id)
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
		httputil.InternalError(c, "failed to write back metadata", err)
		return
	}

	msg := fmt.Sprintf("metadata written to %d file(s)", writtenCount)
	if writtenCount == 0 {
		msg = "no files needed tag updates (tags already match DB values)"
	}
	if renamed > 0 {
		msg += ", files renamed"
	}

	httputil.RespondWithOK(c, gin.H{
		"message":       msg,
		"written_count": writtenCount,
		"renamed":       renamed > 0,
	})
}

// bulkFetchMetadata fetches external metadata for multiple audiobooks and applies
// fields only when they are missing and not manually overridden or locked.
func (s *Server) bulkFetchMetadata(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req bulkFetchMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.BookIDs) == 0 {
		httputil.RespondWithBadRequest(c, "book_ids is required")
		return
	}

	onlyMissing := true
	if req.OnlyMissing != nil {
		onlyMissing = *req.OnlyMissing
	}

	results := make([]bulkFetchMetadataResult, 0, len(req.BookIDs))
	updatedCount := 0

	for _, bookID := range req.BookIDs {
		result := bulkFetchMetadataResult{
			BookID: bookID,
			Status: "skipped",
		}

		book, err := s.Store().GetBookByID(bookID)
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

		// Delegate search to service using empty query (uses book's title).
		// Service handles source chain, caching, and candidate scoring.
		searchResp, searchErr := s.metadataFetchService.SearchMetadataForBookWithOptions(
			bookID, "", "", "", "",
			metafetch.SearchOptions{},
		)
		if searchErr != nil {
			result.Status = "error"
			result.Message = fmt.Sprintf("search failed: %v", searchErr)
			results = append(results, result)
			continue
		}
		if searchResp == nil || len(searchResp.Results) == 0 {
			result.Status = "not_found"
			result.Message = "no metadata found from any source"
			results = append(results, result)
			continue
		}

		// Pick best match from service's scored candidates (first is already best)
		candidate := searchResp.Results[0]
		// Convert MetadataCandidate to BookMetadata for field mapping
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
			DurationSec:    candidate.DurationSec,
			AudibleRatingOverall:     candidate.AudibleRatingOverall,
			AudibleRatingPerformance: 0, // not available from candidate
			AudibleRatingStory:       0, // not available from candidate
			AudibleRatingCount:       candidate.AudibleRatingCount,
			AudibleNumReviews:        0, // not available from candidate
			GoogleRatingAverage:      candidate.GoogleRatingAverage,
			GoogleRatingCount:        candidate.GoogleRatingCount,
		}
		sourceName := candidate.Source
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
			case "duration":
				return book.Duration != nil && *book.Duration > 0
			case "audible_rating_overall":
				return book.AudibleRatingOverall != nil && *book.AudibleRatingOverall > 0
			case "google_rating_average":
				return book.GoogleRatingAverage != nil && *book.GoogleRatingAverage > 0
			default:
				return false
			}
		}

		didUpdate := false

		if meta.Title != "" && !metafetch.IsGarbageValue(meta.Title) {
			addFetched("title", meta.Title)
			if shouldApply("title", hasBookValue("title")) {
				book.Title = meta.Title
				appliedFields = append(appliedFields, "title")
				didUpdate = true
			}
		}

		if meta.Author != "" && !metafetch.IsGarbageValue(meta.Author) {
			addFetched("author_name", meta.Author)
			if shouldApply("author_name", hasBookValue("author_name")) {
				author, err := s.Store().GetAuthorByName(meta.Author)
				if err != nil {
					result.Status = "error"
					result.Message = "failed to resolve author"
					results = append(results, result)
					continue
				}
				if author == nil {
					author, err = s.Store().CreateAuthor(meta.Author)
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

		if meta.Publisher != "" && !metafetch.IsGarbageValue(meta.Publisher) {
			addFetched("publisher", meta.Publisher)
			if shouldApply("publisher", hasBookValue("publisher")) {
				book.Publisher = stringPtr(meta.Publisher)
				appliedFields = append(appliedFields, "publisher")
				didUpdate = true
			}
		}

		if meta.Language != "" && !metafetch.IsGarbageValue(meta.Language) {
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

		if meta.DurationSec > 0 {
			addFetched("duration", meta.DurationSec)
			if shouldApply("duration", hasBookValue("duration")) {
				dur := meta.DurationSec
				book.Duration = &dur
				appliedFields = append(appliedFields, "duration")
				didUpdate = true
			}
		}

		// Audible ratings — always overwrite (ratings update over time; newer > older).
		if meta.AudibleRatingOverall > 0 {
			addFetched("audible_rating_overall", meta.AudibleRatingOverall)
			if shouldApply("audible_rating_overall", hasBookValue("audible_rating_overall")) {
				v := meta.AudibleRatingOverall
				book.AudibleRatingOverall = &v
				v2 := meta.AudibleRatingPerformance
				book.AudibleRatingPerformance = &v2
				v3 := meta.AudibleRatingStory
				book.AudibleRatingStory = &v3
				c := meta.AudibleRatingCount
				book.AudibleRatingCount = &c
				r := meta.AudibleNumReviews
				book.AudibleNumReviews = &r
				appliedFields = append(appliedFields, "audible_ratings")
				didUpdate = true
			}
		}

		// Google Books rating.
		if meta.GoogleRatingAverage > 0 {
			addFetched("google_rating_average", meta.GoogleRatingAverage)
			if shouldApply("google_rating_average", hasBookValue("google_rating_average")) {
				v := meta.GoogleRatingAverage
				book.GoogleRatingAverage = &v
				c := meta.GoogleRatingCount
				book.GoogleRatingCount = &c
				appliedFields = append(appliedFields, "google_rating")
				didUpdate = true
			}
		}

		if len(fetchedValues) > 0 {
			if err := updateFetchedMetadataState(bookID, fetchedValues); err != nil {
				log.Printf("[WARN] bulkFetchMetadata: failed to persist fetched metadata state for %s: %v", bookID, err)
			}
		}

		if didUpdate {
			// Record change history before applying
			s.metadataFetchService.RecordChangeHistory(book, meta, sourceName)

			if _, err := s.Store().UpdateBook(bookID, book); err != nil {
				result.Status = "error"
				result.Message = fmt.Sprintf("failed to update book: %v", err)
				results = append(results, result)
				continue
			}
			updatedCount++
			result.Status = "updated"

			// System tag the source and language so the review UI
			// and future upgrade jobs know where this came from.
			s.metadataFetchService.ApplyMetadataSystemTags(bookID, sourceName, meta.Language)
		} else if len(fetchedValues) > 0 {
			result.Status = "fetched"
		}

		result.AppliedFields = appliedFields
		result.FetchedFields = fetchedFields
		results = append(results, result)
	}

	httputil.RespondWithOK(c, gin.H{
		"updated_count": updatedCount,
		"total_count":   len(req.BookIDs),
		"results":       results,
	})
}

// handleBulkMetadataFetchAll starts an async, resumable full-library metadata
// fetch as a queued operation and returns the operation ID immediately (HTTP 202).
//
// Fetches from the full source chain (Audible, OpenLibrary, etc.) for every book
// and populates the metadata cache. Nothing is written to book records — the user
// reviews and applies per-book through the normal UI. Already-processed books are
// skipped on resume so it is safe to restart.
//
// Query params:
//   - prefer_audible=true — move Audible to the front of the source chain
//   - skip_cached=true    — skip books that already have a valid cache entry
//
// Poll progress via GET /api/v1/operations/{id}.
func (s *Server) handleBulkMetadataFetchAll(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operations registry not initialized")
		return
	}
	params := bulkMetadataFetchV2Params{
		PreferAudible: c.DefaultQuery("prefer_audible", "false") == "true",
		SkipCached:    c.DefaultQuery("skip_cached", "false") == "true",
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "library.bulk-metadata-fetch", params)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}
	log.Printf("[INFO] bulk-metadata-fetch: queued %s prefer_audible=%v skip_cached=%v",
		opID, params.PreferAudible, params.SkipCached)
	httputil.RespondWithSuccess(c, http.StatusAccepted, gin.H{
		"operation_id":   opID,
		"op_id":          opID,
		"message":        "bulk metadata fetch started — poll GET /api/v1/operations/v2/" + opID + " for progress",
		"prefer_audible": params.PreferAudible,
		"skip_cached":    params.SkipCached,
	})
}

// runBulkMetadataFetchAll is the resumable core of the full-library metadata
// fetch. It ONLY fetches and caches — it never writes to book records.
// Results land in PutCachedMetadataFetch so the per-book review UI can show
// them immediately when the user clicks "apply". Idempotent: books with an
// existing OperationResult row are skipped on resume.
func (s *Server) runBulkMetadataFetchAll(
	ctx context.Context,
	opID string,
	params operations.BulkMetadataFetchParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading books")

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}

	maxAge := time.Duration(config.AppConfig.MetadataFetchCacheTTLDays) * 24 * time.Hour

	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true
	}

	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}

	type bookWork struct {
		book       database.Book
		authorName string
	}
	var work []bookWork
	for i := range allBooks {
		b := &allBooks[i]
		if done[b.ID] || strings.TrimSpace(b.Title) == "" {
			continue
		}
		// skip_cached: skip books that already have a valid (non-expired) cache entry
		// from any source so we only hit the API for books with no cached data.
		if params.SkipCached {
			hasFreshCache := false
			for _, src := range s.metadataFetchService.BuildSourceChain() {
				if cached, _, cerr := database.GetCachedMetadataFetchWithMaxAge(store, b.ID, src.Name(), maxAge); cerr == nil && cached != nil {
					hasFreshCache = true
					break
				}
			}
			if hasFreshCache {
				continue
			}
		}
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		work = append(work, bookWork{book: *b, authorName: author})
	}

	totalBooks := len(existingResults) + len(work)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] bulk-metadata-fetch %s: %d books total, %d already cached, %d to fetch",
		opID, totalBooks, alreadyDone, len(work))
	_ = progress.UpdateProgress(alreadyDone, totalBooks,
		fmt.Sprintf("resuming: %d/%d already cached", alreadyDone, totalBooks))

	if len(work) == 0 {
		_ = progress.UpdateProgress(totalBooks, totalBooks, "all books already cached")
		return nil
	}

	sourceChain := s.metadataFetchService.BuildSourceChain()
	if len(sourceChain) == 0 {
		sourceChain = []metadata.MetadataSource{metadata.NewAudibleClient()}
	}
	// Move Audible to front of chain when preferred.
	if params.PreferAudible {
		audible := metadata.NewAudibleClient()
		var rest []metadata.MetadataSource
		for _, src := range sourceChain {
			if src.Name() != audible.Name() {
				rest = append(rest, src)
			}
		}
		sourceChain = append([]metadata.MetadataSource{audible}, rest...)
	}

	completed := int64(alreadyDone)
	found := 0
	notFound := 0

	for i, w := range work {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		bookID := w.book.ID
		currentAuthor := w.authorName
		searchTitle := stripChapterFromTitle(w.book.Title)

		var metaResults []metadata.BookMetadata
		var sourceName string
		cacheHit := false

		for _, src := range sourceChain {
			if cached, _, cerr := database.GetCachedMetadataFetchWithMaxAge(store, bookID, src.Name(), maxAge); cerr == nil && cached != nil {
				var cachedResults []metadata.BookMetadata
				if jerr := json.Unmarshal(cached.Results, &cachedResults); jerr == nil && len(cachedResults) > 0 {
					metaResults = cachedResults
					sourceName = src.Name()
					cacheHit = true
					break
				}
			}
			var fetchErr error
			if currentAuthor != "" {
				metaResults, fetchErr = src.SearchByTitleAndAuthor(ctx, searchTitle, currentAuthor)
				if fetchErr == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			metaResults, fetchErr = src.SearchByTitle(ctx, searchTitle)
			if fetchErr == nil && len(metaResults) > 0 {
				sourceName = src.Name()
				break
			}
			if searchTitle != w.book.Title {
				if currentAuthor != "" {
					metaResults, fetchErr = src.SearchByTitleAndAuthor(ctx, w.book.Title, currentAuthor)
					if fetchErr == nil && len(metaResults) > 0 {
						sourceName = src.Name()
						break
					}
				}
				metaResults, fetchErr = src.SearchByTitle(ctx, w.book.Title)
				if fetchErr == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
		}

		resultStatus := "not_found"
		if len(metaResults) > 0 && sourceName != "" {
			if !cacheHit {
				if blob, merr := json.Marshal(metaResults); merr == nil {
					_ = database.PutCachedMetadataFetch(store, bookID, sourceName, blob, 0)
				}
			}
			found++
			resultStatus = "cached"
		} else {
			notFound++
		}

		_ = store.CreateOperationResult(&database.OperationResult{
			OperationID: opID,
			BookID:      bookID,
			ResultJSON:  fmt.Sprintf(`{"status":%q,"source":%q}`, resultStatus, sourceName),
			Status:      resultStatus,
		})

		n := atomic.AddInt64(&completed, 1)
		if i%50 == 0 || int(n) == totalBooks {
			_ = progress.UpdateProgress(int(n), totalBooks,
				fmt.Sprintf("fetched %d/%d — cached:%d not_found:%d", n, totalBooks, found, notFound))
		}

		// Rate-limit live API calls; cache hits are instant so skip the delay.
		if !cacheHit && sourceName != "" && i < len(work)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	finalCount := atomic.LoadInt64(&completed)
	_ = progress.UpdateProgress(int(finalCount), totalBooks,
		fmt.Sprintf("complete — cached:%d not_found:%d", found, notFound))
	log.Printf("[INFO] bulk-metadata-fetch %s: done %d books — cached:%d not_found:%d",
		opID, finalCount, found, notFound)
	return nil
}

// registryProgressAdapter bridges registry.Reporter → operations.ProgressReporter
// so runBulkMetadataFetchAll can be called from a v2 op Run function without changes.
type registryProgressAdapter struct{ r opsregistry.Reporter }

func (a registryProgressAdapter) UpdateProgress(current, total int, message string) error {
	return a.r.UpdateProgress(current, total, message)
}
func (a registryProgressAdapter) Log(level, message string, details *string) error {
	l := slog.LevelInfo
	switch level {
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	case "debug":
		l = slog.LevelDebug
	}
	var attrs []slog.Attr
	if details != nil {
		attrs = append(attrs, slog.String("details", *details))
	}
	return a.r.Log(l, message, attrs...)
}
func (a registryProgressAdapter) IsCanceled() bool { return a.r.IsCanceled() }

// bulkMetadataFetchV2Params is the JSON params for the v2 bulk_metadata_fetch op.
// Selection replaces the old BookIDs field: the client sends either
//   - book_ids: an explicit list of IDs (page-level selection), or
//   - filter: a FilterSpec that the server resolves to IDs at run time
//     with IsPrimaryVersion=true always applied.
type bulkMetadataFetchV2Params struct {
	Selection     operations.SelectionSpec `json:"selection"`
	PreferAudible bool                     `json:"prefer_audible"`
	SkipCached    bool                     `json:"skip_cached"`
}

// resolveFilterToBookIDs translates a FilterSpec into a concrete list of primary-
// version book IDs.  IsPrimaryVersion=true and quarantine exclusion are always
// applied.  If f.OnlyUnmatched is set, books that already have a "matched"
// candidate in the most-recent metadata_candidate_fetch result are removed.
// Per-user FieldFilters are silently dropped (no user context in background ops).
func (s *Server) resolveFilterToBookIDs(ctx context.Context, f operations.FilterSpec) ([]string, error) {
	trueVal := true
	filters := ListFilters{
		IsPrimaryVersion: &trueVal,
		LibraryState:     f.LibraryState,
		Tag:              f.Tag,
	}
	for _, ff := range f.FieldFilters {
		if IsPerUserField(ff.Field) {
			continue
		}
		filters.FieldFilters = append(filters.FieldFilters, FieldFilter{
			Field:   ff.Field,
			Value:   ff.Value,
			Negated: ff.Negated,
		})
	}
	var authorID, seriesID *int
	if f.AuthorID != nil {
		v := int(*f.AuthorID)
		authorID = &v
	}
	if f.SeriesID != nil {
		v := int(*f.SeriesID)
		seriesID = &v
	}
	books, err := s.audiobookService.GetAudiobooks(ctx, 100000, 0, f.Search, authorID, seriesID, filters)
	if err != nil {
		return nil, fmt.Errorf("resolve filter: %w", err)
	}
	ids := make([]string, 0, len(books))
	for _, b := range books {
		if b.QuarantinedAt != nil {
			continue
		}
		ids = append(ids, b.ID)
	}
	if f.OnlyUnmatched {
		matched := latestMatchedBookIDs(s.Store())
		filtered := ids[:0]
		for _, id := range ids {
			if !matched[id] {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}
	return ids, nil
}

// RegisterBulkMetadataFetchOp registers the "library.bulk-metadata-fetch" v2
// OperationDef so that POST /api/v1/operations/v2 with def_id "bulk_metadata_fetch"
// shows in the bell, is resumable, and can be cancelled.
func (s *Server) RegisterBulkMetadataFetchOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "library.bulk-metadata-fetch",
		Plugin:          "library",
		DisplayName:     "Bulk Metadata Fetch",
		Description:     "Fetch and cache external metadata for a set of audiobooks. Nothing is written to book records — results appear in the per-book review UI.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         6 * time.Hour,
		ResumePolicy:    opsregistry.ResumeRestart,
		ConcurrencyKey:  "library.bulk-metadata-fetch",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapNetworkGeneric, opsregistry.CapLibraryRead},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p bulkMetadataFetchV2Params
			if len(rawParams) > 0 {
				if err := json.Unmarshal(rawParams, &p); err != nil {
					return fmt.Errorf("bulk_metadata_fetch: decode params: %w", err)
				}
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("bulk_metadata_fetch: database not initialized")
			}

			// Generate a stable opID for OperationResult rows (resume key).
			// The registry assigns its own run ID; we derive a deterministic
			// sub-ID so OperationResult rows survive restarts.
			opID := ulid.Make().String()

			fetchParams := operations.BulkMetadataFetchParams{
				PreferAudible: p.PreferAudible,
				SkipCached:    p.SkipCached,
			}

			progress := registryProgressAdapter{r: reporter}

			bookIDs, err := operations.ResolveBookIDs(p.Selection, func(f operations.FilterSpec) ([]string, error) {
				return s.resolveFilterToBookIDs(ctx, f)
			})
			if err != nil {
				return fmt.Errorf("bulk_metadata_fetch: resolve selection: %w", err)
			}

			if len(bookIDs) > 0 {
				return s.runBulkMetadataFetchForBookIDs(ctx, opID, bookIDs, fetchParams, store, progress)
			}
			return s.runBulkMetadataFetchAll(ctx, opID, fetchParams, store, progress)
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterBulkMetadataFetchOp(reg) })
}

// runBulkMetadataFetchForBookIDs fetches and caches metadata for a specific set
// of books identified by ID. It shares resume semantics with runBulkMetadataFetchAll:
// books that already have an OperationResult row for this opID are skipped.
func (s *Server) runBulkMetadataFetchForBookIDs(
	ctx context.Context,
	opID string,
	bookIDs []string,
	params operations.BulkMetadataFetchParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, len(bookIDs), "loading books")

	maxAge := time.Duration(config.AppConfig.MetadataFetchCacheTTLDays) * 24 * time.Hour

	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true
	}

	allAuthors, _ := store.GetAllAuthors()
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}

	type bookWork struct {
		book       database.Book
		authorName string
	}
	var work []bookWork
	for _, id := range bookIDs {
		if done[id] {
			continue
		}
		b, err := store.GetBookByID(id)
		if err != nil || b == nil || strings.TrimSpace(b.Title) == "" {
			continue
		}
		if params.SkipCached {
			hasFresh := false
			for _, src := range s.metadataFetchService.BuildSourceChain() {
				if cached, _, cerr := database.GetCachedMetadataFetchWithMaxAge(store, id, src.Name(), maxAge); cerr == nil && cached != nil {
					hasFresh = true
					break
				}
			}
			if hasFresh {
				continue
			}
		}
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		work = append(work, bookWork{book: *b, authorName: author})
	}

	alreadyDone := len(existingResults)
	totalBooks := alreadyDone + len(work)
	log.Printf("[INFO] bulk-metadata-fetch-ids %s: %d total, %d done, %d to fetch",
		opID, totalBooks, alreadyDone, len(work))
	_ = progress.UpdateProgress(alreadyDone, totalBooks,
		fmt.Sprintf("resuming: %d/%d already done", alreadyDone, totalBooks))

	sourceChain := s.metadataFetchService.BuildSourceChain()
	if params.PreferAudible {
		audible := metadata.NewAudibleClient()
		var rest []metadata.MetadataSource
		for _, src := range sourceChain {
			if src.Name() != audible.Name() {
				rest = append(rest, src)
			}
		}
		sourceChain = append([]metadata.MetadataSource{audible}, rest...)
	}

	completed := int64(alreadyDone)
	found, notFound := 0, 0
	for i, w := range work {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		bookID := w.book.ID
		searchTitle := stripChapterFromTitle(w.book.Title)

		var metaResults []metadata.BookMetadata
		var sourceName string
		cacheHit := false
		for _, src := range sourceChain {
			if cached, _, cerr := database.GetCachedMetadataFetchWithMaxAge(store, bookID, src.Name(), maxAge); cerr == nil && cached != nil {
				var cr []metadata.BookMetadata
				if jerr := json.Unmarshal(cached.Results, &cr); jerr == nil && len(cr) > 0 {
					metaResults, sourceName, cacheHit = cr, src.Name(), true
					break
				}
			}
			var ferr error
			if w.authorName != "" {
				metaResults, ferr = src.SearchByTitleAndAuthor(ctx, searchTitle, w.authorName)
				if ferr == nil && len(metaResults) > 0 {
					sourceName = src.Name()
					break
				}
			}
			metaResults, ferr = src.SearchByTitle(ctx, searchTitle)
			if ferr == nil && len(metaResults) > 0 {
				sourceName = src.Name()
				break
			}
		}

		resultStatus := "not_found"
		if len(metaResults) > 0 && sourceName != "" {
			if !cacheHit {
				if blob, merr := json.Marshal(metaResults); merr == nil {
					_ = database.PutCachedMetadataFetch(store, bookID, sourceName, blob, 0)
				}
			}
			found++
			resultStatus = "cached"
		} else {
			notFound++
		}
		_ = store.CreateOperationResult(&database.OperationResult{
			OperationID: opID,
			BookID:      bookID,
			ResultJSON:  fmt.Sprintf(`{"status":%q,"source":%q}`, resultStatus, sourceName),
			Status:      resultStatus,
		})

		n := atomic.AddInt64(&completed, 1)
		if i%50 == 0 || int(n) == totalBooks {
			_ = progress.UpdateProgress(int(n), totalBooks,
				fmt.Sprintf("fetched %d/%d — cached:%d not_found:%d", n, totalBooks, found, notFound))
		}
		if !cacheHit && sourceName != "" && i < len(work)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	finalCount := atomic.LoadInt64(&completed)
	_ = progress.UpdateProgress(int(finalCount), totalBooks,
		fmt.Sprintf("complete — cached:%d not_found:%d", found, notFound))
	log.Printf("[INFO] bulk-metadata-fetch-ids %s: done %d books — cached:%d not_found:%d",
		opID, finalCount, found, notFound)
	return nil
}

// handleBulkWriteBack handles POST /api/v1/audiobooks/bulk-write-back.
// It creates an async operation that writes metadata tags and renames files
// for all books matching the provided filters (or all organized/imported books).
func (s *Server) handleBulkWriteBack(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operations registry not initialized")
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

	store := s.Store()

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
		httputil.InternalError(c, "failed to query books", err)
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
		httputil.RespondWithOK(c, gin.H{
			"estimated_books": estimatedBooks,
			"dry_run":         true,
		})
		return
	}

	if estimatedBooks == 0 {
		httputil.RespondWithOK(c, gin.H{
			"estimated_books": 0,
			"message":         "no books match the given filters",
		})
		return
	}

	doRename := req.Rename
	bookIDs := make([]string, len(filtered))
	for i, b := range filtered {
		bookIDs[i] = b.ID
	}

	rawParams, _ := json.Marshal(bulkWriteBackOpParams{BookIDs: bookIDs, Rename: doRename})
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "library.bulk-write-back", rawParams)
	if err != nil {
		httputil.InternalError(c, "enqueue failed", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, gin.H{
		"operation_id":    opID,
		"id":              opID,
		"estimated_books": estimatedBooks,
	})
}

// runBulkWriteBack writes tags (and optionally renames) for each book in bookIDs,
// starting at startIdx. Uses a parallel worker pool — cover embedding and tag
// writes both go through TagLib so there is no ffmpeg ordering constraint.
// Checkpoints every 10 completions so a restart can resume near where it left off.
func (s *Server) runBulkWriteBack(
	ctx context.Context,
	opID string,
	bookIDs []string,
	doRename bool,
	startIdx int,
	progress operations.ProgressReporter,
) error {
	const workers = 2

	store := s.Store()
	mfs := s.metadataFetchService
	total := len(bookIDs)

	if startIdx > 0 {
		_ = progress.Log("info", fmt.Sprintf("resuming bulk write-back from index %d/%d", startIdx, total), nil)
	}

	type job struct {
		id   string
		book *database.Book
	}

	jobCh := make(chan job, workers*2)
	var wg sync.WaitGroup
	var written, failed atomic.Int64
	var mu sync.Mutex

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				if ctx.Err() != nil {
					return
				}
				count, writeErr := mfs.WriteBackMetadataForBook(j.id)
				if writeErr != nil {
					failed.Add(1)
					mu.Lock()
					_ = progress.Log("warn", fmt.Sprintf("book %s: write-back failed: %v", j.id, writeErr), nil)
					mu.Unlock()
				} else {
					written.Add(1)
					if count > 0 && s.activityWriter != nil {
						activity.LogBatch(s.activityWriter, opID, "metadata-apply", "write-back",
							activity.BatchItem{Name: j.book.Title, Count: count})
					}
				}
				done := written.Load() + failed.Load()
				mu.Lock()
				_ = progress.UpdateProgress(int(done), total, fmt.Sprintf("processing %d/%d (%d written, %d failed)", done, total, written.Load(), failed.Load()))
				if done%10 == 0 {
					_ = operations.SaveCheckpoint(store, opID, "bulk_write_back", "writing", int(done), total)
				}
				mu.Unlock()
			}
		}()
	}

	for i := startIdx; i < total; i++ {
		if ctx.Err() != nil || progress.IsCanceled() {
			mu.Lock()
			_ = progress.Log("info", fmt.Sprintf("canceled after feeding %d/%d books", i-startIdx, total-startIdx), nil)
			mu.Unlock()
			break
		}

		bookID := bookIDs[i]
		book, err := store.GetBookByID(bookID)
		if err != nil || book == nil {
			failed.Add(1)
			mu.Lock()
			_ = progress.Log("warn", fmt.Sprintf("book %s: not found", bookID), nil)
			mu.Unlock()
			continue
		}
		if isProtectedPath(book.FilePath) {
			mu.Lock()
			_ = progress.Log("info", fmt.Sprintf("book %s: skipping protected path", bookID), nil)
			mu.Unlock()
			continue
		}
		if doRename {
			if renameErr := mfs.RunApplyPipelineRenameOnly(bookID, book); renameErr != nil {
				mu.Lock()
				_ = progress.Log("warn", fmt.Sprintf("book %s: rename failed: %v", bookID, renameErr), nil)
				mu.Unlock()
			}
		}

		select {
		case jobCh <- job{id: bookID, book: book}:
		case <-ctx.Done():
		}
	}
	close(jobCh)
	wg.Wait()

	_ = operations.ClearState(store, opID)
	summary := fmt.Sprintf("bulk write-back complete: %d written, %d failed out of %d", written.Load(), failed.Load(), total)
	_ = progress.Log("info", summary, nil)
	if s.activityWriter != nil {
		activity.FlushOperation(s.activityWriter, opID)
	}
	return nil
}

// runIsbnEnrichment enriches missing ISBN identifiers from external sources.
// Idempotent — books that already have an ISBN are skipped, so a restart
// safely re-runs from scratch (no checkpoint needed).
func (s *Server) runIsbnEnrichment(ctx context.Context, progress operations.ProgressReporter, opID string) error {
	if s.metadataFetchService == nil || s.metadataFetchService.ISBNEnrichment() == nil {
		_ = progress.Log("info", "ISBN enrichment service is not configured, skipping", nil)
		return nil
	}
	startMsg := "Scanning for books missing ISBN identifiers"
	_ = progress.Log("info", startMsg, nil)
	if operations.IsManual(ctx) {
		activity.EmitInfo(s.activityWriter, opID, "isbn-enrich", "isbn-enrichment", startMsg, activity.AlwaysShow)
	}
	checked, updated, err := s.metadataFetchService.ISBNEnrichment().EnrichMissingISBNs(ctx, 100, s.activityWriter, opID)
	if err != nil {
		return err
	}
	activity.FlushOperation(s.activityWriter, opID)
	msg := fmt.Sprintf("ISBN enrichment complete: checked %d, updated %d", checked, updated)
	_ = progress.Log("info", msg, nil)
	_ = progress.UpdateProgress(100, 100, msg)
	tags := activity.TagsIf(updated == 0, activity.NoOpTag)
	if operations.IsManual(ctx) {
		tags = append(tags, activity.AlwaysShow)
	}
	activity.EmitInfo(s.activityWriter, opID, "isbn-enrich", "isbn-enrichment", msg, tags...)
	return nil
}

// runMetadataRefreshScan reports books with incomplete metadata. Read-only,
// safe to re-run on restart with no state.
func (s *Server) runMetadataRefreshScan(ctx context.Context, progress operations.ProgressReporter) error {
	store := s.Store()
	if store == nil {
		return fmt.Errorf("database not initialized")
	}
	_ = progress.Log("info", "Starting metadata refresh scan", nil)
	_ = progress.UpdateProgress(0, 100, "Scanning books for incomplete metadata...")
	books, err := store.GetAllBooks(10000, 0)
	if err != nil {
		return fmt.Errorf("failed to get books: %w", err)
	}
	_ = progress.Log("info", fmt.Sprintf("Checking %d books for incomplete metadata", len(books)), nil)
	incomplete := 0
	for i, book := range books {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if book.AuthorID == nil || book.Title == "" {
			incomplete++
			_ = progress.Log("debug", fmt.Sprintf("Incomplete: %q (id=%s)", book.Title, book.ID), nil)
		}
		if (i+1)%200 == 0 {
			_ = progress.UpdateProgress(i+1, len(books), fmt.Sprintf("Checked %d/%d books", i+1, len(books)))
		}
	}
	resultMsg := fmt.Sprintf("Found %d books with incomplete metadata out of %d total", incomplete, len(books))
	_ = progress.Log("info", resultMsg, nil)
	_ = progress.UpdateProgress(len(books), len(books), resultMsg)
	return nil
}

func (s *Server) batchWriteBackAudiobooks(c *gin.Context) {
	var req struct {
		BookIDs  []string `json:"book_ids"`
		Rename   bool     `json:"rename"`
		Organize bool     `json:"organize"`
		Force    bool     `json:"force"` // skip change detection, rewrite everything
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.BookIDs) == 0 {
		httputil.RespondWithBadRequest(c, "book_ids is required")
		return
	}

	store := s.Store()
	doOrganize := req.Organize || req.Rename

	// Create a supervisor operation for tracking
	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "batch_save_to_files", nil); err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	bookIDs := make([]string, len(req.BookIDs))
	copy(bookIDs, req.BookIDs)
	totalBooks := len(bookIDs)

	params := batchSaveOpParams{
		LegacyOpID: opID,
		BookIDs:    bookIDs,
		Organize:   doOrganize,
		Force:      req.Force,
	}
	if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "metadata.batch-save", params); enqErr != nil {
		httputil.InternalError(c, "failed to enqueue operation", enqErr)
		return
	}

	httputil.RespondWithOK(c, gin.H{
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

	httputil.RespondWithOK(c, gin.H{
		"fields": fields,
	})
}

// ratingPatchRequest is the JSON body for PATCH /api/v1/audiobooks/:id/rating.
// Each field is a json.RawMessage so the handler can distinguish null (clear)
// from absent (don't touch) from a numeric value.
type ratingPatchRequest struct {
	Overall     json.RawMessage `json:"overall"`
	Story       json.RawMessage `json:"story"`
	Performance json.RawMessage `json:"performance"`
	Notes       json.RawMessage `json:"notes"`
}

// parseOptionalRating decodes a json.RawMessage into a *float64 and a clear flag.
// Returns (nil, false, nil) if raw is empty (field omitted).
// Returns (nil, true, nil) if raw is JSON null (clear).
// Returns (&v, false, nil) if raw is a valid number in [0,5] step 0.5.
// Returns (nil, false, err) on invalid value.
func parseOptionalRating(raw json.RawMessage, fieldName string) (*float64, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if string(raw) == "null" {
		return nil, true, nil
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false, fmt.Errorf("%s: must be a number", fieldName)
	}
	if v < 0 || v > 5 {
		return nil, false, fmt.Errorf("%s: must be between 0 and 5", fieldName)
	}
	// check 0.5 step: v*2 must be an integer
	if math.Round(v*2) != v*2 {
		return nil, false, fmt.Errorf("%s: must be a multiple of 0.5", fieldName)
	}
	return &v, false, nil
}

// handleUpdateBookRating handles PATCH /api/v1/audiobooks/:id/rating.
func (s *Server) handleUpdateBookRating(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "missing book id")
		return
	}

	var body ratingPatchRequest
	// Allow empty body (no changes)
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			httputil.RespondWithBadRequest(c, err.Error())
			return
		}
	}

	req := database.UpdateBookRatingRequest{}

	if overall, clear, err := parseOptionalRating(body.Overall, "overall"); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	} else {
		req.Overall = overall
		req.ClearOverall = clear
	}

	if story, clear, err := parseOptionalRating(body.Story, "story"); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	} else {
		req.Story = story
		req.ClearStory = clear
	}

	if perf, clear, err := parseOptionalRating(body.Performance, "performance"); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	} else {
		req.Performance = perf
		req.ClearPerf = clear
	}

	// Notes: null clears, string sets, absent leaves alone
	if len(body.Notes) > 0 {
		if string(body.Notes) == "null" {
			req.ClearNotes = true
		} else {
			var notes string
			if err := json.Unmarshal(body.Notes, &notes); err != nil {
				httputil.RespondWithBadRequest(c, "notes: must be a string or null")
				return
			}
			req.Notes = &notes
		}
	}

	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if err := store.UpdateBookRating(id, req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithError(c, http.StatusNotFound, "book not found", "NOT_FOUND")
			return
		}
		httputil.InternalError(c, "UpdateBookRating failed", err)
		return
	}

	// Return the updated book
	book, err := store.GetBookByID(id)
	if err != nil || book == nil {
		if errors.Is(err, nil) && book == nil {
			httputil.RespondWithError(c, http.StatusNotFound, "book not found", "NOT_FOUND")
			return
		}
		httputil.InternalError(c, "GetBook after rating update failed", err)
		return
	}
	httputil.RespondWithOK(c, book)
}
