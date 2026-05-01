// file: internal/server/maintenance_series.go
// version: 1.0.0
// guid: 6016e897-43cb-479a-8fa0-b3d6c1b32e61
// last-edited: 2026-05-01

package server

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ---------------------------------------------------------------------------
// Series dedup/cleanup
// ---------------------------------------------------------------------------

// seriesCleanupSingleResult describes a 1-book series that was (or would be) removed.
type seriesCleanupSingleResult struct {
	SeriesID   int    `json:"series_id"`
	SeriesName string `json:"series_name"`
	BookID     string `json:"book_id"`
	BookTitle  string `json:"book_title"`
	Applied    bool   `json:"applied"`
	Error      string `json:"error,omitempty"`
}

// seriesCleanupDupGroup describes a group of duplicate series that were (or would be) merged.
type seriesCleanupDupGroup struct {
	NormalizedName string   `json:"normalized_name"`
	KeepSeriesID   int      `json:"keep_series_id"`
	KeepSeriesName string   `json:"keep_series_name"`
	KeepBookCount  int      `json:"keep_book_count"`
	MergedIDs      []int    `json:"merged_ids"`
	MergedNames    []string `json:"merged_names"`
	BooksMoved     int      `json:"books_moved"`
	Applied        bool     `json:"applied"`
	Error          string   `json:"error,omitempty"`
}

// handleCleanupSeries finds and optionally removes 1-book series and merges
// duplicate series.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually execute the cleanup
func (s *Server) handleCleanupSeries(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// --- Fetch all series ---
	allSeries, err := store.GetAllSeries()
	if err != nil {
		internalError(c, "failed to list series", err)
		return
	}

	// --- Fetch book counts per series ---
	bookCounts, err := store.GetAllSeriesBookCounts()
	if err != nil {
		internalError(c, "failed to get series book counts", err)
		return
	}

	// -----------------------------------------------------------------------
	// Phase 1: Find 1-book series
	// -----------------------------------------------------------------------
	var singleResults []seriesCleanupSingleResult

	for _, ser := range allSeries {
		count := bookCounts[ser.ID]
		if count != 1 {
			continue
		}

		// Fetch the one book in this series
		books, bErr := store.GetBooksBySeriesID(ser.ID)
		if bErr != nil || len(books) == 0 {
			continue
		}
		book := books[0]

		// Safety: if series_sequence > 1 the book is explicitly numbered,
		// suggesting other volumes may exist elsewhere — skip it.
		if book.SeriesSequence != nil && *book.SeriesSequence > 1 {
			continue
		}

		result := seriesCleanupSingleResult{
			SeriesID:   ser.ID,
			SeriesName: ser.Name,
			BookID:     book.ID,
			BookTitle:  book.Title,
		}

		if !dryRun {
			applyErr := unlinkAndDeleteSeries(store, &book, ser.ID)
			if applyErr != nil {
				result.Error = applyErr.Error()
				log.Printf("[WARN] cleanup-series: failed to remove 1-book series %d (%q): %v", ser.ID, ser.Name, applyErr)
			} else {
				result.Applied = true
				log.Printf("[INFO] cleanup-series: removed 1-book series %d (%q), unlinked book %s", ser.ID, ser.Name, book.ID)
			}
		}

		singleResults = append(singleResults, result)
	}

	// -----------------------------------------------------------------------
	// Phase 2: Find duplicate series (by normalized name)
	// -----------------------------------------------------------------------
	// Build a set of series IDs that were already deleted in phase 1 so we
	// don't try to merge them.
	deletedIDs := make(map[int]bool)
	if !dryRun {
		for _, r := range singleResults {
			if r.Applied {
				deletedIDs[r.SeriesID] = true
			}
		}
	}

	// Group series by normalized name
	normGroups := make(map[string][]database.Series)
	for _, ser := range allSeries {
		if deletedIDs[ser.ID] {
			continue
		}
		key := normalizeSeriesName(ser.Name)
		normGroups[key] = append(normGroups[key], ser)
	}

	var dupGroups []seriesCleanupDupGroup

	for normName, group := range normGroups {
		if len(group) < 2 {
			continue
		}

		// Pick the series with the most books as the keeper
		keepIdx := 0
		for i, ser := range group {
			if bookCounts[ser.ID] > bookCounts[group[keepIdx].ID] {
				keepIdx = i
			}
		}
		keeper := group[keepIdx]

		var mergeIDs []int
		var mergeNames []string
		for i, ser := range group {
			if i == keepIdx {
				continue
			}
			mergeIDs = append(mergeIDs, ser.ID)
			mergeNames = append(mergeNames, ser.Name)
		}

		totalMoved := 0
		for _, sid := range mergeIDs {
			totalMoved += bookCounts[sid]
		}

		dupResult := seriesCleanupDupGroup{
			NormalizedName: normName,
			KeepSeriesID:   keeper.ID,
			KeepSeriesName: keeper.Name,
			KeepBookCount:  bookCounts[keeper.ID],
			MergedIDs:      mergeIDs,
			MergedNames:    mergeNames,
			BooksMoved:     totalMoved,
		}

		if !dryRun {
			mergeErr := mergeSeriesGroup(store, keeper.ID, mergeIDs)
			if mergeErr != nil {
				dupResult.Error = mergeErr.Error()
				log.Printf("[WARN] cleanup-series: failed to merge series group %q: %v", normName, mergeErr)
			} else {
				dupResult.Applied = true
				log.Printf("[INFO] cleanup-series: merged %d duplicate series into %d (%q), moved %d books",
					len(mergeIDs), keeper.ID, keeper.Name, totalMoved)
			}
		}

		dupGroups = append(dupGroups, dupResult)
	}

	// -----------------------------------------------------------------------
	// Summary response
	// -----------------------------------------------------------------------
	singleApplied := 0
	singleErrors := 0
	for _, r := range singleResults {
		if r.Applied {
			singleApplied++
		}
		if r.Error != "" {
			singleErrors++
		}
	}

	dupApplied := 0
	dupErrors := 0
	for _, g := range dupGroups {
		if g.Applied {
			dupApplied++
		}
		if g.Error != "" {
			dupErrors++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"single_book_series": gin.H{
			"found":   len(singleResults),
			"applied": singleApplied,
			"errors":  singleErrors,
			"items":   singleResults,
		},
		"duplicate_series": gin.H{
			"groups_found":   len(dupGroups),
			"groups_applied": dupApplied,
			"errors":         dupErrors,
			"groups":         dupGroups,
		},
	})
}

// unlinkAndDeleteSeries sets the book's series_id to nil and then deletes the
// now-empty series record.
func unlinkAndDeleteSeries(store maintenanceStore, book *database.Book, seriesID int) error {
	// Re-fetch to avoid stale data (UpdateBook does FULL column replacement)
	current, err := store.GetBookByID(book.ID)
	if err != nil {
		return fmt.Errorf("GetBookByID: %w", err)
	}
	if current == nil {
		return fmt.Errorf("book %s not found", book.ID)
	}

	current.SeriesID = nil
	current.SeriesSequence = nil

	if _, err = store.UpdateBook(book.ID, current); err != nil {
		return fmt.Errorf("UpdateBook: %w", err)
	}

	if err = store.DeleteSeries(seriesID); err != nil {
		return fmt.Errorf("DeleteSeries: %w", err)
	}

	return nil
}

// mergeSeriesGroup moves all books from each series in mergeIDs to keepID,
// then deletes the now-empty series.
func mergeSeriesGroup(store maintenanceStore, keepID int, mergeIDs []int) error {
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

// nonAlphanumRE matches any run of non-alphanumeric, non-space characters.
var nonAlphanumRE = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// normalizeSeriesName produces a canonical key for duplicate detection:
//   - lowercase
//   - strip leading "the "
//   - strip trailing " series" / " saga" / " trilogy"
//   - remove punctuation
//   - collapse whitespace
func normalizeSeriesName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))

	// Remove leading "the "
	if strings.HasPrefix(s, "the ") {
		s = s[4:]
	}

	// Remove trailing series markers
	for _, suffix := range []string{" series", " saga", " trilogy", " duology", " quartet"} {
		if strings.HasSuffix(s, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}

	// Remove punctuation (keep letters, digits, spaces)
	s = nonAlphanumRE.ReplaceAllString(s, " ")

	// Collapse whitespace
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	return strings.Join(fields, " ")
}
