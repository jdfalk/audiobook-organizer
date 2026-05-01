// file: internal/server/maintenance_author_version.go
// version: 1.0.0
// guid: 70a97d16-f1b6-44a9-b7a5-b1fe492ebc0a
// last-edited: 2026-05-01

package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

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
