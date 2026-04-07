// file: internal/server/metadata_batch_candidates.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-e1f2a3b4c5d6
// last-edited: 2026-04-05

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
	"golang.org/x/time/rate"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// CandidateBookInfo contains summary info about a book used in candidate results.
type CandidateBookInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	FilePath   string `json:"file_path"`
	ITunesPath string `json:"itunes_path,omitempty"`
	CoverURL   string `json:"cover_url,omitempty"`
	Format     string `json:"format,omitempty"`
	Duration   int    `json:"duration_seconds,omitempty"`
	FileSize   int64  `json:"file_size_bytes,omitempty"`
}

// CandidateResult holds the metadata candidate search result for a single book.
type CandidateResult struct {
	Book      CandidateBookInfo  `json:"book"`
	Candidate *MetadataCandidate `json:"candidate,omitempty"`
	Status    string             `json:"status"` // "matched", "no_match", "error"
	Error     string             `json:"error_message,omitempty"`
}

// batchFetchRequest is the JSON body for handleBatchFetchCandidates.
type batchFetchRequest struct {
	BookIDs []string `json:"book_ids" binding:"required"`
}

// batchApplyRequest is the JSON body for handleBatchApplyCandidates.
type batchApplyRequest struct {
	OperationID string   `json:"operation_id" binding:"required"`
	BookIDs     []string `json:"book_ids" binding:"required"`
}

// handleBatchFetchCandidates creates a background operation that spawns parallel
// workers to fetch metadata candidates for the given book IDs.
func (s *Server) handleBatchFetchCandidates(c *gin.Context) {
	var req batchFetchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	opID := ulid.Make().String()

	_, err := store.CreateOperation(opID, "metadata_candidate_fetch", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	totalBooks := len(req.BookIDs)
	bookIDs := make([]string, len(req.BookIDs))
	copy(bookIDs, req.BookIDs)

	mfs := s.metadataFetchService

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, totalBooks, "starting metadata candidate fetch")

		// Rate limiter: 10 requests per second globally across all workers.
		limiter := rate.NewLimiter(rate.Limit(10), 1)

		// Buffered channel for work distribution.
		workCh := make(chan string, len(bookIDs))
		for _, id := range bookIDs {
			workCh <- id
		}
		close(workCh)

		var completed int64
		var wg sync.WaitGroup

		numWorkers := 8
		if numWorkers > totalBooks {
			numWorkers = totalBooks
		}

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for bookID := range workCh {
					if ctx.Err() != nil {
						return
					}

					result := s.fetchCandidateForBook(ctx, mfs, store, limiter, opID, bookID)
					resultJSON, err := json.Marshal(result)
					if err != nil {
						log.Printf("[WARN] failed to marshal candidate result for book %s: %v", bookID, err)
						continue
					}

					opResult := &database.OperationResult{
						OperationID: opID,
						BookID:      bookID,
						ResultJSON:  string(resultJSON),
						Status:      result.Status,
					}
					if err := store.CreateOperationResult(opResult); err != nil {
						log.Printf("[WARN] failed to store candidate result for book %s: %v", bookID, err)
					}

					done := atomic.AddInt64(&completed, 1)
					_ = progress.UpdateProgress(int(done), totalBooks, fmt.Sprintf("fetched %d/%d", done, totalBooks))
				}
			}()
		}

		wg.Wait()

		finalCount := atomic.LoadInt64(&completed)
		_ = progress.UpdateProgress(int(finalCount), totalBooks, "completed")
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "metadata_candidate_fetch", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"total_books":  totalBooks,
		"message":      "metadata candidate fetch started",
	})
}

// fetchCandidateForBook fetches metadata candidates for a single book, respecting
// the rate limiter. Returns a CandidateResult.
func (s *Server) fetchCandidateForBook(
	ctx context.Context,
	mfs *MetadataFetchService,
	store database.Store,
	limiter *rate.Limiter,
	opID, bookID string,
) CandidateResult {
	book, err := store.GetBookByID(bookID)
	if err != nil || book == nil {
		return CandidateResult{
			Book:   CandidateBookInfo{ID: bookID},
			Status: "error",
			Error:  fmt.Sprintf("book not found: %v", err),
		}
	}

	bookInfo := buildCandidateBookInfo(book)

	// Wait for rate limiter before making external requests.
	if err := limiter.Wait(ctx); err != nil {
		return CandidateResult{
			Book:   bookInfo,
			Status: "error",
			Error:  fmt.Sprintf("rate limiter cancelled: %v", err),
		}
	}

	var authorHint []string
	if book.Author != nil && book.Author.Name != "" {
		authorHint = append(authorHint, book.Author.Name)
	}

	resp, err := mfs.SearchMetadataForBook(bookID, book.Title, authorHint...)
	if err != nil {
		return CandidateResult{
			Book:   bookInfo,
			Status: "error",
			Error:  fmt.Sprintf("search failed: %v", err),
		}
	}

	if len(resp.Results) == 0 {
		return CandidateResult{
			Book:   bookInfo,
			Status: "no_match",
		}
	}

	// Pick the top-scoring candidate.
	topCandidate := resp.Results[0]
	return CandidateResult{
		Book:      bookInfo,
		Candidate: &topCandidate,
		Status:    "matched",
	}
}

// buildCandidateBookInfo builds a CandidateBookInfo from a database.Book.
func buildCandidateBookInfo(book *database.Book) CandidateBookInfo {
	info := CandidateBookInfo{
		ID:       book.ID,
		Title:    book.Title,
		FilePath: book.FilePath,
		Format:   book.Format,
	}
	if book.Author != nil {
		info.Author = book.Author.Name
	}
	if book.ITunesPath != nil {
		info.ITunesPath = *book.ITunesPath
	}
	if book.CoverURL != nil {
		info.CoverURL = *book.CoverURL
	}
	if book.Duration != nil {
		info.Duration = *book.Duration
	}
	if book.FileSize != nil {
		info.FileSize = *book.FileSize
	}
	return info
}

// handleGetOperationResults returns the structured candidate results for an operation.
func (s *Server) handleGetOperationResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id is required"})
		return
	}

	store := database.GlobalStore

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	results, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to get operation results", err)
		return
	}

	var candidateResults []CandidateResult
	for _, r := range results {
		var cr CandidateResult
		if err := json.Unmarshal([]byte(r.ResultJSON), &cr); err != nil {
			log.Printf("[WARN] failed to unmarshal result for book %s in op %s: %v", r.BookID, opID, err)
			continue
		}
		candidateResults = append(candidateResults, cr)
	}

	c.JSON(http.StatusOK, gin.H{
		"operation":    op,
		"results":      candidateResults,
		"total":        len(candidateResults),
		"matched":      countByStatus(candidateResults, "matched"),
		"no_match":     countByStatus(candidateResults, "no_match"),
		"errors":       countByStatus(candidateResults, "error"),
	})
}

// countByStatus counts CandidateResults with the given status.
func countByStatus(results []CandidateResult, status string) int {
	n := 0
	for _, r := range results {
		if r.Status == status {
			n++
		}
	}
	return n
}

// handleBatchApplyCandidates applies stored metadata candidates for the selected books.
func (s *Server) handleBatchApplyCandidates(c *gin.Context) {
	var req batchApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation_id and book_ids are required"})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	mfs := s.metadataFetchService

	// Load all operation results for the given operation.
	results, err := store.GetOperationResults(req.OperationID)
	if err != nil {
		internalError(c, "failed to load operation results", err)
		return
	}

	// Index results by book ID for fast lookup.
	resultsByBook := make(map[string]database.OperationResult, len(results))
	for _, r := range results {
		resultsByBook[r.BookID] = r
	}

	applied := 0
	skipped := 0
	var errors []string

	for _, bookID := range req.BookIDs {
		opResult, ok := resultsByBook[bookID]
		if !ok {
			skipped++
			continue
		}

		var cr CandidateResult
		if err := json.Unmarshal([]byte(opResult.ResultJSON), &cr); err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to parse result", bookID))
			continue
		}
		if cr.Candidate == nil || cr.Status != "matched" {
			skipped++
			continue
		}

		candidate := *cr.Candidate
		_, err := mfs.ApplyMetadataCandidate(bookID, candidate, nil)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: apply failed: %v", bookID, err))
			continue
		}

		applied++

		// Queue file I/O through the worker pool (bounded concurrency).
		if GlobalFileIOPool != nil {
			bid := bookID
			GlobalFileIOPool.Submit(bid, func() {
				mfs.ApplyMetadataFileIO(bid)
				if _, err := mfs.WriteBackMetadataForBook(bid); err != nil {
					log.Printf("[WARN] write-back failed for %s: %v", bid, err)
				}
				if GlobalWriteBackBatcher != nil {
					GlobalWriteBackBatcher.Enqueue(bid)
				}
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"applied":      applied,
		"skipped":      skipped,
		"errors":       errors,
		"error_count":  len(errors),
		"operation_id": req.OperationID,
	})
}

// handleRejectCandidates stores rejected candidates so future fetches exclude them.
// The rejection is stored as an operation_result with status "rejected".
func (s *Server) handleRejectCandidates(c *gin.Context) {
	var req struct {
		OperationID string   `json:"operation_id" binding:"required"`
		BookIDs     []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.GlobalStore

	// For each book, update the stored result status to "rejected"
	results, err := store.GetOperationResults(req.OperationID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	rejectSet := make(map[string]bool, len(req.BookIDs))
	for _, id := range req.BookIDs {
		rejectSet[id] = true
	}

	rejected := 0
	for _, r := range results {
		if !rejectSet[r.BookID] {
			continue
		}
		// Update the result JSON to set status to rejected
		var cr CandidateResult
		if err := json.Unmarshal([]byte(r.ResultJSON), &cr); err != nil {
			continue
		}
		cr.Status = "rejected"
		updatedJSON, _ := json.Marshal(cr)

		// Store as a new result with rejected status (overwrites by key in PebbleDB)
		_ = store.CreateOperationResult(&database.OperationResult{
			OperationID: req.OperationID,
			BookID:      r.BookID,
			ResultJSON:  string(updatedJSON),
			Status:      "rejected",
		})
		rejected++
	}

	c.JSON(http.StatusOK, gin.H{"rejected": rejected})
}

// handleGetRecentOperations returns the last 10 completed metadata_candidate_fetch operations.
// Uses the server's listCache (10s TTL) to avoid expensive PebbleDB prefix scans on every poll.
func (s *Server) handleGetRecentOperations(c *gin.Context) {
	cacheKey := "recent_ops"
	if cached, ok := s.listCache.Get(cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	store := database.GlobalStore

	// Get recent operations and filter to metadata_candidate_fetch type.
	ops, err := store.GetRecentOperations(50)
	if err != nil {
		internalError(c, "failed to get recent operations", err)
		return
	}

	var filtered []database.Operation
	for _, op := range ops {
		if op.Type == "metadata_candidate_fetch" && len(filtered) < 10 {
			filtered = append(filtered, op)
		}
	}

	resp := gin.H{
		"operations": filtered,
		"count":      len(filtered),
	}
	s.listCache.Set(cacheKey, resp)
	c.JSON(http.StatusOK, resp)
}
