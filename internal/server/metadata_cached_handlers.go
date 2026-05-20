// file: internal/server/metadata_cached_handlers.go
// version: 1.2.0
//
// METADATA-CACHED-MATCHER: handlers for the persistent metadata-cache
// query surface (Task 8). Adds GET /audiobooks/metadata/cached, the
// list endpoint that powers the Review popup. Each entry is augmented
// with the per-book review-status so the UI can split into
// pending/matched without a second round-trip.
//
// Task 12 additions: cache/review (full CandidateResult list sourced
// from cache), batch-apply-cached, and clear-no-match.

package server

import (
	"encoding/json"
	"log/slog"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/metabatch"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
)

// listCachedCandidates handles GET /api/v1/audiobooks/metadata/cached.
//
// Query params:
//   - status=pending  → only books whose MetadataReviewStatus is empty/null
//   - status=matched  → only books with status == "matched"
//   - (omitted)       → all cached entries
//
// Response is { entries: [...], total: N }. Each entry carries the
// book_id, fetched_at, candidate_count, fresh flag, plus title +
// review_status pulled from the book row so the UI can render without
// a second fetch.
func (s *Server) listCachedCandidates(c *gin.Context) {
	if s.Store() == nil || s.metadataFetchService == nil {
		httputil.RespondWithInternalError(c, "metadata service not initialized")
		return
	}

	summaries, err := s.metadataFetchService.ListCachedSummaries(c.Request.Context())
	if err != nil {
		httputil.InternalError(c, "failed to list metadata cache", err)
		return
	}

	statusFilter := c.Query("status")
	freshCutoff := time.Now().Add(-database.MetadataCacheTTL)

	out := make([]gin.H, 0, len(summaries))
	for _, sum := range summaries {
		book, err := s.Store().GetBookByID(sum.BookID)
		if err != nil || book == nil {
			continue
		}
		var reviewStatus string
		if book.MetadataReviewStatus != nil {
			reviewStatus = *book.MetadataReviewStatus
		}
		switch statusFilter {
		case "pending":
			if reviewStatus != "" && reviewStatus != "pending" {
				continue
			}
		case "matched":
			if reviewStatus != "matched" {
				continue
			}
		}
		out = append(out, gin.H{
			"book_id":         sum.BookID,
			"fetched_at":      sum.FetchedAt,
			"candidate_count": sum.CandidateCount,
			"is_fresh":        sum.FetchedAt.After(freshCutoff),
			"title":           book.Title,
			"review_status":   reviewStatus,
		})
	}

	httputil.RespondWithOK(c, gin.H{"entries": out, "total": len(out)})
}

// getCacheReviewResults handles GET /api/v1/audiobooks/metadata/cache/review.
//
// Returns a paginated list of CandidateResult items sourced entirely from the
// persistent metadata cache. The shape matches getOperationResults so
// MetadataReviewDialog can consume it without changing its render logic.
//
// Status semantics in the response:
//   - "matched"  — candidate found, book is pending user review
//   - "no_match" — user previously rejected (MetadataReviewStatus="no_match")
//   - "applied"  — metadata was already applied (MetadataReviewStatus="matched")
func (s *Server) getCacheReviewResults(c *gin.Context) {
	if s.Store() == nil || s.metadataFetchService == nil {
		httputil.RespondWithInternalError(c, "metadata service not initialized")
		return
	}
	// Parse limit/offset directly rather than via ParsePaginationParams —
	// the review dialog needs to request the full set in one call
	// (limit=0 means "no cap"), which the shared parser clamps to ≤500.
	limit := httputil.ParseQueryInt(c, "limit", 0)
	offset := httputil.ParseQueryInt(c, "offset", 0)
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}

	summaries, err := s.metadataFetchService.ListCachedSummaries(c.Request.Context())
	if err != nil {
		httputil.InternalError(c, "failed to list metadata cache", err)
		return
	}

	total := len(summaries)

	// Pre-resolve each summary's reviewStatus so we can sort matched-first
	// before slicing. Without this the client sees pending matched rows
	// scattered across pages in cache-insertion order.
	type entryWithStatus struct {
		sum    metafetch.MetadataCacheSummary
		status string // "matched" | "no_match" | "applied"
	}
	prepared := make([]entryWithStatus, 0, total)
	for _, sum := range summaries {
		book, err := s.Store().GetBookByID(sum.BookID)
		if err != nil || book == nil {
			continue
		}
		st := "matched"
		if book.MetadataReviewStatus != nil {
			switch *book.MetadataReviewStatus {
			case "no_match":
				st = "no_match"
			case "matched":
				st = "applied"
			}
		}
		prepared = append(prepared, entryWithStatus{sum: sum, status: st})
	}
	// Stable sort: matched (pending review) first, then no_match, then applied.
	statusRank := map[string]int{"matched": 0, "no_match": 1, "applied": 2}
	sort.SliceStable(prepared, func(i, j int) bool {
		return statusRank[prepared[i].status] < statusRank[prepared[j].status]
	})

	// limit=0 means "return all rows" — the client now paginates locally.
	start := offset
	if start > len(prepared) {
		start = len(prepared)
	}
	end := len(prepared)
	if limit > 0 {
		end = start + limit
		if end > len(prepared) {
			end = len(prepared)
		}
	}
	page := prepared[start:end]

	// Global counts over the full prepared set so the UI can show accurate
	// totals even when paginating a subset.
	var matched, noMatch, applied int
	for _, p := range prepared {
		switch p.status {
		case "no_match":
			noMatch++
		case "applied":
			applied++
		default:
			matched++
		}
	}

	results := make([]CandidateResult, 0, len(page))
	for _, p := range page {
		sum := p.sum
		book, err := s.Store().GetBookByID(sum.BookID)
		if err != nil || book == nil {
			continue
		}
		entry, _, err := s.metadataFetchService.GetCachedCandidates(sum.BookID)
		if err != nil || entry == nil || len(entry.Candidates) == 0 {
			continue
		}
		var cand metafetch.MetadataCandidate
		if err := json.Unmarshal(entry.Candidates[0], &cand); err != nil {
			slog.Warn("getCacheReviewResults decode candidate for", "sum", sum.BookID, "err", err)
			continue
		}

		results = append(results, CandidateResult{
			Book:      metabatch.BuildCandidateBookInfo(s.Store(), book),
			Candidate: &cand,
			Status:    p.status,
		})
	}

	httputil.RespondWithOK(c, gin.H{
		"results":       results,
		"total_count":   total,
		"matched":       matched,
		"no_match":      noMatch,
		"errors":        0,
		"total_applied": applied,
	})
}

// batchApplyFromCache handles POST /api/v1/audiobooks/metadata/batch-apply-cached.
//
// Applies the highest-scored cached candidate for each book_id in the request.
// Equivalent to the legacy batchApplyCandidates but reads from the persistent
// cache rather than an operation's result rows.
func (s *Server) batchApplyFromCache(c *gin.Context) {
	if s.Store() == nil || s.metadataFetchService == nil {
		httputil.RespondWithInternalError(c, "metadata service not initialized")
		return
	}
	var body struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}

	applied := 0
	for _, bookID := range body.BookIDs {
		entry, _, err := s.metadataFetchService.GetCachedCandidates(bookID)
		if err != nil || entry == nil || len(entry.Candidates) == 0 {
			slog.Warn("batchApplyFromCache no cached candidates for", "bookID", bookID)
			continue
		}
		var cand metafetch.MetadataCandidate
		if err := json.Unmarshal(entry.Candidates[0], &cand); err != nil {
			slog.Warn("batchApplyFromCache decode candidate for", "bookID", bookID, "err", err)
			continue
		}
		if _, err := s.metadataFetchService.ApplyMetadataCandidate(bookID, cand, nil); err != nil {
			slog.Warn("batchApplyFromCache apply for", "bookID", bookID, "err", err)
			continue
		}
		_ = s.metadataFetchService.InvalidateCachedCandidates(bookID)
		if s.writeBackBatcher != nil {
			s.writeBackBatcher.Enqueue(bookID)
		}
		applied++
	}

	httputil.RespondWithOK(c, gin.H{"applied": applied})
}

// clearMetadataNoMatch handles POST /api/v1/audiobooks/:id/clear-no-match.
//
// Clears a book's MetadataReviewStatus back to null so it re-surfaces in the
// Review dialog. Inverse of mark-no-match. Does not create a rejection record.
func (s *Server) clearMetadataNoMatch(c *gin.Context) {
	id := c.Param("id")
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	book, err := store.GetBookByID(id)
	if err != nil || book == nil {
		httputil.RespondWithNotFound(c, "audiobook", id)
		return
	}
	book.MetadataReviewStatus = nil
	if _, err := store.UpdateBook(id, book); err != nil {
		httputil.InternalError(c, "failed to clear review status", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "Review status cleared"})
}
