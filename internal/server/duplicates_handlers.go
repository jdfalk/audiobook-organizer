// file: internal/server/duplicates_handlers.go
// version: 3.0.0
// guid: 47a3e3fb-f5cf-4970-a2fc-d2ef481368c9
// last-edited: 2026-05-02
//
// SQL-backed duplicate detection handlers split out of server.go:
// find, list, merge, skip, dismiss, pair-level actions, and series
// prune. Distinct from the embedding-based dedup flow living in
// dedup_handlers.go.

package server

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/util"
	ulid "github.com/oklog/ulid/v2"
)

func (s *Server) listDuplicateAudiobooks(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-duplicates"); ok {
		httputil.RespondWithOK(c, cached)
		return
	}

	result, err := s.audiobookService.GetDuplicateBooks(c.Request.Context())
	if err != nil {
		httputil.InternalError(c, "failed to list duplicate audiobooks", err)
		return
	}

	resp := gin.H{
		"groups":          result.Groups,
		"group_count":     result.GroupCount,
		"duplicate_count": result.DuplicateCount,
	}
	s.dedupCache.Set("book-duplicates", resp)
	httputil.RespondWithOK(c, resp)
}

// listBookDuplicateScanResults returns cached results from the last async book-dedup scan.
func (s *Server) listBookDuplicateScanResults(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-dedup-scan"); ok {
		httputil.RespondWithOK(c, cached)
		return
	}
	httputil.RespondWithOK(c, gin.H{"groups": []any{}, "group_count": 0, "duplicate_count": 0, "needs_refresh": true})
}

// scanBookDuplicates triggers an async scan for book duplicates using metadata matching.
func (s *Server) scanBookDuplicates(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "book-dedup-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.book-scan", bookDedupScanOpParams{LegacyOpID: op.ID}); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

// mergeBookDuplicatesAsVersions merges a group of duplicate books into a version group.
func (s *Server) mergeBookDuplicatesAsVersions(c *gin.Context) {
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.BookIDs) < 2 {
		httputil.RespondWithBadRequest(c, "need at least 2 book IDs")
		return
	}

	ms := s.mergeService
	if ms == nil {
		ms = merge.NewService(s.Store())
	}

	result, err := ms.MergeBooks(req.BookIDs, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithNotFound(c, "book", "")
		} else {
			httputil.InternalError(c, "failed to merge duplicate books", err)
		}
		return
	}

	s.dedupCache.Invalidate("book-dedup-scan")
	s.dedupCache.Invalidate("book-duplicates")

	httputil.RespondWithOK(c, gin.H{
		"message":          fmt.Sprintf("Merged %d books into version group", result.MergedCount),
		"version_group_id": result.VersionGroupID,
		"primary_id":       result.PrimaryID,
	})
}

// dismissBookDuplicateGroup marks a book duplicate group as not-duplicates.
func (s *Server) dismissBookDuplicateGroup(c *gin.Context) {
	var req struct {
		GroupKey string `json:"group_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	// Load existing dismissed groups
	dismissed := loadDismissedDedupGroups(store)
	dismissed[req.GroupKey] = true
	saveDismissedDedupGroups(store, dismissed)

	s.dedupCache.Invalidate("book-dedup-scan")

	httputil.RespondWithOK(c, gin.H{"message": "Group dismissed"})
}

func (s *Server) mergeBooks(c *gin.Context) {
	var req struct {
		KeepID   string   `json:"keep_id" binding:"required"`
		MergeIDs []string `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if len(req.MergeIDs) == 0 {
		httputil.RespondWithBadRequest(c, "merge_ids must not be empty")
		return
	}

	store := s.Store()
	keepBook, err := store.GetBookByID(req.KeepID)
	if err != nil || keepBook == nil {
		httputil.RespondWithNotFound(c, "book", req.KeepID)
		return
	}

	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-books:keep=%s,merge=%d", req.KeepID, len(req.MergeIDs))
	op, err := store.CreateOperation(opID, "book-merge", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := bookMergeOpParams{
		LegacyOpID: op.ID,
		KeepID:     req.KeepID,
		MergeIDs:   req.MergeIDs,
		Detail:     detail,
	}
	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.book-merge", params); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

func (s *Server) listDuplicateAuthors(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("author-duplicates"); ok {
		httputil.RespondWithOK(c, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	httputil.RespondWithOK(c, gin.H{"groups": []any{}, "count": 0, "needs_refresh": true})
}

func (s *Server) refreshDuplicateAuthors(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "author-dedup-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.author-scan", authorDedupScanOpParams{LegacyOpID: op.ID}); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

// filterReviewedAuthorGroups removes author dedup groups where all author IDs
// have already been reviewed via AI scans (applied results with skip/split/merge).
func (s *Server) filterReviewedAuthorGroups(groups []dedup.AuthorDedupGroup) []dedup.AuthorDedupGroup {
	if s.aiScanStore == nil {
		return groups
	}
	applied, err := s.aiScanStore.GetAllAppliedResults()
	if err != nil || len(applied) == 0 {
		return groups
	}

	// Build set of reviewed author ID sets (key = sorted comma-joined IDs)
	reviewedSets := make(map[string]bool)
	for _, r := range applied {
		if len(r.Suggestion.AuthorIDs) < 2 {
			continue
		}
		ids := make([]int, len(r.Suggestion.AuthorIDs))
		copy(ids, r.Suggestion.AuthorIDs)
		sort.Ints(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		reviewedSets[strings.Join(parts, ",")] = true
	}

	if len(reviewedSets) == 0 {
		return groups
	}

	// Filter: exclude groups whose author IDs match a reviewed set
	filtered := make([]dedup.AuthorDedupGroup, 0, len(groups))
	for _, g := range groups {
		ids := make([]int, 0, 1+len(g.Variants))
		ids = append(ids, g.Canonical.ID)
		for _, v := range g.Variants {
			ids = append(ids, v.ID)
		}
		sort.Ints(ids)
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = strconv.Itoa(id)
		}
		key := strings.Join(parts, ",")
		if !reviewedSets[key] {
			filtered = append(filtered, g)
		}
	}
	return filtered
}

func (s *Server) listSeriesDuplicates(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("series-duplicates"); ok {
		httputil.RespondWithOK(c, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	httputil.RespondWithOK(c, gin.H{"groups": []any{}, "count": 0, "total_series": 0, "needs_refresh": true})
}

func (s *Server) refreshSeriesDuplicates(c *gin.Context) {
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "series-dedup-scan", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.series-scan", seriesDedupScanOpParams{LegacyOpID: op.ID}); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

// validateDedupEntry searches metadata sources (OpenLibrary, Audible, etc.) to validate
// a series name, author name, or book title during dedup review.
func (s *Server) validateDedupEntry(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
		Type  string `json:"type"` // "series", "author", "book"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, "query is required")
		return
	}
	if req.Type == "" {
		req.Type = "series"
	}

	chain := s.metadataFetchService.BuildSourceChain()
	if len(chain) == 0 {
		httputil.RespondWithOK(c, gin.H{"results": []interface{}{}, "message": "no metadata sources configured"})
		return
	}

	type validationResult struct {
		Source         string `json:"source"`
		Title          string `json:"title"`
		Author         string `json:"author"`
		Series         string `json:"series,omitempty"`
		SeriesPosition string `json:"series_position,omitempty"`
		CoverURL       string `json:"cover_url,omitempty"`
		ISBN           string `json:"isbn,omitempty"`
	}

	var results []validationResult
	ctx := c.Request.Context()
	for _, src := range chain {
		matches, err := src.SearchByTitle(ctx, req.Query)
		if err != nil {
			continue
		}
		for _, m := range matches {
			r := validationResult{
				Source:         src.Name(),
				Title:          m.Title,
				Author:         m.Author,
				Series:         m.Series,
				SeriesPosition: m.SeriesPosition,
				CoverURL:       m.CoverURL,
				ISBN:           m.ISBN,
			}
			// For series validation, prioritize results that have series info
			if req.Type == "series" && m.Series == "" {
				continue
			}
			results = append(results, r)
		}
		// Limit total results
		if len(results) >= 20 {
			results = results[:20]
			break
		}
	}

	if results == nil {
		results = []validationResult{}
	}
	httputil.RespondWithOK(c, gin.H{"results": results, "query": req.Query, "type": req.Type})
}

func (s *Server) deduplicateSeriesHandler(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := "series-deduplicate"
	op, err := store.CreateOperation(opID, "series-dedup", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := seriesDedupOpParams{LegacyOpID: op.ID, Detail: detail}
	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.series-dedup", params); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

func (s *Server) seriesPrunePreview(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	preview, err := computeSeriesPrunePreview(store)
	if err != nil {
		httputil.InternalError(c, "failed to compute series prune preview", err)
		return
	}

	httputil.RespondWithOK(c, preview)
}

func (s *Server) seriesPrune(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := "series-prune"
	op, err := store.CreateOperation(opID, "series-prune", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := seriesPruneOpParams{LegacyOpID: op.ID, Detail: detail}
	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.series-prune", params); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

// executeSeriesPrune performs the actual series prune logic (used by both HTTP handler and scheduler).
func (s *Server) executeSeriesPrune(ctx context.Context, store interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.OperationStore
}, progress operations.ProgressReporter, operationID string) error {
	_ = progress.Log("info", "Starting series auto-prune...", nil)

	allSeries, err := store.GetAllSeries()
	if err != nil {
		return fmt.Errorf("failed to get series: %w", err)
	}

	_ = progress.UpdateProgress(0, len(allSeries), fmt.Sprintf("Scanning %d series...", len(allSeries)))

	// Group by LOWER(TRIM(name)) + author_id
	type groupKey struct {
		name     string
		authorID int
	}
	groups := make(map[groupKey][]database.Series)
	for _, s := range allSeries {
		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: util.NormalizeString(s.Name), authorID: aid}
		groups[key] = append(groups[key], s)
	}

	// Phase 1: Merge duplicates
	totalMerged := 0
	var mergeErrors []string
	dupGroupCount := 0

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		dupGroupCount++

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Pick canonical: most books, then lowest ID
		canonicalIdx := 0
		canonicalBookCount := 0
		for i, s := range group {
			books, err := store.GetBooksBySeriesID(s.ID)
			if err != nil {
				continue
			}
			bc := len(books)
			if bc > canonicalBookCount || (bc == canonicalBookCount && s.ID < group[canonicalIdx].ID) {
				canonicalIdx = i
				canonicalBookCount = bc
			}
		}
		keepID := group[canonicalIdx].ID

		for i, ser := range group {
			if i == canonicalIdx {
				continue
			}
			books, err := store.GetBooksBySeriesID(ser.ID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", ser.ID, err))
				continue
			}
			for _, book := range books {
				oldSeriesID := ser.ID
				book.SeriesID = &keepID
				if _, err := store.UpdateBook(book.ID, &book); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
				} else if operationID != "" {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: operationID,
						BookID:      book.ID,
						ChangeType:  "series_merge",
						FieldName:   "series_id",
						OldValue:    fmt.Sprintf("%d (%s)", oldSeriesID, ser.Name),
						NewValue:    fmt.Sprintf("%d (%s)", keepID, group[canonicalIdx].Name),
					})
				}
			}
			if err := store.DeleteSeries(ser.ID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", ser.ID, err))
			} else {
				totalMerged++
				if operationID != "" {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: operationID,
						ChangeType:  "series_delete",
						FieldName:   "series",
						OldValue:    fmt.Sprintf("%d: %s", ser.ID, ser.Name),
						NewValue:    fmt.Sprintf("merged into %d: %s", keepID, group[canonicalIdx].Name),
					})
				}
			}
		}
	}

	_ = progress.Log("info", fmt.Sprintf("Phase 1 complete: merged %d duplicate series from %d groups", totalMerged, dupGroupCount), nil)
	_ = progress.UpdateProgress(50, 100, "Scanning for orphan series...")

	// Phase 2: Delete orphan series (0 books)
	orphansDeleted := 0
	// Re-fetch series to account for merges
	refreshedSeries, err := store.GetAllSeries()
	if err != nil {
		_ = progress.Log("warn", fmt.Sprintf("Failed to refresh series list: %v", err), nil)
	} else {
		for _, ser := range refreshedSeries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			books, err := store.GetBooksBySeriesID(ser.ID)
			if err != nil {
				continue
			}
			if len(books) == 0 {
				if err := store.DeleteSeries(ser.ID); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete orphan series %d: %v", ser.ID, err))
				} else {
					orphansDeleted++
					if operationID != "" {
						_ = store.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							ChangeType:  "series_delete",
							FieldName:   "orphan_series",
							OldValue:    fmt.Sprintf("%d: %s", ser.ID, ser.Name),
							NewValue:    "deleted (0 books)",
						})
					}
				}
			}
		}
	}

	totalCleaned := totalMerged + orphansDeleted
	resultMsg := fmt.Sprintf("Series prune complete: %d duplicates merged, %d orphans deleted (%d total cleaned, %d errors)",
		totalMerged, orphansDeleted, totalCleaned, len(mergeErrors))
	_ = progress.Log("info", resultMsg, nil)

	// Record summary change
	if operationID != "" {
		_ = store.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			ChangeType:  "series_prune_summary",
			FieldName:   "summary",
			OldValue:    fmt.Sprintf("%d total series scanned", len(allSeries)),
			NewValue:    resultMsg,
		})
	}
	if len(mergeErrors) > 0 {
		errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
		_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
	}
	_ = progress.UpdateProgress(100, 100, resultMsg)

	if s.dedupCache != nil {
		s.dedupCache.InvalidateAll()
	}

	return nil
}

// mergeSeriesGroup merges multiple series into one, reassigning all books.
func (s *Server) mergeSeriesGroup(c *gin.Context) {
	var req struct {
		KeepID     int    `json:"keep_id" binding:"required"`
		MergeIDs   []int  `json:"merge_ids" binding:"required"`
		CustomName string `json:"custom_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if len(req.MergeIDs) == 0 {
		httputil.RespondWithBadRequest(c, "merge_ids must not be empty")
		return
	}

	store := s.Store()
	keepSeries, err := store.GetSeriesByID(req.KeepID)
	if err != nil || keepSeries == nil {
		httputil.RespondWithNotFound(c, "series", "")
		return
	}

	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-series:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "series-merge", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := seriesMergeOpParams{
		LegacyOpID: op.ID,
		KeepID:     req.KeepID,
		MergeIDs:   req.MergeIDs,
		CustomName: req.CustomName,
		Detail:     detail,
	}
	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.series-merge", params); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}

// seriesNormalizeAction describes a single action the normalize pass would take.
type seriesNormalizeAction struct {
	SeriesID      int    `json:"series_id"`
	OldName       string `json:"old_name"`
	NewName       string `json:"new_name"`
	NewPosition   string `json:"new_position,omitempty"`
	Action        string `json:"action"` // "rename", "merge_into", "flag"
	MergeTargetID *int   `json:"merge_target_id,omitempty"`
	BookCount     int    `json:"book_count"`
}

// seriesNormalizePreviewResult is the response body for the dry-run preview endpoint.
type seriesNormalizePreviewResult struct {
	Actions             []seriesNormalizeAction `json:"actions"`
	TotalSeriesAffected int                     `json:"total_series_affected"`
	TotalBooksAffected  int                     `json:"total_books_affected"`
	FlaggedForReview    []seriesNormalizeAction `json:"flagged_for_review"`
}

// computeSeriesNormalizeActions iterates all series, strips contamination from
// each name, and returns the list of rename / merge_into / flag actions that
// would be taken by a full normalize run. No writes are performed.
func computeSeriesNormalizeActions(store interface {
	database.SeriesStore
	database.BookStore
}) []seriesNormalizeAction {
	allSeries, err := store.GetAllSeries()
	if err != nil {
		return nil
	}

	type groupKey struct {
		name     string
		authorID int
	}
	canonical := make(map[groupKey]int)
	var actions []seriesNormalizeAction

	for _, s := range allSeries {
		cleaned, pos, flagged := metadata.StripSeriesContamination(s.Name, "")

		if flagged {
			books, _ := store.GetBooksBySeriesID(s.ID)
			actions = append(actions, seriesNormalizeAction{
				SeriesID:  s.ID,
				OldName:   s.Name,
				NewName:   s.Name,
				Action:    "flag",
				BookCount: len(books),
			})
			continue
		}

		if cleaned == s.Name && pos == "" {
			continue
		}

		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: strings.ToLower(cleaned), authorID: aid}
		books, _ := store.GetBooksBySeriesID(s.ID)

		if existingID, ok := canonical[key]; ok {
			actions = append(actions, seriesNormalizeAction{
				SeriesID:      s.ID,
				OldName:       s.Name,
				NewName:       cleaned,
				NewPosition:   pos,
				Action:        "merge_into",
				MergeTargetID: &existingID,
				BookCount:     len(books),
			})
		} else {
			canonical[key] = s.ID
			actions = append(actions, seriesNormalizeAction{
				SeriesID:    s.ID,
				OldName:     s.Name,
				NewName:     cleaned,
				NewPosition: pos,
				Action:      "rename",
				BookCount:   len(books),
			})
		}
	}
	return actions
}

// seriesNormalizePreview returns a dry-run preview of what the series
// name-normalization pass would do, with no database writes.
func (s *Server) seriesNormalizePreview(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	actions := computeSeriesNormalizeActions(store)

	flagged := make([]seriesNormalizeAction, 0)
	normal := make([]seriesNormalizeAction, 0)
	totalBooks := 0
	for _, a := range actions {
		if a.Action == "flag" {
			flagged = append(flagged, a)
		} else {
			normal = append(normal, a)
			totalBooks += a.BookCount
		}
	}

	httputil.RespondWithOK(c, seriesNormalizePreviewResult{
		Actions:             normal,
		TotalSeriesAffected: len(normal),
		TotalBooksAffected:  totalBooks,
		FlaggedForReview:    flagged,
	})
}

// mergeSeriesGroupHelper moves all books from each series in mergeIDs to keepID,
// then deletes the now-empty series. Named with "Helper" suffix to avoid
// collision with the (s *Server) mergeSeriesGroup HTTP handler.
func mergeSeriesGroupHelper(store maintenanceStore, keepID int, mergeIDs []int) error {
	for _, fromID := range mergeIDs {
		books, err := store.GetBooksBySeriesID(fromID)
		if err != nil {
			return fmt.Errorf("GetBooksBySeriesID(%d): %w", fromID, err)
		}

		for _, book := range books {
			current, err := store.GetBookByID(book.ID)
			if err != nil {
				return fmt.Errorf("GetBookByID(%s): %w", book.ID, err)
			}
			if current == nil {
				continue
			}

			current.SeriesID = &keepID
			if _, err = store.UpdateBook(book.ID, current); err != nil {
				return fmt.Errorf("UpdateBook(%s): %w", book.ID, err)
			}
		}

		if err = store.DeleteSeries(fromID); err != nil {
			return fmt.Errorf("DeleteSeries(%d): %w", fromID, err)
		}
	}

	return nil
}

// executeSeriesNormalizeCore renames and merges contaminated series, enqueues
// write-back for affected books, and returns the affected book IDs for the
// caller to run organize on.
// maintenanceStore is used because mergeSeriesGroupHelper requires it.
func executeSeriesNormalizeCore(
	ctx context.Context,
	store maintenanceStore,
	enqueueWriteBack func(bookID string),
) (affectedBookIDs []string, err error) {
	actions := computeSeriesNormalizeActions(store)

	// Collect affected book IDs BEFORE renaming/merging.
	seen := make(map[string]bool)
	for _, a := range actions {
		if a.Action == "flag" {
			continue
		}
		books, bErr := store.GetBooksBySeriesID(a.SeriesID)
		if bErr != nil {
			continue
		}
		for _, b := range books {
			if !seen[b.ID] {
				seen[b.ID] = true
				affectedBookIDs = append(affectedBookIDs, b.ID)
			}
		}
	}

	var errs []string

	// First pass: rename.
	for _, a := range actions {
		if a.Action != "rename" {
			continue
		}
		if ctx.Err() != nil {
			return affectedBookIDs, ctx.Err()
		}
		if rErr := store.UpdateSeriesName(a.SeriesID, a.NewName); rErr != nil {
			errs = append(errs, fmt.Sprintf("UpdateSeriesName(%d, %q): %v", a.SeriesID, a.NewName, rErr))
		}
	}

	// Second pass: merge.
	for _, a := range actions {
		if a.Action != "merge_into" || a.MergeTargetID == nil {
			continue
		}
		if ctx.Err() != nil {
			return affectedBookIDs, ctx.Err()
		}
		if mErr := mergeSeriesGroupHelper(store, *a.MergeTargetID, []int{a.SeriesID}); mErr != nil {
			errs = append(errs, fmt.Sprintf("mergeSeriesGroupHelper(keep=%d, merge=%d): %v", *a.MergeTargetID, a.SeriesID, mErr))
		}
	}

	for _, id := range affectedBookIDs {
		enqueueWriteBack(id)
	}

	if len(errs) > 0 {
		return affectedBookIDs, fmt.Errorf("series normalize errors: %s", strings.Join(errs, "; "))
	}
	return affectedBookIDs, nil
}

// seriesNormalize enqueues an async operation that renames/merges contaminated
// series and re-organizes affected books in place.
func (s *Server) seriesNormalize(c *gin.Context) {
	store := s.Store()
	if store == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
		return
	}

	opID := ulid.Make().String()
	detail := "series-normalize"
	op, err := store.CreateOperation(opID, "series-normalize", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	if _, err := s.opRegistry.EnqueueOp(c.Request.Context(), "dedup.series-normalize", seriesNormalizeOpParams{LegacyOpID: op.ID}); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, 202, op)
}
