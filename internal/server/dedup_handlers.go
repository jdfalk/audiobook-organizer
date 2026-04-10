// file: internal/server/dedup_handlers.go
// version: 1.2.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// listDedupCandidates handles GET /api/v1/dedup/candidates.
//
// Query params: entity_type, status, layer, min_similarity (float), limit (int, default 50), offset (int).
func (s *Server) listDedupCandidates(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	filter := database.CandidateFilter{Limit: 50}

	if v := c.Query("entity_type"); v != "" {
		filter.EntityType = v
	}
	if v := c.Query("status"); v != "" {
		filter.Status = v
	}
	if v := c.Query("layer"); v != "" {
		filter.Layer = v
	}
	if v := c.Query("min_similarity"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_similarity"})
			return
		}
		filter.MinSimilarity = &f
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		filter.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		filter.Offset = n
	}

	candidates, total, err := s.embeddingStore.ListCandidates(filter)
	if err != nil {
		internalError(c, "failed to list dedup candidates", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"candidates": candidates,
		"total":      total,
	})
}

// getDedupStats handles GET /api/v1/dedup/stats.
func (s *Server) getDedupStats(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	stats, err := s.embeddingStore.GetCandidateStats()
	if err != nil {
		internalError(c, "failed to get dedup stats", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// bulkMergeDedupCandidates handles POST /api/v1/dedup/candidates/bulk-merge.
//
// Accepts the same filter params as listDedupCandidates in the JSON body
// (entity_type, status, layer, min_similarity, max_similarity) and merges
// every matching candidate by calling MergeService.MergeBooks. Returns a
// summary with counts of attempted, merged, and failed candidates.
//
// The endpoint is intended for the "Merge Filtered" bulk action in the
// Embedding Dedup UI. It only operates on book candidates; author
// candidates are skipped (and counted as failed with a reason) since
// they're merged through a different service.
//
// Safety: caller should confirm with the user before invoking, because
// this is destructive and irreversible.
func (s *Server) bulkMergeDedupCandidates(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}
	if s.mergeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "merge service not available"})
		return
	}

	var body struct {
		EntityType    string   `json:"entity_type"`
		Status        string   `json:"status"`
		Layer         string   `json:"layer"`
		MinSimilarity *float64 `json:"min_similarity"`
		MaxSimilarity *float64 `json:"max_similarity"`
	}
	_ = c.ShouldBindJSON(&body)

	// Default to pending status if caller did not set one. Merging already-
	// merged or already-dismissed rows makes no sense.
	if body.Status == "" {
		body.Status = "pending"
	}
	// Only book candidates are mergeable through this endpoint.
	if body.EntityType == "" {
		body.EntityType = "book"
	}
	if body.EntityType != "book" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bulk merge only supports entity_type=book"})
		return
	}

	filter := database.CandidateFilter{
		EntityType:    body.EntityType,
		Status:        body.Status,
		Layer:         body.Layer,
		MinSimilarity: body.MinSimilarity,
		MaxSimilarity: body.MaxSimilarity,
		Limit:         100000,
	}

	candidates, total, err := s.embeddingStore.ListCandidates(filter)
	if err != nil {
		internalError(c, "failed to list candidates for bulk merge", err)
		return
	}

	type failure struct {
		CandidateID int64  `json:"candidate_id"`
		Reason      string `json:"reason"`
	}
	var failures []failure
	merged := 0

	for _, cand := range candidates {
		_, mergeErr := s.mergeService.MergeBooks([]string{cand.EntityAID, cand.EntityBID}, "")
		if mergeErr != nil {
			failures = append(failures, failure{CandidateID: cand.ID, Reason: mergeErr.Error()})
			log.Printf("[dedup] bulk merge candidate %d failed: %v", cand.ID, mergeErr)
			continue
		}
		if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "merged"); err != nil {
			// The books were merged on the server side, but we couldn't
			// update the candidate row — log it and count as merged
			// since the destructive action already happened.
			log.Printf("[dedup] bulk merge candidate %d merged but status update failed: %v", cand.ID, err)
		}
		merged++
	}

	log.Printf("[dedup] bulk merge complete: %d merged, %d failed out of %d matched",
		merged, len(failures), total)

	c.JSON(http.StatusOK, gin.H{
		"attempted": total,
		"merged":    merged,
		"failed":    len(failures),
		"failures":  failures,
	})
}

// mergeDedupCandidate handles POST /api/v1/dedup/candidates/:id/merge.
func (s *Server) mergeDedupCandidate(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid candidate id"})
		return
	}

	candidate, err := s.embeddingStore.GetCandidateByID(id)
	if err != nil {
		internalError(c, "failed to get candidate", err)
		return
	}
	if candidate == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "candidate not found"})
		return
	}

	var result interface{}
	if candidate.EntityType == "book" && s.mergeService != nil {
		mergeResult, mergeErr := s.mergeService.MergeBooks([]string{candidate.EntityAID, candidate.EntityBID}, "")
		if mergeErr != nil {
			internalError(c, "failed to merge books", mergeErr)
			return
		}
		result = mergeResult
	}

	if err := s.embeddingStore.UpdateCandidateStatus(id, "merged"); err != nil {
		internalError(c, "failed to update candidate status", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "merged", "result": result})
}

// dismissDedupCandidate handles POST /api/v1/dedup/candidates/:id/dismiss.
func (s *Server) dismissDedupCandidate(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid candidate id"})
		return
	}

	if err := s.embeddingStore.UpdateCandidateStatus(id, "dismissed"); err != nil {
		internalError(c, "failed to dismiss candidate", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "dismissed"})
}

// triggerDedupScan handles POST /api/v1/dedup/scan.
// Starts a background full embedding-based dedup scan. Before scanning,
// any stale candidates (non-primary versions, same-group pairs, orphaned
// book IDs) are purged so the scan starts from a clean slate.
func (s *Server) triggerDedupScan(c *gin.Context) {
	if s.dedupEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dedup engine not available"})
		return
	}

	go func() {
		ctx := context.Background()
		if deleted, err := s.dedupEngine.PurgeStaleCandidates(ctx); err != nil {
			log.Printf("[dedup] purge stale candidates error: %v", err)
		} else if deleted > 0 {
			log.Printf("[dedup] purged %d stale candidate(s) before scan", deleted)
		}
		if err := s.dedupEngine.FullScan(ctx, func(done, total int) {
			log.Printf("[dedup] scan progress: %d/%d", done, total)
		}); err != nil {
			log.Printf("[dedup] FullScan error: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

// triggerDedupLLM handles POST /api/v1/dedup/scan-llm.
// Starts a background LLM review pass over existing candidates.
func (s *Server) triggerDedupLLM(c *gin.Context) {
	if s.dedupEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dedup engine not available"})
		return
	}

	go func() {
		ctx := context.Background()
		if err := s.dedupEngine.RunLLMReview(ctx); err != nil {
			log.Printf("[dedup] RunLLMReview error: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

// triggerDedupRefresh handles POST /api/v1/dedup/refresh.
// Re-runs the full scan (re-embeds stale entries then scans for candidates).
// Purges stale candidates first so the refresh starts clean.
func (s *Server) triggerDedupRefresh(c *gin.Context) {
	if s.dedupEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dedup engine not available"})
		return
	}

	go func() {
		ctx := context.Background()
		if deleted, err := s.dedupEngine.PurgeStaleCandidates(ctx); err != nil {
			log.Printf("[dedup] purge stale candidates error: %v", err)
		} else if deleted > 0 {
			log.Printf("[dedup] purged %d stale candidate(s) before refresh", deleted)
		}
		if err := s.dedupEngine.FullScan(ctx, func(done, total int) {
			log.Printf("[dedup] refresh progress: %d/%d", done, total)
		}); err != nil {
			log.Printf("[dedup] refresh FullScan error: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "started"})
}
