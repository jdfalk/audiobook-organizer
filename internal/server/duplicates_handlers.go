// file: internal/server/duplicates_handlers.go
// version: 1.2.0
// guid: 47a3e3fb-f5cf-4970-a2fc-d2ef481368c9
//
// SQL-backed duplicate detection handlers split out of server.go:
// find, list, merge, skip, dismiss, pair-level actions, and series
// prune. Distinct from the embedding-based dedup flow living in
// dedup_handlers.go.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/merge"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

func (s *Server) listDuplicateAudiobooks(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-duplicates"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	result, err := s.audiobookService.GetDuplicateBooks(c.Request.Context())
	if err != nil {
		internalError(c, "failed to list duplicate audiobooks", err)
		return
	}

	resp := gin.H{
		"groups":          result.Groups,
		"group_count":     result.GroupCount,
		"duplicate_count": result.DuplicateCount,
	}
	s.dedupCache.Set("book-duplicates", resp)
	c.JSON(http.StatusOK, resp)
}

// listBookDuplicateScanResults returns cached results from the last async book-dedup scan.
func (s *Server) listBookDuplicateScanResults(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("book-dedup-scan"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "group_count": 0, "duplicate_count": 0, "needs_refresh": true})
}

// scanBookDuplicates triggers an async scan for book duplicates using metadata matching.
func (s *Server) scanBookDuplicates(c *gin.Context) {
	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "book-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Scanning for duplicate books...")

		// Step 1: Hash-based duplicates (high confidence)
		_ = progress.UpdateProgress(10, 100, "Finding hash-based duplicates...")
		hashGroups, err := store.GetDuplicateBooks()
		if err != nil {
			return fmt.Errorf("hash-based dedup failed: %w", err)
		}

		// Step 2: Folder duplicates (same title in same folder)
		_ = progress.UpdateProgress(30, 100, "Finding folder-based duplicates...")
		folderGroups, err := store.GetFolderDuplicates()
		if err != nil {
			log.Printf("[WARN] folder dedup failed: %v", err)
			folderGroups = nil
		}

		// Step 3: Metadata-based fuzzy matching
		_ = progress.UpdateProgress(50, 100, "Finding metadata-based duplicates...")
		metadataGroups, err := store.GetDuplicateBooksByMetadata(0.85)
		if err != nil {
			log.Printf("[WARN] metadata dedup failed: %v", err)
			metadataGroups = nil
		}

		_ = progress.UpdateProgress(80, 100, "Merging results...")

		// Load dismissed groups
		dismissed := loadDismissedDedupGroups(store)

		// Combine all groups, deduplicating by book ID
		seenBookIDs := map[string]bool{}
		type dupGroup struct {
			Books      []database.Book `json:"books"`
			Confidence string          `json:"confidence"` // "high", "medium", "low"
			Reason     string          `json:"reason"`
			GroupKey   string          `json:"group_key"`
		}
		var allGroups []dupGroup

		addGroups := func(groups [][]database.Book, confidence, reason string) {
			for _, group := range groups {
				allSeen := true
				for _, b := range group {
					if !seenBookIDs[b.ID] {
						allSeen = false
						break
					}
				}
				if allSeen {
					continue
				}
				// Generate a stable group key from sorted book IDs
				ids := make([]string, len(group))
				for i, b := range group {
					ids[i] = b.ID
				}
				groupKey := strings.Join(ids, "+")
				if dismissed[groupKey] {
					continue
				}
				allGroups = append(allGroups, dupGroup{
					Books:      group,
					Confidence: confidence,
					Reason:     reason,
					GroupKey:   groupKey,
				})
				for _, b := range group {
					seenBookIDs[b.ID] = true
				}
			}
		}

		addGroups(hashGroups, "high", "Identical file hash")
		addGroups(folderGroups, "medium", "Same title in same folder")
		addGroups(metadataGroups, "low", "Similar title and author")

		totalDuplicates := 0
		for _, g := range allGroups {
			totalDuplicates += len(g.Books) - 1
		}

		result := gin.H{
			"groups":          allGroups,
			"group_count":     len(allGroups),
			"duplicate_count": totalDuplicates,
		}
		s.dedupCache.SetWithTTL("book-dedup-scan", result, 30*time.Minute)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups (%d duplicates)", len(allGroups), totalDuplicates))
		return nil
	}

	if err := s.queue.Enqueue(opID, "book-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// mergeBookDuplicatesAsVersions merges a group of duplicate books into a version group.
func (s *Server) mergeBookDuplicatesAsVersions(c *gin.Context) {
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "need at least 2 book IDs"})
		return
	}

	ms := s.mergeService
	if ms == nil {
		ms = merge.NewService(s.Store())
	}

	result, err := ms.MergeBooks(req.BookIDs, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			internalError(c, "failed to merge duplicate books", err)
		}
		return
	}

	s.dedupCache.Invalidate("book-dedup-scan")
	s.dedupCache.Invalidate("book-duplicates")

	c.JSON(http.StatusOK, gin.H{
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Load existing dismissed groups
	dismissed := loadDismissedDedupGroups(store)
	dismissed[req.GroupKey] = true
	saveDismissedDedupGroups(store, dismissed)

	s.dedupCache.Invalidate("book-dedup-scan")

	c.JSON(http.StatusOK, gin.H{"message": "Group dismissed"})
}

func (s *Server) mergeBooks(c *gin.Context) {
	var req struct {
		KeepID   string   `json:"keep_id" binding:"required"`
		MergeIDs []string `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := s.Store()
	keepBook, err := store.GetBookByID(req.KeepID)
	if err != nil || keepBook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep book not found"})
		return
	}

	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-books:keep=%s,merge=%d", req.KeepID, len(req.MergeIDs))
	op, err := store.CreateOperation(opID, "book-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", fmt.Sprintf("Merging %d book(s) into \"%s\"", len(mergeIDs), keepBook.Title), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting book merge...")

		kBook, err := store.GetBookByID(keepID)
		if err != nil || kBook == nil {
			return fmt.Errorf("keep book %s not found", keepID)
		}

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			mergeBook, err := store.GetBookByID(mergeID)
			if err != nil || mergeBook == nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("book %s not found", mergeID))
				continue
			}

			// Transfer useful metadata
			if (kBook.ITunesPersistentID == nil || *kBook.ITunesPersistentID == "") &&
				mergeBook.ITunesPersistentID != nil && *mergeBook.ITunesPersistentID != "" {
				kBook.ITunesPersistentID = mergeBook.ITunesPersistentID
			}
			if kBook.ITunesPlayCount == nil && mergeBook.ITunesPlayCount != nil {
				kBook.ITunesPlayCount = mergeBook.ITunesPlayCount
			}
			if kBook.ITunesRating == nil && mergeBook.ITunesRating != nil {
				kBook.ITunesRating = mergeBook.ITunesRating
			}
			if kBook.ITunesDateAdded == nil && mergeBook.ITunesDateAdded != nil {
				kBook.ITunesDateAdded = mergeBook.ITunesDateAdded
			}
			if kBook.ITunesLastPlayed == nil && mergeBook.ITunesLastPlayed != nil {
				kBook.ITunesLastPlayed = mergeBook.ITunesLastPlayed
			}
			if kBook.ITunesBookmark == nil && mergeBook.ITunesBookmark != nil {
				kBook.ITunesBookmark = mergeBook.ITunesBookmark
			}

			if err := store.DeleteBook(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete book %s: %v", mergeID, err))
			} else {
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      mergeID,
					ChangeType:  "book_delete",
					FieldName:   "book",
					OldValue:    fmt.Sprintf("%s (%s)", mergeBook.Title, mergeBook.FilePath),
					NewValue:    fmt.Sprintf("merged_into:%s", keepID),
				})
				merged++
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d books", i+1, len(mergeIDs)))
		}

		if _, err := store.UpdateBook(kBook.ID, kBook); err != nil {
			mergeErrors = append(mergeErrors, fmt.Sprintf("failed to update keep book: %v", err))
		}

		resultMsg := fmt.Sprintf("Book merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := s.queue.Enqueue(op.ID, "book-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) listDuplicateAuthors(c *gin.Context) {
	if cached, ok := s.dedupCache.Get("author-duplicates"); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "count": 0, "needs_refresh": true})
}

func (s *Server) refreshDuplicateAuthors(c *gin.Context) {
	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "author-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Fetching authors...")

		authors, err := store.GetAllAuthors()
		if err != nil {
			return err
		}
		_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Loaded %d authors, fetching book counts...", len(authors)))

		bookCounts, err := store.GetAllAuthorBookCounts()
		if err != nil {
			return err
		}
		bookCountFn := func(authorID int) int { return bookCounts[authorID] }
		_ = progress.UpdateProgress(20, 100, "Finding duplicate authors...")

		progressFn := func(current, total int, message string) {
			// Map author comparison progress to 20-90% range
			pct := 20 + (current*70)/max(total, 1)
			_ = progress.UpdateProgress(pct, 100, message)
		}

		groups := FindDuplicateAuthors(authors, 0.9, bookCountFn, progressFn)

		// Filter out groups already reviewed by AI scans
		groups = s.filterReviewedAuthorGroups(groups)

		result := gin.H{"groups": groups, "count": len(groups)}
		s.dedupCache.SetWithTTL("author-duplicates", result, 30*time.Minute)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups (after filtering reviewed)", len(groups)))
		return nil
	}

	if err := s.queue.Enqueue(opID, "author-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// filterReviewedAuthorGroups removes author dedup groups where all author IDs
// have already been reviewed via AI scans (applied results with skip/split/merge).
func (s *Server) filterReviewedAuthorGroups(groups []AuthorDedupGroup) []AuthorDedupGroup {
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
	filtered := make([]AuthorDedupGroup, 0, len(groups))
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
		c.JSON(http.StatusOK, cached)
		return
	}

	// No cache — return empty with needs_refresh flag so frontend triggers async scan
	c.JSON(http.StatusOK, gin.H{"groups": []any{}, "count": 0, "total_series": 0, "needs_refresh": true})
}

func (s *Server) refreshSeriesDuplicates(c *gin.Context) {
	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "series-dedup-scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.UpdateProgress(0, 100, "Fetching series...")

		allSeries, err := store.GetAllSeries()
		if err != nil {
			return err
		}
		_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Loaded %d series, grouping...", len(allSeries)))

		// Reuse the same logic from listSeriesDuplicates
		isGarbageSeries := func(name string) bool {
			trimmed := strings.TrimSpace(name)
			if len(trimmed) == 0 {
				return true
			}
			for _, r := range trimmed {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}

		exactGroups := make(map[string][]database.Series)
		for _, s := range allSeries {
			if isGarbageSeries(s.Name) {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(s.Name))
			exactGroups[key] = append(exactGroups[key], s)
		}

		_ = progress.UpdateProgress(20, 100, "Building author lookup...")

		type seriesBookSummary struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			CoverURL string `json:"cover_url,omitempty"`
		}
		type seriesWithBooks struct {
			database.Series
			Books      []seriesBookSummary `json:"books"`
			AuthorName string              `json:"author_name,omitempty"`
		}

		allAuthors, _ := store.GetAllAuthors()
		authorNameMap := make(map[int]string, len(allAuthors))
		for _, a := range allAuthors {
			authorNameMap[a.ID] = a.Name
		}

		type seriesDupGroup struct {
			Name          string            `json:"name"`
			Count         int               `json:"count"`
			Series        []seriesWithBooks `json:"series"`
			SuggestedName string            `json:"suggested_name,omitempty"`
			MatchType     string            `json:"match_type"`
		}

		enrichSeries := func(seriesList []database.Series) []seriesWithBooks {
			result := make([]seriesWithBooks, 0, len(seriesList))
			for _, s := range seriesList {
				authorName := ""
				if s.AuthorID != nil {
					authorName = authorNameMap[*s.AuthorID]
				}
				sw := seriesWithBooks{Series: s, AuthorName: authorName}
				if books, err := store.GetBooksBySeriesID(s.ID); err == nil {
					limit := 5
					if len(books) < limit {
						limit = len(books)
					}
					for _, b := range books[:limit] {
						cover := ""
						if b.CoverURL != nil {
							cover = *b.CoverURL
						}
						sw.Books = append(sw.Books, seriesBookSummary{
							ID:       b.ID,
							Title:    b.Title,
							CoverURL: cover,
						})
					}
				}
				result = append(result, sw)
			}
			return result
		}

		var result []seriesDupGroup
		seen := make(map[int]bool)

		_ = progress.UpdateProgress(30, 100, "Finding exact duplicates...")

		groupKeys := make([]string, 0, len(exactGroups))
		for k := range exactGroups {
			groupKeys = append(groupKeys, k)
		}

		processed := 0
		totalGroups := len(groupKeys)
		for _, k := range groupKeys {
			group := exactGroups[k]
			if len(group) < 2 {
				continue
			}
			for _, s := range group {
				seen[s.ID] = true
			}
			suggested, _ := extractSeriesNameForDedup(group[0].Name)
			result = append(result, seriesDupGroup{
				Name:          group[0].Name,
				Count:         len(group),
				Series:        enrichSeries(group),
				SuggestedName: suggested,
				MatchType:     "exact",
			})
			processed++
			if processed%10 == 0 {
				pct := 30 + (processed*40)/max(totalGroups, 1)
				_ = progress.UpdateProgress(min(pct, 70), 100, fmt.Sprintf("Processing groups... (%d/%d)", processed, totalGroups))
			}
		}

		_ = progress.UpdateProgress(70, 100, "Finding sub-series patterns...")

		seriesByNormalizedName := make(map[string][]database.Series)
		for _, s := range allSeries {
			seriesByNormalizedName[strings.ToLower(strings.TrimSpace(s.Name))] = append(
				seriesByNormalizedName[strings.ToLower(strings.TrimSpace(s.Name))], s)
		}

		for _, s := range allSeries {
			if seen[s.ID] || isGarbageSeries(s.Name) {
				continue
			}
			suggested, ok := extractSeriesNameForDedup(s.Name)
			if !ok {
				continue
			}
			suggestedKey := strings.ToLower(strings.TrimSpace(suggested))
			if matches, exists := seriesByNormalizedName[suggestedKey]; exists {
				group := []database.Series{s}
				seen[s.ID] = true
				for _, m := range matches {
					if !seen[m.ID] {
						group = append(group, m)
						seen[m.ID] = true
					}
				}
				if len(group) >= 2 {
					result = append(result, seriesDupGroup{
						Name:          s.Name,
						Count:         len(group),
						Series:        enrichSeries(group),
						SuggestedName: suggested,
						MatchType:     "subseries",
					})
				}
			}
		}

		resp := gin.H{"groups": result, "count": len(result), "total_series": len(allSeries)}
		s.dedupCache.Set("series-duplicates", resp)

		_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups", len(result)))
		return nil
	}

	if err := s.queue.Enqueue(opID, "series-dedup-scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// validateDedupEntry searches metadata sources (OpenLibrary, Audible, etc.) to validate
// a series name, author name, or book title during dedup review.
func (s *Server) validateDedupEntry(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
		Type  string `json:"type"` // "series", "author", "book"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.Type == "" {
		req.Type = "series"
	}

	chain := s.metadataFetchService.BuildSourceChain()
	if len(chain) == 0 {
		c.JSON(http.StatusOK, gin.H{"results": []interface{}{}, "message": "no metadata sources configured"})
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
	for _, src := range chain {
		matches, err := src.SearchByTitle(req.Query)
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
	c.JSON(http.StatusOK, gin.H{"results": results, "query": req.Query, "type": req.Type})
}

func (s *Server) deduplicateSeriesHandler(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := "series-deduplicate"
	op, err := store.CreateOperation(opID, "series-dedup", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", "Starting series deduplication...", nil)

		allSeries, err := store.GetAllSeries()
		if err != nil {
			return fmt.Errorf("failed to get series: %w", err)
		}

		_ = progress.UpdateProgress(0, len(allSeries), fmt.Sprintf("Scanning %d series for duplicates...", len(allSeries)))

		// Group by normalized name only
		groups := make(map[string][]database.Series)
		for _, s := range allSeries {
			key := strings.ToLower(strings.TrimSpace(s.Name))
			groups[key] = append(groups[key], s)
		}

		// Count total duplicate groups
		var dupGroups [][]database.Series
		for _, group := range groups {
			if len(group) >= 2 {
				dupGroups = append(dupGroups, group)
			}
		}

		msg := fmt.Sprintf("Found %d duplicate groups to merge", len(dupGroups))
		_ = progress.Log("info", msg, nil)
		_ = progress.UpdateProgress(0, len(dupGroups), msg)

		totalMerged := 0
		var mergeErrors []string
		for gi, group := range dupGroups {
			if progress.IsCanceled() {
				_ = progress.Log("warn", "Operation cancelled by user", nil)
				return fmt.Errorf("cancelled")
			}

			keepIdx := 0
			for i, s := range group {
				if s.AuthorID != nil && group[keepIdx].AuthorID == nil {
					keepIdx = i
				} else if (s.AuthorID != nil) == (group[keepIdx].AuthorID != nil) && s.ID < group[keepIdx].ID {
					keepIdx = i
				}
			}
			keepID := group[keepIdx].ID

			for i, s := range group {
				if i == keepIdx {
					continue
				}
				books, err := store.GetBooksBySeriesID(s.ID)
				if err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", s.ID, err))
					continue
				}
				for _, book := range books {
					book.SeriesID = &keepID
					if _, err := store.UpdateBook(book.ID, &book); err != nil {
						mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
					}
				}
				if err := store.DeleteSeries(s.ID); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", s.ID, err))
				} else {
					totalMerged++
				}
			}

			_ = progress.UpdateProgress(gi+1, len(dupGroups),
				fmt.Sprintf("Merged %d/%d groups (%d series merged)", gi+1, len(dupGroups), totalMerged))
		}

		resultMsg := fmt.Sprintf("Series deduplication complete: merged %d duplicates, %d errors", totalMerged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Merge errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := s.queue.Enqueue(op.ID, "series-dedup", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) seriesPrunePreview(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	preview, err := computeSeriesPrunePreview(store)
	if err != nil {
		internalError(c, "failed to compute series prune preview", err)
		return
	}

	c.JSON(http.StatusOK, preview)
}

func (s *Server) seriesPrune(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := "series-prune"
	op, err := store.CreateOperation(opID, "series-prune", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.executeSeriesPrune(ctx, store, progress, op.ID)
	}

	if err := s.queue.Enqueue(op.ID, "series-prune", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// executeSeriesPrune performs the actual series prune logic (used by both HTTP handler and scheduler).
func (s *Server) executeSeriesPrune(ctx context.Context, store interface { database.BookStore; database.AuthorStore; database.SeriesStore; database.OperationStore }, progress operations.ProgressReporter, operationID string) error {
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
		key := groupKey{name: strings.ToLower(strings.TrimSpace(s.Name)), authorID: aid}
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := s.Store()
	keepSeries, err := store.GetSeriesByID(req.KeepID)
	if err != nil || keepSeries == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep series not found"})
		return
	}

	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-series:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "series-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs
	customName := strings.TrimSpace(req.CustomName)
	keepName := keepSeries.Name
	if customName != "" {
		keepName = customName
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		// Rename the kept series if a custom name was provided
		if customName != "" {
			oldName := keepSeries.Name
			if err := store.UpdateSeriesName(keepID, customName); err != nil {
				return fmt.Errorf("failed to rename series to %q: %w", customName, err)
			}
			_ = store.CreateOperationChange(&database.OperationChange{
				ID:          ulid.Make().String(),
				OperationID: opID,
				ChangeType:  "metadata_update",
				FieldName:   "series_name",
				OldValue:    oldName,
				NewValue:    customName,
			})
			_ = progress.Log("info", fmt.Sprintf("Renamed series from %q to %q", oldName, customName), nil)
		}

		_ = progress.Log("info", fmt.Sprintf("Merging %d series into \"%s\"", len(mergeIDs), keepName), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting series merge...")

		// Collect all unique author IDs from all series being merged (including keep)
		allAuthorIDs := make(map[int]bool)
		allSeriesIDs := append([]int{keepID}, mergeIDs...)
		for _, sid := range allSeriesIDs {
			s, err := store.GetSeriesByID(sid)
			if err == nil && s != nil && s.AuthorID != nil {
				allAuthorIDs[*s.AuthorID] = true
			}
		}

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			books, err := store.GetBooksBySeriesID(mergeID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for series %d: %v", mergeID, err))
				continue
			}

			for _, book := range books {
				oldSeriesID := ""
				if book.SeriesID != nil {
					oldSeriesID = fmt.Sprintf("%d", *book.SeriesID)
				}
				book.SeriesID = &keepID
				if _, err := store.UpdateBook(book.ID, &book); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to reassign book %s: %v", book.ID, err))
				} else {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: opID,
						BookID:      book.ID,
						ChangeType:  "metadata_update",
						FieldName:   "series_id",
						OldValue:    oldSeriesID,
						NewValue:    fmt.Sprintf("%d", keepID),
					})
				}
			}

			// Record the series deletion
			mergeSeries, _ := store.GetSeriesByID(mergeID)
			mergeSeriesName := ""
			if mergeSeries != nil {
				mergeSeriesName = mergeSeries.Name
			}
			if err := store.DeleteSeries(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete series %d: %v", mergeID, err))
			} else {
				merged++
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      "",
					ChangeType:  "series_delete",
					FieldName:   "series",
					OldValue:    fmt.Sprintf("%d:%s", mergeID, mergeSeriesName),
					NewValue:    fmt.Sprintf("merged_into:%d", keepID),
				})
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d series", i+1, len(mergeIDs)))
		}

		// Link all books in the kept series to all unique authors
		if len(allAuthorIDs) > 1 {
			_ = progress.Log("info", fmt.Sprintf("Linking books to %d authors", len(allAuthorIDs)), nil)
			allBooks, err := store.GetBooksBySeriesID(keepID)
			if err == nil {
				for _, book := range allBooks {
					existing, _ := store.GetBookAuthors(book.ID)
					existingMap := make(map[int]bool)
					for _, ba := range existing {
						existingMap[ba.AuthorID] = true
					}
					authors := existing
					var addedAuthors []int
					for aid := range allAuthorIDs {
						if !existingMap[aid] {
							authors = append(authors, database.BookAuthor{BookID: book.ID, AuthorID: aid})
							addedAuthors = append(addedAuthors, aid)
						}
					}
					if len(authors) > len(existing) {
						if err := store.SetBookAuthors(book.ID, authors); err != nil {
							mergeErrors = append(mergeErrors, fmt.Sprintf("failed to set authors for book %s: %v", book.ID, err))
						} else {
							_ = store.CreateOperationChange(&database.OperationChange{
								ID:          ulid.Make().String(),
								OperationID: opID,
								BookID:      book.ID,
								ChangeType:  "author_link",
								FieldName:   "book_authors",
								OldValue:    fmt.Sprintf("%d authors", len(existing)),
								NewValue:    fmt.Sprintf("%d authors (added %v)", len(authors), addedAuthors),
							})
						}
					}
				}
			}
		}

		resultMsg := fmt.Sprintf("Series merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := s.queue.Enqueue(op.ID, "series-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}
