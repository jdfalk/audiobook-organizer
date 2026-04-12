// file: internal/server/entities_handlers.go
// version: 1.0.0
// guid: 52cb6f75-cb3e-44e3-bf36-a8bba8a24d21
//
// Entity HTTP handlers split out of server.go: works, authors, series,
// and narrators — CRUD plus merges and listing. Grouped here so the
// per-entity plumbing stays close to related code.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

func (s *Server) listWorks(c *gin.Context) {
	resp, err := s.workService.ListWorks()
	if err != nil {
		internalError(c, "failed to list works", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) createWork(c *gin.Context) {
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := s.workService.CreateWork(&work)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) getWork(c *gin.Context) {
	id := c.Param("id")
	work, err := s.workService.GetWork(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, work)
}

func (s *Server) updateWork(c *gin.Context) {
	id := c.Param("id")
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(work.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	updated, err := s.workService.UpdateWork(id, &work)
	if err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to update work", err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deleteWork(c *gin.Context) {
	id := c.Param("id")
	if err := s.workService.DeleteWork(id); err != nil {
		if err.Error() == "work not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to delete work", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) listWorkBooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	books, err := database.GlobalStore.GetBooksByWorkID(id)
	if err != nil {
		internalError(c, "failed to list work books", err)
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	c.JSON(http.StatusOK, gin.H{"items": books, "count": len(books)})
}

// listWork returns all work items (audiobooks grouped by work entity)
func (s *Server) listWork(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get all works
	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	// For each work, get associated books
	items := make([]map[string]any, 0, len(works))
	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			books = []database.Book{}
		}

		items = append(items, map[string]any{
			"id":         work.ID,
			"title":      work.Title,
			"author_id":  work.AuthorID,
			"book_count": len(books),
			"books":      books,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": len(items),
	})
}

// getWorkStats returns statistics about work items
func (s *Server) getWorkStats(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	works, err := database.GlobalStore.GetAllWorks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve works"})
		return
	}

	totalWorks := len(works)
	totalBooks := 0
	worksWithMultipleEditions := 0

	for _, work := range works {
		books, err := database.GlobalStore.GetBooksByWorkID(work.ID)
		if err != nil {
			continue
		}
		bookCount := len(books)
		totalBooks += bookCount
		if bookCount > 1 {
			worksWithMultipleEditions++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_works":                  totalWorks,
		"total_books":                  totalBooks,
		"works_with_multiple_editions": worksWithMultipleEditions,
		"average_editions_per_work":    float64(totalBooks) / float64(max(totalWorks, 1)),
	})
}

func (s *Server) listAuthors(c *gin.Context) {
	resp, err := s.authorSeriesService.ListAuthorsWithCounts()
	if err != nil {
		internalError(c, "failed to list authors", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) countAuthors(c *gin.Context) {
	count, err := database.GlobalStore.CountAuthors()
	if err != nil {
		internalError(c, "failed to count authors", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) renameAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be empty"})
		return
	}

	store := database.GlobalStore
	if err := store.UpdateAuthorName(authorID, name); err != nil {
		internalError(c, "failed to rename author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"id": authorID, "name": name})
}

// splitCompositeAuthor splits an author like "Author1 / Author2" or "Author1, Author2"
// into individual author records, relinking all books to each new author.
func (s *Server) splitCompositeAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	// Optional: caller can provide explicit names to split into
	var req struct {
		Names []string `json:"names"`
	}
	_ = c.ShouldBindJSON(&req)

	// If no explicit names, auto-detect split
	names := req.Names
	if len(names) == 0 {
		names = SplitCompositeAuthorName(author.Name)
	}
	if len(names) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "author name does not appear to be composite"})
		return
	}

	// Create or find each individual author
	var newAuthors []database.Author
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		existing, err := store.GetAuthorByName(name)
		if err == nil && existing != nil {
			newAuthors = append(newAuthors, *existing)
			continue
		}
		created, err := store.CreateAuthor(name)
		if err != nil {
			internalError(c, "failed to create author", err)
			return
		}
		newAuthors = append(newAuthors, *created)
	}

	// Get all books linked to the composite author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}

	booksUpdated := 0
	for _, book := range books {
		bookAuthors, err := store.GetBookAuthors(book.ID)
		if err != nil {
			continue
		}

		// Find the role/position of the composite author entry
		role := "author"
		for _, ba := range bookAuthors {
			if ba.AuthorID == authorID {
				role = ba.Role
				break
			}
		}

		// Remove composite author, add individual authors
		var updated []database.BookAuthor
		for _, ba := range bookAuthors {
			if ba.AuthorID != authorID {
				updated = append(updated, ba)
			}
		}
		for i, na := range newAuthors {
			// Check not already linked
			alreadyLinked := false
			for _, ba := range updated {
				if ba.AuthorID == na.ID {
					alreadyLinked = true
					break
				}
			}
			if !alreadyLinked {
				updated = append(updated, database.BookAuthor{
					BookID:   book.ID,
					AuthorID: na.ID,
					Role:     role,
					Position: len(updated) + i,
				})
			}
		}
		if err := store.SetBookAuthors(book.ID, updated); err != nil {
			continue
		}
		booksUpdated++
	}

	// Delete the composite author
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}

	result := make([]gin.H, len(newAuthors))
	for i, a := range newAuthors {
		result[i] = gin.H{"id": a.ID, "name": a.Name}
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"authors": result, "books_updated": booksUpdated})
}

func (s *Server) mergeAuthors(c *gin.Context) {
	var req struct {
		KeepID   int   `json:"keep_id" binding:"required"`
		MergeIDs []int `json:"merge_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.MergeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merge_ids must not be empty"})
		return
	}

	store := database.GlobalStore
	keepAuthor, err := store.GetAuthorByID(req.KeepID)
	if err != nil || keepAuthor == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "keep author not found"})
		return
	}

	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-authors:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "author-merge", &detail)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	keepID := req.KeepID
	mergeIDs := req.MergeIDs
	keepName := keepAuthor.Name

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		_ = progress.Log("info", fmt.Sprintf("Merging %d author(s) into \"%s\"", len(mergeIDs), keepName), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting author merge...")

		merged := 0
		var mergeErrors []string
		for i, mergeID := range mergeIDs {
			if progress.IsCanceled() {
				return fmt.Errorf("cancelled")
			}
			if mergeID == keepID {
				continue
			}
			books, err := store.GetBooksByAuthorIDWithRole(mergeID)
			if err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to get books for author %d: %v", mergeID, err))
				continue
			}

			mergeAuthor, _ := store.GetAuthorByID(mergeID)
			mergeAuthorName := ""
			if mergeAuthor != nil {
				mergeAuthorName = mergeAuthor.Name
			}

			for _, book := range books {
				bookAuthors, err := store.GetBookAuthors(book.ID)
				if err != nil {
					continue
				}
				hasKeep := false
				for _, ba := range bookAuthors {
					if ba.AuthorID == keepID {
						hasKeep = true
						break
					}
				}
				var newAuthors []database.BookAuthor
				for _, ba := range bookAuthors {
					if ba.AuthorID == mergeID {
						if !hasKeep {
							ba.AuthorID = keepID
							newAuthors = append(newAuthors, ba)
							hasKeep = true
						}
					} else {
						newAuthors = append(newAuthors, ba)
					}
				}
				if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
					mergeErrors = append(mergeErrors, fmt.Sprintf("failed to update book %s: %v", book.ID, err))
				} else {
					_ = store.CreateOperationChange(&database.OperationChange{
						ID:          ulid.Make().String(),
						OperationID: opID,
						BookID:      book.ID,
						ChangeType:  "author_reassign",
						FieldName:   "book_authors",
						OldValue:    fmt.Sprintf("author_id:%d (%s)", mergeID, mergeAuthorName),
						NewValue:    fmt.Sprintf("author_id:%d (%s)", keepID, keepName),
					})
				}

				// Sync the denormalized `book.AuthorID` pointer
				// on the Book row itself. SetBookAuthors above
				// updates the join table, but callers that read
				// the Book struct directly — organize path,
				// metadata fetcher, search indexer — expect
				// book.AuthorID to match the primary author in
				// the join table. Without this sync, the field
				// still points at the losing author ID, which
				// has been hard-deleted on the next iteration.
				//
				// Tombstones cover most lookups (GetAuthorByID
				// follows the tombstone chain), but any code that
				// uses book.AuthorID as a map key or as an equality
				// check without going through the lookup helpers
				// sees the stale ID. This closes that gap.
				//
				// Backlog 7.11 — found while investigating the
				// merge ITL cleanup bug (#251).
				current, gbErr := store.GetBookByID(book.ID)
				if gbErr != nil || current == nil {
					continue
				}
				if current.AuthorID != nil && *current.AuthorID == mergeID {
					newID := keepID
					current.AuthorID = &newID
					if _, upErr := store.UpdateBook(book.ID, current); upErr != nil {
						log.Printf("[WARN] author merge: failed to sync denormalized AuthorID on book %s: %v", book.ID, upErr)
					}
				}
			}

			if err := store.DeleteAuthor(mergeID); err != nil {
				mergeErrors = append(mergeErrors, fmt.Sprintf("failed to delete author %d: %v", mergeID, err))
			} else {
				_ = store.CreateAuthorTombstone(mergeID, keepID)
				_ = store.CreateOperationChange(&database.OperationChange{
					ID:          ulid.Make().String(),
					OperationID: opID,
					BookID:      "",
					ChangeType:  "author_delete",
					FieldName:   "author",
					OldValue:    fmt.Sprintf("%d:%s", mergeID, mergeAuthorName),
					NewValue:    fmt.Sprintf("merged_into:%d:%s", keepID, keepName),
				})
				merged++
			}

			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d authors", i+1, len(mergeIDs)))
		}

		resultMsg := fmt.Sprintf("Author merge complete: merged %d, %d errors", merged, len(mergeErrors))
		_ = progress.Log("info", resultMsg, nil)
		if len(mergeErrors) > 0 {
			errDetail := strings.Join(mergeErrors[:min(len(mergeErrors), 10)], "; ")
			_ = progress.Log("warn", fmt.Sprintf("Errors: %s", errDetail), nil)
		}
		s.dedupCache.InvalidateAll()
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "author-merge", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

func (s *Server) deleteAuthorHandler(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil || authorID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}
	if len(books) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete author with books"})
		return
	}
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "author deleted"})
}

// bulkDeleteAuthors deletes multiple zero-book authors at once.
func (s *Server) bulkDeleteAuthors(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	store := database.GlobalStore
	deleted := 0
	skipped := 0
	var errors []string
	for _, id := range req.IDs {
		books, err := store.GetBooksByAuthorID(id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("author %d: %v", id, err))
			continue
		}
		if len(books) > 0 {
			skipped++
			continue
		}
		if err := store.DeleteAuthor(id); err != nil {
			errors = append(errors, fmt.Sprintf("author %d: %v", id, err))
			continue
		}
		deleted++
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"skipped": skipped,
		"errors":  errors,
		"total":   len(req.IDs),
	})
}

func (s *Server) getAuthorBooks(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i])
	}
	c.JSON(http.StatusOK, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) getAuthorAliases(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	aliases, err := database.GlobalStore.GetAuthorAliases(authorID)
	if err != nil {
		internalError(c, "failed to get author aliases", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"aliases": aliases})
}

func (s *Server) createAuthorAlias(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}
	var req struct {
		AliasName string `json:"alias_name"`
		AliasType string `json:"alias_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AliasName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alias_name is required"})
		return
	}
	if req.AliasType == "" {
		req.AliasType = "alias"
	}
	alias, err := database.GlobalStore.CreateAuthorAlias(authorID, req.AliasName, req.AliasType)
	if err != nil {
		internalError(c, "failed to create author alias", err)
		return
	}
	c.JSON(http.StatusCreated, alias)
}

func (s *Server) deleteAuthorAlias(c *gin.Context) {
	aliasID, err := strconv.Atoi(c.Param("aliasId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alias ID"})
		return
	}
	if err := database.GlobalStore.DeleteAuthorAlias(aliasID); err != nil {
		internalError(c, "failed to delete author alias", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) reclassifyAuthorAsNarrator(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	// Create or find narrator with same name
	narrator, err := store.GetNarratorByName(author.Name)
	if err != nil || narrator == nil {
		narrator, err = store.CreateNarrator(author.Name)
		if err != nil {
			internalError(c, "failed to create narrator", err)
			return
		}
	}

	// Get all books linked to this author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		internalError(c, "failed to get author books", err)
		return
	}

	booksUpdated := 0
	for _, book := range books {
		// Remove author link
		bookAuthors, err := store.GetBookAuthors(book.ID)
		if err != nil {
			continue
		}
		var newAuthors []database.BookAuthor
		for _, ba := range bookAuthors {
			if ba.AuthorID != authorID {
				newAuthors = append(newAuthors, ba)
			}
		}
		if err := store.SetBookAuthors(book.ID, newAuthors); err != nil {
			continue
		}

		// Add narrator link if not already present
		bookNarrators, err := store.GetBookNarrators(book.ID)
		if err != nil {
			continue
		}
		hasNarrator := false
		for _, bn := range bookNarrators {
			if bn.NarratorID == narrator.ID {
				hasNarrator = true
				break
			}
		}
		if !hasNarrator {
			bookNarrators = append(bookNarrators, database.BookNarrator{
				BookID:     book.ID,
				NarratorID: narrator.ID,
				Role:       "narrator",
				Position:   len(bookNarrators),
			})
			if err := store.SetBookNarrators(book.ID, bookNarrators); err != nil {
				continue
			}
		}
		booksUpdated++
	}

	// Delete the author record
	if err := store.DeleteAuthor(authorID); err != nil {
		internalError(c, "failed to delete author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	c.JSON(http.StatusOK, gin.H{"narrator_id": narrator.ID, "books_updated": booksUpdated})
}

// resolveProductionAuthor attempts to find real authors for books attributed to
// a production company by searching metadata sources by title only and optionally
// using AI cover art analysis.
func (s *Server) resolveProductionAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author ID"})
		return
	}

	store := database.GlobalStore
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "author not found"})
		return
	}

	if !isProductionCompany(author.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%q is not a recognized production company", author.Name)})
		return
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "resolve-production-author", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	prodAuthorName := author.Name
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		books, err := store.GetBooksByAuthorIDWithRole(authorID)
		if err != nil {
			return fmt.Errorf("failed to get books: %w", err)
		}
		_ = progress.Log("info", fmt.Sprintf("Resolving %d books for production company %q", len(books), prodAuthorName), nil)

		resolved := 0
		failed := 0
		for i, book := range books {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			_ = progress.UpdateProgress(i, len(books), fmt.Sprintf("Processing %d/%d: %s", i+1, len(books), book.Title))

			// Try metadata fetch by title only
			resp, fetchErr := s.metadataFetchService.FetchMetadataForBookByTitle(book.ID)
			if fetchErr == nil && resp != nil && resp.Book != nil && resp.Book.AuthorID != nil {
				// Check if the found author is different from the production company
				newAuthor, _ := store.GetAuthorByID(*resp.Book.AuthorID)
				if newAuthor != nil && !isProductionCompany(newAuthor.Name) {
					_ = progress.Log("info", fmt.Sprintf("Resolved %q → author %q (source: %s)", book.Title, newAuthor.Name, resp.Source), nil)
					// Reclassify production company as publisher
					if book.Publisher == nil || *book.Publisher == "" {
						pub := prodAuthorName
						book.Publisher = &pub
						store.UpdateBook(book.ID, &database.Book{Publisher: &pub})
					}
					resolved++
					continue
				}
			}

			// If metadata failed and AI is enabled, try cover art analysis
			aiParser := newAIParser(config.AppConfig.OpenAIAPIKey, config.AppConfig.EnableAIParsing)
			if aiParser.IsEnabled() && book.FilePath != "" {
				imgData, mime, imgErr := metadata.ExtractCoverArtBytes(book.FilePath)
				if imgErr == nil && len(imgData) > 0 {
					parsed, aiErr := aiParser.ParseCoverArt(ctx, imgData, mime)
					if aiErr == nil && parsed != nil && parsed.Author != "" && parsed.Confidence != "low" {
						_ = progress.Log("info", fmt.Sprintf("AI cover analysis for %q found author: %q (confidence: %s)", book.Title, parsed.Author, parsed.Confidence), nil)
						// Look up or create the discovered author
						existing, _ := store.GetAuthorByName(parsed.Author)
						if existing == nil {
							existing, _ = store.CreateAuthor(parsed.Author)
						}
						if existing != nil {
							aid := existing.ID
							book.AuthorID = &aid
							store.UpdateBook(book.ID, &database.Book{AuthorID: &aid})
							// Update book_authors
							bookAuthors, _ := store.GetBookAuthors(book.ID)
							var updated []database.BookAuthor
							for _, ba := range bookAuthors {
								if ba.AuthorID != authorID {
									updated = append(updated, ba)
								}
							}
							updated = append(updated, database.BookAuthor{
								BookID:   book.ID,
								AuthorID: existing.ID,
								Role:     "author",
								Position: 0,
							})
							store.SetBookAuthors(book.ID, updated)
							resolved++
							continue
						}
					}
				}
			}

			failed++
			_ = progress.Log("debug", fmt.Sprintf("Could not resolve author for %q", book.Title), nil)
		}

		if s.dedupCache != nil {
			s.dedupCache.Invalidate("author-duplicates")
		}

		resultMsg := fmt.Sprintf("Resolved %d/%d books for %q (%d unresolved)", resolved, len(books), prodAuthorName, failed)
		_ = progress.Log("info", resultMsg, nil)
		_ = progress.UpdateProgress(len(books), len(books), resultMsg)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(opID, "resolve-production-author", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"operation": op})
}

func (s *Server) countSeries(c *gin.Context) {
	count, err := database.GlobalStore.CountSeries()
	if err != nil {
		internalError(c, "failed to count series", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) listSeries(c *gin.Context) {
	resp, err := s.authorSeriesService.ListSeriesWithCounts()
	if err != nil {
		internalError(c, "failed to list series", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) getSeriesBooks(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		internalError(c, "failed to get series books", err)
		return
	}
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i])
	}
	c.JSON(http.StatusOK, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) renameSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be empty"})
		return
	}
	store := database.GlobalStore
	if err := store.UpdateSeriesName(seriesID, name); err != nil {
		internalError(c, "failed to rename series", err)
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.Invalidate("series-duplicates")
	}
	series, _ := store.GetSeriesByID(seriesID)
	c.JSON(http.StatusOK, series)
}

func (s *Server) splitSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids must not be empty"})
		return
	}
	store := database.GlobalStore
	oldSeries, err := store.GetSeriesByID(seriesID)
	if err != nil || oldSeries == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}
	newSeries, err := store.CreateSeries(oldSeries.Name+" (Split)", oldSeries.AuthorID)
	if err != nil {
		internalError(c, "failed to create new series", err)
		return
	}
	moved := 0
	for _, bookID := range req.BookIDs {
		book, err := store.GetBookByID(bookID)
		if err != nil || book == nil {
			continue
		}
		if book.SeriesID == nil || *book.SeriesID != seriesID {
			continue
		}
		book.SeriesID = &newSeries.ID
		if _, err := store.UpdateBook(book.ID, book); err != nil {
			continue
		}
		moved++
	}
	if s.dedupCache != nil {
		s.dedupCache.Invalidate("series-duplicates")
	}
	c.JSON(http.StatusOK, gin.H{"new_series": newSeries, "books_moved": moved})
}

func (s *Server) deleteEmptySeries(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series ID"})
		return
	}
	store := database.GlobalStore
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		internalError(c, "failed to get series books", err)
		return
	}
	if len(books) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete series with books"})
		return
	}
	if err := store.DeleteSeries(seriesID); err != nil {
		internalError(c, "failed to delete series", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "series deleted"})
}

// bulkDeleteSeries deletes multiple empty series at once.
func (s *Server) bulkDeleteSeries(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	store := database.GlobalStore
	deleted := 0
	skipped := 0
	var errors []string
	for _, id := range req.IDs {
		books, err := store.GetBooksBySeriesID(id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("series %d: %v", id, err))
			continue
		}
		if len(books) > 0 {
			skipped++
			continue
		}
		if err := store.DeleteSeries(id); err != nil {
			errors = append(errors, fmt.Sprintf("series %d: %v", id, err))
			continue
		}
		deleted++
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"skipped": skipped,
		"errors":  errors,
		"total":   len(req.IDs),
	})
}

func (s *Server) updateSeriesName(c *gin.Context) {
	idStr := c.Param("id")
	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
		return
	}
	store := database.GlobalStore
	if err := store.UpdateSeriesName(id, name); err != nil {
		internalError(c, "failed to update series", err)
		return
	}
	s.dedupCache.Invalidate("series-duplicates")
	series, _ := store.GetSeriesByID(id)
	c.JSON(http.StatusOK, series)
}

func (s *Server) listNarrators(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.ListNarrators()
	if err != nil {
		internalError(c, "failed to list narrators", err)
		return
	}
	c.JSON(http.StatusOK, narrators)
}

func (s *Server) countNarrators(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.ListNarrators()
	if err != nil {
		internalError(c, "failed to count narrators", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(narrators)})
}

func (s *Server) listAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	narrators, err := database.GlobalStore.GetBookNarrators(id)
	if err != nil {
		internalError(c, "failed to list audiobook narrators", err)
		return
	}
	if narrators == nil {
		narrators = []database.BookNarrator{}
	}
	c.JSON(http.StatusOK, narrators)
}

func (s *Server) setAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var narrators []database.BookNarrator
	if err := c.ShouldBindJSON(&narrators); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := database.GlobalStore.SetBookNarrators(id, narrators); err != nil {
		internalError(c, "failed to set audiobook narrators", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
