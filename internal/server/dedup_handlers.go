// file: internal/server/dedup_handlers.go
// version: 1.6.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
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

// mergeDedupCluster handles POST /api/v1/dedup/candidates/merge-cluster.
//
// Body: {"book_ids": ["id1", "id2", "id3", ...]}
//
// Merges the supplied book IDs into a single version group with one call to
// MergeService.MergeBooks, then marks every dedup_candidate row whose pair
// is fully contained in the set as status=merged. This is the backend for
// the Embedding tab's multi-book cluster card, where 3+ candidate books form
// a connected component in the pairwise candidate graph and should be
// merged together as one logical group rather than one pairwise merge at a
// time (which would fight the version-group state mid-way).
func (s *Server) mergeDedupCluster(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}
	if s.mergeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "merge service not available"})
		return
	}

	var body struct {
		BookIDs       []string `json:"book_ids"`
		PrimaryBookID string   `json:"primary_book_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.BookIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must contain at least 2 entries"})
		return
	}
	// If primary_book_id is set, it must be one of the books in the
	// cluster. Empty means "let bookIsBetter auto-pick".
	if body.PrimaryBookID != "" {
		found := false
		for _, id := range body.BookIDs {
			if id == body.PrimaryBookID {
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": "primary_book_id must be one of book_ids"})
			return
		}
	}

	mergeResult, err := s.mergeService.MergeBooks(body.BookIDs, body.PrimaryBookID)
	if err != nil {
		internalError(c, "failed to merge books in cluster", err)
		return
	}

	// Mark every candidate whose pair is fully contained in the cluster
	// as merged. Using a set for O(1) membership so a cluster of N books
	// checks each row's pair in constant time.
	inCluster := make(map[string]struct{}, len(body.BookIDs))
	for _, id := range body.BookIDs {
		inCluster[id] = struct{}{}
	}
	candidates, _, listErr := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	updated := 0
	if listErr != nil {
		log.Printf("[dedup] cluster merge: list candidates failed: %v", listErr)
	} else {
		for _, cand := range candidates {
			_, aIn := inCluster[cand.EntityAID]
			_, bIn := inCluster[cand.EntityBID]
			if !aIn || !bIn {
				continue
			}
			if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "merged"); err != nil {
				log.Printf("[dedup] cluster merge: status update %d: %v", cand.ID, err)
				continue
			}
			updated++
		}
	}
	log.Printf("[dedup] cluster merge: merged %d books, marked %d candidate row(s) as merged",
		len(body.BookIDs), updated)

	c.JSON(http.StatusOK, gin.H{
		"status":              "merged",
		"merged_books":        len(body.BookIDs),
		"candidates_updated":  updated,
		"result":              mergeResult,
	})
}

// dismissDedupCluster handles POST /api/v1/dedup/candidates/dismiss-cluster.
//
// Body: {"book_ids": ["id1", "id2", ...]}
//
// Marks every dedup_candidate row whose pair is fully contained in the set
// as status=dismissed. No books are modified — this just removes the pair
// from the pending queue.
func (s *Server) dismissDedupCluster(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	var body struct {
		BookIDs []string `json:"book_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.BookIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must contain at least 2 entries"})
		return
	}

	inCluster := make(map[string]struct{}, len(body.BookIDs))
	for _, id := range body.BookIDs {
		inCluster[id] = struct{}{}
	}
	candidates, _, err := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		internalError(c, "failed to list candidates for cluster dismiss", err)
		return
	}
	dismissed := 0
	for _, cand := range candidates {
		_, aIn := inCluster[cand.EntityAID]
		_, bIn := inCluster[cand.EntityBID]
		if !aIn || !bIn {
			continue
		}
		if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "dismissed"); err != nil {
			log.Printf("[dedup] cluster dismiss: status update %d: %v", cand.ID, err)
			continue
		}
		dismissed++
	}
	log.Printf("[dedup] cluster dismiss: dismissed %d candidate row(s) across %d books",
		dismissed, len(body.BookIDs))

	c.JSON(http.StatusOK, gin.H{
		"status":    "dismissed",
		"dismissed": dismissed,
	})
}

// removeFromDedupCluster handles POST /api/v1/dedup/candidates/remove-from-cluster.
//
// Body: {"cluster_book_ids": [...], "remove_book_id": "X"}
//
// Dismisses every pending candidate whose pair is one-side-X and other-side
// in (cluster \ X). In other words: "this one book is NOT a duplicate of
// the other books in this cluster". Pairs involving X that point to books
// OUTSIDE the cluster are left alone — this is a scoped split, not a
// global ban on the book.
//
// The effect on the UI: a 3-way cluster (A, B, C) where the user removes
// C drops the (A,C) and (B,C) edges but leaves (A,B). On the next page
// load the union-find produces a 2-way cluster (A, B) and C disappears
// from the pending view. C can still show up in future dedup scans if
// something new hits it.
func (s *Server) removeFromDedupCluster(c *gin.Context) {
	if s.embeddingStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding store not available"})
		return
	}

	var body struct {
		ClusterBookIDs []string `json:"cluster_book_ids"`
		RemoveBookID   string   `json:"remove_book_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.RemoveBookID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "remove_book_id is required"})
		return
	}
	if len(body.ClusterBookIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_book_ids must contain at least 2 entries"})
		return
	}

	// Build the set of "other books in this cluster" — everything in the
	// cluster except the one being removed.
	others := make(map[string]struct{}, len(body.ClusterBookIDs))
	for _, id := range body.ClusterBookIDs {
		if id != body.RemoveBookID {
			others[id] = struct{}{}
		}
	}
	if len(others) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster must contain at least one book other than remove_book_id"})
		return
	}

	candidates, _, err := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		internalError(c, "failed to list candidates for cluster remove", err)
		return
	}

	dismissed := 0
	for _, cand := range candidates {
		// The pair must involve the removed book on one side and an
		// "other cluster member" on the opposite side. Pairs where the
		// removed book touches a book OUTSIDE the cluster are deliberately
		// skipped — those represent different clusters that the user
		// hasn't expressed an opinion on.
		var otherID string
		switch {
		case cand.EntityAID == body.RemoveBookID:
			otherID = cand.EntityBID
		case cand.EntityBID == body.RemoveBookID:
			otherID = cand.EntityAID
		default:
			continue
		}
		if _, ok := others[otherID]; !ok {
			continue
		}
		if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "dismissed"); err != nil {
			log.Printf("[dedup] remove-from-cluster: status update %d: %v", cand.ID, err)
			continue
		}
		dismissed++
	}
	log.Printf("[dedup] remove-from-cluster: dismissed %d edge(s) between %s and %d other cluster member(s)",
		dismissed, body.RemoveBookID, len(others))

	c.JSON(http.StatusOK, gin.H{
		"status":    "removed",
		"dismissed": dismissed,
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
// Runs a full embedding-based dedup scan as a tracked Operation so the
// UI can show progress and completion. Before scanning, stale candidates
// (non-primary versions, same-group pairs, orphaned book IDs) are purged.
//
// Previously this fired a fire-and-forget goroutine and returned
// {"status": "started"} with no way for the UI to see progress or
// completion. Now the operation shows up in the Operations panel with
// live progress updates and final-completion messages.
func (s *Server) triggerDedupScan(c *gin.Context) {
	if s.dedupEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dedup engine not available"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create dedup-scan operation", err)
		return
	}

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Purging stale candidates...")
		if deleted, err := s.dedupEngine.PurgeStaleCandidates(ctx); err != nil {
			log.Printf("[dedup] purge stale candidates error: %v", err)
		} else if deleted > 0 {
			log.Printf("[dedup] purged %d stale candidate(s) before scan", deleted)
		}

		// FullScan reports progress via a callback (done, total). Translate
		// that into ProgressReporter updates, reserving 5% at the start for
		// the purge pass and 5% at the end for the "complete" line so the
		// bar actually moves all the way to 100.
		var lastPct int
		fullScanErr := s.dedupEngine.FullScan(ctx, func(done, total int) {
			if total <= 0 {
				return
			}
			pct := 5 + (90 * done / total)
			if pct == lastPct {
				return
			}
			lastPct = pct
			_ = progress.UpdateProgress(pct, 100, fmt.Sprintf("Scanning books: %d / %d", done, total))
		})
		if fullScanErr != nil {
			log.Printf("[dedup] FullScan error: %v", fullScanErr)
			return fmt.Errorf("dedup scan: %w", fullScanErr)
		}

		// Fetch final candidate counts for the completion message.
		pendingCount := 0
		if s.embeddingStore != nil {
			filter := database.CandidateFilter{EntityType: "book", Status: "pending", Limit: 1}
			if _, total, listErr := s.embeddingStore.ListCandidates(filter); listErr == nil {
				pendingCount = total
			}
		}
		_ = progress.UpdateProgress(100, 100,
			fmt.Sprintf("Dedup scan complete — %d pending candidates", pendingCount))
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "dedup-scan", operations.PriorityLow, opFunc); err != nil {
		internalError(c, "failed to enqueue dedup scan", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// triggerDedupLLM handles POST /api/v1/dedup/scan-llm.
// Runs an LLM review pass over ambiguous embedding-layer candidates as a
// tracked Operation so the UI can display progress and completion.
func (s *Server) triggerDedupLLM(c *gin.Context) {
	if s.dedupEngine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dedup engine not available"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "dedup-llm-review", nil)
	if err != nil {
		internalError(c, "failed to create dedup-llm-review operation", err)
		return
	}

	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Starting LLM review of ambiguous candidates...")
		if err := s.dedupEngine.RunLLMReview(ctx); err != nil {
			return fmt.Errorf("LLM review: %w", err)
		}
		_ = progress.UpdateProgress(100, 100, "LLM review complete")
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "dedup-llm-review", operations.PriorityLow, opFunc); err != nil {
		internalError(c, "failed to enqueue LLM review", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// triggerDedupRefresh handles POST /api/v1/dedup/refresh.
// Re-runs the full scan as a tracked Operation. Identical behavior to
// triggerDedupScan — kept as a separate endpoint for backwards compatibility.
func (s *Server) triggerDedupRefresh(c *gin.Context) {
	s.triggerDedupScan(c)
}
