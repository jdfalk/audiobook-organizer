// file: internal/server/metadata_batch_candidates.go
// version: 1.1.0
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
	"time"

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
	// Language is the book's current language as stored on the
	// Book row (ISO code or full name, whatever was last applied).
	// Used by the review dialog's language filter to hide
	// candidates whose language disagrees with the book's — the
	// motivating Spanish/English "Ancillary Sword" screenshot fix.
	// Empty when the book has no language set, in which case the
	// filter is a no-op for that row.
	Language string `json:"language,omitempty"`
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

	// Exclude books already in an active metadata fetch to avoid duplicate API calls
	activeOps, _ := store.GetRecentOperations(50)
	alreadyFetching := make(map[string]bool)
	for _, op := range activeOps {
		if op.Type != "metadata_candidate_fetch" {
			continue
		}
		if op.Status != "pending" && op.Status != "running" && op.Status != "queued" {
			continue
		}
		params, err := store.GetOperationParams(op.ID)
		if err != nil || len(params) == 0 {
			continue
		}
		var ids []string
		if err := json.Unmarshal(params, &ids); err == nil {
			for _, id := range ids {
				alreadyFetching[id] = true
			}
		}
	}

	var bookIDs []string
	var skippedCount int
	for _, id := range req.BookIDs {
		if alreadyFetching[id] {
			skippedCount++
		} else {
			bookIDs = append(bookIDs, id)
		}
	}

	if len(bookIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":      fmt.Sprintf("All %d books are already being fetched in another operation", skippedCount),
			"operation_id": "",
			"book_count":   0,
			"skipped":      skippedCount,
		})
		return
	}

	opID := ulid.Make().String()
	_, err := store.CreateOperation(opID, "metadata_candidate_fetch", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	totalBooks := len(bookIDs)

	// Save book IDs as operation params for recovery on restart
	if paramsJSON, err := json.Marshal(bookIDs); err == nil {
		_ = store.SaveOperationParams(opID, paramsJSON)
	}

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

	// Load previously rejected candidates for this book (across all operations)
	// and filter them out so we pick the next best match.
	rejectedKeys := loadRejectedCandidateKeys(store, bookID)
	var filtered []MetadataCandidate
	for _, c := range resp.Results {
		key := c.Source + "|" + c.Title
		if !rejectedKeys[key] {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return CandidateResult{
			Book:   bookInfo,
			Status: "no_match",
			Error:  "all candidates previously rejected",
		}
	}

	// Pick the top-scoring non-rejected candidate.
	topCandidate := filtered[0]
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
	if book.Language != nil {
		info.Language = *book.Language
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

// handleListMetadataFetchOperations returns recent completed
// metadata_candidate_fetch operations that have persisted results.
//
// Returns up to the last 10 operations where:
//   - type = metadata_candidate_fetch
//   - status = completed
//   - at least one persisted result row exists
//
// The frontend Resume Review dialog displays these so the user can
// pick which fetch to review. Without this, firing two fetches
// back-to-back without reviewing the first leaves the first's
// results invisible in the UI — the operation id is only held in
// React state, and only the latest gets tracked.
//
// 10 is a soft cap chosen as "enough to cover back-to-back fetches
// plus a review backlog without overwhelming the dialog".
func (s *Server) handleGetLatestMetadataFetch(c *gin.Context) {
	const maxOps = 10
	store := database.GlobalStore
	// Scan more than maxOps from recent history because the filter
	// (type + completed + non-empty results) can reject many rows.
	ops, err := store.GetRecentOperations(200)
	if err != nil {
		internalError(c, "failed to list recent operations", err)
		return
	}
	type fetchOpSummary struct {
		ID           string    `json:"id"`
		Type         string    `json:"type"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"created_at"`
		CompletedAt  time.Time `json:"completed_at,omitempty"`
		ResultCount  int       `json:"result_count"`
		MatchedCount int       `json:"matched_count"`
		NoMatchCount int       `json:"no_match_count"`
		ErrorCount   int       `json:"error_count"`
	}
	var out []fetchOpSummary
	for _, op := range ops {
		if len(out) >= maxOps {
			break
		}
		if op.Type != "metadata_candidate_fetch" {
			continue
		}
		if op.Status != "completed" {
			continue
		}
		results, err := store.GetOperationResults(op.ID)
		if err != nil {
			log.Printf("[WARN] list-metadata-fetches: get results for %s: %v", op.ID, err)
			continue
		}
		if len(results) == 0 {
			continue
		}
		var matched, noMatch, errCount int
		for _, r := range results {
			switch r.Status {
			case "matched":
				matched++
			case "no_match":
				noMatch++
			case "error":
				errCount++
			}
		}
		summary := fetchOpSummary{
			ID:           op.ID,
			Type:         op.Type,
			Status:       op.Status,
			CreatedAt:    op.CreatedAt,
			ResultCount:  len(results),
			MatchedCount: matched,
			NoMatchCount: noMatch,
			ErrorCount:   errCount,
		}
		if op.CompletedAt != nil {
			summary.CompletedAt = *op.CompletedAt
		}
		out = append(out, summary)
	}
	c.JSON(http.StatusOK, gin.H{"operations": out, "count": len(out)})
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
		if pool := GetGlobalFileIOPool(); pool != nil {
			bid := bookID
			pool.Submit(bid, func() {
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

// loadRejectedCandidateKeys finds previously rejected candidates for a book.
// Uses a dedicated rejection key prefix for fast lookup instead of scanning
// all operation results.
func loadRejectedCandidateKeys(store database.Store, bookID string) map[string]bool {
	keys := make(map[string]bool)
	// Scan only rejection keys for this specific book
	pairs, err := store.ScanPrefix(fmt.Sprintf("rejected_candidate:%s:", bookID))
	if err != nil {
		return keys
	}
	for _, kv := range pairs {
		// Key format: rejected_candidate:{bookID}:{source}|{title}
		// Value is just "1" — we only need the key
		keyStr := string(kv.Key)
		prefix := fmt.Sprintf("rejected_candidate:%s:", bookID)
		if len(keyStr) > len(prefix) {
			keys[keyStr[len(prefix):]] = true
		}
	}
	return keys
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

		// Store a fast-lookup rejection key for the batch fetch dedup
		if cr.Candidate != nil {
			rejectKey := fmt.Sprintf("rejected_candidate:%s:%s|%s", r.BookID, cr.Candidate.Source, cr.Candidate.Title)
			_ = store.SetRaw(rejectKey, []byte("1"))
		}
		rejected++
	}

	c.JSON(http.StatusOK, gin.H{"rejected": rejected})
}

// handleUnrejectCandidates reverses a rejection — restores the candidate to "matched" status
// and removes the fast-lookup rejection key so it can be fetched again.
func (s *Server) handleUnrejectCandidates(c *gin.Context) {
	var req struct {
		OperationID string   `json:"operation_id"`
		BookIDs     []string `json:"book_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.GlobalStore

	results, err := store.GetOperationResults(req.OperationID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	unrejectSet := make(map[string]bool, len(req.BookIDs))
	for _, id := range req.BookIDs {
		unrejectSet[id] = true
	}

	unrejected := 0
	for _, r := range results {
		if !unrejectSet[r.BookID] {
			continue
		}
		var cr CandidateResult
		if err := json.Unmarshal([]byte(r.ResultJSON), &cr); err != nil {
			continue
		}
		if cr.Status != "rejected" {
			continue
		}
		cr.Status = "matched"
		updatedJSON, _ := json.Marshal(cr)

		_ = store.CreateOperationResult(&database.OperationResult{
			OperationID: req.OperationID,
			BookID:      r.BookID,
			ResultJSON:  string(updatedJSON),
			Status:      "matched",
		})

		// Remove the fast-lookup rejection key
		if cr.Candidate != nil {
			rejectKey := fmt.Sprintf("rejected_candidate:%s:%s|%s", r.BookID, cr.Candidate.Source, cr.Candidate.Title)
			_ = store.DeleteRaw(rejectKey)
		}
		unrejected++
	}

	c.JSON(http.StatusOK, gin.H{"unrejected": unrejected})
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

// resumeInterruptedMetadataFetch checks for metadata_candidate_fetch operations
// that were interrupted (status=running) and re-enqueues the remaining books.
func (s *Server) resumeInterruptedMetadataFetch() {
	store := database.GlobalStore
	if store == nil {
		return
	}

	interrupted, err := store.GetInterruptedOperations()
	if err != nil {
		return
	}

	for _, op := range interrupted {
		if op.Type != "metadata_candidate_fetch" {
			continue
		}

		// Load the original book IDs from saved params
		paramsJSON, err := store.GetOperationParams(op.ID)
		if err != nil || len(paramsJSON) == 0 {
			log.Printf("[WARN] no saved params for interrupted metadata fetch %s, marking failed", op.ID)
			_ = store.UpdateOperationStatus(op.ID, "failed", op.Progress, op.Total, "interrupted, no params to resume")
			continue
		}

		var allBookIDs []string
		if err := json.Unmarshal(paramsJSON, &allBookIDs); err != nil {
			log.Printf("[WARN] invalid params for interrupted metadata fetch %s: %v", op.ID, err)
			_ = store.UpdateOperationStatus(op.ID, "failed", op.Progress, op.Total, "interrupted, invalid params")
			continue
		}

		// Find which books already have results
		existingResults, _ := store.GetOperationResults(op.ID)
		completed := make(map[string]bool, len(existingResults))
		for _, r := range existingResults {
			completed[r.BookID] = true
		}

		// Filter to remaining books
		var remaining []string
		for _, id := range allBookIDs {
			if !completed[id] {
				remaining = append(remaining, id)
			}
		}

		if len(remaining) == 0 {
			log.Printf("[INFO] interrupted metadata fetch %s has all results, marking completed", op.ID)
			_ = store.UpdateOperationStatus(op.ID, "completed", len(allBookIDs), len(allBookIDs), "recovered — all books fetched")
			continue
		}

		log.Printf("[INFO] resuming metadata fetch %s: %d/%d books remaining", op.ID, len(remaining), len(allBookIDs))

		// Re-enqueue the remaining books as a continuation of the same operation
		opID := op.ID
		totalBooks := len(allBookIDs)
		alreadyDone := len(allBookIDs) - len(remaining)
		mfs := s.metadataFetchService

		opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
			_ = progress.UpdateProgress(alreadyDone, totalBooks, fmt.Sprintf("resuming: %d/%d already fetched", alreadyDone, totalBooks))

			limiter := rate.NewLimiter(rate.Limit(10), 1)
			var completed int64 = int64(alreadyDone)
			const numWorkers = 8
			ch := make(chan string, len(remaining))
			for _, id := range remaining {
				ch <- id
			}
			close(ch)

			var wg sync.WaitGroup
			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for bookID := range ch {
						if ctx.Err() != nil {
							return
						}
						_ = limiter.Wait(ctx)
						result := s.fetchCandidateForBook(ctx, mfs, store, limiter, opID, bookID)
						resultJSON, _ := json.Marshal(result)
						_ = store.CreateOperationResult(&database.OperationResult{
							OperationID: opID,
							BookID:      bookID,
							ResultJSON:  string(resultJSON),
							Status:      result.Status,
						})
						done := atomic.AddInt64(&completed, 1)
						_ = progress.UpdateProgress(int(done), totalBooks, fmt.Sprintf("fetched %d/%d", done, totalBooks))
					}
				}()
			}
			wg.Wait()
			return nil
		}

		if err := operations.GlobalQueue.Enqueue(opID, "metadata_candidate_fetch", operations.PriorityNormal, opFunc); err != nil {
			log.Printf("[WARN] failed to re-enqueue metadata fetch %s: %v", opID, err)
			_ = store.UpdateOperationStatus(opID, "failed", alreadyDone, totalBooks, "failed to resume")
		}
	}
}
