// file: internal/server/maintenance_fixups.go
// version: 1.6.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// readByFixResult describes one book that was (or would be) fixed.
type readByFixResult struct {
	BookID      string  `json:"book_id"`
	Pattern     string  `json:"pattern"`           // "read_by_swap", "title_dash_read_by", "both_broken"
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

	store := database.GlobalStore
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
func applyReadByFix(store database.Store, book *database.Book, fix *readByFixResult) error {
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

	store := database.GlobalStore
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
func unlinkAndDeleteSeries(store database.Store, book *database.Book, seriesID int) error {
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
func mergeSeriesGroup(store database.Store, keepID int, mergeIDs []int) error {
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

	store := database.GlobalStore
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
			filesToCreate = audioFilesInDir(book.FilePath)
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
		"dry_run":      dryRun,
		"books_total":  len(allBooks),
		"books_found":  len(results) - skipped,
		"books_skipped": skipped,
		"files_total":  totalFiles,
		"applied":      applied,
		"errors":       errors,
		"results":      results,
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
		"dry_run":        dryRun,
		"root_dir":       rootDir,
		"folders_found":  len(results),
		"folders_removed": removedCount,
		"folders":        results,
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
// Author/narrator swap fix
// ---------------------------------------------------------------------------

// authorNarratorSwapResult describes one book where the author field contains
// the narrator name (or vice versa).
type authorNarratorSwapResult struct {
	BookID      string `json:"book_id"`
	BookTitle   string `json:"book_title"`
	AuthorName  string `json:"author_name"`
	NarratorName string `json:"narrator_name"`
	Applied     bool   `json:"applied"`
	Error       string `json:"error,omitempty"`
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

	store := database.GlobalStore
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
func createBookFilesForBook(store database.Store, book *database.Book, filePaths []string, missing bool) error {
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
	VersionGroupID string    `json:"version_group_id"`
	Books          []vgBook  `json:"books"`
	Applied        bool      `json:"applied"`
	Error          string    `json:"error,omitempty"`
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

	store := database.GlobalStore
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
func unlinkVersionGroupOutliers(store database.Store, outliers []vgBook) error {
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
		if len(audioFilesInDir(subPath)) > 0 {
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
		if len(audioFilesInDir(sub)) == 0 {
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
func fixAuthorDirPath(store database.Store, book *database.Book, subdir string) error {
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

	newFiles := audioFilesInDir(subdir)
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
	FileID           string `json:"file_id"`
	BookID           string `json:"book_id"`
	FilePath         string `json:"file_path"`
	TrackNumberOld   int    `json:"track_number_old,omitempty"`
	TrackNumberNew   int    `json:"track_number_new,omitempty"`
	TrackCountOld    int    `json:"track_count_old,omitempty"`
	TrackCountNew    int    `json:"track_count_new,omitempty"`
	FileSizeOld      int64  `json:"file_size_old,omitempty"`
	FileSizeNew      int64  `json:"file_size_new,omitempty"`
	FormatOld        string `json:"format_old,omitempty"`
	FormatNew        string `json:"format_new,omitempty"`
	OrigFilenameSet  bool   `json:"original_filename_set,omitempty"`
	Changed          bool   `json:"changed"`
	Applied          bool   `json:"applied"`
	Error            string `json:"error,omitempty"`
}

// handleEnrichBookFiles iterates all book_files rows and fills in missing or
// zero-valued fields:
//   - track_number: parsed from leading digits in the filename
//   - track_count:  total number of files for the owning book
//   - file_size:    from os.Stat when currently 0
//   - format:       from filepath.Ext when empty
//   - original_filename: from filepath.Base when empty
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleEnrichBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := database.GlobalStore
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
			if f.FileSize == 0 && !f.Missing {
				if info, statErr := os.Stat(f.FilePath); statErr == nil {
					newSize := info.Size()
					if newSize > 0 {
						result.FileSizeOld = f.FileSize
						result.FileSizeNew = newSize
						f.FileSize = newSize
						changed = true
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
