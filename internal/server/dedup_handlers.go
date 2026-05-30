// file: internal/server/dedup_handlers.go
// version: 2.10.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-29

package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
)

// listDedupCandidates handles GET /api/v1/dedup/candidates.
//
// Query params: entity_type, status, layer, min_similarity (float), limit (int, default 50), offset (int).
func (s *Server) listDedupCandidates(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
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
			httputil.RespondWithBadRequest(c, "invalid min_similarity")
			return
		}
		filter.MinSimilarity = &f
	}
	p := httputil.ParsePaginationParams(c)
	limit, offset := p.Limit, p.Offset
	filter.Limit = limit
	filter.Offset = offset

	candidates, total, err := s.embeddingStore.ListCandidates(filter)
	if err != nil {
		httputil.InternalError(c, "failed to list dedup candidates", err)
		return
	}

	// MAYDEPLOY-B4: defensive filter — drop candidate rows whose
	// referenced book IDs no longer exist in the book table. This is
	// the safety net to B3's proactive cleanup: even if a stale row
	// slips through (race, missed delete, crash between cleanup runs),
	// the UI never shows a candidate that would 404 when clicked.
	// Non-book entities (e.g. author) skip the existence check.
	filtered := candidates[:0]
	existCache := make(map[string]bool, len(candidates)*2)
	bookExists := func(id string) bool {
		if id == "" {
			return false
		}
		if v, ok := existCache[id]; ok {
			return v
		}
		book, gerr := s.Store().GetBookByID(id)
		exists := gerr == nil && book != nil
		existCache[id] = exists
		return exists
	}
	dropped := 0
	for _, cand := range candidates {
		if cand.EntityType == "book" {
			if !bookExists(cand.EntityAID) || !bookExists(cand.EntityBID) {
				dropped++
				continue
			}
		}
		filtered = append(filtered, cand)
	}
	if dropped > 0 {
		slog.Warn("dedup.list_candidates: filtered dead-book candidate rows",
			"dropped", dropped,
			"returned", len(filtered),
			"page_size", len(candidates),
			"note", "B3 cleanup may be lagging")
		// Reflect the filtered count in the total so pagination
		// hints stay roughly accurate from the client's view.
		if total >= dropped {
			total -= dropped
		}
	}

	httputil.RespondWithOK(c, gin.H{
		"candidates": filtered,
		"total":      total,
	})
}

// exportDedupCandidates handles GET /api/v1/dedup/candidates/export.
//
// Query params:
//
//	format = "csv" (default) or "json"
//	status, layer, min_similarity, entity_type — same as list endpoint
//
// Unlike the list endpoint, export doesn't paginate — it walks every
// matching row up to an internal hard cap (100K) to prevent runaway
// downloads. Each row is enriched with the book titles and author names
// of both sides so the CSV is readable in a spreadsheet without needing
// to cross-reference IDs.
//
// Columns (CSV): candidate_id, status, layer, similarity,
// entity_a_id, entity_a_title, entity_a_author,
// entity_b_id, entity_b_title, entity_b_author,
// llm_verdict, llm_reason, created_at, updated_at.
func (s *Server) exportDedupCandidates(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "json" {
		httputil.RespondWithBadRequest(c, "format must be csv or json")
		return
	}

	filter := database.CandidateFilter{Limit: 100000}
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
			httputil.RespondWithBadRequest(c, "invalid min_similarity")
			return
		}
		filter.MinSimilarity = &f
	}

	candidates, _, err := s.embeddingStore.ListCandidates(filter)
	if err != nil {
		httputil.InternalError(c, "failed to list candidates for export", err)
		return
	}

	// Enrich: lookup titles + author names for every entity involved,
	// memoized so a book that appears in multiple candidates is only
	// fetched once. Books-only for now — authors export would need the
	// author table which we can add later if needed.
	type enriched struct {
		title  string
		author string
	}
	cache := make(map[string]enriched, len(candidates)*2)
	lookup := func(id string) enriched {
		if e, ok := cache[id]; ok {
			return e
		}
		e := enriched{}
		if book, err := s.Store().GetBookByID(id); err == nil && book != nil {
			e.title = book.Title
			if book.AuthorID != nil {
				if a, err := s.Store().GetAuthorByID(*book.AuthorID); err == nil && a != nil {
					e.author = a.Name
				}
			}
		}
		cache[id] = e
		return e
	}

	filename := fmt.Sprintf("dedup-candidates-%s.%s", time.Now().Format("20060102-150405"), format)

	if format == "json" {
		type row struct {
			CandidateID   int64   `json:"candidate_id"`
			Status        string  `json:"status"`
			Layer         string  `json:"layer"`
			Similarity    float64 `json:"similarity"`
			EntityType    string  `json:"entity_type"`
			EntityAID     string  `json:"entity_a_id"`
			EntityATitle  string  `json:"entity_a_title"`
			EntityAAuthor string  `json:"entity_a_author"`
			EntityBID     string  `json:"entity_b_id"`
			EntityBTitle  string  `json:"entity_b_title"`
			EntityBAuthor string  `json:"entity_b_author"`
			LLMVerdict    string  `json:"llm_verdict,omitempty"`
			LLMReason     string  `json:"llm_reason,omitempty"`
			CreatedAt     string  `json:"created_at"`
			UpdatedAt     string  `json:"updated_at"`
		}
		rows := make([]row, 0, len(candidates))
		for _, cand := range candidates {
			a := lookup(cand.EntityAID)
			b := lookup(cand.EntityBID)
			sim := 0.0
			if cand.Similarity != nil {
				sim = *cand.Similarity
			}
			rows = append(rows, row{
				CandidateID:   cand.ID,
				Status:        cand.Status,
				Layer:         cand.Layer,
				Similarity:    sim,
				EntityType:    cand.EntityType,
				EntityAID:     cand.EntityAID,
				EntityATitle:  a.title,
				EntityAAuthor: a.author,
				EntityBID:     cand.EntityBID,
				EntityBTitle:  b.title,
				EntityBAuthor: b.author,
				LLMVerdict:    cand.LLMVerdict,
				LLMReason:     cand.LLMReason,
				CreatedAt:     cand.CreatedAt.Format(time.RFC3339),
				UpdatedAt:     cand.UpdatedAt.Format(time.RFC3339),
			})
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Header("Content-Type", "application/json")
		enc := json.NewEncoder(c.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(gin.H{"count": len(rows), "candidates": rows}); err != nil {
			slog.Info("dedup export json encode", "err", err)
		}
		return
	}

	// CSV path.
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "text/csv")
	w := csv.NewWriter(c.Writer)
	defer w.Flush()
	_ = w.Write([]string{
		"candidate_id", "status", "layer", "similarity",
		"entity_type",
		"entity_a_id", "entity_a_title", "entity_a_author",
		"entity_b_id", "entity_b_title", "entity_b_author",
		"llm_verdict", "llm_reason",
		"created_at", "updated_at",
	})
	for _, cand := range candidates {
		a := lookup(cand.EntityAID)
		b := lookup(cand.EntityBID)
		simStr := ""
		if cand.Similarity != nil {
			simStr = strconv.FormatFloat(*cand.Similarity, 'f', 4, 64)
		}
		_ = w.Write([]string{
			strconv.FormatInt(cand.ID, 10),
			cand.Status,
			cand.Layer,
			simStr,
			cand.EntityType,
			cand.EntityAID,
			a.title,
			a.author,
			cand.EntityBID,
			b.title,
			b.author,
			cand.LLMVerdict,
			cand.LLMReason,
			cand.CreatedAt.Format(time.RFC3339),
			cand.UpdatedAt.Format(time.RFC3339),
		})
	}
	slog.Info("dedup export wrote candidate rows as", "candidates_count", len(candidates), "format", format)
}

// series-aware dedup helpers below. These exist to support "merge
// every cluster in this series" — a common workflow after rescanning
// a whole collection where every book in a series produces its own
// cluster and the user wants to commit all of them with one action
// instead of N clicks.

// dedupSeriesSummary is one entry in the response of
// listDedupCandidateSeries — one row per series that has pending
// candidates, with counts so the user can pick a series to merge
// without having to drill into each one.
type dedupSeriesSummary struct {
	SeriesID       int    `json:"series_id"`
	SeriesName     string `json:"series_name"`
	ClusterCount   int    `json:"cluster_count"`
	BookCount      int    `json:"book_count"`
	CandidateCount int    `json:"candidate_count"`
}

// listDedupCandidateSeries handles
// GET /api/v1/dedup/candidates/series-summary.
//
// Walks every pending book candidate, looks up both sides' series_id,
// and returns one row per series where BOTH sides of at least one
// candidate pair belong to that series. Clusters are computed via
// union-find per-series so the count reflects what "merge every
// cluster in this series" would actually touch.
//
// Candidates whose two sides belong to different series are excluded
// from every summary — they're cross-series and don't fit the
// "series-aware bulk merge" workflow.
func (s *Server) listDedupCandidateSeries(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	cands, _, err := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		httputil.InternalError(c, "failed to list pending candidates", err)
		return
	}

	// Memoize book → series_id lookups across candidates.
	bookSeries := make(map[string]int, len(cands)*2)
	lookup := func(id string) int {
		if v, ok := bookSeries[id]; ok {
			return v
		}
		book, err := s.Store().GetBookByID(id)
		if err != nil || book == nil || book.SeriesID == nil {
			bookSeries[id] = 0
			return 0
		}
		bookSeries[id] = *book.SeriesID
		return *book.SeriesID
	}

	// Group candidate pairs by series. candsBySeries[seriesID] holds
	// every (a_id, b_id) that's entirely within that series.
	type pair struct{ a, b string }
	candsBySeries := make(map[int][]pair)
	for _, cand := range cands {
		sa := lookup(cand.EntityAID)
		sb := lookup(cand.EntityBID)
		if sa == 0 || sb == 0 || sa != sb {
			continue
		}
		candsBySeries[sa] = append(candsBySeries[sa], pair{cand.EntityAID, cand.EntityBID})
	}

	// For each series, cluster via union-find to compute how many
	// merge operations would actually run.
	summary := make([]dedupSeriesSummary, 0, len(candsBySeries))
	for seriesID, pairs := range candsBySeries {
		parent := make(map[string]string)
		var find func(string) string = func(x string) string {
			for parent[x] != x {
				parent[x] = parent[parent[x]]
				x = parent[x]
			}
			return x
		}
		union := func(a, b string) {
			for _, id := range []string{a, b} {
				if _, ok := parent[id]; !ok {
					parent[id] = id
				}
			}
			ra, rb := find(a), find(b)
			if ra != rb {
				parent[ra] = rb
			}
		}
		for _, p := range pairs {
			union(p.a, p.b)
		}
		roots := make(map[string]struct{})
		books := make(map[string]struct{})
		for id := range parent {
			roots[find(id)] = struct{}{}
			books[id] = struct{}{}
		}

		name := ""
		if series, err := s.Store().GetSeriesByID(seriesID); err == nil && series != nil {
			name = series.Name
		}
		summary = append(summary, dedupSeriesSummary{
			SeriesID:       seriesID,
			SeriesName:     name,
			ClusterCount:   len(roots),
			BookCount:      len(books),
			CandidateCount: len(pairs),
		})
	}

	// Stable sort: highest cluster count first, then series name.
	sort.Slice(summary, func(i, j int) bool {
		if summary[i].ClusterCount != summary[j].ClusterCount {
			return summary[i].ClusterCount > summary[j].ClusterCount
		}
		return summary[i].SeriesName < summary[j].SeriesName
	})

	httputil.RespondWithOK(c, gin.H{"series": summary})
}

// mergeDedupCandidateSeries handles
// POST /api/v1/dedup/candidates/merge-series.
//
// Body: {"series_id": N}
//
// Finds every pending book candidate whose both sides belong to the
// given series, builds clusters via union-find, and merges each
// cluster with MergeService.MergeBooks. Returns a summary of how many
// clusters were touched and how many books were merged in total.
//
// Cross-series candidates (one side in this series, the other
// somewhere else) are deliberately untouched — the series filter is
// a scope, not a selector. If the user wants those pairs merged, they
// can use the regular Merge Filtered action.
func (s *Server) mergeDedupCandidateSeries(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}
	if s.mergeService == nil {
		httputil.RespondWithServiceUnavailable(c, "merge service not available")
		return
	}

	var body struct {
		SeriesID int `json:"series_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.SeriesID <= 0 {
		httputil.RespondWithBadRequest(c, "series_id must be a positive integer")
		return
	}

	cands, _, err := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		httputil.InternalError(c, "failed to list pending candidates", err)
		return
	}

	// Filter to same-series candidates only.
	bookSeries := make(map[string]int, len(cands)*2)
	lookup := func(id string) int {
		if v, ok := bookSeries[id]; ok {
			return v
		}
		book, err := s.Store().GetBookByID(id)
		if err != nil || book == nil || book.SeriesID == nil {
			bookSeries[id] = 0
			return 0
		}
		bookSeries[id] = *book.SeriesID
		return *book.SeriesID
	}
	var inScope []database.DedupCandidate
	for _, cand := range cands {
		if lookup(cand.EntityAID) == body.SeriesID && lookup(cand.EntityBID) == body.SeriesID {
			inScope = append(inScope, cand)
		}
	}

	// Union-find cluster build.
	parent := make(map[string]string)
	var find func(string) string = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b string) {
		for _, id := range []string{a, b} {
			if _, ok := parent[id]; !ok {
				parent[id] = id
			}
		}
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for _, cand := range inScope {
		union(cand.EntityAID, cand.EntityBID)
	}
	clusters := make(map[string][]string)
	for id := range parent {
		root := find(id)
		clusters[root] = append(clusters[root], id)
	}

	// Merge each cluster. Candidate rows contained in each cluster get
	// marked as merged inside the same loop (same membership check as
	// mergeDedupCluster) so the Merged tab reflects the action.
	mergedClusters := 0
	mergedBooks := 0
	candidatesUpdated := 0
	var failures []string
	for _, bookIDs := range clusters {
		if len(bookIDs) < 2 {
			continue
		}
		if _, err := s.mergeService.MergeBooks(bookIDs, ""); err != nil {
			failures = append(failures, fmt.Sprintf("cluster of %d: %v", len(bookIDs), err))
			continue
		}
		mergedClusters++
		mergedBooks += len(bookIDs)

		inCluster := make(map[string]struct{}, len(bookIDs))
		for _, id := range bookIDs {
			inCluster[id] = struct{}{}
		}
		for _, cand := range inScope {
			_, aIn := inCluster[cand.EntityAID]
			_, bIn := inCluster[cand.EntityBID]
			if !aIn || !bIn {
				continue
			}
			if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "merged"); err != nil {
				slog.Info("dedup series merge status update", "cand", cand.ID, "err", err)
				continue
			}
			candidatesUpdated++
		}
	}

	slog.Info("dedup series merge series clusters_merged books_merged candidates_updated failures", "body", body.SeriesID, "mergedClusters", mergedClusters, "mergedBooks", mergedBooks, "candidatesUpdated", candidatesUpdated, "failures_count", len(failures))

	httputil.RespondWithOK(c, gin.H{
		"series_id":          body.SeriesID,
		"clusters_merged":    mergedClusters,
		"books_merged":       mergedBooks,
		"candidates_updated": candidatesUpdated,
		"failures":           failures,
	})
}

// getDedupStats handles GET /api/v1/dedup/stats.
func (s *Server) getDedupStats(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	stats, err := s.embeddingStore.GetCandidateStats()
	if err != nil {
		httputil.InternalError(c, "failed to get dedup stats", err)
		return
	}

	httputil.RespondWithOK(c, gin.H{"stats": stats})
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
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}
	if s.mergeService == nil {
		httputil.RespondWithServiceUnavailable(c, "merge service not available")
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
		httputil.RespondWithBadRequest(c, "bulk merge only supports entity_type=book")
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
		httputil.InternalError(c, "failed to list candidates for bulk merge", err)
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
			slog.Info("dedup bulk merge candidate failed", "cand", cand.ID, "mergeErr", mergeErr)
			continue
		}
		if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "merged"); err != nil {
			// The books were merged on the server side, but we couldn't
			// update the candidate row — log it and count as merged
			// since the destructive action already happened.
			slog.Info("dedup bulk merge candidate merged but status update failed", "cand", cand.ID, "err", err)
		}
		merged++
	}

	slog.Info("dedup bulk merge complete merged, failed out of matched", "merged", merged, "failures_count", len(failures), "total", total)

	httputil.RespondWithOK(c, gin.H{
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
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}
	if s.mergeService == nil {
		httputil.RespondWithServiceUnavailable(c, "merge service not available")
		return
	}

	var body struct {
		BookIDs       []string `json:"book_ids"`
		PrimaryBookID string   `json:"primary_book_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}
	if len(body.BookIDs) < 2 {
		httputil.RespondWithBadRequest(c, "book_ids must contain at least 2 entries")
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
			httputil.RespondWithBadRequest(c, "primary_book_id must be one of book_ids")
			return
		}
	}

	mergeResult, err := s.mergeService.MergeBooks(body.BookIDs, body.PrimaryBookID)
	if err != nil {
		httputil.InternalError(c, "failed to merge books in cluster", err)
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
		slog.Info("dedup cluster merge list candidates failed", "listErr", listErr)
	} else {
		for _, cand := range candidates {
			_, aIn := inCluster[cand.EntityAID]
			_, bIn := inCluster[cand.EntityBID]
			if !aIn || !bIn {
				continue
			}
			if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "merged"); err != nil {
				slog.Info("dedup cluster merge status update", "cand", cand.ID, "err", err)
				continue
			}
			updated++
		}
	}
	slog.Info("dedup cluster merge merged books, marked candidate row(s) as merged", "count", len(body.BookIDs), "updated", updated)

	httputil.RespondWithOK(c, gin.H{
		"status":             "merged",
		"merged_books":       len(body.BookIDs),
		"candidates_updated": updated,
		"result":             mergeResult,
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
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	var body struct {
		BookIDs []string `json:"book_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}
	if len(body.BookIDs) < 2 {
		httputil.RespondWithBadRequest(c, "book_ids must contain at least 2 entries")
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
		httputil.InternalError(c, "failed to list candidates for cluster dismiss", err)
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
			slog.Info("dedup cluster dismiss status update", "cand", cand.ID, "err", err)
			continue
		}
		dismissed++
	}
	slog.Info("dedup cluster dismiss dismissed candidate row(s) across books", "dismissed", dismissed, "count", len(body.BookIDs))
	s.markDuplicatesFlaggedDirty("dismiss_cluster")

	httputil.RespondWithOK(c, gin.H{
		"status":    "dismissed",
		"dismissed": dismissed,
	})
}

// removeFromDedupCluster handles POST /api/v1/dedup/candidates/remove-from-cluster.
//
//	Body: {
//	  "cluster_book_ids": [...],
//	  "remove_book_id": "X"      // singular, backwards-compat
//	  "remove_book_ids": [...]   // plural, preferred
//	}
//
// Dismisses every pending candidate whose pair is one-side-in-remove-set
// and other-side in (cluster \ remove-set). In other words: "these books
// are NOT duplicates of the remaining books in this cluster". Pairs where
// both sides are in the remove set are ALSO dismissed — removing two
// wrong-books from a cluster means neither should stay paired with
// anything that was in the original cluster.
//
// Pairs involving a removed book with books OUTSIDE the cluster are
// left alone — this is a scoped split, not a global ban.
//
// Accepts both singular and plural forms for backwards compatibility.
// If both are provided, they're merged into a single set.
func (s *Server) removeFromDedupCluster(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	var body struct {
		ClusterBookIDs []string `json:"cluster_book_ids"`
		RemoveBookID   string   `json:"remove_book_id,omitempty"`
		RemoveBookIDs  []string `json:"remove_book_ids,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httputil.RespondWithBadRequest(c, "invalid request body")
		return
	}

	// Normalize into one remove set. Singular and plural both supported
	// so the existing × button (singular) keeps working while the new
	// multi-select UI sends the plural.
	removeSet := make(map[string]struct{})
	if body.RemoveBookID != "" {
		removeSet[body.RemoveBookID] = struct{}{}
	}
	for _, id := range body.RemoveBookIDs {
		if id != "" {
			removeSet[id] = struct{}{}
		}
	}
	if len(removeSet) == 0 {
		httputil.RespondWithBadRequest(c, "remove_book_id or remove_book_ids is required")
		return
	}
	if len(body.ClusterBookIDs) < 2 {
		httputil.RespondWithBadRequest(c, "cluster_book_ids must contain at least 2 entries")
		return
	}

	// Build the set of "remaining books in this cluster" — cluster
	// minus the remove set.
	remaining := make(map[string]struct{}, len(body.ClusterBookIDs))
	for _, id := range body.ClusterBookIDs {
		if _, removed := removeSet[id]; removed {
			continue
		}
		remaining[id] = struct{}{}
	}
	if len(remaining) == 0 {
		httputil.RespondWithBadRequest(c, "cluster must contain at least one book that is not in remove set")
		return
	}

	candidates, _, err := s.embeddingStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		httputil.InternalError(c, "failed to list candidates for cluster remove", err)
		return
	}

	dismissed := 0
	for _, cand := range candidates {
		// A pair is dismissible if at least one side is in the remove
		// set AND the other side is either also in the remove set or in
		// the remaining cluster. Pairs where a removed book touches a
		// book OUTSIDE the cluster are not touched — those are
		// different clusters.
		_, aRemoved := removeSet[cand.EntityAID]
		_, bRemoved := removeSet[cand.EntityBID]
		if !aRemoved && !bRemoved {
			continue
		}
		_, aRemaining := remaining[cand.EntityAID]
		_, bRemaining := remaining[cand.EntityBID]

		touchesCluster := false
		switch {
		case aRemoved && (bRemaining || bRemoved):
			touchesCluster = true
		case bRemoved && (aRemaining || aRemoved):
			touchesCluster = true
		}
		if !touchesCluster {
			continue
		}

		if err := s.embeddingStore.UpdateCandidateStatus(cand.ID, "dismissed"); err != nil {
			slog.Info("dedup remove-from-cluster status update", "cand", cand.ID, "err", err)
			continue
		}
		dismissed++
	}
	slog.Info("dedup remove-from-cluster dismissed edge(s), removed book(s) from cluster of", "dismissed", dismissed, "removeSet_count", len(removeSet), "count", len(body.ClusterBookIDs))

	httputil.RespondWithOK(c, gin.H{
		"status":    "removed",
		"dismissed": dismissed,
		"removed":   len(removeSet),
	})
}

// mergeDedupCandidate handles POST /api/v1/dedup/candidates/:id/merge.
func (s *Server) mergeDedupCandidate(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid candidate id")
		return
	}

	candidate, err := s.embeddingStore.GetCandidateByID(id)
	if err != nil {
		httputil.InternalError(c, "failed to get candidate", err)
		return
	}
	if candidate == nil {
		httputil.RespondWithNotFound(c, "candidate", idStr)
		return
	}

	// Optional JSON body {"keep_id": "<bookID>"} lets the caller pick
	// which side of the candidate pair becomes the merge primary. When
	// absent or empty, fall back to MergeService's auto-select (by
	// format/bitrate/size). When present, it must match one of the two
	// candidate entities — otherwise the caller is confused and we
	// refuse rather than silently auto-picking.
	var body struct {
		KeepID string `json:"keep_id"`
	}
	// Ignore parse errors: an empty/missing body is valid (back-compat).
	_ = c.ShouldBindJSON(&body)
	keepID := body.KeepID
	if keepID != "" && keepID != candidate.EntityAID && keepID != candidate.EntityBID {
		httputil.RespondWithBadRequest(c, "keep_id must match the candidate's entity_a_id or entity_b_id")
		return
	}

	var result interface{}
	if candidate.EntityType == "book" && s.mergeService != nil {
		mergeResult, mergeErr := s.mergeService.MergeBooks([]string{candidate.EntityAID, candidate.EntityBID}, keepID)
		if mergeErr != nil {
			// MAYDEPLOY-B2: when one of the source books no longer exists (a previous
			// merge already absorbed it), the candidate is stale rather than the
			// request being a server error. Treat that as 409 Conflict and mark the
			// candidate merged so the UI's next refresh drops it.
			//
			// We use a substring match on "not found" because the underlying merge
			// service returns a plain fmt.Errorf("book %s not found", ...) without an
			// exported sentinel error. Switch to errors.Is if/when that error type
			// becomes exported.
			if strings.Contains(mergeErr.Error(), "not found") {
				if statusErr := s.embeddingStore.UpdateCandidateStatus(id, "merged"); statusErr != nil {
					slog.Warn("dedup merge already-merged: failed to update candidate status", "candidate_id", id, "err", statusErr)
				}
				s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventDedupMerged, candidate.EntityAID, map[string]any{
					"entity_b_id":   candidate.EntityBID,
					"entity_type":   candidate.EntityType,
					"candidate_id":  id,
					"already_merged": true,
				}))
				slog.Info("dedup merge skipped: source book already merged away",
					"candidate_id", id,
					"entity_a", candidate.EntityAID,
					"entity_b", candidate.EntityBID,
					"err", mergeErr,
				)
				c.JSON(http.StatusConflict, gin.H{
					"status":       "already_merged",
					"candidate_id": id,
				})
				return
			}
			httputil.InternalError(c, "failed to merge books", mergeErr)
			return
		}
		result = mergeResult

		// MAYDEPLOY-B3: sweep orphan candidates. Any *other* pending candidate
		// row that still references the merged-away book(s) is now stale —
		// clicking Merge on it would 500 ("book not found") absent the
		// PR #1160 409-hotfix path. Mark them merged proactively so the UI
		// drops them on the next refresh rather than the user having to
		// dismiss each by hand.
		if s.dedupEngine != nil && mergeResult != nil {
			var mergedAway []string
			for _, bid := range []string{candidate.EntityAID, candidate.EntityBID} {
				if bid != "" && bid != mergeResult.PrimaryID {
					mergedAway = append(mergedAway, bid)
				}
			}
			s.dedupEngine.CleanupCandidatesAfterMerge(mergedAway)
		}
	}

	if err := s.embeddingStore.UpdateCandidateStatus(id, "merged"); err != nil {
		httputil.InternalError(c, "failed to update candidate status", err)
		return
	}

	s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventDedupMerged, candidate.EntityAID, map[string]any{
		"entity_b_id":  candidate.EntityBID,
		"entity_type":  candidate.EntityType,
		"candidate_id": id,
	}))

	httputil.RespondWithOK(c, gin.H{"status": "merged", "result": result, "keep_id": keepID})
}

// dismissDedupCandidate handles POST /api/v1/dedup/candidates/:id/dismiss.
func (s *Server) dismissDedupCandidate(c *gin.Context) {
	if s.embeddingStore == nil {
		httputil.RespondWithServiceUnavailable(c, "embedding store not available")
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid candidate id")
		return
	}

	if err := s.embeddingStore.UpdateCandidateStatus(id, "dismissed"); err != nil {
		httputil.InternalError(c, "failed to dismiss candidate", err)
		return
	}
	s.markDuplicatesFlaggedDirty("dismiss_candidate")

	httputil.RespondWithOK(c, gin.H{"status": "dismissed"})
}

// triggerDedupScan handles POST /api/v1/dedup/scan.
// Delegates to the UOS registry (dedup.full-scan op) since UOS-09.
func (s *Server) triggerDedupScan(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.full-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue dedup scan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// triggerDedupLLM handles POST /api/v1/dedup/scan-llm.
// Delegates to the UOS registry (dedup.llm-review op) since UOS-09.
func (s *Server) triggerDedupLLM(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.llm-review", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue LLM review", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// triggerDedupRefresh handles POST /api/v1/dedup/refresh.
// Re-runs the full scan as a tracked Operation. Identical behavior to
// triggerDedupScan — kept as a separate endpoint for backwards compatibility.
func (s *Server) triggerDedupRefresh(c *gin.Context) {
	s.triggerDedupScan(c)
}

// triggerDedupAcoustID handles POST /api/v1/dedup/scan-acoustid.
// Delegates to the UOS registry (acoustid.scan op) since UOS-09.
func (s *Server) triggerDedupAcoustID(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "acoustid.scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue acoustid scan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// purgeStaleCandidates handles POST /api/v1/dedup/purge-stale.
// Enqueues the dedup.purge-stale UOS op so the cleanup shows up in the
// bell with proper start/end log lines, instead of silently running and
// returning a count. Engine.PurgeStaleCandidates still does the actual
// work — see internal/plugins/dedup/purge_stale.go.
func (s *Server) purgeStaleCandidates(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.purge-stale", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue dedup purge-stale", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// triggerEmbedScan handles POST /api/v1/dedup/embed.
// Delegates to the UOS registry (dedup.embed-scan op) since UOS-07.
func (s *Server) triggerEmbedScan(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.embed-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue embed scan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// triggerEmbedAsync handles POST /api/v1/dedup/embed-async.
// Enqueues the nightly embed-async UOS op on demand.
func (s *Server) triggerEmbedAsync(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.embed-async", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue embed-async", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// triggerBookSignatureScan handles POST /api/v1/dedup/scan-book-signature.
// Delegates to the UOS registry (dedup.book-signature-scan op) since UOS-09.
func (s *Server) triggerBookSignatureScan(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}
	opID, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.book-signature-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to enqueue book signature scan", err)
		return
	}
	httputil.RespondWithSuccess(c, http.StatusAccepted, map[string]string{"op_id": opID})
}

// AcoustIDSegmentComparison holds comparison data for one fingerprint segment.
type AcoustIDSegmentComparison struct {
	Segment string `json:"segment"` // "seg0" … "seg6"
	HashA   string `json:"hash_a"`  // fingerprint hex or ""
	HashB   string `json:"hash_b"`
	Match   bool   `json:"match"`
}

// AcoustIDCompareResponse is the response body for the compare-acoustid endpoint.
type AcoustIDCompareResponse struct {
	BookA         database.Book               `json:"book_a"`
	BookB         database.Book               `json:"book_b"`
	OverallScore  float64                     `json:"overall_score"`  // 0.0–1.0
	SegmentScores []AcoustIDSegmentComparison `json:"segment_scores"` // 7 entries
}

// handleCompareAcoustID handles POST /api/v1/books/:id/compare-acoustid?other=<bookID2>.
// Computes per-segment fingerprint comparison between two books' primary files.
func (s *Server) handleCompareAcoustID(c *gin.Context) {
	idA := c.Param("id")
	idB := c.Query("other")
	if idB == "" {
		httputil.RespondWithBadRequest(c, "missing ?other= query parameter")
		return
	}

	bookA, err := s.Store().GetBookByID(idA)
	if err != nil || bookA == nil {
		httputil.RespondWithNotFound(c, "book", idA)
		return
	}
	bookB, err := s.Store().GetBookByID(idB)
	if err != nil || bookB == nil {
		httputil.RespondWithNotFound(c, "book", idB)
		return
	}

	filesA, _ := s.Store().GetBookFiles(idA)
	filesB, _ := s.Store().GetBookFiles(idB)

	primary := func(files []database.BookFile) *database.BookFile {
		for i := range files {
			if files[i].AcoustIDSeg0 != "" {
				return &files[i]
			}
		}
		if len(files) > 0 {
			return &files[0]
		}
		return nil
	}

	fa := primary(filesA)
	fb := primary(filesB)

	segNames := []string{"seg0", "seg1", "seg2", "seg3", "seg4", "seg5", "seg6"}

	segsA := []string{"", "", "", "", "", "", ""}
	segsB := []string{"", "", "", "", "", "", ""}
	if fa != nil {
		segsA = []string{fa.AcoustIDSeg0, fa.AcoustIDSeg1, fa.AcoustIDSeg2, fa.AcoustIDSeg3, fa.AcoustIDSeg4, fa.AcoustIDSeg5, fa.AcoustIDSeg6}
	}
	if fb != nil {
		segsB = []string{fb.AcoustIDSeg0, fb.AcoustIDSeg1, fb.AcoustIDSeg2, fb.AcoustIDSeg3, fb.AcoustIDSeg4, fb.AcoustIDSeg5, fb.AcoustIDSeg6}
	}

	var comparisons []AcoustIDSegmentComparison
	var matching, total int
	for i, name := range segNames {
		a, b := segsA[i], segsB[i]
		comp := AcoustIDSegmentComparison{Segment: name, HashA: a, HashB: b}
		if a != "" && b != "" {
			total++
			comp.Match = a == b
			if comp.Match {
				matching++
			}
		}
		comparisons = append(comparisons, comp)
	}

	overall := 0.0
	if total > 0 {
		overall = float64(matching) / float64(total)
	}

	httputil.RespondWithOK(c, AcoustIDCompareResponse{
		BookA:         *bookA,
		BookB:         *bookB,
		OverallScore:  overall,
		SegmentScores: comparisons,
	})
}
