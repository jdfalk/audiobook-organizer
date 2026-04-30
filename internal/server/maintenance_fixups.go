// file: internal/server/maintenance_fixups.go
// version: 1.25.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/oklog/ulid/v2"
)

// Ensure deluge import is only done via the package-level accessor
// getDelugeClient() defined in deluge_integration.go — no direct import needed.

// maintenanceStore is the narrow slice of database.Store that
// maintenance-fixup helpers share. Every free function in this file
// accepts it — the shape is wide but still far narrower than full
// Store (no sessions, no tags, no operations tracking, no auth).
type maintenanceStore interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.BookFileStore
	database.UserTagStore
	database.ExternalIDStore
	database.StatsStore
}

// readByFixResult describes one book that was (or would be) fixed.
type readByFixResult struct {
	BookID      string  `json:"book_id"`
	Pattern     string  `json:"pattern"` // "read_by_swap", "title_dash_read_by", "both_broken"
	OldTitle    string  `json:"old_title"`
	OldAuthor   string  `json:"old_author"`
	OldNarrator *string `json:"old_narrator,omitempty"`
	NewTitle    string  `json:"new_title"`
	NewNarrator string  `json:"new_narrator"`
	FilePath    string  `json:"file_path,omitempty"`
	Applied     bool    `json:"applied"`
	Error       string  `json:"error,omitempty"`
}

// handleFixReadByNarrator finds books where the title/author metadata is
// swapped (title starts with "read by" or contains " - read by ") and
// corrects the fields.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleFixReadByNarrator(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books (non-deleted). With ~11K books this is fine.
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []readByFixResult

	for i := range allBooks {
		book := &allBooks[i]
		titleLower := strings.ToLower(book.Title)

		// Resolve author name for this book
		authorName := ""
		if book.AuthorID != nil {
			if author, aErr := store.GetAuthorByID(*book.AuthorID); aErr == nil && author != nil {
				authorName = author.Name
			}
		}

		var fix *readByFixResult

		switch {
		// Pattern 2: "Real Title - Narrator - read by Author"
		case strings.Contains(titleLower, " - read by "):
			fix = parsePattern2(book, authorName)

		// Pattern 3: both title and author are "read by ..."
		case strings.HasPrefix(titleLower, "read by ") && strings.HasPrefix(strings.ToLower(authorName), "read by "):
			fix = parsePattern3(book, authorName)

		// Pattern 1: title = "read by [narrator]", author = "[real title]"
		case strings.HasPrefix(titleLower, "read by "):
			fix = parsePattern1(book, authorName)
		}

		if fix == nil {
			continue
		}

		// Skip if nothing would actually change
		if fix.NewTitle == book.Title && fix.NewNarrator == stringDeref(book.Narrator) {
			continue
		}

		if !dryRun {
			applyErr := applyReadByFix(store, book, fix)
			if applyErr != nil {
				fix.Error = applyErr.Error()
				log.Printf("[WARN] fix-read-by-narrator: failed to update book %s: %v", book.ID, applyErr)
			} else {
				fix.Applied = true
				log.Printf("[INFO] fix-read-by-narrator: fixed book %s pattern=%s title=%q -> %q narrator=%q",
					book.ID, fix.Pattern, fix.OldTitle, fix.NewTitle, fix.NewNarrator)
			}
		}

		results = append(results, *fix)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":     dryRun,
		"total_found": len(results),
		"applied":     countApplied(results),
		"errors":      countErrors(results),
		"results":     results,
	})
}

// parsePattern1 handles: title = "read by [narrator]", author = "[real title]" or "[real title]_"
func parsePattern1(book *database.Book, authorName string) *readByFixResult {
	narrator := strings.TrimSpace(book.Title[len("read by "):])
	if strings.EqualFold(narrator, "") {
		return nil
	}

	// The real title is in the author name field
	newTitle := strings.TrimRight(authorName, "_")
	newTitle = strings.TrimSpace(newTitle)
	if newTitle == "" {
		return nil
	}

	return &readByFixResult{
		BookID:      book.ID,
		Pattern:     "read_by_swap",
		OldTitle:    book.Title,
		OldAuthor:   authorName,
		OldNarrator: book.Narrator,
		NewTitle:    newTitle,
		NewNarrator: narrator,
		FilePath:    book.FilePath,
	}
}

// parsePattern2 handles: title = "Real Title - Narrator - read by Author"
func parsePattern2(book *database.Book, authorName string) *readByFixResult {
	// Split on " - read by " (case-insensitive)
	idx := caseInsensitiveIndex(book.Title, " - read by ")
	if idx < 0 {
		return nil
	}

	beforeReadBy := book.Title[:idx]
	afterReadBy := strings.TrimSpace(book.Title[idx+len(" - read by "):])

	// beforeReadBy = "Real Title - Narrator" — split on last " - "
	var newTitle, narrator string
	lastDash := strings.LastIndex(beforeReadBy, " - ")
	if lastDash >= 0 {
		newTitle = strings.TrimSpace(beforeReadBy[:lastDash])
		narrator = strings.TrimSpace(beforeReadBy[lastDash+3:])
	} else {
		newTitle = strings.TrimSpace(beforeReadBy)
		narrator = ""
	}

	// afterReadBy might be the real author name — but we keep author_id unchanged
	_ = afterReadBy

	if newTitle == "" {
		return nil
	}

	return &readByFixResult{
		BookID:      book.ID,
		Pattern:     "title_dash_read_by",
		OldTitle:    book.Title,
		OldAuthor:   authorName,
		OldNarrator: book.Narrator,
		NewTitle:    newTitle,
		NewNarrator: narrator,
		FilePath:    book.FilePath,
	}
}

// parsePattern3 handles: both title and author are "read by ..."
// Try to extract info from file_path: .../Author/Title/file.m4b
func parsePattern3(book *database.Book, authorName string) *readByFixResult {
	narrator := strings.TrimSpace(book.Title[len("read by "):])

	// Try to get title from file path
	newTitle := titleFromFilePath(book.FilePath)
	if newTitle == "" {
		// Last resort: use the filename without extension
		base := filepath.Base(book.FilePath)
		ext := filepath.Ext(base)
		newTitle = strings.TrimSuffix(base, ext)
		newTitle = strings.TrimSpace(newTitle)
	}

	if newTitle == "" || strings.HasPrefix(strings.ToLower(newTitle), "read by ") {
		return nil
	}

	return &readByFixResult{
		BookID:      book.ID,
		Pattern:     "both_broken",
		OldTitle:    book.Title,
		OldAuthor:   authorName,
		OldNarrator: book.Narrator,
		NewTitle:    newTitle,
		NewNarrator: narrator,
		FilePath:    book.FilePath,
	}
}

// titleFromFilePath extracts a meaningful title from the directory structure.
// Typical layout: .../Author/Title/file.m4b — we want the parent directory name.
func titleFromFilePath(fp string) string {
	if fp == "" {
		return ""
	}
	dir := filepath.Dir(fp) // .../Author/Title
	title := filepath.Base(dir)
	if title == "." || title == "/" || title == "" {
		return ""
	}
	// If title looks like a generic name (e.g. just a number or very short), try grandparent
	return title
}

// applyReadByFix updates the book in the database with corrected title and narrator.
// It fetches the current book first (UpdateBook does full column replacement).
func applyReadByFix(store maintenanceStore, book *database.Book, fix *readByFixResult) error {
	// Re-fetch to get the latest state (UpdateBook does FULL column replacement)
	current, err := store.GetBookByID(book.ID)
	if err != nil {
		return fmt.Errorf("GetBookByID: %w", err)
	}
	if current == nil {
		return fmt.Errorf("book %s not found", book.ID)
	}

	current.Title = fix.NewTitle
	if fix.NewNarrator != "" {
		current.Narrator = &fix.NewNarrator
	}

	_, err = store.UpdateBook(book.ID, current)
	return err
}

// caseInsensitiveIndex finds the first occurrence of substr in s, case-insensitive.
func caseInsensitiveIndex(s, substr string) int {
	lower := strings.ToLower(s)
	return strings.Index(lower, strings.ToLower(substr))
}

func stringDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func countApplied(results []readByFixResult) int {
	n := 0
	for _, r := range results {
		if r.Applied {
			n++
		}
	}
	return n
}

func countErrors(results []readByFixResult) int {
	n := 0
	for _, r := range results {
		if r.Error != "" {
			n++
		}
	}
	return n
}

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

// ---------------------------------------------------------------------------
// Backfill book_files
// ---------------------------------------------------------------------------

// bookFilesBackfillResult describes one book processed during the backfill.
type bookFilesBackfillResult struct {
	BookID       string   `json:"book_id"`
	BookTitle    string   `json:"book_title"`
	FilePath     string   `json:"file_path"`
	FilesCreated int      `json:"files_created"`
	FilePaths    []string `json:"file_paths"`
	Skipped      bool     `json:"skipped,omitempty"`
	SkipReason   string   `json:"skip_reason,omitempty"`
	Missing      bool     `json:"missing,omitempty"`
	Applied      bool     `json:"applied"`
	Error        string   `json:"error,omitempty"`
}

// handleBackfillBookFiles scans all books and creates book_files rows where
// none exist yet.
//
// Query params:
//   - dry_run=true  (default) — report what would be created without modifying
//   - dry_run=false — actually create the rows
func (s *Server) handleBackfillBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books (0,0 = no pagination).
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []bookFilesBackfillResult
	totalFiles := 0

	for i := range allBooks {
		book := &allBooks[i]

		// Check whether book_files rows already exist for this book.
		existing, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			results = append(results, bookFilesBackfillResult{
				BookID:    book.ID,
				BookTitle: book.Title,
				FilePath:  book.FilePath,
				Error:     fmt.Sprintf("GetBookFiles: %v", bfErr),
			})
			continue
		}
		if len(existing) > 0 {
			results = append(results, bookFilesBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				FilePath:   book.FilePath,
				Skipped:    true,
				SkipReason: fmt.Sprintf("already has %d book_file row(s)", len(existing)),
			})
			continue
		}

		// Determine what files to create rows for.
		var filesToCreate []string
		var isMissing bool

		if book.FilePath == "" {
			results = append(results, bookFilesBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				Skipped:    true,
				SkipReason: "empty file_path",
			})
			continue
		}

		fi, statErr := os.Stat(book.FilePath)
		if statErr != nil {
			// Path doesn't exist — create one row marked missing.
			filesToCreate = []string{book.FilePath}
			isMissing = true
		} else if fi.IsDir() {
			// Directory: glob for audio files using the shared audioFilesInDir helper.
			filesToCreate = metafetch.AudioFilesInDir(book.FilePath)
			if len(filesToCreate) == 0 {
				results = append(results, bookFilesBackfillResult{
					BookID:     book.ID,
					BookTitle:  book.Title,
					FilePath:   book.FilePath,
					Skipped:    true,
					SkipReason: "directory contains no recognised audio files",
				})
				continue
			}
		} else {
			// Single file.
			filesToCreate = []string{book.FilePath}
		}

		result := bookFilesBackfillResult{
			BookID:       book.ID,
			BookTitle:    book.Title,
			FilePath:     book.FilePath,
			FilesCreated: len(filesToCreate),
			FilePaths:    filesToCreate,
			Missing:      isMissing,
		}

		if !dryRun {
			createErr := createBookFilesForBook(store, book, filesToCreate, isMissing)
			if createErr != nil {
				result.Error = createErr.Error()
				log.Printf("[WARN] backfill-book-files: book %s (%q): %v", book.ID, book.Title, createErr)
			} else {
				result.Applied = true
				// If file_path pointed directly at a file (not a directory), normalise
				// book.file_path to the parent directory.
				if !isMissing && fi != nil && !fi.IsDir() && len(filesToCreate) == 1 {
					current, getErr := store.GetBookByID(book.ID)
					if getErr == nil && current != nil {
						current.FilePath = filepath.Dir(book.FilePath)
						if _, upErr := store.UpdateBook(book.ID, current); upErr != nil {
							log.Printf("[WARN] backfill-book-files: normalise file_path for book %s: %v", book.ID, upErr)
						}
					}
				}
				log.Printf("[INFO] backfill-book-files: created %d book_file row(s) for book %s (%q)",
					len(filesToCreate), book.ID, book.Title)
			}
		}

		results = append(results, result)
		totalFiles += len(filesToCreate)
	}

	// Compute summary counts.
	applied := 0
	skipped := 0
	errors := 0
	for _, r := range results {
		switch {
		case r.Error != "":
			errors++
		case r.Skipped:
			skipped++
		case r.Applied || dryRun:
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_total":   len(allBooks),
		"books_found":   len(results) - skipped,
		"books_skipped": skipped,
		"files_total":   totalFiles,
		"applied":       applied,
		"errors":        errors,
		"results":       results,
	})
}

// ---------------------------------------------------------------------------
// Empty folder cleanup
// ---------------------------------------------------------------------------

// emptyFolderResult describes a directory that was (or would be) removed.
type emptyFolderResult struct {
	Path    string `json:"path"`
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

// handleCleanupEmptyFolders walks the audiobook root directory, finds empty
// directories (no files; only empty subdirectories), and removes them
// bottom-up (deepest first).
//
// Query params:
//   - dry_run=true  (default) — report what would be removed without deleting
//   - dry_run=false — actually delete the directories
func (s *Server) handleCleanupEmptyFolders(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	rootDir := config.AppConfig.RootDir

	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}

	if _, err := os.Stat(rootDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("root_dir not accessible: %v", err)})
		return
	}

	// Collect all directories (depth-first, pre-order). We reverse the list
	// afterward so we process deepest entries first (bottom-up).
	var dirs []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Non-fatal: log and continue.
			log.Printf("[WARN] cleanup-empty-folders: walk error at %q: %v", path, walkErr)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == rootDir {
			return nil // Never remove the root itself.
		}
		// Skip hidden directories (starting with a dot).
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		internalError(c, "failed to walk root directory", err)
		return
	}

	// Reverse so deepest directories come first.
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	var results []emptyFolderResult
	removedCount := 0

	for _, dir := range dirs {
		empty, checkErr := isDirEmpty(dir)
		if checkErr != nil {
			results = append(results, emptyFolderResult{
				Path:  dir,
				Error: fmt.Sprintf("stat error: %v", checkErr),
			})
			continue
		}
		if !empty {
			continue
		}

		result := emptyFolderResult{Path: dir}

		if !dryRun {
			if removeErr := os.Remove(dir); removeErr != nil {
				result.Error = removeErr.Error()
				log.Printf("[WARN] cleanup-empty-folders: failed to remove %q: %v", dir, removeErr)
			} else {
				result.Removed = true
				removedCount++
				log.Printf("[INFO] cleanup-empty-folders: removed empty directory %q", dir)
			}
		} else {
			removedCount++
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":         dryRun,
		"root_dir":        rootDir,
		"folders_found":   len(results),
		"folders_removed": removedCount,
		"folders":         results,
	})
}

// isDirEmpty reports whether dir contains no files or non-hidden subdirectories.
// It reads only the immediate children of dir.
func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		// Any non-hidden entry means the directory is not empty.
		if !strings.HasPrefix(e.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Garbage directory detection (cleanup-organize-mess)
// ---------------------------------------------------------------------------

// garbageDirResult describes a directory that looks like a file-fragment garbage
// directory left behind by a failed or partial organize run.
type garbageDirResult struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// isGarbageDirectory returns a non-empty reason string if the directory name
// looks like a file fragment rather than a real book/author/series directory.
// Examples of garbage:
//   - "04_ Intro"     — starts with digits and underscore (chapter file fragment)
//   - "04 - Intro"    — starts with digits and space-dash (chapter fragment)
//   - "Hero's Trial - 04 - Intro"  — contains double-nested path fragment
//   - Very short names (1-2 chars) that are not normal
func isGarbageDirectory(name string) string {
	if name == "" {
		return ""
	}

	// Pattern: starts with 2-3 digits followed by underscore or space-dash
	// e.g. "04_", "04 -", "004_", "1 -"
	chapterFragmentRe := regexp.MustCompile(`^\d{1,3}[_ ][_\-\s]`)
	if chapterFragmentRe.MatchString(name) {
		return "starts with chapter number fragment"
	}

	// Pattern: purely numeric name (e.g. "04", "004")
	pureNumericRe := regexp.MustCompile(`^\d+$`)
	if pureNumericRe.MatchString(name) {
		return "purely numeric directory name"
	}

	// Pattern: contains " - NN - " which looks like a double-nested segment
	// e.g. "Hero's Trial - 04 - Intro"
	doubleSegmentRe := regexp.MustCompile(` - \d{1,3} - `)
	if doubleSegmentRe.MatchString(name) {
		return "contains double-nested chapter segment pattern"
	}

	// Pattern: very short name (1 or 2 non-whitespace chars) that isn't a known
	// abbreviation — typically leftover from a bad path split.
	trimmed := strings.TrimSpace(name)
	if len([]rune(trimmed)) <= 2 && !allAlpha(trimmed) {
		return "suspiciously short non-alphabetic directory name"
	}

	return ""
}

// allAlpha returns true if every rune in s is a letter (handles Unicode).
func allAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

// handleCleanupOrganizeMess walks the audiobook root directory and reports
// (or removes) directories that look like garbage left behind by a partial or
// broken organize run, as well as empty directories.
//
// Query params:
//   - dry_run=true  (default) — report what would be removed without deleting
//   - dry_run=false — actually delete empty directories; garbage dirs with files
//     are always just reported (manual review required for non-empty garbage dirs)
func (s *Server) handleCleanupOrganizeMess(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	rootDir := config.AppConfig.RootDir

	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}
	if _, err := os.Stat(rootDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("root_dir not accessible: %v", err)})
		return
	}

	var dirs []string
	walkErr := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[WARN] cleanup-organize-mess: walk error at %q: %v", path, err)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == rootDir {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if walkErr != nil {
		internalError(c, "failed to walk root directory", walkErr)
		return
	}

	// Process deepest directories first (bottom-up).
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	var emptyResults []emptyFolderResult
	var garbageResults []garbageDirResult
	emptyRemoved := 0

	for _, dir := range dirs {
		name := filepath.Base(dir)

		// Check for garbage name pattern first.
		if reason := isGarbageDirectory(name); reason != "" {
			garbageResults = append(garbageResults, garbageDirResult{
				Path:   dir,
				Reason: reason,
			})
			// Garbage directories with files are NOT auto-removed — log for manual review.
			// If they are also empty, they will be caught below and removed if !dryRun.
		}

		// Check emptiness.
		empty, checkErr := isDirEmpty(dir)
		if checkErr != nil {
			emptyResults = append(emptyResults, emptyFolderResult{
				Path:  dir,
				Error: fmt.Sprintf("stat error: %v", checkErr),
			})
			continue
		}
		if !empty {
			continue
		}

		result := emptyFolderResult{Path: dir}
		if !dryRun {
			if removeErr := os.Remove(dir); removeErr != nil {
				result.Error = removeErr.Error()
				log.Printf("[WARN] cleanup-organize-mess: failed to remove %q: %v", dir, removeErr)
			} else {
				result.Removed = true
				emptyRemoved++
				log.Printf("[INFO] cleanup-organize-mess: removed empty directory %q", dir)
			}
		} else {
			emptyRemoved++
		}
		emptyResults = append(emptyResults, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":               dryRun,
		"root_dir":              rootDir,
		"empty_folders_found":   len(emptyResults),
		"empty_folders_removed": emptyRemoved,
		"garbage_dirs_found":    len(garbageResults),
		"garbage_dirs_note":     "Non-empty garbage directories require manual review; only empty ones are removed.",
		"empty_folders":         emptyResults,
		"garbage_dirs":          garbageResults,
	})
}

// ---------------------------------------------------------------------------
// Author/narrator swap fix
// ---------------------------------------------------------------------------

// authorNarratorSwapResult describes one book where the author field contains
// the narrator name (or vice versa).
type authorNarratorSwapResult struct {
	BookID       string `json:"book_id"`
	BookTitle    string `json:"book_title"`
	AuthorName   string `json:"author_name"`
	NarratorName string `json:"narrator_name"`
	Applied      bool   `json:"applied"`
	Error        string `json:"error,omitempty"`
}

// handleFixAuthorNarratorSwap finds books where the author field contains the
// narrator name (i.e. the author and narrator have been swapped at scan time)
// and optionally clears the wrong author association.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleFixAuthorNarratorSwap(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Paginate over all books in batches of 500.
	const batchSize = 500
	offset := 0
	var results []authorNarratorSwapResult

	for {
		batch, err := store.GetAllBooks(batchSize, offset)
		if err != nil {
			internalError(c, "failed to list books", err)
			return
		}
		if len(batch) == 0 {
			break
		}

		for i := range batch {
			book := &batch[i]

			// Only examine books that have both an author_id and a narrator set.
			if book.AuthorID == nil || book.Narrator == nil || *book.Narrator == "" {
				continue
			}

			author, aErr := store.GetAuthorByID(*book.AuthorID)
			if aErr != nil || author == nil {
				continue
			}

			// Detect swap: author name equals narrator name.
			if !strings.EqualFold(author.Name, *book.Narrator) {
				continue
			}

			result := authorNarratorSwapResult{
				BookID:       book.ID,
				BookTitle:    book.Title,
				AuthorName:   author.Name,
				NarratorName: *book.Narrator,
			}

			if !dryRun {
				current, getErr := store.GetBookByID(book.ID)
				if getErr != nil || current == nil {
					result.Error = fmt.Sprintf("GetBookByID: %v", getErr)
					log.Printf("[WARN] fix-author-narrator-swap: failed to fetch book %s: %v", book.ID, getErr)
				} else {
					// Clear the wrong author association; keep narrator intact.
					current.AuthorID = nil
					if _, updateErr := store.UpdateBook(book.ID, current); updateErr != nil {
						result.Error = updateErr.Error()
						log.Printf("[WARN] fix-author-narrator-swap: failed to update book %s: %v", book.ID, updateErr)
					} else {
						result.Applied = true
						log.Printf("[INFO] fix-author-narrator-swap: cleared author %q (= narrator) from book %s (%q)",
							author.Name, book.ID, book.Title)
					}
				}
			}

			results = append(results, result)
		}

		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	applied := 0
	errors := 0
	for _, r := range results {
		if r.Applied {
			applied++
		}
		if r.Error != "" {
			errors++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":     dryRun,
		"total_found": len(results),
		"applied":     applied,
		"errors":      errors,
		"results":     results,
	})
}

// createBookFilesForBook inserts a BookFile row for each path in filePaths.
func createBookFilesForBook(store maintenanceStore, book *database.Book, filePaths []string, missing bool) error {
	for _, fp := range filePaths {
		ext := strings.ToLower(filepath.Ext(fp))
		// Strip leading dot from extension for the format field.
		format := strings.TrimPrefix(ext, ".")

		var fileSize int64
		if !missing {
			if info, err := os.Stat(fp); err == nil {
				fileSize = info.Size()
			}
		}

		bf := &database.BookFile{
			ID:               ulid.Make().String(),
			BookID:           book.ID,
			FilePath:         fp,
			OriginalFilename: filepath.Base(fp),
			Format:           format,
			FileSize:         fileSize,
			Missing:          missing,
		}
		if err := store.CreateBookFile(bf); err != nil {
			return fmt.Errorf("CreateBookFile(%q): %w", fp, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Version group integrity checker and fixer
// ---------------------------------------------------------------------------

// vgMismatchGroup describes a version group where book titles differ
// significantly, indicating books that should not be linked together.
type vgMismatchGroup struct {
	VersionGroupID string   `json:"version_group_id"`
	Books          []vgBook `json:"books"`
	Applied        bool     `json:"applied"`
	Error          string   `json:"error,omitempty"`
}

// vgBook is a lightweight book summary used inside vgMismatchGroup.
type vgBook struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CoreTitle string `json:"core_title"`
}

// authorDirBook describes a book whose file_path appears to point at an
// author-level directory (containing multiple book subdirectories).
type authorDirBook struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	CurrentPath   string `json:"current_path"`
	SuggestedPath string `json:"suggested_path,omitempty"`
	Applied       bool   `json:"applied"`
	Error         string `json:"error,omitempty"`
}

// handleFixVersionGroups detects and optionally fixes two problems:
//  1. Books in the same version_group_id that have significantly different titles.
//  2. Books whose file_path points at an author directory (not a specific book dir).
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually fix the database
func (s *Server) handleFixVersionGroups(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// -----------------------------------------------------------------
	// Fetch all books (no pagination — ~11K is fine).
	// -----------------------------------------------------------------
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	// -----------------------------------------------------------------
	// Phase 1: Title mismatch within version groups.
	// -----------------------------------------------------------------

	// Group books by VersionGroupID.
	groupMap := make(map[string][]database.Book)
	for i := range allBooks {
		b := &allBooks[i]
		if b.VersionGroupID == nil || *b.VersionGroupID == "" {
			continue
		}
		groupMap[*b.VersionGroupID] = append(groupMap[*b.VersionGroupID], *b)
	}

	var mismatchGroups []vgMismatchGroup

	for groupID, books := range groupMap {
		if len(books) < 2 {
			continue
		}

		// Compute core title for each book.
		cores := make([]bookCore, len(books))
		for i, b := range books {
			cores[i] = bookCore{book: b, core: extractCoreTitle(b.Title)}
		}

		// Determine the majority core title (most books share it).
		majorityCore := findMajorityCore(cores)

		// Find books that don't match the majority.
		var outliers []vgBook
		for _, bc := range cores {
			if !coreTitlesMatch(bc.core, majorityCore) {
				outliers = append(outliers, vgBook{
					ID:        bc.book.ID,
					Title:     bc.book.Title,
					CoreTitle: bc.core,
				})
			}
		}

		if len(outliers) == 0 {
			continue
		}

		// Include all books in the report so the caller has full context.
		allVgBooks := make([]vgBook, len(cores))
		for i, bc := range cores {
			allVgBooks[i] = vgBook{
				ID:        bc.book.ID,
				Title:     bc.book.Title,
				CoreTitle: bc.core,
			}
		}

		mg := vgMismatchGroup{
			VersionGroupID: groupID,
			Books:          allVgBooks,
		}

		if !dryRun {
			applyErr := unlinkVersionGroupOutliers(store, outliers)
			if applyErr != nil {
				mg.Error = applyErr.Error()
				log.Printf("[WARN] fix-version-groups: failed to unlink outliers in group %s: %v", groupID, applyErr)
			} else {
				mg.Applied = true
				log.Printf("[INFO] fix-version-groups: unlinked %d outlier(s) from version group %s", len(outliers), groupID)
			}
		}

		mismatchGroups = append(mismatchGroups, mg)
	}

	// -----------------------------------------------------------------
	// Phase 2: Author-directory file_path detection.
	// -----------------------------------------------------------------
	var authorDirBooks []authorDirBook

	for i := range allBooks {
		b := &allBooks[i]
		if b.FilePath == "" {
			continue
		}

		fi, statErr := os.Stat(b.FilePath)
		if statErr != nil || !fi.IsDir() {
			// Not a directory, or doesn't exist — skip.
			continue
		}

		if !isAuthorDirectory(b.FilePath) {
			continue
		}

		// It's an author directory. Try to find the subdirectory that best
		// matches this book's title.
		suggested := bestMatchSubdir(b.FilePath, b.Title)

		adb := authorDirBook{
			ID:            b.ID,
			Title:         b.Title,
			CurrentPath:   b.FilePath,
			SuggestedPath: suggested,
		}

		if !dryRun && suggested != "" {
			fixErr := fixAuthorDirPath(store, b, suggested)
			if fixErr != nil {
				adb.Error = fixErr.Error()
				log.Printf("[WARN] fix-version-groups: failed to fix author-dir path for book %s (%q): %v", b.ID, b.Title, fixErr)
			} else {
				adb.Applied = true
				log.Printf("[INFO] fix-version-groups: updated file_path for book %s (%q): %q -> %q", b.ID, b.Title, b.FilePath, suggested)
			}
		}

		authorDirBooks = append(authorDirBooks, adb)
	}

	// -----------------------------------------------------------------
	// Response
	// -----------------------------------------------------------------
	mismatchApplied := 0
	for _, g := range mismatchGroups {
		if g.Applied {
			mismatchApplied++
		}
	}
	authorDirApplied := 0
	for _, a := range authorDirBooks {
		if a.Applied {
			authorDirApplied++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"title_mismatches": gin.H{
			"found":   len(mismatchGroups),
			"applied": mismatchApplied,
			"groups":  mismatchGroups,
		},
		"author_dir_paths": gin.H{
			"found":   len(authorDirBooks),
			"applied": authorDirApplied,
			"books":   authorDirBooks,
		},
	})
}

// parentheticalRE matches common parenthetical suffixes like "(Unabridged)",
// "(Abridged)", "(12/85)", etc.
var parentheticalRE = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// leadingNumberRE matches leading numeric prefixes like "1. ", "01 - ", etc.
var leadingNumberRE = regexp.MustCompile(`^\d+[\s.\-–]+`)

// colonSeriesRE matches ": Series Name" or ": Book N" style suffixes used
// to disambiguate short titles, e.g. "Tarkin: Star Wars".  We keep everything
// *before* the colon so "Tarkin: Star Wars" → "Tarkin".  This prevents
// false mismatches when the subtitle differs while the main title is the same.
// NOTE: we do NOT strip here — we just use the full normalised string for
// word-overlap comparison, which handles it naturally.

// extractCoreTitle strips common suffixes and normalises a book title for
// comparison purposes.
func extractCoreTitle(title string) string {
	s := title

	// Repeatedly strip trailing parentheticals, e.g. "(Unabridged) (MP3)".
	for {
		trimmed := parentheticalRE.ReplaceAllString(s, "")
		if trimmed == s {
			break
		}
		s = strings.TrimSpace(trimmed)
	}

	// Strip leading numeric prefixes.
	s = leadingNumberRE.ReplaceAllString(s, "")

	return strings.TrimSpace(s)
}

// findMajorityCore returns the core title shared by the most books.
// In a tie it returns the first one encountered (stable across versions).
type bookCore struct {
	book database.Book
	core string
}

func findMajorityCore(cores []bookCore) string {
	counts := make(map[string]int)
	for _, bc := range cores {
		counts[bc.core]++
	}
	best := ""
	bestCount := 0
	for core, count := range counts {
		if count > bestCount {
			bestCount = count
			best = core
		}
	}
	return best
}

// coreTitlesMatch returns true if the two core titles are "close enough" to be
// considered the same book.  The heuristic:
//   - Exact match (case-insensitive): trivially true.
//   - Substring: one contains the other (handles "Tarkin" vs "Tarkin: Star Wars").
//   - Word overlap: share at least one word of 4+ characters.
func coreTitlesMatch(a, b string) bool {
	aLow := strings.ToLower(a)
	bLow := strings.ToLower(b)

	if aLow == bLow {
		return true
	}
	if strings.Contains(aLow, bLow) || strings.Contains(bLow, aLow) {
		return true
	}

	// Word-overlap heuristic.
	aWords := longWords(aLow)
	bWords := longWords(bLow)
	for w := range aWords {
		if bWords[w] {
			return true
		}
	}
	return false
}

// longWords returns a set of unique words of 4+ characters from s.
func longWords(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		// Strip punctuation from word edges.
		w = strings.Trim(w, ".,;:!?\"'")
		if len([]rune(w)) >= 4 {
			set[w] = true
		}
	}
	return set
}

// unlinkVersionGroupOutliers gives each outlier book its own fresh
// version_group_id, effectively removing it from the shared group.
func unlinkVersionGroupOutliers(store maintenanceStore, outliers []vgBook) error {
	for _, ob := range outliers {
		current, err := store.GetBookByID(ob.ID)
		if err != nil {
			return fmt.Errorf("GetBookByID(%s): %w", ob.ID, err)
		}
		if current == nil {
			return fmt.Errorf("book %s not found", ob.ID)
		}
		newGroupID := ulid.Make().String()
		current.VersionGroupID = &newGroupID
		if _, err = store.UpdateBook(ob.ID, current); err != nil {
			return fmt.Errorf("UpdateBook(%s): %w", ob.ID, err)
		}
	}
	return nil
}

// isAuthorDirectory returns true when dir appears to be an author-level
// directory: it contains at least two subdirectories that each hold audio
// files.  A single-book directory usually has zero or one such sub.
func isAuthorDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	bookSubdirs := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subPath := filepath.Join(dir, e.Name())
		if len(metafetch.AudioFilesInDir(subPath)) > 0 {
			bookSubdirs++
			if bookSubdirs >= 2 {
				return true
			}
		}
	}
	return false
}

// bestMatchSubdir returns the subdirectory of parent whose name best matches
// title.  It uses word-overlap scoring; returns "" if no reasonable match is
// found.
func bestMatchSubdir(parent, title string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}

	titleWords := longWords(strings.ToLower(extractCoreTitle(title)))

	bestPath := ""
	bestScore := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only consider subdirs that actually contain audio files.
		sub := filepath.Join(parent, e.Name())
		if len(metafetch.AudioFilesInDir(sub)) == 0 {
			continue
		}

		dirWords := longWords(strings.ToLower(e.Name()))
		score := 0
		for w := range titleWords {
			if dirWords[w] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPath = sub
		}
	}

	// Require at least one matching word.
	if bestScore == 0 {
		return ""
	}
	return bestPath
}

// fixAuthorDirPath updates the book's file_path to the given subdir, then
// rebuilds book_files rows from that directory.
func fixAuthorDirPath(store maintenanceStore, book *database.Book, subdir string) error {
	// Re-fetch to avoid stale data (UpdateBook does FULL column replacement).
	current, err := store.GetBookByID(book.ID)
	if err != nil {
		return fmt.Errorf("GetBookByID: %w", err)
	}
	if current == nil {
		return fmt.Errorf("book %s not found", book.ID)
	}

	current.FilePath = subdir

	if _, err = store.UpdateBook(book.ID, current); err != nil {
		return fmt.Errorf("UpdateBook: %w", err)
	}

	// Delete existing book_files for this book and rebuild from the new directory.
	if err = store.DeleteBookFilesForBook(book.ID); err != nil {
		return fmt.Errorf("DeleteBookFilesForBook: %w", err)
	}

	newFiles := metafetch.AudioFilesInDir(subdir)
	if len(newFiles) == 0 {
		// No audio files found — leave book_files empty for now (not an error).
		return nil
	}

	return createBookFilesForBook(store, current, newFiles, false)
}

// ---------------------------------------------------------------------------
// Enrich book_files — track numbers, file sizes, formats, original filenames
// ---------------------------------------------------------------------------

// enrichBookFileResult describes one book_files row that was (or would be)
// enriched.
type enrichBookFileResult struct {
	FileID          string `json:"file_id"`
	BookID          string `json:"book_id"`
	FilePath        string `json:"file_path"`
	TrackNumberOld  int    `json:"track_number_old,omitempty"`
	TrackNumberNew  int    `json:"track_number_new,omitempty"`
	TrackCountOld   int    `json:"track_count_old,omitempty"`
	TrackCountNew   int    `json:"track_count_new,omitempty"`
	FileSizeOld     int64  `json:"file_size_old,omitempty"`
	FileSizeNew     int64  `json:"file_size_new,omitempty"`
	FormatOld       string `json:"format_old,omitempty"`
	FormatNew       string `json:"format_new,omitempty"`
	OrigFilenameSet bool   `json:"original_filename_set,omitempty"`
	Changed         bool   `json:"changed"`
	Applied         bool   `json:"applied"`
	Error           string `json:"error,omitempty"`
	Warning         string `json:"warning,omitempty"`
}

// handleEnrichBookFiles iterates all book_files rows and fills in missing or
// zero-valued fields:
//   - track_number: parsed from leading digits in the filename
//   - track_count:  total number of files for the owning book
//   - file_size:    from os.Stat when currently 0 or suspiciously small (<1000 bytes)
//   - format:       from filepath.Ext when empty
//   - original_filename: from filepath.Base when empty
//
// Also detects book_files where file_path points to a directory (not an audio
// file) and flags them with a warning.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleEnrichBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books so we can iterate book_files per book.
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []enrichBookFileResult
	totalChanged := 0
	totalApplied := 0
	totalErrors := 0

	for i := range allBooks {
		book := &allBooks[i]

		files, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			log.Printf("[WARN] enrich-book-files: GetBookFiles book %s: %v", book.ID, bfErr)
			continue
		}
		if len(files) == 0 {
			continue
		}

		trackCount := len(files)

		for j := range files {
			f := &files[j]
			result := enrichBookFileResult{
				FileID:   f.ID,
				BookID:   f.BookID,
				FilePath: f.FilePath,
			}

			changed := false

			// --- 1. original_filename ----------------------------------------
			if f.OriginalFilename == "" {
				f.OriginalFilename = filepath.Base(f.FilePath)
				result.OrigFilenameSet = true
				changed = true
			}

			// --- 2. format from extension -------------------------------------
			if f.Format == "" {
				ext := strings.ToLower(filepath.Ext(f.FilePath))
				if ext != "" {
					newFmt := strings.TrimPrefix(ext, ".")
					result.FormatOld = f.Format
					result.FormatNew = newFmt
					f.Format = newFmt
					changed = true
				}
			}

			// --- 3. file_size from os.Stat ------------------------------------
			// Fix sizes that are zero, suspiciously small (< 1000 bytes, likely
			// a directory inode size), or where the file_path points to a
			// directory instead of an actual audio file.
			if !f.Missing {
				needsSizeFix := f.FileSize == 0 || f.FileSize < 1000
				if info, statErr := os.Stat(f.FilePath); statErr == nil {
					if info.IsDir() {
						// file_path points to a directory, not a file — flag
						// it so it can be fixed. We can't determine the real
						// size without knowing the actual file.
						result.Warning = "file_path is a directory, not an audio file"
						result.FileSizeOld = f.FileSize
						changed = true
					} else if needsSizeFix {
						newSize := info.Size()
						if newSize > 0 && newSize != f.FileSize {
							result.FileSizeOld = f.FileSize
							result.FileSizeNew = newSize
							f.FileSize = newSize
							changed = true
						}
					}
				}
			}

			// --- 4. track_number from filename --------------------------------
			if f.TrackNumber == 0 {
				parsed := parseTrackNumberFromFilename(f.FilePath)
				if parsed > 0 {
					result.TrackNumberOld = f.TrackNumber
					result.TrackNumberNew = parsed
					f.TrackNumber = parsed
					changed = true
				}
			}

			// --- 5. track_count -----------------------------------------------
			if f.TrackCount != trackCount {
				result.TrackCountOld = f.TrackCount
				result.TrackCountNew = trackCount
				f.TrackCount = trackCount
				changed = true
			}

			result.Changed = changed

			if changed {
				totalChanged++
				if !dryRun {
					if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
						result.Error = upErr.Error()
						totalErrors++
						log.Printf("[WARN] enrich-book-files: UpdateBookFile %s: %v", f.ID, upErr)
					} else {
						result.Applied = true
						totalApplied++
					}
				}
				results = append(results, result)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_scanned": len(allBooks),
		"files_changed": totalChanged,
		"applied":       totalApplied,
		"errors":        totalErrors,
		"results":       results,
	})
}

// parseTrackNumberFromFilename extracts a leading track number from an audio
// filename.  Supported patterns (case-insensitive for the "Track" prefix):
//
//	"01 Chapter.mp3"      → 1
//	"02_Head of Dragon.m4b" → 2
//	"003-Epilogue.mp3"    → 3
//	"Track 05.mp3"        → 5
//
// Returns 0 if no number can be determined.
func parseTrackNumberFromFilename(filePath string) int {
	base := filepath.Base(filePath)
	// Remove extension for cleaner matching.
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Pattern A: optional "Track " prefix, then 1-3 leading digits followed by
	// a non-digit separator (space, underscore, dash, dot) or end-of-string.
	reLeading := regexp.MustCompile(`(?i)^(?:track\s*)?(\d{1,3})(?:[\s_\-.]|$)`)
	if m := reLeading.FindStringSubmatch(name); len(m) > 1 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n
		}
	}

	// Pattern B: entire stem is a number (e.g. "05.mp3").
	reOnly := regexp.MustCompile(`^(\d{1,3})$`)
	if m := reOnly.FindStringSubmatch(name); len(m) > 1 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Fix Book File Paths (directory → individual audio files)
// ---------------------------------------------------------------------------

// fixBookFilePathsResult describes the outcome for one book_files row whose
// file_path pointed to a directory (or whose file_size was suspiciously small).
type fixBookFilePathsResult struct {
	FileID      string   `json:"file_id"`
	BookID      string   `json:"book_id"`
	OldPath     string   `json:"old_path"`
	Action      string   `json:"action"`              // "updated", "expanded", "marked_missing", "size_fixed"
	NewPath     string   `json:"new_path,omitempty"`  // for "updated"
	NewPaths    []string `json:"new_paths,omitempty"` // for "expanded"
	FileSizeOld int64    `json:"file_size_old,omitempty"`
	FileSizeNew int64    `json:"file_size_new,omitempty"`
	Applied     bool     `json:"applied"`
	Error       string   `json:"error,omitempty"`
}

// handleFixBookFilePaths iterates every book_files row and:
//
//  1. If file_path doesn't exist but a file with the same stem prefix exists in
//     the same directory, updates the row to the real filename (repairs truncated
//     filenames left by old path-length limiting logic).
//
//  2. If file_path is a directory, globs for audio files inside it:
//     - 1 file found  → update the row's file_path to that file
//     - N>1 files     → create new book_file rows (one per file), delete the directory row
//     - 0 files found → mark the row missing=true
//
//  3. If file_path is a real file and file_size < 100 bytes (likely measured
//     from a directory inode), re-reads the size with os.Stat.
//
// For new/updated rows the handler also populates file_size, format, and
// original_filename from the actual file on disk.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update/create/delete rows
func (s *Server) handleFixBookFilePaths(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []fixBookFilePathsResult
	totalChanged := 0
	totalApplied := 0
	totalErrors := 0

	for i := range allBooks {
		book := &allBooks[i]

		files, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			log.Printf("[WARN] fix-book-file-paths: GetBookFiles book %s: %v", book.ID, bfErr)
			continue
		}

		for j := range files {
			f := &files[j]

			info, statErr := os.Stat(f.FilePath)
			if statErr != nil {
				// File doesn't exist on disk. Try to find the real file by
				// prefix-matching the truncated stem against siblings in the
				// same directory (handles filenames truncated by old organizer
				// runs that used the wrong path-length calculation).
				dir := filepath.Dir(f.FilePath)
				base := filepath.Base(f.FilePath)
				ext := filepath.Ext(base)
				stem := strings.TrimSuffix(base, ext)

				dirEntries, readErr := os.ReadDir(dir)
				var match string
				if readErr == nil {
					for _, de := range dirEntries {
						if de.IsDir() {
							continue
						}
						name := de.Name()
						nameExt := filepath.Ext(name)
						nameStem := strings.TrimSuffix(name, nameExt)
						// Match: same extension and full name starts with truncated stem
						// Only accept if the real file's name starts with the truncated
					// stem AND the extra characters don't begin with a space.
					// A genuine truncation cuts mid-word, so the suffix should
					// start with a non-space character (e.g. "Unabri" → "Unabridged").
					// A space prefix means we found a different longer variant
					// (e.g. "Book" → "Book 2"), which is not a truncation.
					if strings.EqualFold(nameExt, ext) &&
						strings.HasPrefix(nameStem, stem) &&
						name != base &&
						len(nameStem) > len(stem) &&
						nameStem[len(stem)] != ' ' {
						match = filepath.Join(dir, name)
						break
					}
					}
				}
				if match == "" {
					continue
				}

				fi, statErr2 := os.Stat(match)
				res := fixBookFilePathsResult{
					FileID:      f.ID,
					BookID:      f.BookID,
					OldPath:     f.FilePath,
					NewPath:     match,
					Action:      "truncated_name_repaired",
					FileSizeOld: f.FileSize,
				}
				if statErr2 == nil {
					res.FileSizeNew = fi.Size()
				}
				totalChanged++
				if !dryRun {
					f.FilePath = match
					f.OriginalFilename = filepath.Base(match)
					if statErr2 == nil {
						f.FileSize = fi.Size()
					}
					f.Missing = false
					if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
						res.Error = upErr.Error()
						totalErrors++
					} else {
						res.Applied = true
						totalApplied++
					}
				}
				results = append(results, res)
				continue
			}

			if info.IsDir() {
				// file_path points to a directory — find real audio files.
				audioFiles := metafetch.AudioFilesInDir(f.FilePath)

				switch len(audioFiles) {
				case 0:
					// No audio files found — mark as missing.
					res := fixBookFilePathsResult{
						FileID:  f.ID,
						BookID:  f.BookID,
						OldPath: f.FilePath,
						Action:  "marked_missing",
					}
					totalChanged++
					if !dryRun {
						f.Missing = true
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)

				case 1:
					// Exactly one file — update the existing row.
					audioPath := audioFiles[0]
					fi2, statErr2 := os.Stat(audioPath)
					res := fixBookFilePathsResult{
						FileID:  f.ID,
						BookID:  f.BookID,
						OldPath: f.FilePath,
						NewPath: audioPath,
						Action:  "updated",
					}
					totalChanged++
					if !dryRun {
						f.FilePath = audioPath
						f.OriginalFilename = filepath.Base(audioPath)
						ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(audioPath), "."))
						if ext != "" {
							f.Format = ext
						}
						if statErr2 == nil {
							res.FileSizeOld = f.FileSize
							res.FileSizeNew = fi2.Size()
							f.FileSize = fi2.Size()
						}
						f.Missing = false
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)

				default:
					// Multiple files — expand into individual rows.
					res := fixBookFilePathsResult{
						FileID:   f.ID,
						BookID:   f.BookID,
						OldPath:  f.FilePath,
						NewPaths: audioFiles,
						Action:   "expanded",
					}
					totalChanged++
					if !dryRun {
						applyErr := false
						for _, audioPath := range audioFiles {
							fi3, statErr3 := os.Stat(audioPath)
							newFile := &database.BookFile{
								ID:               ulid.Make().String(),
								BookID:           f.BookID,
								FilePath:         audioPath,
								OriginalFilename: filepath.Base(audioPath),
								Format:           strings.ToLower(strings.TrimPrefix(filepath.Ext(audioPath), ".")),
								Missing:          statErr3 != nil,
							}
							if statErr3 == nil {
								newFile.FileSize = fi3.Size()
							}
							if crErr := store.CreateBookFile(newFile); crErr != nil {
								res.Error = fmt.Sprintf("CreateBookFile %s: %v", audioPath, crErr)
								totalErrors++
								applyErr = true
								break
							}
						}
						if !applyErr {
							if delErr := store.DeleteBookFile(f.ID); delErr != nil {
								res.Error = fmt.Sprintf("DeleteBookFile %s: %v", f.ID, delErr)
								totalErrors++
							} else {
								res.Applied = true
								totalApplied++
							}
						}
					}
					results = append(results, res)
				}
				continue
			}

			// file_path is a real file — check for suspiciously small file_size
			// (< 100 bytes likely came from os.Stat on a directory inode).
			if !f.Missing && f.FileSize < 100 {
				realSize := info.Size()
				if realSize != f.FileSize {
					res := fixBookFilePathsResult{
						FileID:      f.ID,
						BookID:      f.BookID,
						OldPath:     f.FilePath,
						Action:      "size_fixed",
						FileSizeOld: f.FileSize,
						FileSizeNew: realSize,
					}
					totalChanged++
					if !dryRun {
						f.FileSize = realSize
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_scanned": len(allBooks),
		"files_changed": totalChanged,
		"applied":       totalApplied,
		"errors":        totalErrors,
		"results":       results,
	})
}

// ---------------------------------------------------------------------------
// Book Deduplication
// ---------------------------------------------------------------------------

// dedupBooksResult summarises the outcome of handleDedupBooks.
type dedupBooksResult struct {
	DryRun                 bool     `json:"dry_run"`
	Phase1JunkDeleted      int      `json:"phase1_junk_deleted"`
	Phase2PathDupesMerged  int      `json:"phase2_path_dupes_merged"`
	Phase3TitleDupesMerged int      `json:"phase3_title_dupes_merged"`
	Phase4VGDupesCleaned   int      `json:"phase4_vg_dupes_cleaned"`
	TotalBooksRemoved      int      `json:"total_books_removed"`
	Errors                 int      `json:"errors"`
	Details                gin.H    `json:"details"`
	ErrorMessages          []string `json:"error_messages,omitempty"`
}

// dedupMergeDetail describes one merge action.
type dedupMergeDetail struct {
	KeeperID    string   `json:"keeper_id"`
	KeeperTitle string   `json:"keeper_title"`
	RemovedIDs  []string `json:"removed_ids"`
	Reason      string   `json:"reason"`
}

// handleDedupBooks runs a 4-phase book deduplication scan (dry_run=true by default).
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually execute
//
// Phases:
//  1. Delete junk "read by narrator" records with no useful metadata
//  2. Merge books pointing to the same file_path (keep most metadata)
//  3. Merge books with same normalised title+author in the same directory
//  4. Remove duplicate entries inside version groups
func (s *Server) handleDedupBooks(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	result := dedupBooksResult{DryRun: dryRun}
	var errorMessages []string

	// Fetch all books in batches (12K+ books in production).
	allBooks, err := fetchAllBooksPaginated(store)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	// deletedIDs tracks books already removed in earlier phases so later
	// phases can skip them.
	deletedIDs := make(map[string]bool)

	// -----------------------------------------------------------------------
	// Phase 1: Delete junk "read by narrator" records
	// -----------------------------------------------------------------------
	var phase1Details []dedupMergeDetail

	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] {
			continue
		}
		if !isJunkReadByNarrator(book) {
			continue
		}

		log.Printf("[INFO] dedup-books phase1: junk record %s title=%q", book.ID, book.Title)
		if !dryRun {
			if delErr := softDeleteBook(store, book.ID); delErr != nil {
				errorMessages = append(errorMessages, fmt.Sprintf("phase1 delete %s: %v", book.ID, delErr))
				result.Errors++
				continue
			}
		}
		deletedIDs[book.ID] = true
		result.Phase1JunkDeleted++
		phase1Details = append(phase1Details, dedupMergeDetail{
			KeeperID:    "",
			KeeperTitle: "",
			RemovedIDs:  []string{book.ID},
			Reason:      "junk: title is 'read by narrator' with no useful metadata",
		})
	}

	// -----------------------------------------------------------------------
	// Phase 2: Merge books with the same file_path
	// -----------------------------------------------------------------------
	pathGroups := make(map[string][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] || book.FilePath == "" {
			continue
		}
		pathGroups[book.FilePath] = append(pathGroups[book.FilePath], *book)
	}

	var phase2Details []dedupMergeDetail

	for fp, group := range pathGroups {
		if len(group) < 2 {
			continue
		}

		// Filter out already-deleted.
		live := filterLive(group, deletedIDs)
		if len(live) < 2 {
			continue
		}

		keepIdx := pickKeeperIdx(live)
		keeper := &live[keepIdx]
		var dups []*database.Book
		for i := range live {
			if i != keepIdx {
				dups = append(dups, &live[i])
			}
		}

		detail := dedupMergeDetail{
			KeeperID:    keeper.ID,
			KeeperTitle: keeper.Title,
			Reason:      fmt.Sprintf("same file_path: %s", fp),
		}

		var mergeErrs []string
		for _, dup := range dups {
			if mergeErr := mergeDuplicateBook(store, keeper, dup, dryRun, s.writeBackBatcher); mergeErr != nil {
				msg := fmt.Sprintf("phase2 merge %s->%s: %v", dup.ID, keeper.ID, mergeErr)
				errorMessages = append(errorMessages, msg)
				mergeErrs = append(mergeErrs, mergeErr.Error())
				result.Errors++
				continue
			}
			detail.RemovedIDs = append(detail.RemovedIDs, dup.ID)
			deletedIDs[dup.ID] = true
			result.Phase2PathDupesMerged++
		}
		if len(mergeErrs) > 0 {
			detail.Reason += " [errors: " + strings.Join(mergeErrs, "; ") + "]"
		}
		if len(detail.RemovedIDs) > 0 {
			phase2Details = append(phase2Details, detail)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 3: Merge books with same normalised title + author in same dir
	// -----------------------------------------------------------------------
	type titleAuthorKey struct {
		NormTitle string
		AuthorID  int // 0 if nil
		Dir       string
	}

	taGroups := make(map[titleAuthorKey][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] {
			continue
		}
		normTitle := normalizeDedupTitle(book.Title)
		if normTitle == "" {
			continue
		}
		authorID := 0
		if book.AuthorID != nil {
			authorID = *book.AuthorID
		}
		// Only group books in the same directory (or with empty path).
		dir := ""
		if book.FilePath != "" {
			dir = filepath.Dir(book.FilePath)
		}
		key := titleAuthorKey{NormTitle: normTitle, AuthorID: authorID, Dir: dir}
		taGroups[key] = append(taGroups[key], *book)
	}

	var phase3Details []dedupMergeDetail

	for key, group := range taGroups {
		if len(group) < 2 {
			continue
		}
		live := filterLive(group, deletedIDs)
		if len(live) < 2 {
			continue
		}
		// Skip groups where author is unknown (authorID==0) and there are
		// different actual titles — could be false positives.
		if key.AuthorID == 0 {
			titles := make(map[string]bool)
			for _, b := range live {
				titles[strings.ToLower(strings.TrimSpace(b.Title))] = true
			}
			if len(titles) > 1 {
				continue // Different titles, skip
			}
		}

		keepIdx := pickKeeperIdx(live)
		keeper := &live[keepIdx]
		var dups []*database.Book
		for i := range live {
			if i != keepIdx {
				dups = append(dups, &live[i])
			}
		}

		detail := dedupMergeDetail{
			KeeperID:    keeper.ID,
			KeeperTitle: keeper.Title,
			Reason:      fmt.Sprintf("same title+author dir=%q normTitle=%q", key.Dir, key.NormTitle),
		}

		for _, dup := range dups {
			if mergeErr := mergeDuplicateBook(store, keeper, dup, dryRun, s.writeBackBatcher); mergeErr != nil {
				msg := fmt.Sprintf("phase3 merge %s->%s: %v", dup.ID, keeper.ID, mergeErr)
				errorMessages = append(errorMessages, msg)
				result.Errors++
				continue
			}
			detail.RemovedIDs = append(detail.RemovedIDs, dup.ID)
			deletedIDs[dup.ID] = true
			result.Phase3TitleDupesMerged++
		}
		if len(detail.RemovedIDs) > 0 {
			phase3Details = append(phase3Details, detail)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 4: Clean up version group duplicate entries
	// -----------------------------------------------------------------------
	// Build a map: versionGroupID → []Book
	vgGroups := make(map[string][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] || book.VersionGroupID == nil || *book.VersionGroupID == "" {
			continue
		}
		vgGroups[*book.VersionGroupID] = append(vgGroups[*book.VersionGroupID], *book)
	}

	var phase4Details []dedupMergeDetail

	for vgID, group := range vgGroups {
		// Deduplicate by book ID within the group (the same book ID appearing
		// multiple times in a version group).
		seen := make(map[string]bool)
		var dupeIDs []string
		for _, b := range group {
			if seen[b.ID] {
				dupeIDs = append(dupeIDs, b.ID)
			}
			seen[b.ID] = true
		}
		if len(dupeIDs) == 0 {
			continue
		}

		detail := dedupMergeDetail{
			KeeperID:    "",
			KeeperTitle: "",
			RemovedIDs:  dupeIDs,
			Reason:      fmt.Sprintf("duplicate entries in version group %s", vgID),
		}

		if !dryRun {
			// Unlink duplicate version group entries by nulling the VersionGroupID
			// on the extra copies after keeping one.
			for _, dupID := range dupeIDs {
				current, gbErr := store.GetBookByID(dupID)
				if gbErr != nil || current == nil {
					continue
				}
				current.VersionGroupID = nil
				current.IsPrimaryVersion = nil
				if _, upErr := store.UpdateBook(dupID, current); upErr != nil {
					msg := fmt.Sprintf("phase4 unlink vg %s from book %s: %v", vgID, dupID, upErr)
					errorMessages = append(errorMessages, msg)
					result.Errors++
					continue
				}
				result.Phase4VGDupesCleaned++
			}
		} else {
			result.Phase4VGDupesCleaned += len(dupeIDs)
		}

		phase4Details = append(phase4Details, detail)
	}

	result.TotalBooksRemoved = result.Phase1JunkDeleted + result.Phase2PathDupesMerged + result.Phase3TitleDupesMerged
	result.ErrorMessages = errorMessages
	result.Details = gin.H{
		"phase1_junk":        phase1Details,
		"phase2_path_dupes":  phase2Details,
		"phase3_title_dupes": phase3Details,
		"phase4_vg_dupes":    phase4Details,
	}

	c.JSON(http.StatusOK, result)
}

// fetchAllBooksPaginated retrieves all books in pages of 500 to avoid
// loading 12K+ records in one shot.
func fetchAllBooksPaginated(store maintenanceStore) ([]database.Book, error) {
	const pageSize = 500
	var all []database.Book
	offset := 0
	for {
		page, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// isJunkReadByNarrator returns true if the book title is literally
// "read by narrator" (or a close variant) AND the book has no useful
// metadata that would justify keeping it.
func isJunkReadByNarrator(book *database.Book) bool {
	t := strings.ToLower(strings.TrimSpace(book.Title))
	if t != "read by narrator" {
		return false
	}
	// Has useful data — don't delete.
	if book.AuthorID != nil {
		return false
	}
	if book.SeriesID != nil {
		return false
	}
	if book.Description != nil && strings.TrimSpace(*book.Description) != "" {
		return false
	}
	if book.ISBN10 != nil || book.ISBN13 != nil || book.ASIN != nil {
		return false
	}
	if book.ITunesPersistentID != nil {
		return false
	}
	return true
}

// pickKeeperIdx returns the index of the "best" book to keep from a group.
// Priority:
//  1. Book with the most non-nil pointer fields (most metadata)
//  2. Prefer the one with author_id set
//  3. Prefer the oldest created_at
func pickKeeperIdx(books []database.Book) int {
	best := 0
	for i := 1; i < len(books); i++ {
		if bookScore(&books[i]) > bookScore(&books[best]) {
			best = i
		}
	}
	return best
}

// bookScore returns a comparable quality score for a Book.
// Higher is better / more complete.
func bookScore(b *database.Book) int {
	score := 0
	if b.AuthorID != nil {
		score += 100
	}
	if b.SeriesID != nil {
		score += 20
	}
	if b.Description != nil && *b.Description != "" {
		score += 10
	}
	if b.Narrator != nil && *b.Narrator != "" {
		score += 5
	}
	if b.Duration != nil {
		score += 5
	}
	if b.ISBN10 != nil || b.ISBN13 != nil || b.ASIN != nil {
		score += 10
	}
	if b.ITunesPersistentID != nil {
		score += 10
	}
	if b.Publisher != nil && *b.Publisher != "" {
		score += 3
	}
	if b.Language != nil && *b.Language != "" {
		score += 2
	}
	if b.Genre != nil && *b.Genre != "" {
		score += 2
	}
	if b.CoverURL != nil && *b.CoverURL != "" {
		score += 3
	}
	// Older record is likely the authoritative one.
	if b.CreatedAt != nil {
		// Earlier creation time → higher score (subtract millis since epoch / big divisor)
		score -= int(b.CreatedAt.Unix() / 1_000_000)
	}
	return score
}

// mergeDuplicateBook transfers data from dup into keeper and then soft-deletes dup.
// When dryRun is true the function returns nil without modifying the database.
func mergeDuplicateBook(store maintenanceStore, keeper *database.Book, dup *database.Book, dryRun bool, batcher Enqueuer) error {
	if dryRun {
		return nil
	}

	// Collect dup's iTunes PIDs before reassignment (for ITL removal).
	dupMappings, _ := store.GetExternalIDsForBook(dup.ID)
	var dupPIDs []string
	for _, m := range dupMappings {
		if m.Source == "itunes" && m.ExternalID != "" && !m.Tombstoned {
			dupPIDs = append(dupPIDs, m.ExternalID)
		}
	}

	// Transfer book_files rows.
	files, err := store.GetBookFiles(dup.ID)
	if err == nil {
		for i := range files {
			f := &files[i]
			f.BookID = keeper.ID
			if upErr := store.UpsertBookFile(f); upErr != nil {
				log.Printf("[WARN] dedup-books: UpsertBookFile %s -> keeper %s: %v", f.ID, keeper.ID, upErr)
			}
		}
	}

	// Transfer external ID mappings.
	if reassignErr := store.ReassignExternalIDs(dup.ID, keeper.ID); reassignErr != nil {
		log.Printf("[WARN] dedup-books: ReassignExternalIDs %s -> %s: %v", dup.ID, keeper.ID, reassignErr)
	}

	// Queue ITL removal for the dup's tracks (they now point to the wrong file).
	// The keeper's tracks remain; dup's tracks are redundant entries in iTunes.
	if batcher != nil && len(dupPIDs) > 0 {
		for _, pid := range dupPIDs {
			batcher.EnqueueRemove(pid)
		}
		log.Printf("[INFO] dedup-books: queued %d ITL removals for dup %s", len(dupPIDs), dup.ID)
	}

	// Transfer user tags.
	tags, tagsErr := store.GetBookUserTags(dup.ID)
	if tagsErr == nil && len(tags) > 0 {
		for _, tag := range tags {
			_ = store.AddBookUserTag(keeper.ID, tag)
		}
	}

	// Merge missing metadata fields into keeper.
	current, gbErr := store.GetBookByID(keeper.ID)
	if gbErr != nil {
		return fmt.Errorf("GetBookByID keeper %s: %w", keeper.ID, gbErr)
	}
	if current == nil {
		return fmt.Errorf("keeper book %s not found", keeper.ID)
	}

	mergeBookFields(current, dup)

	if _, upErr := store.UpdateBook(keeper.ID, current); upErr != nil {
		return fmt.Errorf("UpdateBook keeper %s: %w", keeper.ID, upErr)
	}

	// Soft-delete the duplicate.
	return softDeleteBook(store, dup.ID)
}

// mergeBookFields copies non-nil/non-empty fields from src into dst when
// dst's field is currently nil/empty.  Does not overwrite existing data.
func mergeBookFields(dst, src *database.Book) {
	if dst.AuthorID == nil && src.AuthorID != nil {
		dst.AuthorID = src.AuthorID
	}
	if dst.SeriesID == nil && src.SeriesID != nil {
		dst.SeriesID = src.SeriesID
		if dst.SeriesSequence == nil && src.SeriesSequence != nil {
			dst.SeriesSequence = src.SeriesSequence
		}
	}
	if dst.Narrator == nil && src.Narrator != nil && *src.Narrator != "" {
		dst.Narrator = src.Narrator
	}
	if dst.Description == nil && src.Description != nil && *src.Description != "" {
		dst.Description = src.Description
	}
	if dst.Duration == nil && src.Duration != nil {
		dst.Duration = src.Duration
	}
	if dst.Publisher == nil && src.Publisher != nil {
		dst.Publisher = src.Publisher
	}
	if dst.Language == nil && src.Language != nil {
		dst.Language = src.Language
	}
	if dst.Genre == nil && src.Genre != nil {
		dst.Genre = src.Genre
	}
	if dst.ISBN10 == nil && src.ISBN10 != nil {
		dst.ISBN10 = src.ISBN10
	}
	if dst.ISBN13 == nil && src.ISBN13 != nil {
		dst.ISBN13 = src.ISBN13
	}
	if dst.ASIN == nil && src.ASIN != nil {
		dst.ASIN = src.ASIN
	}
	if dst.ITunesPersistentID == nil && src.ITunesPersistentID != nil {
		dst.ITunesPersistentID = src.ITunesPersistentID
	}
	if dst.ITunesDateAdded == nil && src.ITunesDateAdded != nil {
		dst.ITunesDateAdded = src.ITunesDateAdded
	}
	if dst.ITunesPlayCount == nil && src.ITunesPlayCount != nil {
		dst.ITunesPlayCount = src.ITunesPlayCount
	}
	if dst.ITunesRating == nil && src.ITunesRating != nil {
		dst.ITunesRating = src.ITunesRating
	}
	if dst.ITunesBookmark == nil && src.ITunesBookmark != nil {
		dst.ITunesBookmark = src.ITunesBookmark
	}
	if dst.CoverURL == nil && src.CoverURL != nil {
		dst.CoverURL = src.CoverURL
	}
	if dst.OpenLibraryID == nil && src.OpenLibraryID != nil {
		dst.OpenLibraryID = src.OpenLibraryID
	}
	if dst.GoogleBooksID == nil && src.GoogleBooksID != nil {
		dst.GoogleBooksID = src.GoogleBooksID
	}
	if dst.HardcoverID == nil && src.HardcoverID != nil {
		dst.HardcoverID = src.HardcoverID
	}
	if dst.WorkID == nil && src.WorkID != nil {
		dst.WorkID = src.WorkID
	}
	if (dst.VersionGroupID == nil || *dst.VersionGroupID == "") && src.VersionGroupID != nil && *src.VersionGroupID != "" {
		dst.VersionGroupID = src.VersionGroupID
	}
}

// softDeleteBook marks a book as deleted using the MarkedForDeletion flag.
// If UpdateBook fails, falls back to hard-delete via DeleteBook.
func softDeleteBook(store maintenanceStore, bookID string) error {
	current, err := store.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("GetBookByID %s: %w", bookID, err)
	}
	if current == nil {
		return nil // Already gone
	}

	t := true
	now := time.Now()
	current.MarkedForDeletion = &t
	current.MarkedForDeletionAt = &now

	if _, upErr := store.UpdateBook(bookID, current); upErr != nil {
		// Fall back to hard delete.
		log.Printf("[WARN] dedup-books: soft-delete failed for %s (%v), falling back to hard delete", bookID, upErr)
		return store.DeleteBook(bookID)
	}
	return nil
}

// normalizeDedupTitle produces a canonical key for title-based duplicate
// detection:
//   - lowercase + trim
//   - strip "(Unabridged)" suffix
//   - strip leading track/number patterns like "(12/85)" or "12."
//   - remove punctuation
//   - collapse whitespace
func normalizeDedupTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	if s == "" {
		return ""
	}

	// Strip "(Unabridged)" anywhere
	s = strings.ReplaceAll(s, "(unabridged)", "")

	// Strip leading numeric patterns: "(12/85) " or "12. " or "12 - "
	reLeadNum := regexp.MustCompile(`^\s*(\(\d+[/\-]\d+\)|\d+[\.\-\s])\s*`)
	s = reLeadNum.ReplaceAllString(s, "")

	// Remove punctuation (keep letters, digits, spaces)
	s = nonAlphanumRE.ReplaceAllString(s, " ")

	// Collapse whitespace
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	return strings.Join(fields, " ")
}

// filterLive filters out books whose IDs are in the deletedIDs set.
func filterLive(books []database.Book, deletedIDs map[string]bool) []database.Book {
	out := books[:0:len(books)]
	for _, b := range books {
		if !deletedIDs[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Refetch missing authors
// ---------------------------------------------------------------------------

// refetchMissingAuthorsResult describes one book that was examined/fixed.
type refetchMissingAuthorsResult struct {
	BookID       string `json:"book_id"`
	BookTitle    string `json:"book_title"`
	FilePath     string `json:"file_path,omitempty"`
	AuthorFound  string `json:"author_found,omitempty"`
	AuthorSource string `json:"author_source,omitempty"` // e.g. "tag.AlbumArtist (album_artist)", "tag.Artist"
	AuthorID     *int   `json:"author_id,omitempty"`
	Applied      bool   `json:"applied"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// handleRefetchMissingAuthors queries books with a NULL author_id and attempts
// to resolve the author by re-reading audio file tags (album_artist > artist).
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleRefetchMissingAuthors(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	const batchSize = 500
	offset := 0
	var results []refetchMissingAuthorsResult
	resolvedCount := 0
	skippedCount := 0
	errorCount := 0

	for {
		batch, err := store.GetAllBooks(batchSize, offset)
		if err != nil {
			internalError(c, "failed to list books", err)
			return
		}
		if len(batch) == 0 {
			break
		}

		for i := range batch {
			book := &batch[i]

			// Only consider books with no author and a non-empty title that
			// isn't itself a "read by narrator" leftover.
			if book.AuthorID != nil {
				continue
			}
			if book.Title == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(book.Title), "read by ") {
				continue
			}

			result := refetchMissingAuthorsResult{
				BookID:    book.ID,
				BookTitle: book.Title,
				FilePath:  book.FilePath,
			}

			// Determine which file path to read tags from.
			// Prefer the book's own file_path; fall back to the first book_file.
			tagPath := book.FilePath
			if tagPath == "" {
				files, fErr := store.GetBookFiles(book.ID)
				if fErr == nil && len(files) > 0 {
					tagPath = files[0].FilePath
				}
			}

			if tagPath == "" {
				result.Skipped = true
				result.SkipReason = "no file path available"
				skippedCount++
				results = append(results, result)
				continue
			}

			// Extract tags from the audio file.
			meta, mErr := metadata.ExtractMetadata(tagPath, nil)
			if mErr != nil {
				result.Error = fmt.Sprintf("ExtractMetadata: %v", mErr)
				errorCount++
				results = append(results, result)
				continue
			}

			// Resolve the narrator name for this book (used to skip the artist
			// field when it clearly holds the narrator, not the author).
			narratorName := ""
			if book.Narrator != nil {
				narratorName = strings.ToLower(strings.TrimSpace(*book.Narrator))
			}
			if narratorName == "" && meta.Narrator != "" {
				narratorName = strings.ToLower(strings.TrimSpace(meta.Narrator))
			}

			// Apply tag priority: album_artist > artist (skip artist if it
			// matches the known narrator).
			// meta.Artist is already resolved from album_artist > artist > composer
			// by ExtractMetadata. We trust album_artist unconditionally; for
			// artist-only sources we check it doesn't equal the narrator.
			candidateAuthor := strings.TrimSpace(meta.Artist)
			if candidateAuthor == "" {
				result.Skipped = true
				result.SkipReason = "no author found in file tags"
				skippedCount++
				results = append(results, result)
				continue
			}

			// If the resolved author comes from the plain artist tag (not
			// album_artist) and it matches the narrator, skip it.
			lc := strings.ToLower(candidateAuthor)
			if narratorName != "" && lc == narratorName {
				// Only skip when the source was artist (not album_artist).
				// meta.AuthorSource contains the tag source string.
				if !strings.Contains(meta.AuthorSource, "album_artist") {
					result.Skipped = true
					result.SkipReason = "artist tag matches narrator; cannot determine author"
					skippedCount++
					results = append(results, result)
					continue
				}
			}

			normalizedName := dedup.NormalizeAuthorName(candidateAuthor)
			if normalizedName == "" {
				result.Skipped = true
				result.SkipReason = "normalized author name is empty"
				skippedCount++
				results = append(results, result)
				continue
			}

			result.AuthorFound = normalizedName
			result.AuthorSource = meta.AuthorSource

			if dryRun {
				// In dry-run mode, look up (but don't create) the author so
				// the response shows whether they already exist.
				existing, _ := store.GetAuthorByName(normalizedName)
				if existing != nil {
					result.AuthorID = &existing.ID
				}
				resolvedCount++
				results = append(results, result)
				continue
			}

			// Look up or create the author.
			author, lookupErr := store.GetAuthorByName(normalizedName)
			if lookupErr != nil {
				author, lookupErr = store.CreateAuthor(normalizedName)
				if lookupErr != nil {
					result.Error = fmt.Sprintf("CreateAuthor: %v", lookupErr)
					errorCount++
					results = append(results, result)
					continue
				}
			}
			if author == nil {
				result.Error = "author lookup returned nil"
				errorCount++
				results = append(results, result)
				continue
			}

			// Re-fetch book to avoid stale data (UpdateBook does full column replacement).
			current, getErr := store.GetBookByID(book.ID)
			if getErr != nil || current == nil {
				result.Error = fmt.Sprintf("GetBookByID: %v", getErr)
				errorCount++
				results = append(results, result)
				continue
			}

			current.AuthorID = &author.ID
			if _, updateErr := store.UpdateBook(book.ID, current); updateErr != nil {
				result.Error = updateErr.Error()
				errorCount++
				log.Printf("[WARN] refetch-missing-authors: failed to update book %s: %v", book.ID, updateErr)
			} else {
				result.AuthorID = &author.ID
				result.Applied = true
				resolvedCount++
				log.Printf("[INFO] refetch-missing-authors: set author %q (id=%d) on book %s (%q)",
					normalizedName, author.ID, book.ID, book.Title)
			}

			results = append(results, result)
		}

		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":        dryRun,
		"total_examined": len(results),
		"resolved":       resolvedCount,
		"skipped":        skippedCount,
		"errors":         errorCount,
		"results":        results,
	})
}

// handleWipe handles POST /api/v1/maintenance/wipe.
//
// Request body:
//
//	{
//	  "targets": ["books","book_files","segments","files","organized_folders",
//	              "activity","authors","series","external_ids","all"],
//	  "confirm": "WIPE",
//	  "dry_run": true
//	}
//
// Safety: requires confirm == "WIPE". dry_run defaults to true.
func (s *Server) handleWipe(c *gin.Context) {
	var req struct {
		Targets []string `json:"targets"`
		Confirm string   `json:"confirm"`
		DryRun  bool     `json:"dry_run"`
	}
	// Default dry_run to true before binding.
	req.DryRun = true

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Confirm != "WIPE" {
		c.JSON(http.StatusBadRequest, gin.H{"error": `must include "confirm": "WIPE" in the request body`})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Expand "all" to every individual target.
	targetSet := make(map[string]bool, len(req.Targets))
	for _, t := range req.Targets {
		targetSet[t] = true
	}
	if targetSet["all"] {
		for _, t := range []string{
			"books", "book_files", "segments", "files",
			"organized_folders", "activity", "authors", "series", "external_ids",
		} {
			targetSet[t] = true
		}
	}

	results := make(map[string]int64)
	dryRun := req.DryRun

	// ── organized_folders ──────────────────────────────────────────────────
	if targetSet["organized_folders"] {
		rootDir := config.AppConfig.RootDir
		keep := map[string]bool{
			".covers":           true,
			".itunes-writeback": true,
			"openlibrary-dumps": true,
		}
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			log.Printf("[WARN] wipe: can't read root dir %q: %v", rootDir, err)
		} else {
			var count int64
			for _, e := range entries {
				// Skip hidden dirs (starting with ".") that are not in the keeplist,
				// but only delete non-hidden dirs or explicitly non-kept hidden dirs.
				if strings.HasPrefix(e.Name(), ".") && !keep[e.Name()] {
					continue // skip unknown hidden dirs
				}
				if keep[e.Name()] {
					continue
				}
				fullPath := filepath.Join(rootDir, e.Name())
				log.Printf("[INFO] wipe: organized_folders: %s %q", dryRunLabel(dryRun), fullPath)
				if !dryRun {
					if err := os.RemoveAll(fullPath); err != nil {
						log.Printf("[WARN] wipe: RemoveAll %q: %v", fullPath, err)
					}
				}
				count++
			}
			results["organized_folders"] = count
		}
	}

	// ── files (disk + db rows) ─────────────────────────────────────────────
	// "files" implies "book_files" as well — collect file paths first, then delete rows.
	if targetSet["files"] {
		rootDir := config.AppConfig.RootDir
		var count int64
		offset := 0
		batchSize := 500
		for {
			books, err := store.GetAllBooks(batchSize, offset)
			if err != nil {
				log.Printf("[WARN] wipe: files: GetAllBooks: %v", err)
				break
			}
			for _, book := range books {
				files, ferr := store.GetBookFiles(book.ID)
				if ferr != nil {
					log.Printf("[WARN] wipe: files: GetBookFiles %s: %v", book.ID, ferr)
					continue
				}
				for _, bf := range files {
					if bf.FilePath == "" {
						continue
					}
					// Only remove files inside the organizer root dir — never iTunes paths.
					if !strings.HasPrefix(filepath.Clean(bf.FilePath), filepath.Clean(rootDir)) {
						continue
					}
					log.Printf("[INFO] wipe: files: %s %q", dryRunLabel(dryRun), bf.FilePath)
					if !dryRun {
						if rerr := os.Remove(bf.FilePath); rerr != nil && !os.IsNotExist(rerr) {
							log.Printf("[WARN] wipe: os.Remove %q: %v", bf.FilePath, rerr)
						}
					}
					count++
				}
			}
			if len(books) < batchSize {
				break
			}
			offset += batchSize
		}
		results["files"] = count
		// "files" also deletes the book_file rows — mark book_files as well.
		targetSet["book_files"] = true
	}

	// ── book_files (db rows only) ──────────────────────────────────────────
	if targetSet["book_files"] {
		n, err := wipeBookFiles(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: book_files: %v", err)
		}
		results["book_files"] = n
	}

	// ── segments ──────────────────────────────────────────────────────────
	if targetSet["segments"] {
		n, err := wipeSegments(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: segments: %v", err)
		}
		results["segments"] = n
	}

	// ── books ──────────────────────────────────────────────────────────────
	if targetSet["books"] {
		n, err := wipeBooks(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: books: %v", err)
		}
		results["books"] = n
	}

	// ── authors ────────────────────────────────────────────────────────────
	if targetSet["authors"] {
		n, err := wipeAuthors(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: authors: %v", err)
		}
		results["authors"] = n
	}

	// ── series ─────────────────────────────────────────────────────────────
	if targetSet["series"] {
		n, err := wipeSeries(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: series: %v", err)
		}
		results["series"] = n
	}

	// ── external_ids ───────────────────────────────────────────────────────
	if targetSet["external_ids"] {
		n, err := wipeExternalIDs(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: external_ids: %v", err)
		}
		results["external_ids"] = n
	}

	// ── activity ──────────────────────────────────────────────────────────
	if targetSet["activity"] {
		if s.activityService != nil {
			n, err := wipeActivity(s.activityService, dryRun)
			if err != nil {
				log.Printf("[WARN] wipe: activity: %v", err)
			}
			results["activity"] = n
		} else {
			log.Printf("[INFO] wipe: activity: activityService not initialized, skipping")
		}
	}

	log.Printf("[INFO] wipe: complete dry_run=%v targets=%v results=%v", dryRun, req.Targets, results)
	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"results": results,
	})
}

// dryRunLabel returns a label for logging.
func dryRunLabel(dryRun bool) string {
	if dryRun {
		return "[dry-run] would delete"
	}
	return "deleting"
}

// wipeBookFiles deletes all book_file rows using the appropriate store backend.
func wipeBookFiles(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		// Count only.
		n, err := store.CountFiles()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("book_files")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"book_file:"})
		return int64(n), err
	default:
		// Fallback: iterate all books and delete via interface.
		var count int64
		offset := 0
		for {
			books, err := store.GetAllBooks(500, offset)
			if err != nil {
				return count, err
			}
			for _, book := range books {
				if err := store.DeleteBookFilesForBook(book.ID); err != nil {
					log.Printf("[WARN] wipeBookFiles: DeleteBookFilesForBook %s: %v", book.ID, err)
				}
				count++ // approximate
			}
			if len(books) < 500 {
				break
			}
			offset += 500
		}
		return count, nil
	}
}

// wipeSegments deletes all book_segment rows using the appropriate store backend.
func wipeSegments(store maintenanceStore, dryRun bool) (int64, error) {
	switch s := store.(type) {
	case *database.SQLiteStore:
		if dryRun {
			return s.CountTableRows("book_segments")
		}
		return s.WipeTable("book_segments")
	case *database.PebbleStore:
		// Pebble segments use "bf:" (primary) and "bfs:" (secondary) prefixes.
		if dryRun {
			n, err := s.CountByPrefix("bf:")
			return int64(n), err
		}
		n, err := s.WipeByPrefixes([]string{"bf:", "bfs:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeSegments: unsupported store type %T", store)
	}
}

// wipeBooks deletes all book rows using the appropriate store backend.
func wipeBooks(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountBooks()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("books")
	case *database.PebbleStore:
		// Book keys: "book:" prefix. Include secondary indexes.
		n, err := s.WipeByPrefixes([]string{"book:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeBooks: unsupported store type %T", store)
	}
}

// wipeAuthors deletes all author rows using the appropriate store backend.
func wipeAuthors(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountAuthors()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("authors")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"author:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeAuthors: unsupported store type %T", store)
	}
}

// wipeSeries deletes all series rows using the appropriate store backend.
func wipeSeries(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountSeries()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("series")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"series:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeSeries: unsupported store type %T", store)
	}
}

// wipeExternalIDs deletes all external_id_map rows using the appropriate store backend.
func wipeExternalIDs(store maintenanceStore, dryRun bool) (int64, error) {
	switch s := store.(type) {
	case *database.SQLiteStore:
		if dryRun {
			return s.CountTableRows("external_id_map")
		}
		return s.WipeTable("external_id_map")
	case *database.PebbleStore:
		if dryRun {
			n, err := s.CountByPrefix("ext_id:")
			return int64(n), err
		}
		// "ext_id:" covers both "ext_id:<source>:<id>" and "ext_id:book:<bookID>:<source>:<id>"
		n, err := s.WipeByPrefixes([]string{"ext_id:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeExternalIDs: unsupported store type %T", store)
	}
}

// wipeActivity deletes all activity log entries.
func wipeActivity(svc *activity.Service, dryRun bool) (int64, error) {
	if dryRun {
		entries, total, err := svc.Query(database.ActivityFilter{Limit: 1})
		if err != nil {
			return 0, err
		}
		_ = entries
		return int64(total), nil
	}
	return svc.Store().WipeAllActivity()
}

// libraryStateFixResult describes one book that was (or would be) fixed.
type libraryStateFixResult struct {
	BookID       string `json:"book_id"`
	Title        string `json:"title"`
	OldState     string `json:"old_state"`
	NewState     string `json:"new_state"`
	VersionGroup string `json:"version_group"`
	IsPrimary    bool   `json:"is_primary"`
	Applied      bool   `json:"applied"`
	Error        string `json:"error,omitempty"`
}

// handleFixLibraryStates fixes library_state for books that have organized versions.
// Books with library_state = 'imported' AND version_group_id set AND is_primary_version = false
// should have library_state = 'organized_source'.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleFixLibraryStates(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books (non-deleted). With ~11K books this is fine.
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []libraryStateFixResult
	fixCount := 0
	skipCount := 0
	errorCount := 0

	for i := range allBooks {
		book := &allBooks[i]

		// Look for books with library_state = 'imported'
		if book.LibraryState == nil || *book.LibraryState != "imported" {
			skipCount++
			continue
		}

		// Must have a version_group_id
		if book.VersionGroupID == nil || *book.VersionGroupID == "" {
			skipCount++
			continue
		}

		// Must NOT be a primary version
		if book.IsPrimaryVersion == nil || *book.IsPrimaryVersion {
			skipCount++
			continue
		}

		// This book qualifies for fixing: organized source version in imported state
		result := libraryStateFixResult{
			BookID:       book.ID,
			Title:        book.Title,
			OldState:     "imported",
			NewState:     "organized_source",
			VersionGroup: *book.VersionGroupID,
			IsPrimary:    false,
			Applied:      !dryRun,
		}

		if !dryRun {
			// Update the book
			newState := "organized_source"
			book.LibraryState = &newState
			if _, updateErr := store.UpdateBook(book.ID, book); updateErr != nil {
				result.Error = updateErr.Error()
				errorCount++
			} else {
				fixCount++
			}
		} else {
			fixCount++
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"fixed":   fixCount,
		"skipped": skipCount,
		"errors":  errorCount,
		"results": results,
	})
}

// recomputeITunesPathResult describes one book_file that was (or would be) fixed.
type recomputeITunesPathResult struct {
	BookFileID    string `json:"book_file_id"`
	BookID        string `json:"book_id"`
	FilePath      string `json:"file_path"`
	OldITunesPath string `json:"old_itunes_path"`
	NewITunesPath string `json:"new_itunes_path"`
	Applied       bool   `json:"applied"`
	Error         string `json:"error,omitempty"`
}

// handleRecomputeITunesPaths iterates all book_files on PRIMARY books and
// recomputes itunes_path from file_path whenever they differ.  Books whose
// file_path lives under the audiobook-organizer root but whose itunes_path
// still points at the old iTunes location (e.g. W:/itunes/…) are the primary
// target, but the handler fixes any book_file where the recomputed value
// differs from the stored value.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleRecomputeITunesPaths(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var (
		fixCount   int
		skipCount  int
		errorCount int
		results    []recomputeITunesPathResult
	)

	for i := range allBooks {
		book := &allBooks[i]
		// Only consider PRIMARY books; originals/non-primaries are not the
		// organized copies and should not have their itunes_path changed here.
		if book.IsPrimaryVersion == nil || !*book.IsPrimaryVersion {
			continue
		}

		bookFiles, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil || len(bookFiles) == 0 {
			continue
		}

		for _, bf := range bookFiles {
			if bf.FilePath == "" {
				skipCount++
				continue
			}

			want := metafetch.ComputeITunesPath(bf.FilePath)
			if bf.ITunesPath == want {
				skipCount++
				continue
			}

			result := recomputeITunesPathResult{
				BookFileID:    bf.ID,
				BookID:        book.ID,
				FilePath:      bf.FilePath,
				OldITunesPath: bf.ITunesPath,
				NewITunesPath: want,
			}

			if !dryRun {
				bf.ITunesPath = want
				if updateErr := store.UpdateBookFile(bf.ID, &bf); updateErr != nil {
					result.Error = updateErr.Error()
					errorCount++
				} else {
					result.Applied = true
					fixCount++
				}
			} else {
				fixCount++
			}

			results = append(results, result)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"fixed":   fixCount,
		"skipped": skipCount,
		"errors":  errorCount,
		"results": results,
	})
}

// handleGenerateITLTests generates a suite of .itl test files for iTunes testing.
func (s *Server) handleGenerateITLTests(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	outputDir := config.AppConfig.RootDir + "/.itunes-writeback/tests"

	// Wipe existing test data so we get a clean slate
	if err := os.RemoveAll(outputDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to clean output dir: %v", err)})
		return
	}

	// Gather all books and book_files for the full-library test case
	allBooks, err := store.GetAllBooks(100000, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch books: %v", err)})
		return
	}

	var allBookFiles []database.BookFile
	for _, b := range allBooks {
		files, _ := store.GetBookFiles(b.ID)
		allBookFiles = append(allBookFiles, files...)
	}

	if err := itunes.GenerateTestITLSuite(outputDir, allBooks, allBookFiles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate test suite: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"output_dir": outputDir,
		"books":      len(allBooks),
		"book_files": len(allBookFiles),
		"message":    fmt.Sprintf("Generated ITL test suite in %s with %d books and %d book_files", outputDir, len(allBooks), len(allBookFiles)),
	})
}

// backupCleanupResult summarizes a cleanup-backups run.
type backupCleanupResult struct {
	DryRun       bool     `json:"dry_run"`
	RootDir      string   `json:"root_dir"`
	FilesFound   int      `json:"files_found"`
	FilesRemoved int      `json:"files_removed"`
	BytesFreed   int64    `json:"bytes_freed"`
	Errors       []string `json:"errors,omitempty"`
}

// handleCleanupBackups sweeps the library for stale tag-write backup files
// and deletes them. Two patterns are matched:
//
//  1. `*.backup` and `*.backup.*.backup` — created by the older
//     fileops.FileOperation.Execute() path, which retains 5 per file but
//     never garbage-collects when a file stops being written.
//  2. `*.bak-YYYYMMDD-HHMMSS` — created by the write-back path in
//     metadata_fetch_service.backupFileBeforeWrite. That function is now
//     gated on the WriteBackupBeforeTagWrite config flag (default off)
//     so new backups stop accumulating, but the historical pile (tens of
//     thousands of files, multi-TB apparent size) still needs sweeping.
//
// Protected paths:
//   - Every directory whose name starts with `.` is skipped via
//     filepath.SkipDir. This covers the iTunes writeback folder
//     (.itunes-writeback) and the cover dedup store (.covers).
//   - The iTunes Media tree outside the managed library is not walked
//     because we only scan under config.AppConfig.RootDir.
//
// Query params:
//   - dry_run=true  (default) — report what would be removed
//   - dry_run=false — actually delete
func (s *Server) handleCleanupBackups(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	rootDir := config.AppConfig.RootDir
	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}
	if _, err := os.Stat(rootDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("root_dir not accessible: %v", err)})
		return
	}

	result := backupCleanupResult{
		DryRun:  dryRun,
		RootDir: rootDir,
	}

	// Regex matches a timestamped .bak-YYYYMMDD-HHMMSS suffix anywhere in
	// the filename. Anchored at end-of-string so it doesn't accidentally
	// eat filenames that happen to contain `.bak-1` earlier.
	bakTimestampRe := regexp.MustCompile(`\.bak-[0-9]{8}-[0-9]{6}$`)

	walkErr := filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Non-fatal, keep going — record and continue.
			result.Errors = append(result.Errors, fmt.Sprintf("walk %q: %v", path, walkErr))
			return nil
		}
		if info.IsDir() {
			// Skip any hidden directory. This intentionally catches
			// .itunes-writeback, .covers, and any other dotfolder a user
			// might add later — explicit deny-list is fragile, prefix
			// check is robust. The root itself never starts with `.`
			// so we don't have to guard against skipping it.
			if path != rootDir && strings.HasPrefix(filepath.Base(path), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		name := filepath.Base(path)
		isBackupCopy := strings.HasSuffix(name, ".backup")
		isBakTimestamp := bakTimestampRe.MatchString(name)
		if !isBackupCopy && !isBakTimestamp {
			return nil
		}

		result.FilesFound++
		size := info.Size()

		if dryRun {
			result.BytesFreed += size
			return nil
		}

		if removeErr := os.Remove(path); removeErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %q: %v", path, removeErr))
			log.Printf("[WARN] cleanup-backups: failed to remove %q: %v", path, removeErr)
			return nil
		}
		result.FilesRemoved++
		result.BytesFreed += size
		return nil
	})
	if walkErr != nil {
		internalError(c, "failed to walk root directory", walkErr)
		return
	}

	log.Printf("[INFO] cleanup-backups: dry_run=%v found=%d removed=%d bytes=%d errors=%d",
		dryRun, result.FilesFound, result.FilesRemoved, result.BytesFreed, len(result.Errors))

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       result.DryRun,
		"root_dir":      result.RootDir,
		"files_found":   result.FilesFound,
		"files_removed": result.FilesRemoved,
		"bytes_freed":   result.BytesFreed,
		"human_freed":   humanizeBytes(result.BytesFreed),
		"errors":        result.Errors,
	})
}

// composerTagResult describes the COMPOSER field state for one audio file.
type composerTagResult struct {
	BookID    string `json:"book_id"`
	BookTitle string `json:"book_title"`
	FilePath  string `json:"file_path"`
	// Category is one of: "ok", "composer_equals_author", "composer_equals_narrator",
	// "composer_mismatch", "missing_narrator", "read_error".
	Category  string `json:"category"`
	Composer  string `json:"composer_on_disk"`
	Author    string `json:"author,omitempty"`
	Narrator  string `json:"narrator,omitempty"`
	WillWrite string `json:"will_write,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
	Error     string `json:"error,omitempty"`
}

// categorizeComposer returns the problem category and the value that should
// be written in the given fix_mode ("set_narrator" or "clear").
func categorizeComposer(composer, author, narrator, fixMode string) (category, willWrite string) {
	composerLower := strings.ToLower(strings.TrimSpace(composer))
	authorLower := strings.ToLower(strings.TrimSpace(author))
	narratorLower := strings.ToLower(strings.TrimSpace(narrator))

	if fixMode == "set_narrator" {
		willWrite = strings.TrimSpace(narrator)
	} else {
		willWrite = ""
	}

	if strings.TrimSpace(composer) == "" {
		if fixMode == "set_narrator" && strings.TrimSpace(narrator) != "" {
			return "missing_narrator", strings.TrimSpace(narrator)
		}
		return "ok", ""
	}

	if author != "" && composerLower == authorLower {
		// Old wrong mapping: author ended up in COMPOSER.
		return "composer_equals_author", willWrite
	}
	if narrator != "" && composerLower == narratorLower {
		if fixMode == "set_narrator" {
			return "ok", strings.TrimSpace(narrator) // already correct
		}
		return "composer_equals_narrator", ""
	}
	// Non-empty COMPOSER that matches neither author nor narrator.
	return "composer_mismatch", willWrite
}

// composerScanWork is one unit of work dispatched to the parallel reader pool.
type composerScanWork struct {
	bookID    string
	bookTitle string
	filePath  string
	author    string
	narrator  string
}

// handleScanComposerTags starts an async, resumable COMPOSER-tag scan as a
// queued operation and returns the operation ID immediately (HTTP 202).
//
// The operation bulk-loads all books/authors/files, fans out tag reads across
// 8 goroutines, persists per-file results to the OperationResult table, and
// survives server restarts — on startup resumeInterruptedOperations() picks up
// any interrupted composer_tag_scan and re-enqueues from where it left off.
//
// Query params:
//   - dry_run=true (default) — scan and report without writing
//   - dry_run=false — apply the fix to problematic files
//   - fix_mode=set_narrator (default) — write COMPOSER=narrator; "clear" to always empty it
//
// Poll progress via GET /api/v1/operations/{id}.
// View results via GET /api/v1/maintenance/scan-composer-tags/{id}.
func (s *Server) handleScanComposerTags(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	fixMode := c.DefaultQuery("fix_mode", "set_narrator")
	if fixMode != "set_narrator" && fixMode != "clear" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fix_mode must be 'set_narrator' or 'clear'"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "composer_tag_scan", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.ComposerScanParams{DryRun: dryRun, FixMode: fixMode}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] scan-composer-tags: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runComposerTagScan(ctx, capturedOpID, capturedParams, store, progress)
	}

	if err := s.queue.Enqueue(opID, "composer_tag_scan", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] scan-composer-tags: queued operation %s dry_run=%v fix_mode=%s", opID, dryRun, fixMode)

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "composer tag scan started — poll GET /api/v1/operations/" + opID + " for progress",
		"dry_run":      dryRun,
		"fix_mode":     fixMode,
	})
}

// runComposerTagScan is the resumable core of the composer-tag scan. It is
// called both on first run (from handleScanComposerTags) and on resume (from
// resumeInterruptedOperations). Already-processed files are skipped by
// checking existing OperationResult rows, making the function idempotent.
func (s *Server) runComposerTagScan(
	ctx context.Context,
	opID string,
	params operations.ComposerScanParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading library data")

	// --- Bulk load (eliminates N+1 DB queries) ---
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}
	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}
	allFiles, err := store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("GetAllBookFiles: %w", err)
	}
	filesByBook := make(map[string][]database.BookFile, len(allFiles))
	for i := range allFiles {
		f := &allFiles[i]
		filesByBook[f.BookID] = append(filesByBook[f.BookID], *f)
	}

	// Load already-processed file paths from a previous (interrupted) run.
	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true // BookID field stores the file path for this operation
	}

	// Build work queue, skipping already-processed files.
	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true}
	var workItems []composerScanWork
	for i := range allBooks {
		b := &allBooks[i]
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		narrator := ""
		if b.Narrator != nil {
			narrator = *b.Narrator
		}
		for _, f := range filesByBook[b.ID] {
			if f.FilePath == "" || f.Missing {
				continue
			}
			if !audioExts[strings.ToLower(filepath.Ext(f.FilePath))] {
				continue
			}
			if done[f.FilePath] {
				continue // already processed in a previous run
			}
			workItems = append(workItems, composerScanWork{
				bookID:    b.ID,
				bookTitle: b.Title,
				filePath:  f.FilePath,
				author:    author,
				narrator:  narrator,
			})
		}
	}

	totalFiles := len(existingResults) + len(workItems)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] scan-composer-tags %s: %d files total, %d already done, %d to process",
		opID, totalFiles, alreadyDone, len(workItems))
	_ = progress.UpdateProgress(alreadyDone, totalFiles,
		fmt.Sprintf("resuming: %d/%d already processed", alreadyDone, totalFiles))

	if len(workItems) == 0 {
		_ = progress.UpdateProgress(totalFiles, totalFiles, "all files already processed")
		return nil
	}

	// --- Parallel NAS reads ---
	const workers = 8
	workCh := make(chan composerScanWork, len(workItems))
	for _, w := range workItems {
		workCh <- w
	}
	close(workCh)

	var completed int64 = int64(alreadyDone)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				if ctx.Err() != nil {
					return
				}
				if _, statErr := os.Stat(w.filePath); statErr != nil {
					// File missing on disk — record as skipped so it's not retried
					_ = store.CreateOperationResult(&database.OperationResult{
						OperationID: opID,
						BookID:      w.filePath,
						ResultJSON:  `{"category":"missing"}`,
						Status:      "missing",
					})
					atomic.AddInt64(&completed, 1)
					continue
				}

				tags, readErr := metadata.ReadRawTags(w.filePath)
				var r composerTagResult
				if readErr != nil {
					r = composerTagResult{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: "read_error", Error: readErr.Error(),
					}
				} else {
					composer := ""
					if vs, ok := tags["COMPOSER"]; ok && len(vs) > 0 {
						composer = strings.TrimSpace(vs[0])
					}
					category, willWrite := categorizeComposer(composer, w.author, w.narrator, params.FixMode)
					r = composerTagResult{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: category, Composer: composer,
						Author: w.author, Narrator: w.narrator, WillWrite: willWrite,
					}
					if !params.DryRun && category != "ok" && willWrite != composer {
						if writeErr := metadata.WriteSingleTag(w.filePath, "COMPOSER", willWrite); writeErr != nil {
							r.Error = writeErr.Error()
							log.Printf("[WARN] scan-composer-tags %s: write failed %s: %v", opID, w.filePath, writeErr)
						} else {
							r.Applied = true
							log.Printf("[INFO] scan-composer-tags %s: COMPOSER %q→%q %s", opID, composer, willWrite, w.filePath)
						}
					}
				}

				resultJSON, _ := json.Marshal(r)
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      w.filePath, // file path as unique key per file
					ResultJSON:  string(resultJSON),
					Status:      r.Category,
				})

				n := atomic.AddInt64(&completed, 1)
				mu.Lock()
				_ = progress.UpdateProgress(int(n), totalFiles,
					fmt.Sprintf("scanned %d/%d files", n, totalFiles))
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	finalCount := atomic.LoadInt64(&completed)
	_ = progress.UpdateProgress(int(finalCount), totalFiles, "scan complete")
	log.Printf("[INFO] scan-composer-tags %s: finished %d/%d files", opID, finalCount, totalFiles)
	return nil
}

// handleGetComposerScanResults returns the aggregated results for a completed
// (or in-progress) composer_tag_scan operation.
func (s *Server) handleGetComposerScanResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "composer_tag_scan" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a composer_tag_scan operation"})
		return
	}

	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	counts := map[string]int{}
	var problems []composerTagResult
	for _, raw := range rawResults {
		var r composerTagResult
		if err := json.Unmarshal([]byte(raw.ResultJSON), &r); err != nil {
			continue
		}
		counts[r.Category]++
		if r.Category != "ok" && r.Category != "missing" {
			problems = append(problems, r)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_category":  counts,
		"problems":     len(problems),
		"details":      problems,
	})
}

// humanizeBytes turns a byte count into a short "1.23 GB" style string.
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---------------------------------------------------------------------------
// Relink missing organizer books to iTunes source files
// ---------------------------------------------------------------------------

type relinkMissingResult struct {
	BookID     string   `json:"book_id"`
	Title      string   `json:"title"`
	OldPath    string   `json:"old_path"`
	NewPath    string   `json:"new_path,omitempty"`
	Action     string   `json:"action"` // "relinked", "unresolved", "ambiguous"
	Matches    int      `json:"matches,omitempty"`
	MatchPaths []string `json:"match_paths,omitempty"`
	Applied    bool     `json:"applied"`
	Error      string   `json:"error,omitempty"`
}

// handleRelinkMissingToiTunes finds books whose file_path is under the organizer
// root but no longer exists on disk, then searches the iTunes media folder for
// the original source file by author+title and relinks the DB records to it.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update book and book_files rows
//   - itunes_root   — override config.ITunesMediaRoot for this call
func (s *Server) handleRelinkMissingToiTunes(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	iTunesRoot := c.DefaultQuery("itunes_root", config.AppConfig.ITunesMediaRoot)
	organizerRoot := config.AppConfig.RootDir

	if iTunesRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itunes_media_root not configured; pass ?itunes_root=<path> or set itunes_media_root in settings"})
		return
	}
	if organizerRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir not configured"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}

	// findInITunes searches iTunesRoot for iTunes album directories (or single
	// audio files) matching the given author + title. Results are deduplicated
	// by album directory so a 10-track book returns exactly one match, not 10.
	findInITunes := func(authorName, title string) []string {
		// 25-char prefix keeps enough specificity while accommodating iTunes
		// filename truncation (many files are cut off before 40 chars).
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		titlePrefixLower := strings.ToLower(titlePrefix)

		// First significant word of author for loose directory matching.
		authorWord := authorName
		if idx := strings.Index(authorName, " "); idx > 0 {
			authorWord = authorName[:idx]
		}
		authorWordLower := strings.ToLower(authorWord)

		// dirMatches collects unique iTunes album dirs (or single files).
		dirMatches := map[string]struct{}{}

		entries, err := os.ReadDir(iTunesRoot)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Name()), authorWordLower) {
				continue
			}
			authorDir := filepath.Join(iTunesRoot, entry.Name())

			albumEntries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, album := range albumEntries {
				albumPath := filepath.Join(authorDir, album.Name())
				if album.IsDir() {
					// Match on album dir name first (fast path).
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
					// Fall back: scan files inside the album dir.
					_ = filepath.WalkDir(albumPath, func(path string, d os.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return nil
						}
						if !audioExts[strings.ToLower(filepath.Ext(path))] {
							return nil
						}
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
						return nil
					})
				} else {
					// Single audio file directly under the author dir.
					if !audioExts[strings.ToLower(filepath.Ext(albumPath))] {
						continue
					}
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
				}
			}
		}

		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}

	// leadingNumRE strips leading track numbers like "01 ", "01 - ", "12 " from
	// iTunes filenames before comparing them to the book title.
	leadingNumRE := regexp.MustCompile(`^\d+\s*[-.]?\s*`)

	trailingNumRE := regexp.MustCompile(`\s+\d+$`)

	// disambiguate narrows multiple iTunes matches to a single best match using
	// a scoring heuristic. Returns "" if still ambiguous after scoring.
	disambiguate := func(matches []string, authorName, title string) string {
		titleLower := strings.ToLower(title)

		type candidate struct {
			path  string
			score int
		}
		cands := make([]candidate, 0, len(matches))

		for _, p := range matches {
			base := filepath.Base(p)
			ext := filepath.Ext(base)
			stemRaw := strings.TrimSuffix(base, ext)
			leadingNum := leadingNumRE.FindString(stemRaw)
			stemNoNum := leadingNumRE.ReplaceAllString(stemRaw, "")
			stemLower := strings.ToLower(stemNoNum)
			// Normalize underscores/colons for comparison.
			stemNorm := strings.ReplaceAll(strings.ReplaceAll(stemLower, "_", " "), ":", " ")

			sc := 0

			switch {
			case stemNorm == titleLower:
				// Perfect stem match.
				sc += 100

			case strings.HasPrefix(stemNorm, titleLower):
				// Title is a prefix of the stem — check the trailing rest.
				rest := stemNorm[len(titleLower):] // intentionally NOT TrimSpace
				switch {
				case regexp.MustCompile(`^\s+book\s+\d`).MatchString(rest),
					regexp.MustCompile(`^\s+\d+$`).MatchString(rest):
					// Trailing "book N" or bare " 2" → likely a sequel.
					sc += 20
				default:
					// Subtitle / series tag after the title — acceptable.
					sc += 60
				}

			case strings.HasPrefix(titleLower, stemNorm) && len(stemNorm) >= 10:
				// The stem is a prefix of the title: iTunes truncated the filename
				// mid-word. Only credit this if the match is long enough (≥10 chars)
				// to avoid false positives.
				sc += 80

			case strings.Contains(stemNorm, titleLower):
				sc += 10
			}

			// Penalize stems that end with a plain number: likely "part 1" / "part 2".
			if trailingNumRE.MatchString(stemNorm) {
				sc -= 30
			}

			// Prefer files without a leading track number — they are usually the
			// "album" file, not an individual track.
			if leadingNum == "" {
				sc += 20
			} else {
				// Among tracked files, lower track numbers are preferred.
				// Use integer value so "01" beats "12" by a small margin.
				if n, err := strconv.Atoi(strings.TrimSpace(
					strings.TrimRight(strings.TrimRight(leadingNum, " "), "-."))); err == nil {
					sc -= n * 2
				}
			}

			// Prefer the author dir that best matches the book's stored author.
			authorDir := filepath.Base(filepath.Dir(p))
			if strings.EqualFold(authorDir, authorName) {
				sc += 40 // exact author dir match
			} else if strings.Contains(strings.ToLower(authorDir), strings.ToLower(authorName)) {
				sc += 20 // author name is substring of dir
			}

			// Shorter filenames are less likely to carry extra series/subtitle info.
			sc -= len(base) / 8

			cands = append(cands, candidate{path: p, score: sc})
		}

		sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

		// If every candidate has the same normalized stem (all are tracks of the
		// same audiobook), pick the one with the lowest track number, which ends
		// up at the top after sorting by score.
		if len(cands) > 1 {
			stemOf := func(p string) string {
				b := filepath.Base(p)
				s := strings.TrimSuffix(b, filepath.Ext(b))
				s = strings.ToLower(leadingNumRE.ReplaceAllString(s, ""))
				s = strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), ":", " ")
				return s
			}
			first := stemOf(cands[0].path)
			allSame := true
			for _, c := range cands[1:] {
				if stemOf(c.path) != first {
					allSame = false
					break
				}
			}
			if allSame {
				return cands[0].path
			}
		}

		// Require a gap of ≥15 before committing to one candidate.
		if len(cands) >= 2 && cands[0].score-cands[1].score >= 15 {
			return cands[0].path
		}
		if len(cands) == 1 {
			return cands[0].path
		}
		return ""
	}

	var results []relinkMissingResult
	relinked, unresolved, ambiguous, skipped := 0, 0, 0, 0

	for i := range allBooks {
		book := &allBooks[i]
		fp := book.FilePath
		if !strings.HasPrefix(fp, organizerRoot) {
			skipped++
			continue
		}
		if _, err := os.Stat(fp); err == nil {
			skipped++
			continue
		}

		// Book path is under organizer root and doesn't exist — candidate.
		// Derive author name from the organizer path (first component after root)
		// so we don't need a DB join. Fall back to DB author lookup if path is
		// ambiguous (e.g. file directly in root).
		rel := strings.TrimPrefix(fp, organizerRoot)
		rel = strings.TrimPrefix(rel, string(os.PathSeparator))
		authorName := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		if authorName == "" || authorName == filepath.Base(fp) {
			// path is too flat — try DB author
			if book.Author != nil {
				authorName = book.Author.Name
			} else if book.AuthorID != nil {
				if a, err := store.GetAuthorByID(*book.AuthorID); err == nil && a != nil {
					authorName = a.Name
				}
			}
		}
		if authorName == "" {
			results = append(results, relinkMissingResult{
				BookID:  book.ID,
				Title:   book.Title,
				OldPath: fp,
				Action:  "unresolved",
				Error:   "no author name",
			})
			unresolved++
			continue
		}

		matches := findInITunes(authorName, book.Title)

		res := relinkMissingResult{
			BookID:  book.ID,
			Title:   book.Title,
			OldPath: fp,
			Matches: len(matches),
		}

		switch len(matches) {
		case 0:
			res.Action = "unresolved"
			unresolved++
		case 1:
			res.Action = "relinked"
			res.NewPath = matches[0]
			relinked++
			if !dryRun {
				newFP := matches[0]
				fi, _ := os.Stat(newFP)

				// Update book.file_path
				book.FilePath = newFP
				if _, upErr := store.UpdateBook(book.ID, book); upErr != nil {
					res.Error = "UpdateBook: " + upErr.Error()
					res.Action = "unresolved"
					unresolved++
					relinked--
					break
				}

				// Update all book_files that pointed to the old organizer path.
				// newFP may be a directory (multi-file book) or a single audio file.
				bookFiles, bfErr := store.GetBookFiles(book.ID)
				if bfErr == nil {
					for j := range bookFiles {
						bf := &bookFiles[j]
						if !strings.HasPrefix(bf.FilePath, organizerRoot) {
							continue
						}
						bf.FilePath = newFP
						bf.OriginalFilename = filepath.Base(newFP)
						bf.Missing = false
						if fi != nil && !fi.IsDir() {
							bf.FileSize = fi.Size()
							ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(newFP), "."))
							if ext != "" {
								bf.Format = ext
							}
						}
						_ = store.UpdateBookFile(bf.ID, bf)
					}
				}
				res.Applied = true
			}
		default:
			if best := disambiguate(matches, authorName, book.Title); best != "" {
				// Disambiguation picked a winner — treat as single match.
				res.Action = "relinked"
				res.NewPath = best
				res.MatchPaths = matches // keep all matches for auditing
				relinked++
				if !dryRun {
					newFP := best
					fi, _ := os.Stat(newFP)
					book.FilePath = newFP
					if _, upErr := store.UpdateBook(book.ID, book); upErr != nil {
						res.Error = "UpdateBook: " + upErr.Error()
						res.Action = "unresolved"
						unresolved++
						relinked--
						break
					}
					bookFiles, bfErr := store.GetBookFiles(book.ID)
					if bfErr == nil {
						for j := range bookFiles {
							bf := &bookFiles[j]
							if !strings.HasPrefix(bf.FilePath, organizerRoot) {
								continue
							}
							bf.FilePath = newFP
							bf.OriginalFilename = filepath.Base(newFP)
							bf.Missing = false
							if fi != nil && !fi.IsDir() {
								bf.FileSize = fi.Size()
								ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(newFP), "."))
								if ext != "" {
									bf.Format = ext
								}
							}
							_ = store.UpdateBookFile(bf.ID, bf)
						}
					}
					res.Applied = true
				}
			} else {
				res.Action = "ambiguous"
				res.MatchPaths = matches
				ambiguous++
			}
		}

		results = append(results, res)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":    dryRun,
		"relinked":   relinked,
		"ambiguous":  ambiguous,
		"unresolved": unresolved,
		"skipped":    skipped,
		"results":    results,
	})
}

// ---------------------------------------------------------------------------
// Async resumable missing-file path repair
// ---------------------------------------------------------------------------

// bookFileMeta is a lightweight holder used by runMissingFileRepair to pass
// title and author to the per-file worker without keeping all Book fields alive.
type bookFileMeta struct {
	title  string
	author string
}

type missingFileRepairResult struct {
	FileID  string `json:"file_id"`
	BookID  string `json:"book_id"`
	Title   string `json:"book_title"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path,omitempty"`
	// Method values: "pid", "filename", "truncation", "author_title",
	// "skipped", "unresolved", "ambiguous"
	Method  string `json:"method"`
	Matches int    `json:"matches,omitempty"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// handleRepairMissingFiles starts an async, resumable missing-file path-repair
// operation. For each book_file row whose stored path doesn't exist on disk it
// tries four escalating strategies — PID lookup, exact filename, stem
// truncation, author+title walk — and on a confident single match updates only
// the file_path field of the existing record.
//
// Never creates new Book or BookFile rows, so the dedup pipeline is never
// triggered.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update book_file rows
//
// Poll progress: GET /api/v1/operations/{id}
// View results:  GET /api/v1/maintenance/repair-missing-files/{id}
func (s *Server) handleRepairMissingFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	roots := []string{config.AppConfig.ITunesMediaRoot, config.AppConfig.RootDir}
	var searchRoots []string
	for _, r := range roots {
		if r != "" {
			searchRoots = append(searchRoots, r)
		}
	}

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "missing-file-repair", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.MissingFileRepairParams{DryRun: dryRun, SearchRoots: searchRoots}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] repair-missing-files: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runMissingFileRepair(ctx, capturedOpID, capturedParams, store, progress)
	}
	if err := s.queue.Enqueue(opID, "missing-file-repair", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] repair-missing-files: queued %s dry_run=%v roots=%v", opID, dryRun, searchRoots)
	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "missing file repair started — poll GET /api/v1/operations/" + opID,
		"dry_run":      dryRun,
		"search_roots": searchRoots,
	})
}

// runMissingFileRepair is the resumable core. Idempotent: files already
// processed in a prior run are detected via existing OperationResult rows
// (keyed by book_file ID) and skipped.
func (s *Server) runMissingFileRepair(
	ctx context.Context,
	opID string,
	params operations.MissingFileRepairParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading library data")

	allFiles, err := store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("GetAllBookFiles: %w", err)
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}
	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}
	metaByBook := make(map[string]bookFileMeta, len(allBooks))
	for i := range allBooks {
		b := &allBooks[i]
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		metaByBook[b.ID] = bookFileMeta{title: b.Title, author: author}
	}

	// Collect candidates: files the DB thinks exist but os.Stat disagrees.
	var candidates []database.BookFile
	for i := range allFiles {
		f := &allFiles[i]
		if f.FilePath == "" || f.Missing {
			continue
		}
		if _, statErr := os.Stat(f.FilePath); statErr == nil {
			continue
		}
		candidates = append(candidates, *f)
	}

	// Skip files already processed in a prior run.
	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true // BookID stores file_id for this operation
	}
	var work []database.BookFile
	for _, f := range candidates {
		if !done[f.ID] {
			work = append(work, f)
		}
	}

	totalFiles := len(existingResults) + len(work)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] repair-missing-files %s: %d candidates, %d already done, %d to process",
		opID, totalFiles, alreadyDone, len(work))
	_ = progress.UpdateProgress(alreadyDone, totalFiles,
		fmt.Sprintf("resuming: %d/%d already processed", alreadyDone, totalFiles))

	if len(work) == 0 {
		_ = progress.UpdateProgress(totalFiles, totalFiles, "all files already processed")
		return nil
	}

	// Parse iTunes XML once for PID-based lookups.
	pidToLocation := make(map[string]string)
	if xmlPath := config.AppConfig.ITunesLibraryReadPath; xmlPath != "" {
		if lib, parseErr := itunes.ParseLibrary(xmlPath); parseErr != nil {
			log.Printf("[WARN] repair-missing-files %s: iTunes XML parse error: %v", opID, parseErr)
		} else {
			for _, track := range lib.Tracks {
				if track.PersistentID != "" && track.Location != "" {
					pidToLocation[track.PersistentID] = track.Location
				}
			}
			log.Printf("[INFO] repair-missing-files %s: loaded %d PID→location entries", opID, len(pidToLocation))
		}
	}

	itunesOpts := itunes.ImportOptions{PathMappings: make([]itunes.PathMapping, len(config.AppConfig.ITunesPathMappings))}
	for i, m := range config.AppConfig.ITunesPathMappings {
		itunesOpts.PathMappings[i] = itunes.PathMapping{From: m.From, To: m.To}
	}

	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true, ".opus": true}

	// Build a basename→paths filename index across all search roots (once, lazily).
	var filenameIdx map[string][]string
	var idxOnce sync.Once
	buildIdx := func() {
		idxOnce.Do(func() {
			_ = progress.UpdateProgress(alreadyDone, totalFiles, "building filename index…")
			idx := make(map[string][]string, 200000)
			for _, root := range params.SearchRoots {
				_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil || d.IsDir() {
						return nil
					}
					if audioExts[strings.ToLower(filepath.Ext(path))] {
						base := filepath.Base(path)
						idx[base] = append(idx[base], path)
					}
					return nil
				})
			}
			filenameIdx = idx
			log.Printf("[INFO] repair-missing-files %s: filename index built (%d unique names)", opID, len(idx))
		})
	}

	var completed int64 = int64(alreadyDone)
	var mu sync.Mutex

	workCh := make(chan database.BookFile, len(work))
	for _, f := range work {
		workCh <- f
	}
	close(workCh)

	var wg sync.WaitGroup
	const workers = 4
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range workCh {
				if ctx.Err() != nil {
					return
				}
				res := s.repairOneMissingFile(f, metaByBook, pidToLocation, itunesOpts,
					params, audioExts, buildIdx, func() map[string][]string {
						mu.Lock()
						defer mu.Unlock()
						return filenameIdx
					}, store, opID)

				resultJSON, _ := json.Marshal(res)
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      res.Method,
				})
				n := atomic.AddInt64(&completed, 1)
				mu.Lock()
				_ = progress.UpdateProgress(int(n), totalFiles, fmt.Sprintf("processed %d/%d", n, totalFiles))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	finalCount := atomic.LoadInt64(&completed)
	activity.FlushOperation(s.activityWriter, opID)
	msg := fmt.Sprintf("Repaired %d of %d missing files", finalCount, totalFiles)
	_ = progress.UpdateProgress(int(finalCount), totalFiles, msg)
	log.Printf("[INFO] repair-missing-files %s: finished %d/%d files", opID, finalCount, totalFiles)
	activity.EmitInfo(s.activityWriter, opID, "missing-file-repair", "repair-missing-files", msg,
		activity.TagsIf(finalCount == 0, activity.NoOpTag)...)
	return nil
}

// repairOneMissingFile tries four strategies in order and returns a result.
// It only calls UpdateBookFile — never CreateBook or CreateBookFile.
func (s *Server) repairOneMissingFile(
	f database.BookFile,
	metaByBook map[string]bookFileMeta,
	pidToLocation map[string]string,
	itunesOpts itunes.ImportOptions,
	params operations.MissingFileRepairParams,
	audioExts map[string]bool,
	buildIdx func(),
	getIdx func() map[string][]string,
	store database.Store,
	opID string,
) missingFileRepairResult {
	bm := metaByBook[f.BookID]
	res := missingFileRepairResult{
		FileID:  f.ID,
		BookID:  f.BookID,
		Title:   bm.title,
		OldPath: f.FilePath,
	}

	// Re-check: another goroutine or prior session may have fixed it.
	if _, statErr := os.Stat(f.FilePath); statErr == nil {
		res.Method = "skipped"
		return res
	}

	candidate, method := "", ""

	// Tier 1: iTunes PID → XML Location → RemapPath
	if candidate == "" && f.ITunesPersistentID != "" {
		if loc, ok := pidToLocation[f.ITunesPersistentID]; ok {
			remapped := itunesOpts.RemapPath(loc)
			if remapped != "" && remapped != loc {
				if _, statErr := os.Stat(remapped); statErr == nil {
					candidate, method = remapped, "pid"
				}
			}
		}
	}

	// Tier 2: exact basename search across filename index
	if candidate == "" {
		buildIdx()
		base := filepath.Base(f.FilePath)
		idx := getIdx()
		paths := idx[base]
		switch len(paths) {
		case 1:
			candidate, method = paths[0], "filename"
			res.Matches = 1
		case 0:
			// no match
		default:
			// Multiple — narrow by parent dir name (album folder)
			parentDir := filepath.Base(filepath.Dir(f.FilePath))
			var narrowed []string
			for _, p := range paths {
				if strings.EqualFold(filepath.Base(filepath.Dir(p)), parentDir) {
					narrowed = append(narrowed, p)
				}
			}
			// If still multiple, narrow by grandparent dir containing author's last name.
			// iTunes multi-author dirs ("Amy DuBoff, Michael Anderle") contain the stored
			// author ("Michael Anderle") as a substring; this resolves those cases.
			if len(narrowed) > 1 && bm.author != "" {
				lastName := strings.ToLower(bm.author)
				if i := strings.LastIndex(lastName, " "); i > 0 {
					lastName = lastName[i+1:]
				}
				var n2 []string
				for _, p := range narrowed {
					if strings.Contains(strings.ToLower(filepath.Base(filepath.Dir(filepath.Dir(p)))), lastName) {
						n2 = append(n2, p)
					}
				}
				if len(n2) >= 1 {
					narrowed = n2
				}
			}
			switch len(narrowed) {
			case 1:
				candidate, method = narrowed[0], "filename"
				res.Matches = 1
			case 0:
				// Parent-dir narrowing eliminated all candidates — file likely moved to a
				// different parent dir. Fall through to Tier 3/4 for broader search.
			default:
				res.Method = "ambiguous"
				res.Matches = len(narrowed)
				return res
			}
		}
	}

	// Tier 3: stem-prefix match in the same directory (truncated filename)
	if candidate == "" {
		dir := filepath.Dir(f.FilePath)
		base := filepath.Base(f.FilePath)
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		if entries, readErr := os.ReadDir(dir); readErr == nil {
			for _, de := range entries {
				if de.IsDir() {
					continue
				}
				name := de.Name()
				nameExt := filepath.Ext(name)
				nameStem := strings.TrimSuffix(name, nameExt)
				if strings.EqualFold(nameExt, ext) &&
					strings.HasPrefix(nameStem, stem) &&
					name != base &&
					len(nameStem) > len(stem) &&
					nameStem[len(stem)] != ' ' {
					candidate, method = filepath.Join(dir, name), "truncation"
					break
				}
			}
		}
	}

	// Tier 4: author last-name + title-prefixed album dir, then stored basename.
	// Uses the author's last name so it matches both "Michael Anderle" and
	// "Amy DuBoff, Michael Anderle" directories. Matches album dirs whose name
	// starts with the title prefix, then looks for the stored filename within
	// that album dir (avoids false ambiguity from multiple tracks per album).
	if candidate == "" && bm.author != "" && bm.title != "" {
		lastName := bm.author
		if i := strings.LastIndex(bm.author, " "); i > 0 {
			lastName = bm.author[i+1:]
		}
		titlePrefix := bm.title
		if len(titlePrefix) > 30 {
			titlePrefix = titlePrefix[:30]
		}
		storedBase := filepath.Base(f.FilePath)
		var matches []string
		for _, root := range params.SearchRoots {
			entries, rerr := os.ReadDir(root)
			if rerr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(lastName)) {
					continue
				}
				authorDir := filepath.Join(root, entry.Name())
				albumEntries, aErr := os.ReadDir(authorDir)
				if aErr != nil {
					continue
				}
				for _, album := range albumEntries {
					if !album.IsDir() {
						continue
					}
					if !strings.HasPrefix(strings.ToLower(album.Name()), strings.ToLower(titlePrefix)) {
						continue
					}
					// Prefer the exact stored basename within this album dir.
					exact := filepath.Join(authorDir, album.Name(), storedBase)
					if _, statErr := os.Stat(exact); statErr == nil {
						matches = append(matches, exact)
						continue
					}
					// Fall back: any audio file in the album dir (single-track books).
					albumFiles, _ := os.ReadDir(filepath.Join(authorDir, album.Name()))
					var audioInAlbum []string
					for _, af := range albumFiles {
						if !af.IsDir() && audioExts[strings.ToLower(filepath.Ext(af.Name()))] {
							audioInAlbum = append(audioInAlbum, filepath.Join(authorDir, album.Name(), af.Name()))
						}
					}
					if len(audioInAlbum) == 1 {
						matches = append(matches, audioInAlbum[0])
					}
				}
			}
		}
		switch len(matches) {
		case 1:
			candidate, method = matches[0], "author_title"
			res.Matches = 1
		case 0:
			// no match
		default:
			res.Method = "ambiguous"
			res.Matches = len(matches)
			return res
		}
	}

	// Tier 4b: flat iTunes library — M4B files directly in the author dir (no album subdir).
	// iTunes sometimes consolidates individual MP3 tracks into a single M4B per book, stored
	// flat under the author dir. The stored basename looks like "01 Defending the Lost.mp3";
	// after stripping the leading track number we get "Defending the Lost", which we match
	// against stems of audio files directly under any co-author dir containing the last name.
	if candidate == "" && bm.author != "" {
		lastName := bm.author
		if i := strings.LastIndex(bm.author, " "); i > 0 {
			lastName = bm.author[i+1:]
		}
		storedBase := filepath.Base(f.FilePath)
		storedStem := strings.TrimSuffix(storedBase, filepath.Ext(storedBase))
		// Strip leading "NN " or "NN. " track-number prefix.
		titleFromFile := storedStem
		if i := strings.IndexByte(storedStem, ' '); i > 0 {
			prefix := storedStem[:i]
			isNum := true
			for _, r := range prefix {
				if r < '0' || r > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				titleFromFile = strings.TrimSpace(storedStem[i+1:])
			}
		}

		var matches []string
		for _, root := range params.SearchRoots {
			entries, rerr := os.ReadDir(root)
			if rerr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(lastName)) {
					continue
				}
				authorDir := filepath.Join(root, entry.Name())
				dirFiles, _ := os.ReadDir(authorDir)
				for _, df := range dirFiles {
					if df.IsDir() || !audioExts[strings.ToLower(filepath.Ext(df.Name()))] {
						continue
					}
					fileStem := strings.TrimSuffix(df.Name(), filepath.Ext(df.Name()))
					if strings.EqualFold(fileStem, titleFromFile) {
						matches = append(matches, filepath.Join(authorDir, df.Name()))
					}
				}
			}
		}
		// Among multiple matches, prefer dirs whose name starts with the stored author
		// (i.e. "Michael Anderle, Justin Sloan" over "Amy DuBoff, Michael Anderle").
		if len(matches) > 1 {
			authorLower := strings.ToLower(bm.author)
			var preferred []string
			for _, m := range matches {
				dirName := strings.ToLower(filepath.Base(filepath.Dir(m)))
				if strings.HasPrefix(dirName, authorLower) {
					preferred = append(preferred, m)
				}
			}
			if len(preferred) == 1 {
				matches = preferred
			}
		}
		switch len(matches) {
		case 1:
			candidate, method = matches[0], "flat_stem"
			res.Matches = 1
		case 0:
			// no match
		default:
			res.Method = "ambiguous"
			res.Matches = len(matches)
			return res
		}
	}

	if candidate == "" {
		res.Method = "unresolved"
		return res
	}

	res.NewPath = candidate
	res.Method = method
	res.Matches = 1

	if params.DryRun {
		return res
	}

	fi, _ := os.Stat(candidate)
	f.FilePath = candidate
	f.OriginalFilename = filepath.Base(candidate)
	f.Missing = false
	if fi != nil {
		f.FileSize = fi.Size()
	}
	if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(candidate), ".")); ext != "" {
		f.Format = ext
	}
	if upErr := store.UpdateBookFile(f.ID, &f); upErr != nil {
		res.Error = upErr.Error()
		log.Printf("[WARN] repair-missing-files %s: UpdateBookFile %s: %v", opID, f.ID, upErr)
	} else {
		res.Applied = true
		activity.LogBatch(s.activityWriter, opID, "missing-file-repair", "repair-missing-files",
			activity.BatchItem{Name: filepath.Base(res.OldPath), Detail: method + ": " + candidate})
	}
	return res
}

// handleGetMissingFileRepairResults returns aggregated results for a
// missing_file_repair operation (in-progress or completed).
func (s *Server) handleGetMissingFileRepairResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "missing-file-repair" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a missing-file-repair operation"})
		return
	}
	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	byMethod := map[string]int{}
	var problems []missingFileRepairResult
	repaired, unresolved, ambiguous, skipped := 0, 0, 0, 0
	for _, raw := range rawResults {
		var r missingFileRepairResult
		if jsonErr := json.Unmarshal([]byte(raw.ResultJSON), &r); jsonErr != nil {
			continue
		}
		byMethod[r.Method]++
		switch r.Method {
		case "unresolved":
			unresolved++
			problems = append(problems, r)
		case "ambiguous":
			ambiguous++
			problems = append(problems, r)
		case "skipped":
			skipped++
		default:
			repaired++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_method":    byMethod,
		"repaired":     repaired,
		"unresolved":   unresolved,
		"ambiguous":    ambiguous,
		"skipped":      skipped,
		"problems":     problems,
	})
}

// handleRevertMetadataFetch rolls back all DB changes made by one or more
// bulk_metadata_fetch operations. It reads the OperationResult rows to find
// which books were updated, then restores PreviousValue for every
// ChangeType=fetched MetadataChangeRecord recorded after the operation started.
//
// POST /api/v1/maintenance/revert-metadata-fetch
// Body: {"operation_ids": ["01K...", "01K..."]}
func (s *Server) handleRevertMetadataFetch(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		OperationIDs []string `json:"operation_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.OperationIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation_ids required"})
		return
	}

	// Collect the earliest start time across all operations so we only revert
	// changes that were made by this run (not older fetched records).
	var revertAfter time.Time
	bookIDSet := map[string]bool{}

	for _, opID := range req.OperationIDs {
		op, err := store.GetOperationByID(opID)
		if err != nil || op == nil {
			continue
		}
		if op.Type != "bulk_metadata_fetch" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "operation " + opID + " is not a bulk_metadata_fetch"})
			return
		}
		ts := op.CreatedAt
		if op.StartedAt != nil {
			ts = *op.StartedAt
		}
		if revertAfter.IsZero() || ts.Before(revertAfter) {
			revertAfter = ts
		}

		results, err := store.GetOperationResults(opID)
		if err != nil {
			internalError(c, "failed to load results for "+opID, err)
			return
		}
		for _, r := range results {
			if r.Status == "updated" {
				bookIDSet[r.BookID] = true
			}
		}
	}

	log.Printf("[INFO] revert-metadata-fetch: reverting %d books, changes after %s",
		len(bookIDSet), revertAfter.Format(time.RFC3339))

	reverted := 0
	skipped := 0
	errors := 0

	for bookID := range bookIDSet {
		book, err := store.GetBookByID(bookID)
		if err != nil || book == nil {
			errors++
			continue
		}

		history, err := store.GetBookChangeHistory(bookID, 50)
		if err != nil {
			errors++
			continue
		}

		// Gather the most recent fetched change per field after revertAfter.
		// We want the PreviousValue (what the field was before the fetch ran).
		type revertEntry struct {
			field string
			prev  string // empty string means "was empty before"
		}
		// Use a map so we only take the LAST change per field (most recent op).
		byField := map[string]revertEntry{}
		for _, h := range history {
			if h.ChangeType != "fetched" {
				continue
			}
			if h.ChangedAt.Before(revertAfter) {
				continue
			}
			prev := ""
			if h.PreviousValue != nil {
				// PreviousValue is JSON-encoded string: "\"foo\"" → foo
				if err := json.Unmarshal([]byte(*h.PreviousValue), &prev); err != nil {
					prev = *h.PreviousValue
				}
			}
			byField[h.Field] = revertEntry{field: h.Field, prev: prev}
		}

		if len(byField) == 0 {
			skipped++
			continue
		}

		didChange := false
		for _, e := range byField {
			switch e.field {
			case "title":
				book.Title = e.prev
				didChange = true
			case "author_name":
				if e.prev == "" {
					book.AuthorID = nil
				} else {
					if author, aerr := store.GetAuthorByName(e.prev); aerr == nil && author != nil {
						book.AuthorID = &author.ID
						didChange = true
					}
				}
			case "publisher":
				if e.prev == "" {
					book.Publisher = nil
				} else {
					book.Publisher = &e.prev
				}
				didChange = true
			case "language":
				if e.prev == "" {
					book.Language = nil
				} else {
					book.Language = &e.prev
				}
				didChange = true
			case "audiobook_release_year":
				if e.prev == "" {
					book.AudiobookReleaseYear = nil
				} else if yr, yerr := strconv.Atoi(e.prev); yerr == nil {
					book.AudiobookReleaseYear = &yr
				}
				didChange = true
			case "isbn10":
				if e.prev == "" {
					book.ISBN10 = nil
				} else {
					book.ISBN10 = &e.prev
				}
				didChange = true
			case "isbn13":
				if e.prev == "" {
					book.ISBN13 = nil
				} else {
					book.ISBN13 = &e.prev
				}
				didChange = true
			}
		}

		if didChange {
			if _, uerr := store.UpdateBook(bookID, book); uerr != nil {
				log.Printf("[WARN] revert-metadata-fetch: UpdateBook %s: %v", bookID, uerr)
				errors++
			} else {
				reverted++
			}
		} else {
			skipped++
		}
	}

	log.Printf("[INFO] revert-metadata-fetch: done — reverted:%d skipped:%d errors:%d", reverted, skipped, errors)
	c.JSON(http.StatusOK, gin.H{
		"reverted": reverted,
		"skipped":  skipped,
		"errors":   errors,
		"total":    len(bookIDSet),
	})
}

// durationMismatchResult describes one book whose Audible runtime diverges
// significantly from the local file duration.
type durationMismatchResult struct {
	BookID            string `json:"book_id"`
	Title             string `json:"title"`
	ASIN              string `json:"asin,omitempty"`
	FileDurationSec   int    `json:"file_duration_sec"`
	AudibleRuntimeMin int    `json:"audible_runtime_min"`
	AudibleRuntimeSec int    `json:"audible_runtime_sec"`
	DeltaSec          int    `json:"delta_sec"`
}

// handleScanDurationMismatch scans all books that have both a local file
// duration and a stored Audible runtime, and returns those whose delta
// exceeds the configured threshold.
//
// Query params:
//   - max_delta_min=10  (integer, default 10) — threshold in minutes
//
// GET /api/v1/maintenance/scan-duration-mismatch
func (s *Server) handleScanDurationMismatch(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Parse threshold (minutes → seconds). Default = 10 min.
	thresholdMin := 10
	if raw := c.Query("max_delta_min"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			thresholdMin = v
		}
	}
	thresholdSec := thresholdMin * 60

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var mismatches []durationMismatchResult
	scanned := 0

	for i := range allBooks {
		book := &allBooks[i]
		if book.Duration == nil || *book.Duration <= 0 {
			continue
		}
		if book.AudibleRuntimeMin == nil || *book.AudibleRuntimeMin <= 0 {
			continue
		}
		scanned++

		fileDurSec := *book.Duration
		audibleSec := *book.AudibleRuntimeMin * 60
		delta := fileDurSec - audibleSec
		if delta < 0 {
			delta = -delta
		}
		if delta <= thresholdSec {
			continue
		}

		asin := ""
		if book.ASIN != nil {
			asin = *book.ASIN
		}
		mismatches = append(mismatches, durationMismatchResult{
			BookID:            book.ID,
			Title:             book.Title,
			ASIN:              asin,
			FileDurationSec:   fileDurSec,
			AudibleRuntimeMin: *book.AudibleRuntimeMin,
			AudibleRuntimeSec: audibleSec,
			DeltaSec:          delta,
		})
	}

	// Sort by largest delta first so the worst mismatches appear at the top.
	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].DeltaSec > mismatches[j].DeltaSec
	})

	log.Printf("[INFO] scan-duration-mismatch: scanned=%d threshold=%dmin mismatches=%d",
		scanned, thresholdMin, len(mismatches))

	c.JSON(http.StatusOK, gin.H{
		"threshold_min":  thresholdMin,
		"scanned":        scanned,
		"mismatch_count": len(mismatches),
		"mismatches":     mismatches,
	})
}

// ---------------------------------------------------------------------------
// RELINK-4: dry-run relink report (read-only triage endpoint)
// ---------------------------------------------------------------------------

// relinkReportResolved is a single successfully-resolvable book entry returned
// by handleRelinkReport.
type relinkReportResolved struct {
	BookID  string `json:"book_id"`
	Title   string `json:"title"`
	NewPath string `json:"new_path"`
}

// relinkReportUnresolved is a single unresolvable book entry returned by
// handleRelinkReport, annotated with the reason it could not be relinked.
type relinkReportUnresolved struct {
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	OldPath       string   `json:"old_path"`
	WhyUnresolved string   `json:"why_unresolved"`
	MatchPaths    []string `json:"match_paths,omitempty"` // present when action=="ambiguous"
}

// handleRelinkReport re-runs the relink dry-run logic over ALL books and
// returns which ones would be successfully relinked vs. those that remain
// unresolved (with a why_unresolved annotation for triage).
//
// This endpoint is purely read-only — it never modifies the database.
//
// Query params:
//   - limit=N   (integer, default 0 = all)  — page size
//   - offset=N  (integer, default 0)        — page offset (into unresolved list)
//
// GET /api/v1/maintenance/relink-report
func (s *Server) handleRelinkReport(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	iTunesRoot := c.DefaultQuery("itunes_root", config.AppConfig.ITunesMediaRoot)
	organizerRoot := config.AppConfig.RootDir

	if iTunesRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itunes_media_root not configured; pass ?itunes_root=<path> or set itunes_media_root in settings"})
		return
	}
	if organizerRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir not configured"})
		return
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			offset = v
		}
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}

	// findInITunes is identical to the one in handleRelinkMissingToiTunes.
	findInITunes := func(authorName, title string) []string {
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		titlePrefixLower := strings.ToLower(titlePrefix)
		authorWord := authorName
		if idx := strings.Index(authorName, " "); idx > 0 {
			authorWord = authorName[:idx]
		}
		authorWordLower := strings.ToLower(authorWord)

		dirMatches := map[string]struct{}{}
		entries, err := os.ReadDir(iTunesRoot)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Name()), authorWordLower) {
				continue
			}
			authorDir := filepath.Join(iTunesRoot, entry.Name())
			albumEntries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, album := range albumEntries {
				albumPath := filepath.Join(authorDir, album.Name())
				if album.IsDir() {
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
					_ = filepath.WalkDir(albumPath, func(path string, d os.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return nil
						}
						if !audioExts[strings.ToLower(filepath.Ext(path))] {
							return nil
						}
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
						return nil
					})
				} else {
					if !audioExts[strings.ToLower(filepath.Ext(albumPath))] {
						continue
					}
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
				}
			}
		}

		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}

	leadingNumRE := regexp.MustCompile(`^\d+\s*[-.]?\s*`)
	trailingNumRE := regexp.MustCompile(`\s+\d+$`)

	disambiguate := func(matches []string, authorName, title string) string {
		titleLower := strings.ToLower(title)
		type candidate struct {
			path  string
			score int
		}
		cands := make([]candidate, 0, len(matches))
		for _, p := range matches {
			base := filepath.Base(p)
			ext := filepath.Ext(base)
			stemRaw := strings.TrimSuffix(base, ext)
			leadingNum := leadingNumRE.FindString(stemRaw)
			stemNoNum := leadingNumRE.ReplaceAllString(stemRaw, "")
			stemLower := strings.ToLower(stemNoNum)
			stemNorm := strings.ReplaceAll(strings.ReplaceAll(stemLower, "_", " "), ":", " ")
			sc := 0
			switch {
			case stemNorm == titleLower:
				sc += 100
			case strings.HasPrefix(stemNorm, titleLower):
				rest := stemNorm[len(titleLower):]
				switch {
				case regexp.MustCompile(`^\s+book\s+\d`).MatchString(rest),
					regexp.MustCompile(`^\s+\d+$`).MatchString(rest):
					sc += 20
				default:
					sc += 60
				}
			case strings.HasPrefix(titleLower, stemNorm) && len(stemNorm) >= 10:
				sc += 80
			case strings.Contains(stemNorm, titleLower):
				sc += 10
			}
			if trailingNumRE.MatchString(stemNorm) {
				sc -= 30
			}
			if leadingNum == "" {
				sc += 20
			} else {
				if n, err := strconv.Atoi(strings.TrimSpace(
					strings.TrimRight(strings.TrimRight(leadingNum, " "), "-."))); err == nil {
					sc -= n * 2
				}
			}
			authorDir := filepath.Base(filepath.Dir(p))
			if strings.EqualFold(authorDir, authorName) {
				sc += 40
			} else if strings.Contains(strings.ToLower(authorDir), strings.ToLower(authorName)) {
				sc += 20
			}
			sc -= len(base) / 8
			cands = append(cands, candidate{path: p, score: sc})
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
		if len(cands) > 1 {
			stemOf := func(p string) string {
				b := filepath.Base(p)
				s := strings.TrimSuffix(b, filepath.Ext(b))
				s = strings.ToLower(leadingNumRE.ReplaceAllString(s, ""))
				s = strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), ":", " ")
				return s
			}
			first := stemOf(cands[0].path)
			allSame := true
			for _, c := range cands[1:] {
				if stemOf(c.path) != first {
					allSame = false
					break
				}
			}
			if allSame {
				return cands[0].path
			}
		}
		if len(cands) >= 2 && cands[0].score-cands[1].score >= 15 {
			return cands[0].path
		}
		if len(cands) == 1 {
			return cands[0].path
		}
		return ""
	}

	var resolved []relinkReportResolved
	var unresolved []relinkReportUnresolved
	skipped := 0

	for i := range allBooks {
		book := &allBooks[i]
		fp := book.FilePath

		// Only consider books whose path is under the organizer root AND missing.
		if !strings.HasPrefix(fp, organizerRoot) {
			skipped++
			continue
		}
		if _, statErr := os.Stat(fp); statErr == nil {
			skipped++
			continue
		}

		// Derive author name the same way handleRelinkMissingToiTunes does.
		rel := strings.TrimPrefix(fp, organizerRoot)
		rel = strings.TrimPrefix(rel, string(os.PathSeparator))
		authorName := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		if authorName == "" || authorName == filepath.Base(fp) {
			if book.Author != nil {
				authorName = book.Author.Name
			} else if book.AuthorID != nil {
				if a, err := store.GetAuthorByID(*book.AuthorID); err == nil && a != nil {
					authorName = a.Name
				}
			}
		}
		if authorName == "" {
			unresolved = append(unresolved, relinkReportUnresolved{
				BookID:        book.ID,
				Title:         book.Title,
				OldPath:       fp,
				WhyUnresolved: "no author name",
			})
			continue
		}

		matches := findInITunes(authorName, book.Title)

		switch len(matches) {
		case 0:
			unresolved = append(unresolved, relinkReportUnresolved{
				BookID:        book.ID,
				Title:         book.Title,
				OldPath:       fp,
				WhyUnresolved: "no iTunes match found",
			})
		case 1:
			resolved = append(resolved, relinkReportResolved{
				BookID:  book.ID,
				Title:   book.Title,
				NewPath: matches[0],
			})
		default:
			if best := disambiguate(matches, authorName, book.Title); best != "" {
				resolved = append(resolved, relinkReportResolved{
					BookID:  book.ID,
					Title:   book.Title,
					NewPath: best,
				})
			} else {
				unresolved = append(unresolved, relinkReportUnresolved{
					BookID:        book.ID,
					Title:         book.Title,
					OldPath:       fp,
					WhyUnresolved: fmt.Sprintf("ambiguous: %d iTunes matches, none dominant", len(matches)),
					MatchPaths:    matches,
				})
			}
		}
	}

	// Apply pagination to the unresolved list only (resolved list is typically
	// smaller and always useful in full; callers can use offset/limit to page
	// through the unresolved triage queue).
	totalUnresolved := len(unresolved)
	if offset > 0 {
		if offset >= len(unresolved) {
			unresolved = nil
		} else {
			unresolved = unresolved[offset:]
		}
	}
	if limit > 0 && len(unresolved) > limit {
		unresolved = unresolved[:limit]
	}

	log.Printf("[INFO] relink-report: total=%d resolved=%d unresolved=%d skipped=%d (page offset=%d limit=%d)",
		len(allBooks), len(resolved), totalUnresolved, skipped, offset, limit)

	c.JSON(http.StatusOK, gin.H{
		"resolved":          resolved,
		"unresolved":        unresolved,
		"resolved_count":    len(resolved),
		"unresolved_count":  totalUnresolved,
		"skipped":           skipped,
		"offset":            offset,
		"limit":             limit,
	})
}

// handleBulkDelugeImport queues a resumable async operation that calls
// importToLibrary for every book_file that has a deluge_hash but has not
// yet been imported (imported_from_deluge_at IS NULL).
//
// Query params:
//   - dry_run=true (default) — report what would be imported without writing
//   - dry_run=false           — actually copy files
//   - max_books=N             — cap the number of files imported per run (0 = unlimited)
//
// POST /api/v1/maintenance/bulk-deluge-import
func (s *Server) handleBulkDelugeImport(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	maxBooks := 0
	if v := c.Query("max_books"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxBooks = n
		}
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	client := getDelugeClient()

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "bulk-deluge-import", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.BulkImportDelugeParams{DryRun: dryRun, MaxBooks: maxBooks}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] bulk-deluge-import: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	capturedClient := client
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runBulkDelugeImport(ctx, capturedOpID, capturedParams, capturedClient, store, progress)
	}
	if err := s.queue.Enqueue(opID, "bulk-deluge-import", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] bulk-deluge-import: queued %s dry_run=%v max_books=%d", opID, dryRun, maxBooks)
	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "bulk deluge import started — poll GET /api/v1/operations/" + opID,
		"dry_run":      dryRun,
		"max_books":    maxBooks,
	})
}

// runBulkDelugeImport is the resumable core. Files already imported in a
// prior run (imported_from_deluge_at IS NOT NULL) are filtered out by the
// DB query so the operation is inherently idempotent.
func (s *Server) runBulkDelugeImport(
	ctx context.Context,
	opID string,
	params operations.BulkImportDelugeParams,
	client *deluge.Client,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading pending files")

	pending, err := store.GetBookFilesNeedingDelugeImport()
	if err != nil {
		return fmt.Errorf("GetBookFilesNeedingDelugeImport: %w", err)
	}
	if params.MaxBooks > 0 && len(pending) > params.MaxBooks {
		pending = pending[:params.MaxBooks]
	}

	total := len(pending)
	log.Printf("[INFO] bulk-deluge-import %s: %d files pending (dry_run=%v)", opID, total, params.DryRun)
	_ = progress.UpdateProgress(0, total, fmt.Sprintf("found %d files to import", total))

	imported, failed := 0, 0
	for i := range pending {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f := &pending[i]
		if params.DryRun {
			resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "action": "dry_run"})
			_ = store.CreateOperationResult(&database.OperationResult{
				OperationID: opID,
				BookID:      f.ID,
				ResultJSON:  string(resultJSON),
				Status:      "dry_run",
			})
			imported++
		} else {
			newPath, importErr := importToLibrary(&config.AppConfig, client, store, f)
			if importErr != nil {
				log.Printf("[WARN] bulk-deluge-import %s: %s: %v", opID, f.FilePath, importErr)
				resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "error": importErr.Error()})
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      "error",
				})
				failed++
			} else {
				resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "new_path": newPath})
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      "imported",
				})
				imported++
			}
		}
		if (i+1)%100 == 0 || i+1 == total {
			_ = progress.UpdateProgress(i+1, total,
				fmt.Sprintf("imported %d/%d (failed: %d)", imported, total, failed))
		}
	}
	log.Printf("[INFO] bulk-deluge-import %s: done. imported=%d failed=%d", opID, imported, failed)
	return nil
}
