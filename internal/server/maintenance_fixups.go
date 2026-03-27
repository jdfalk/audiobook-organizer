// file: internal/server/maintenance_fixups.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
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
