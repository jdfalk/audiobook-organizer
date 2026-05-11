// file: internal/server/duplicates_ops.go
// version: 1.0.0
// guid: 8b3e1f92-d4c7-4a6e-b5f0-2a7c9d1e3f45

// duplicates_ops registers v2 OperationDefs for the 8 async dedup operations
// that previously used s.queue.Enqueue. HTTP handlers in duplicates_handlers.go
// create v1 op records for backward compatibility and then enqueue these defs.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	opsregistry "github.com/jdfalk/audiobook-organizer/internal/operations/registry"
	"github.com/jdfalk/audiobook-organizer/internal/util"
	ulid "github.com/oklog/ulid/v2"
)

// ── param structs ─────────────────────────────────────────────────────────────

type bookDedupScanOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

type bookMergeOpParams struct {
	LegacyOpID string   `json:"legacy_op_id"`
	KeepID     string   `json:"keep_id"`
	MergeIDs   []string `json:"merge_ids"`
	Detail     string   `json:"detail"`
}

type authorDedupScanOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

type seriesDedupScanOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

type seriesDedupOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	Detail     string `json:"detail"`
}

type seriesPruneOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	Detail     string `json:"detail"`
}

type seriesMergeOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
	KeepID     int    `json:"keep_id"`
	MergeIDs   []int  `json:"merge_ids"`
	CustomName string `json:"custom_name"`
	Detail     string `json:"detail"`
}

type seriesNormalizeOpParams struct {
	LegacyOpID string `json:"legacy_op_id"`
}

// ── OperationDef registrations ────────────────────────────────────────────────

// RegisterBookDedupScanOp registers the "dedup.book-scan" v2 OperationDef.
func (s *Server) RegisterBookDedupScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.book-scan",
		Plugin:          "dedup",
		DisplayName:     "Book Duplicate Scan",
		Description:     "Scan all audiobooks for duplicates using hash, folder, and metadata-based matching.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.book-scan",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p bookDedupScanOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.book-scan: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.book-scan: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}

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
		},
	})
}

// RegisterBookMergeOp registers the "dedup.book-merge" v2 OperationDef.
func (s *Server) RegisterBookMergeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.book-merge",
		Plugin:          "dedup",
		DisplayName:     "Book Merge",
		Description:     "Merge a set of duplicate audiobooks, keeping one and deleting the others.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.book-merge",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p bookMergeOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.book-merge: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.book-merge: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}
			opID := p.LegacyOpID
			keepID := p.KeepID
			mergeIDs := p.MergeIDs

			keepBook, err := store.GetBookByID(keepID)
			if err != nil || keepBook == nil {
				return fmt.Errorf("keep book %s not found", keepID)
			}

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
		},
	})
}

// RegisterAuthorDedupScanOp registers the "dedup.author-scan" v2 OperationDef.
func (s *Server) RegisterAuthorDedupScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.author-scan",
		Plugin:          "dedup",
		DisplayName:     "Author Duplicate Scan",
		Description:     "Scan all authors for duplicates using fuzzy name matching.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.author-scan",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p authorDedupScanOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.author-scan: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.author-scan: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}

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

			groups := dedup.FindDuplicateAuthors(authors, 0.9, bookCountFn, progressFn)

			// Filter out groups already reviewed by AI scans
			groups = s.filterReviewedAuthorGroups(groups)

			result := gin.H{"groups": groups, "count": len(groups)}
			s.dedupCache.SetWithTTL("author-duplicates", result, 30*time.Minute)

			_ = progress.UpdateProgress(100, 100, fmt.Sprintf("Found %d duplicate groups (after filtering reviewed)", len(groups)))
			return nil
		},
	})
}

// RegisterSeriesDedupScanOp registers the "dedup.series-scan" v2 OperationDef.
func (s *Server) RegisterSeriesDedupScanOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.series-scan",
		Plugin:          "dedup",
		DisplayName:     "Series Duplicate Scan",
		Description:     "Scan all series for duplicates using exact and sub-series matching.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.series-scan",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p seriesDedupScanOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.series-scan: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.series-scan: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}

			_ = progress.UpdateProgress(0, 100, "Fetching series...")

			allSeries, err := store.GetAllSeries()
			if err != nil {
				return err
			}
			_ = progress.UpdateProgress(10, 100, fmt.Sprintf("Loaded %d series, grouping...", len(allSeries)))

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
				key := util.NormalizeString(s.Name)
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
				seriesByNormalizedName[util.NormalizeString(s.Name)] = append(
					seriesByNormalizedName[util.NormalizeString(s.Name)], s)
			}

			for _, s := range allSeries {
				if seen[s.ID] || isGarbageSeries(s.Name) {
					continue
				}
				suggested, ok := extractSeriesNameForDedup(s.Name)
				if !ok {
					continue
				}
				suggestedKey := util.NormalizeString(suggested)
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
		},
	})
}

// RegisterSeriesDedupOp registers the "dedup.series-dedup" v2 OperationDef.
func (s *Server) RegisterSeriesDedupOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.series-dedup",
		Plugin:          "dedup",
		DisplayName:     "Series Deduplication",
		Description:     "Merge all series with identical normalized names, reassigning their books.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.series-dedup",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p seriesDedupOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.series-dedup: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.series-dedup: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}

			_ = progress.Log("info", "Starting series deduplication...", nil)

			allSeries, err := store.GetAllSeries()
			if err != nil {
				return fmt.Errorf("failed to get series: %w", err)
			}

			_ = progress.UpdateProgress(0, len(allSeries), fmt.Sprintf("Scanning %d series for duplicates...", len(allSeries)))

			// Group by normalized name only
			groups := make(map[string][]database.Series)
			for _, s := range allSeries {
				key := util.NormalizeString(s.Name)
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
		},
	})
}

// RegisterSeriesPruneOp registers the "dedup.series-prune" v2 OperationDef.
func (s *Server) RegisterSeriesPruneOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.series-prune",
		Plugin:          "dedup",
		DisplayName:     "Series Prune",
		Description:     "Merge duplicate series and delete orphan series with no books.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         2 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.series-prune",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p seriesPruneOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.series-prune: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.series-prune: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}
			return s.executeSeriesPrune(ctx, store, progress, p.LegacyOpID)
		},
	})
}

// RegisterSeriesMergeOp registers the "dedup.series-merge" v2 OperationDef.
func (s *Server) RegisterSeriesMergeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.series-merge",
		Plugin:          "dedup",
		DisplayName:     "Series Merge",
		Description:     "Merge multiple series into one, reassigning all books and optionally renaming.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         1 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.series-merge",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p seriesMergeOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.series-merge: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.series-merge: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}
			opID := p.LegacyOpID
			keepID := p.KeepID
			mergeIDs := p.MergeIDs
			customName := strings.TrimSpace(p.CustomName)

			keepSeries, err := store.GetSeriesByID(keepID)
			if err != nil || keepSeries == nil {
				return fmt.Errorf("keep series %d not found", keepID)
			}

			keepName := keepSeries.Name
			if customName != "" {
				keepName = customName
			}

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
		},
	})
}

// RegisterSeriesNormalizeOp registers the "dedup.series-normalize" v2 OperationDef.
func (s *Server) RegisterSeriesNormalizeOp(reg *opsregistry.Registry) error {
	return reg.RegisterOp(opsregistry.OperationDef{
		ID:              "dedup.series-normalize",
		Plugin:          "dedup",
		DisplayName:     "Series Name Normalization",
		Description:     "Strip contamination from series names, merge sub-series, and re-organize affected books.",
		DefaultPriority: opsregistry.PriorityNormal,
		Cancellable:     true,
		Isolate:         false,
		Timeout:         4 * time.Hour,
		ResumePolicy:    opsregistry.ResumeDrop,
		ConcurrencyKey:  "dedup.series-normalize",
		Permissions:     []auth.Permission{auth.PermLibraryEditMetadata},
		Capabilities:    []opsregistry.Capability{opsregistry.CapLibraryRead, opsregistry.CapLibraryWrite, opsregistry.CapFilesWrite},
		Run: func(ctx context.Context, rawParams json.RawMessage, reporter opsregistry.Reporter) error {
			var p seriesNormalizeOpParams
			if err := json.Unmarshal(rawParams, &p); err != nil {
				return fmt.Errorf("dedup.series-normalize: decode params: %w", err)
			}
			store := s.Store()
			if store == nil {
				return fmt.Errorf("dedup.series-normalize: database not initialized")
			}
			progress := registryProgressAdapter{r: reporter}
			opID := p.LegacyOpID

			_ = progress.Log("info", "Starting series name normalization...", nil)

			enqueueWB := func(bookID string) {
				if s.writeBackBatcher != nil {
					s.writeBackBatcher.Enqueue(bookID)
				}
			}

			affectedBookIDs, opErr := executeSeriesNormalizeCore(ctx, store, enqueueWB)
			if opErr != nil {
				return opErr
			}

			_ = progress.Log("info", fmt.Sprintf("Renamed/merged series; organizing %d affected books...", len(affectedBookIDs)), nil)

			log2 := logger.NewWithActivityLog("series-normalize", store)
			for _, bookID := range affectedBookIDs {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				book, bErr := store.GetBookByID(bookID)
				if bErr != nil || book == nil {
					continue
				}
				if _, oErr := s.organizeService.ReOrganizeInPlace(book, log2); oErr != nil {
					_ = progress.Log("warn", fmt.Sprintf("organize failed for book %s: %v", bookID, oErr), nil)
				}
			}

			if len(affectedBookIDs) > 0 {
				_ = progress.Log("info", fmt.Sprintf("Writing tags for %d affected books...", len(affectedBookIDs)), nil)
				if wbErr := s.runBulkWriteBack(ctx, opID, affectedBookIDs, false, 0, progress); wbErr != nil {
					_ = progress.Log("warn", fmt.Sprintf("tag write-back incomplete: %v", wbErr), nil)
				}
			}

			_ = progress.Log("info", "Series normalization complete.", nil)
			return nil
		},
	})
}

func init() {
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterBookDedupScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterBookMergeOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterAuthorDedupScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterSeriesDedupScanOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterSeriesDedupOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterSeriesPruneOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterSeriesMergeOp(reg) })
	addOpRegistrar(func(s *Server, reg *opsregistry.Registry) error { return s.RegisterSeriesNormalizeOp(reg) })
}
