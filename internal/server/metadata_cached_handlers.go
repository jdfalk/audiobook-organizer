// file: internal/server/metadata_cached_handlers.go
// version: 1.0.1
//
// METADATA-CACHED-MATCHER: handlers for the persistent metadata-cache
// query surface (Task 8). Adds GET /audiobooks/metadata/cached, the
// list endpoint that powers the Review popup. Each entry is augmented
// with the per-book review-status so the UI can split into
// pending/matched without a second round-trip.

package server

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
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
