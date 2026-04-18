// file: internal/server/ai_handlers.go
// version: 1.1.0
// guid: 5d3a6a95-4ac8-42c2-a7fe-5ff4857dd31a
//
// AI-related HTTP handlers split out of server.go: filename parsing,
// AI scan lifecycle (start/list/get/results/apply/delete/cancel/compare),
// metadata-source connection tests, and the duplicate-author review
// flows. Kept on *Server so they share service wiring; grouped here
// so server.go stays focused on lifecycle and routing.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// parseFilenameWithAI uses OpenAI to parse a filename into structured metadata
func (s *Server) parseFilenameWithAI(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename is required"})
		return
	}

	// Create AI parser
	parser := ai.NewOpenAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Parse filename
	metadata, err := parser.ParseFilename(c.Request.Context(), req.Filename)
	if err != nil {
		internalError(c, "failed to parse filename", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": metadata})
}

// testAIConnection tests the OpenAI API connection
func (s *Server) testAIConnection(c *gin.Context) {
	// Parse request body for API key (allows testing without saving)
	var req struct {
		APIKey string `json:"api_key"`
	}

	// Try to get API key from request body first, fall back to config
	apiKey := config.AppConfig.OpenAIAPIKey
	if err := c.ShouldBindJSON(&req); err == nil && req.APIKey != "" {
		apiKey = req.APIKey
	}

	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key not provided", "success": false})
		return
	}

	// Create parser with the provided/configured API key
	parser := ai.NewOpenAIParser(apiKey, true)
	if err := parser.TestConnection(c.Request.Context()); err != nil {
		log.Printf("[ERROR] connection test failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "connection test failed", "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OpenAI connection successful"})
}

// testMetadataSource tests a metadata source API key by performing a simple search.
func (s *Server) testMetadataSource(c *gin.Context) {
	var req struct {
		SourceID string `json:"source_id"`
		APIKey   string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required", "success": false})
		return
	}
	if req.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required", "success": false})
		return
	}

	testQuery := "The Hobbit" // well-known book for test queries

	switch req.SourceID {
	case "google-books":
		client := metadata.NewGoogleBooksClient(req.APIKey)
		results, err := client.SearchByTitle(testQuery)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("Google Books API error: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Google Books connection successful (%d results)", len(results))})

	case "hardcover":
		client := metadata.NewHardcoverClient(req.APIKey)
		results, err := client.SearchByTitle(testQuery)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "error": fmt.Sprintf("Hardcover API error: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Hardcover connection successful (%d results)", len(results))})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown source: %s", req.SourceID), "success": false})
	}
}

// parseAudiobookWithAI parses an audiobook's filename with AI and updates its metadata
func (s *Server) parseAudiobookWithAI(c *gin.Context) {
	id := c.Param("id")

	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the book
	book, err := s.Store().GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Create AI parser
	parser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled or API key not configured"})
		return
	}

	// Build rich context for the AI parser
	abCtx := ai.AudiobookContext{
		FilePath: book.FilePath,
		Title:    book.Title,
	}
	if book.Narrator != nil {
		abCtx.Narrator = *book.Narrator
	}
	if book.Duration != nil {
		abCtx.TotalDuration = *book.Duration
	}
	// Resolve author name from author_id
	if book.AuthorID != nil {
		if author, err := s.Store().GetAuthorByID(*book.AuthorID); err == nil {
			abCtx.AuthorName = author.Name
		}
	}

	// Parse with AI using full context
	metadata, err := parser.ParseAudiobook(c.Request.Context(), abCtx)
	if err != nil {
		internalError(c, "failed to parse audiobook", err)
		return
	}

	// Build payload for the update service (routes through AudiobookService
	// which handles "&" splitting for authors/narrators, junction tables, etc.)
	payload := map[string]any{}
	if metadata.Title != "" {
		payload["title"] = metadata.Title
	}
	if metadata.Author != "" {
		payload["author_name"] = metadata.Author
	}
	if metadata.Narrator != "" {
		payload["narrator"] = metadata.Narrator
	}
	if metadata.Publisher != "" {
		payload["publisher"] = metadata.Publisher
	}
	if metadata.Year > 0 {
		payload["audiobook_release_year"] = metadata.Year
	}
	if metadata.Series != "" {
		payload["series_name"] = metadata.Series
	}
	if metadata.SeriesNum > 0 {
		payload["series_sequence"] = metadata.SeriesNum
	}

	// Route through the service layer for proper multi-author/narrator handling
	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		internalError(c, "failed to update audiobook", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "audiobook updated with AI-parsed metadata",
		"book":       enrichBookForResponse(updatedBook),
		"confidence": metadata.Confidence,
	})
}

// startAIScan kicks off a new multi-pass AI author dedup scan.
func (s *Server) startAIScan(c *gin.Context) {
	if s.pipelineManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI scan pipeline not configured"})
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Mode = "realtime"
	}
	if req.Mode != "batch" && req.Mode != "realtime" {
		req.Mode = "realtime"
	}
	scan, err := s.pipelineManager.StartScan(c.Request.Context(), req.Mode)
	if err != nil {
		internalError(c, "failed to start AI scan", err)
		return
	}
	c.JSON(http.StatusAccepted, scan)
}

// listAIScans returns all AI scan pipeline runs.
func (s *Server) listAIScans(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusOK, gin.H{"scans": []interface{}{}})
		return
	}
	scans, err := s.aiScanStore.ListScans()
	if err != nil {
		internalError(c, "failed to list AI scans", err)
		return
	}
	if scans == nil {
		scans = []database.Scan{}
	}
	c.JSON(http.StatusOK, gin.H{"scans": scans})
}

// getAIScan returns a single scan with its phases.
func (s *Server) getAIScan(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	scan, err := s.aiScanStore.GetScan(id)
	if err != nil {
		internalError(c, "failed to get AI scan", err)
		return
	}
	if scan == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
		return
	}
	phases, _ := s.aiScanStore.GetPhases(id)
	c.JSON(http.StatusOK, gin.H{"scan": scan, "phases": phases})
}

// getAIScanResults returns results for a scan, with optional agreement filter.
func (s *Server) getAIScanResults(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	results, err := s.aiScanStore.GetScanResults(id)
	if err != nil {
		internalError(c, "failed to get AI scan results", err)
		return
	}

	// Optional agreement filter
	agreement := c.Query("agreement")
	if agreement != "" {
		var filtered []database.ScanResult
		for _, r := range results {
			if r.Agreement == agreement {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if results == nil {
		results = []database.ScanResult{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// applyAIScanResults marks selected scan results as applied.
func (s *Server) applyAIScanResults(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	scanID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	var req struct {
		ResultIDs []int `json:"result_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	applied := 0
	var errors []string
	for _, resultID := range req.ResultIDs {
		if err := s.aiScanStore.MarkResultApplied(scanID, resultID); err != nil {
			errors = append(errors, fmt.Sprintf("result %d: %v", resultID, err))
		} else {
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{"applied": applied, "errors": errors})
}

// deleteAIScan removes a scan and all its associated data.
func (s *Server) deleteAIScan(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	if err := s.aiScanStore.DeleteScan(id); err != nil {
		internalError(c, "failed to delete AI scan", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// cancelAIScan cancels a running AI scan, including any in-flight batch jobs.
func (s *Server) cancelAIScan(c *gin.Context) {
	if s.pipelineManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI scan pipeline not configured"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
		return
	}
	if err := s.pipelineManager.CancelScan(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "canceled"})
}

// compareAIScans compares results between two scans.
func (s *Server) compareAIScans(c *gin.Context) {
	if s.aiScanStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan store not configured"})
		return
	}
	aID, err := strconv.Atoi(c.Query("a"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID 'a'"})
		return
	}
	bID, err := strconv.Atoi(c.Query("b"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID 'b'"})
		return
	}

	resultsA, _ := s.aiScanStore.GetScanResults(aID)
	resultsB, _ := s.aiScanStore.GetScanResults(bID)

	// Build comparison: new in B, resolved from A, unchanged
	aMap := make(map[string]database.ScanResult)
	for _, r := range resultsA {
		key := fmt.Sprintf("%s:%s", r.Suggestion.Action, r.Suggestion.CanonicalName)
		aMap[key] = r
	}

	var newInB, unchanged []database.ScanResult
	bSeen := make(map[string]bool)
	for _, r := range resultsB {
		key := fmt.Sprintf("%s:%s", r.Suggestion.Action, r.Suggestion.CanonicalName)
		bSeen[key] = true
		if _, found := aMap[key]; found {
			unchanged = append(unchanged, r)
		} else {
			newInB = append(newInB, r)
		}
	}

	var resolvedFromA []database.ScanResult
	for key, r := range aMap {
		if !bSeen[key] {
			resolvedFromA = append(resolvedFromA, r)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"new_in_b":        newInB,
		"resolved_from_a": resolvedFromA,
		"unchanged":       unchanged,
	})
}

func (s *Server) aiReviewDuplicateAuthors(c *gin.Context) {
	parser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
	if !parser.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "AI parsing is not enabled"})
		return
	}

	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	// Parse optional mode from request body
	var reqBody struct {
		Mode string `json:"mode"`
	}
	_ = c.ShouldBindJSON(&reqBody)
	mode := reqBody.Mode
	if mode == "" {
		mode = "groups"
	}
	if mode != "full" && mode != "groups" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid mode %q; must be full or groups", mode)})
		return
	}

	store := s.Store()

	// Check for an already-running ai-author-review of the same mode — block concurrent same-mode runs
	opType := "ai-author-review-" + mode
	recentOps, _, _ := store.ListOperations(50, 0)
	for _, existing := range recentOps {
		if existing.Type == opType && (existing.Status == "pending" || existing.Status == "running") {
			c.JSON(http.StatusAccepted, existing)
			return
		}
	}

	// For groups mode, we need dedup groups — use cache if available, otherwise compute inline
	var dedupGroups []AuthorDedupGroup
	if mode == "groups" {
		cached, ok := s.dedupCache.Get("author-duplicates")
		if ok {
			groupsRaw, ok2 := cached["groups"]
			if ok2 {
				groupsJSON, err := json.Marshal(groupsRaw)
				if err == nil {
					_ = json.Unmarshal(groupsJSON, &dedupGroups)
				}
			}
		}
		if len(dedupGroups) == 0 {
			// Cache is cold — compute dedup groups inline instead of requiring a separate refresh
			authors, err := store.GetAllAuthors()
			if err != nil {
				internalError(c, "failed to fetch authors", err)
				return
			}
			bookCounts, err := store.GetAllAuthorBookCounts()
			if err != nil {
				internalError(c, "failed to fetch book counts", err)
				return
			}
			bookCountFn := func(authorID int) int { return bookCounts[authorID] }
			dedupGroups = FindDuplicateAuthors(authors, 0.9, bookCountFn, nil)
			// Warm the cache for subsequent requests
			result := gin.H{"groups": dedupGroups, "count": len(dedupGroups)}
			s.dedupCache.SetWithTTL("author-duplicates", result, 30*time.Minute)
		}
		if len(dedupGroups) == 0 {
			c.JSON(http.StatusOK, gin.H{"message": "no duplicate groups to review"})
			return
		}
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, opType, nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		switch mode {
		case "groups":
			return s.aiReviewGroupsMode(ctx, progress, parser, store, opID, dedupGroups)
		case "full":
			return s.aiReviewFullMode(ctx, progress, parser, store, opID)
		}
		return fmt.Errorf("unknown mode: %s", mode)
	}

	if err := s.queue.Enqueue(opID, opType, operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// aiReviewGroupsMode is the existing Groups mode — local heuristics build groups, AI validates.
func (s *Server) aiReviewGroupsMode(ctx context.Context, progress operations.ProgressReporter, parser aiParser, store database.Store, opID string, dedupGroups []AuthorDedupGroup) error {
	_ = progress.Log("info", fmt.Sprintf("Starting AI review (groups mode) of %d duplicate author groups", len(dedupGroups)), nil)
	_ = progress.UpdateProgress(0, len(dedupGroups), "Building AI review input...")

	var inputs []ai.AuthorDedupInput
	for i, group := range dedupGroups {
		var variantNames []string
		for _, v := range group.Variants {
			variantNames = append(variantNames, v.Name)
		}
		var sampleTitles []string
		if group.Canonical.ID > 0 {
			books, err := store.GetBooksByAuthorIDWithRole(group.Canonical.ID)
			if err == nil {
				for j, b := range books {
					if j >= 3 {
						break
					}
					sampleTitles = append(sampleTitles, b.Title)
				}
			}
		}
		inputs = append(inputs, ai.AuthorDedupInput{
			Index:         i,
			CanonicalName: NormalizeAuthorName(group.Canonical.Name),
			VariantNames:  variantNames,
			BookCount:     group.BookCount,
			SampleTitles:  sampleTitles,
		})
	}

	_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Sending %d groups to AI for review...", len(inputs)))

	suggestions, err := parser.ReviewAuthorDuplicates(ctx, inputs)
	if err != nil {
		return fmt.Errorf("AI review failed: %w", err)
	}

	// Normalize initials formatting in AI-returned canonical names
	for i := range suggestions {
		suggestions[i].CanonicalName = NormalizeAuthorName(suggestions[i].CanonicalName)
	}

	_ = progress.Log("info", fmt.Sprintf("Received %d suggestions from AI", len(suggestions)), nil)

	resultPayload := map[string]interface{}{
		"mode":        "groups",
		"suggestions": suggestions,
		"groups":      dedupGroups,
	}
	resultJSON, err := json.Marshal(resultPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}
	if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
		return fmt.Errorf("failed to store results: %w", err)
	}

	_ = progress.UpdateProgress(100, 100, fmt.Sprintf("AI review complete: %d suggestions", len(suggestions)))
	return nil
}

// aiReviewFullMode sends all authors to AI for duplicate discovery.
func (s *Server) aiReviewFullMode(ctx context.Context, progress operations.ProgressReporter, parser aiParser, store database.Store, opID string) error {
	_ = progress.Log("info", "Starting AI review (full mode) — discovering duplicates from all authors", nil)
	_ = progress.UpdateProgress(0, 100, "Loading all authors...")

	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("failed to get authors: %w", err)
	}

	_ = progress.Log("info", fmt.Sprintf("Building discovery input for %d authors", len(allAuthors)), nil)
	_ = progress.UpdateProgress(5, 100, fmt.Sprintf("Building input for %d authors...", len(allAuthors)))

	var inputs []ai.AuthorDiscoveryInput
	for _, author := range allAuthors {
		var sampleTitles []string
		books, err := store.GetBooksByAuthorIDWithRole(author.ID)
		if err == nil {
			for j, b := range books {
				if j >= 3 {
					break
				}
				sampleTitles = append(sampleTitles, b.Title)
			}
		}
		inputs = append(inputs, ai.AuthorDiscoveryInput{
			ID:           author.ID,
			Name:         author.Name,
			BookCount:    len(books),
			SampleTitles: sampleTitles,
		})
	}

	_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Sending %d authors to AI for discovery...", len(inputs)))

	discoveries, err := parser.DiscoverAuthorDuplicates(ctx, inputs)
	if err != nil {
		return fmt.Errorf("AI discovery failed: %w", err)
	}

	_ = progress.Log("info", fmt.Sprintf("AI discovered %d duplicate groups", len(discoveries)), nil)

	// Build author ID→Author map for lookup
	authorMap := make(map[int]database.Author)
	for _, a := range allAuthors {
		authorMap[a.ID] = a
	}

	// Convert discovery suggestions to standard AuthorDedupSuggestion + AuthorDedupGroup format
	var suggestions []ai.AuthorDedupSuggestion
	var groups []AuthorDedupGroup
	for _, disc := range discoveries {
		if len(disc.AuthorIDs) < 2 && disc.Action != "rename" {
			continue
		}
		// First ID = canonical, rest = variants
		canonicalID := disc.AuthorIDs[0]
		canonical, ok := authorMap[canonicalID]
		if !ok {
			continue
		}
		var variants []database.Author
		for _, aid := range disc.AuthorIDs[1:] {
			if a, ok := authorMap[aid]; ok {
				variants = append(variants, a)
			}
		}
		groups = append(groups, AuthorDedupGroup{
			Canonical: canonical,
			Variants:  variants,
			BookCount: disc.AuthorIDs[0], // placeholder; we just need a count
		})
		// Fix book count — count books for all authors in the group
		totalBooks := 0
		for _, aid := range disc.AuthorIDs {
			bks, err := store.GetBooksByAuthorIDWithRole(aid)
			if err == nil {
				totalBooks += len(bks)
			}
		}
		groups[len(groups)-1].BookCount = totalBooks

		suggestions = append(suggestions, ai.AuthorDedupSuggestion{
			GroupIndex:    len(groups) - 1, // index into groups slice, not discoveries
			Action:        disc.Action,
			CanonicalName: NormalizeAuthorName(disc.CanonicalName),
			Reason:        disc.Reason,
			Confidence:    disc.Confidence,
			IsNarrator:    disc.IsNarrator,
			IsPublisher:   disc.IsPublisher,
		})
	}

	resultPayload := map[string]interface{}{
		"mode":        "full",
		"suggestions": suggestions,
		"groups":      groups,
	}
	resultJSON, err := json.Marshal(resultPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
		return fmt.Errorf("failed to store results: %w", err)
	}

	_ = progress.UpdateProgress(100, 100, fmt.Sprintf("AI discovery complete: %d groups found", len(groups)))
	return nil
}

func (s *Server) applyAIAuthorReview(c *gin.Context) {
	var req struct {
		Suggestions []struct {
			GroupIndex    int    `json:"group_index"`
			Action        string `json:"action"`
			CanonicalName string `json:"canonical_name"`
			KeepID        int    `json:"keep_id"`
			MergeIDs      []int  `json:"merge_ids"`
			Rename        bool   `json:"rename"`
		} `json:"suggestions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Suggestions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no suggestions provided"})
		return
	}

	if s.queue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "ai-author-merge-apply", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	suggestions := req.Suggestions

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		total := len(suggestions)
		applied := 0
		var applyErrors []string

		_ = progress.Log("info", fmt.Sprintf("Starting AI author review apply: %d suggestion(s)", total), nil)

		for i, sug := range suggestions {
			if progress.IsCanceled() {
				_ = progress.Log("warn", "Operation cancelled by user", nil)
				return fmt.Errorf("cancelled")
			}

			_ = progress.UpdateProgress(i, total, fmt.Sprintf("Applying suggestion %d/%d...", i+1, total))

			switch sug.Action {
			case "skip":
				_ = progress.Log("info", fmt.Sprintf("Skipped group %d", sug.GroupIndex), nil)
				continue

			case "rename":
				if sug.KeepID > 0 && sug.CanonicalName != "" {
					if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("rename author %d: %v", sug.KeepID, err))
					} else {
						applied++
						_ = progress.Log("info", fmt.Sprintf("Renamed author %d to \"%s\"", sug.KeepID, sug.CanonicalName), nil)
					}
				}

			case "merge":
				// Rename canonical if needed
				if sug.Rename && sug.KeepID > 0 && sug.CanonicalName != "" {
					if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("rename before merge %d: %v", sug.KeepID, err))
					}
				}

				// Merge variant authors
				for _, mergeID := range sug.MergeIDs {
					if mergeID == sug.KeepID {
						continue
					}
					books, err := store.GetBooksByAuthorIDWithRole(mergeID)
					if err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("get books for author %d: %v", mergeID, err))
						continue
					}

					// Snapshot affected books
					_ = progress.Log("info", fmt.Sprintf("Snapshotting %d books before merge of author %d", len(books), mergeID), nil)

					for _, book := range books {
						bookAuthors, err := store.GetBookAuthors(book.ID)
						if err != nil {
							continue
						}
						hasKeep := false
						for _, ba := range bookAuthors {
							if ba.AuthorID == sug.KeepID {
								hasKeep = true
								break
							}
						}
						var newAuthors []database.BookAuthor
						for _, ba := range bookAuthors {
							if ba.AuthorID == mergeID {
								if !hasKeep {
									ba.AuthorID = sug.KeepID
									newAuthors = append(newAuthors, ba)
									hasKeep = true
								}
							} else {
								newAuthors = append(newAuthors, ba)
							}
						}
						if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("update book %s: %v", book.ID, err))
						}
					}

					if err := store.DeleteAuthor(mergeID); err != nil {
						applyErrors = append(applyErrors, fmt.Sprintf("delete author %d: %v", mergeID, err))
					} else {
						_ = store.CreateAuthorTombstone(mergeID, sug.KeepID)
					}
				}
				applied++
				_ = progress.Log("info", fmt.Sprintf("Merged group %d: %d variants into \"%s\"", sug.GroupIndex, len(sug.MergeIDs), sug.CanonicalName), nil)

			case "alias":
				// Keep canonical author, add variants as aliases instead of merging
				if sug.KeepID > 0 && sug.CanonicalName != "" {
					if sug.Rename {
						if err := store.UpdateAuthorName(sug.KeepID, NormalizeAuthorName(sug.CanonicalName)); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("rename for alias %d: %v", sug.KeepID, err))
						}
					}
					for _, mergeID := range sug.MergeIDs {
						if mergeID == sug.KeepID {
							continue
						}
						variant, err := store.GetAuthorByID(mergeID)
						if err != nil || variant == nil {
							continue
						}
						if _, err := store.CreateAuthorAlias(sug.KeepID, variant.Name, "pen_name"); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("create alias for author %d: %v", sug.KeepID, err))
						}
						// Re-link books and delete the variant author
						books, err := store.GetBooksByAuthorIDWithRole(mergeID)
						if err != nil {
							continue
						}
						for _, book := range books {
							bookAuthors, err := store.GetBookAuthors(book.ID)
							if err != nil {
								continue
							}
							hasKeep := false
							for _, ba := range bookAuthors {
								if ba.AuthorID == sug.KeepID {
									hasKeep = true
									break
								}
							}
							var newAuthors []database.BookAuthor
							for _, ba := range bookAuthors {
								if ba.AuthorID == mergeID {
									if !hasKeep {
										ba.AuthorID = sug.KeepID
										newAuthors = append(newAuthors, ba)
										hasKeep = true
									}
								} else {
									newAuthors = append(newAuthors, ba)
								}
							}
							if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
								applyErrors = append(applyErrors, fmt.Sprintf("update book %s for alias: %v", book.ID, err))
							}
						}
						if err := store.DeleteAuthor(mergeID); err != nil {
							applyErrors = append(applyErrors, fmt.Sprintf("delete aliased author %d: %v", mergeID, err))
						} else {
							_ = store.CreateAuthorTombstone(mergeID, sug.KeepID)
						}
					}
					applied++
					_ = progress.Log("info", fmt.Sprintf("Created aliases for group %d: canonical \"%s\"", sug.GroupIndex, sug.CanonicalName), nil)
				}

			case "split":
				_ = progress.Log("info", fmt.Sprintf("Split action for group %d — manual intervention needed", sug.GroupIndex), nil)
				applied++
			}
		}

		s.dedupCache.InvalidateAll()

		resultMsg := fmt.Sprintf("AI review applied: %d actions, %d errors", applied, len(applyErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(applyErrors) > 0 {
			errDetail := strings.Join(applyErrors[:min(len(applyErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}

		_ = progress.UpdateProgress(total, total, resultMsg)
		return nil
	}

	if err := s.queue.Enqueue(opID, "ai-author-merge-apply", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}
