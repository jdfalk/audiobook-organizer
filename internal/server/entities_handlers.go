// file: internal/server/entities_handlers.go
// version: 2.3.0
// guid: 52cb6f75-cb3e-44e3-bf36-a8bba8a24d21
//
// Entity HTTP handlers split out of server.go: works, authors, series,
// and narrators — CRUD plus merges and listing. Grouped here so the
// per-entity plumbing stays close to related code.

package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	ulid "github.com/oklog/ulid/v2"
)

func (s *Server) listWorks(c *gin.Context) {
	resp, err := s.workService.ListWorks()
	if err != nil {
		httputil.InternalError(c, "failed to list works", err)
		return
	}
	httputil.RespondWithOK(c, resp)
}

func (s *Server) createWork(c *gin.Context) {
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	created, err := s.workService.CreateWork(&work)
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	httputil.RespondWithCreated(c, created)
}

func (s *Server) getWork(c *gin.Context) {
	id := c.Param("id")
	work, err := s.workService.GetWork(id)
	if err != nil {
		httputil.RespondWithNotFound(c, "work", id)
		return
	}
	httputil.RespondWithOK(c, work)
}

func (s *Server) updateWork(c *gin.Context) {
	id := c.Param("id")
	var work database.Work
	if err := c.ShouldBindJSON(&work); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(work.Title) == "" {
		httputil.RespondWithBadRequest(c, "title is required")
		return
	}
	updated, err := s.workService.UpdateWork(id, &work)
	if err != nil {
		if err.Error() == "work not found" {
			httputil.RespondWithNotFound(c, "work", id)
			return
		}
		httputil.InternalError(c, "failed to update work", err)
		return
	}
	httputil.RespondWithOK(c, updated)
}

func (s *Server) deleteWork(c *gin.Context) {
	id := c.Param("id")
	if err := s.workService.DeleteWork(id); err != nil {
		if err.Error() == "work not found" {
			httputil.RespondWithNotFound(c, "work", id)
			return
		}
		httputil.InternalError(c, "failed to delete work", err)
		return
	}
	httputil.RespondWithNoContent(c)
}

func (s *Server) listWorkBooks(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	id := c.Param("id")
	books, err := s.Store().GetBooksByWorkID(id)
	if err != nil {
		httputil.InternalError(c, "failed to list work books", err)
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	httputil.RespondWithOK(c, gin.H{"items": books, "count": len(books)})
}

// listWork returns all work items (audiobooks grouped by work entity)
func (s *Server) listWork(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	// Get all works
	works, err := s.Store().GetAllWorks()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to retrieve works")
		return
	}

	// For each work, get associated books
	items := make([]map[string]any, 0, len(works))
	for _, work := range works {
		books, err := s.Store().GetBooksByWorkID(work.ID)
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

	httputil.RespondWithOK(c, gin.H{
		"items": items,
		"total": len(items),
	})
}

// getWorkStats returns statistics about work items
func (s *Server) getWorkStats(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	works, err := s.Store().GetAllWorks()
	if err != nil {
		httputil.RespondWithInternalError(c, "failed to retrieve works")
		return
	}

	totalWorks := len(works)
	totalBooks := 0
	worksWithMultipleEditions := 0

	for _, work := range works {
		books, err := s.Store().GetBooksByWorkID(work.ID)
		if err != nil {
			continue
		}
		bookCount := len(books)
		totalBooks += bookCount
		if bookCount > 1 {
			worksWithMultipleEditions++
		}
	}

	httputil.RespondWithOK(c, gin.H{
		"total_works":                  totalWorks,
		"total_books":                  totalBooks,
		"works_with_multiple_editions": worksWithMultipleEditions,
		"average_editions_per_work":    float64(totalBooks) / float64(max(totalWorks, 1)),
	})
}

func (s *Server) listAuthors(c *gin.Context) {
	resp, err := s.authorSeriesService.ListAuthorsWithCounts()
	if err != nil {
		httputil.InternalError(c, "failed to list authors", err)
		return
	}
	httputil.RespondWithOK(c, resp)
}

func (s *Server) countAuthors(c *gin.Context) {
	count, err := s.Store().CountAuthors()
	if err != nil {
		httputil.InternalError(c, "failed to count authors", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"count": count})
}

func (s *Server) renameAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.RespondWithBadRequest(c, "name must not be empty")
		return
	}

	store := s.Store()
	if err := store.UpdateAuthorName(authorID, name); err != nil {
		httputil.InternalError(c, "failed to rename author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	httputil.RespondWithOK(c, gin.H{"id": authorID, "name": name})
}

// splitCompositeAuthor splits an author like "Author1 / Author2" or "Author1, Author2"
// into individual author records, relinking all books to each new author.
func (s *Server) splitCompositeAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}

	store := s.Store()
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		httputil.RespondWithNotFound(c, "author", "")
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
		names = dedup.SplitCompositeAuthorName(author.Name)
	}
	if len(names) <= 1 {
		httputil.RespondWithBadRequest(c, "author name does not appear to be composite")
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
			httputil.InternalError(c, "failed to create author", err)
			return
		}
		newAuthors = append(newAuthors, *created)
	}

	// Get all books linked to the composite author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		httputil.InternalError(c, "failed to get author books", err)
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
		httputil.InternalError(c, "failed to delete author", err)
		return
	}

	result := make([]gin.H, len(newAuthors))
	for i, a := range newAuthors {
		result[i] = gin.H{"id": a.ID, "name": a.Name}
	}

	s.dedupCache.Invalidate("author-duplicates")
	httputil.RespondWithOK(c, gin.H{"authors": result, "books_updated": booksUpdated})
}

func (s *Server) mergeAuthors(c *gin.Context) {
	var req struct {
		KeepID   int   `json:"keep_id" binding:"required"`
		MergeIDs []int `json:"merge_ids" binding:"required"`
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
	keepAuthor, err := store.GetAuthorByID(req.KeepID)
	if err != nil || keepAuthor == nil {
		httputil.RespondWithNotFound(c, "author", "")
		return
	}

	opID := ulid.Make().String()
	detail := fmt.Sprintf("merge-authors:keep=%d,merge=%v", req.KeepID, req.MergeIDs)
	op, err := store.CreateOperation(opID, "author-merge", &detail)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := authorMergeOpParams{
		LegacyOpID: op.ID,
		KeepID:     req.KeepID,
		MergeIDs:   req.MergeIDs,
		KeepName:   keepAuthor.Name,
	}
	if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "entities.author-merge", params); enqErr != nil {
		httputil.InternalError(c, "failed to enqueue operation", enqErr)
		return
	}
	httputil.RespondWithSuccess(c, 202, op)
}

func (s *Server) deleteAuthorHandler(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil || authorID <= 0 {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}
	store := s.Store()
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		httputil.InternalError(c, "failed to get author books", err)
		return
	}
	if len(books) > 0 {
		httputil.RespondWithConflict(c, "cannot delete author with books")
		return
	}
	if err := store.DeleteAuthor(authorID); err != nil {
		httputil.InternalError(c, "failed to delete author", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "author deleted"})
}

// bulkDeleteAuthors deletes multiple zero-book authors at once.
func (s *Server) bulkDeleteAuthors(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	store := s.Store()
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
	httputil.RespondWithOK(c, gin.H{
		"deleted": deleted,
		"skipped": skipped,
		"errors":  errors,
		"total":   len(req.IDs),
	})
}

func (s *Server) getAuthorBooks(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}
	store := s.Store()
	books, err := store.GetBooksByAuthorID(authorID)
	if err != nil {
		httputil.InternalError(c, "failed to get author books", err)
		return
	}
	
	// Pre-fetch authors and narrators for all books
	bookIDs := make([]string, len(books))
	for i, b := range books {
		bookIDs[i] = b.ID
	}
	bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID := batchFetchBookAuthorsAndNarrators(bookIDs)
	
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i], bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID)
	}
	httputil.RespondWithOK(c, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) getAuthorAliases(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}
	aliases, err := s.Store().GetAuthorAliases(authorID)
	if err != nil {
		httputil.InternalError(c, "failed to get author aliases", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"aliases": aliases})
}

func (s *Server) createAuthorAlias(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}
	var req struct {
		AliasName string `json:"alias_name"`
		AliasType string `json:"alias_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if req.AliasName == "" {
		httputil.RespondWithBadRequest(c, "alias_name is required")
		return
	}
	if req.AliasType == "" {
		req.AliasType = "alias"
	}
	alias, err := s.Store().CreateAuthorAlias(authorID, req.AliasName, req.AliasType)
	if err != nil {
		httputil.InternalError(c, "failed to create author alias", err)
		return
	}
	httputil.RespondWithCreated(c, alias)
}

func (s *Server) deleteAuthorAlias(c *gin.Context) {
	aliasID, err := strconv.Atoi(c.Param("aliasId"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid alias ID")
		return
	}
	if err := s.Store().DeleteAuthorAlias(aliasID); err != nil {
		httputil.InternalError(c, "failed to delete author alias", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"status": "deleted"})
}

func (s *Server) reclassifyAuthorAsNarrator(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}

	store := s.Store()
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		httputil.RespondWithNotFound(c, "author", "")
		return
	}

	// Create or find narrator with same name
	narrator, err := store.GetNarratorByName(author.Name)
	if err != nil || narrator == nil {
		narrator, err = store.CreateNarrator(author.Name)
		if err != nil {
			httputil.InternalError(c, "failed to create narrator", err)
			return
		}
	}

	// Get all books linked to this author
	books, err := store.GetBooksByAuthorIDWithRole(authorID)
	if err != nil {
		httputil.InternalError(c, "failed to get author books", err)
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
		httputil.InternalError(c, "failed to delete author", err)
		return
	}

	s.dedupCache.Invalidate("author-duplicates")
	httputil.RespondWithOK(c, gin.H{"narrator_id": narrator.ID, "books_updated": booksUpdated})
}

// resolveProductionAuthor attempts to find real authors for books attributed to
// a production company by searching metadata sources by title only and optionally
// using AI cover art analysis.
func (s *Server) resolveProductionAuthor(c *gin.Context) {
	authorID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid author ID")
		return
	}

	store := s.Store()
	author, err := store.GetAuthorByID(authorID)
	if err != nil || author == nil {
		httputil.RespondWithNotFound(c, "author", "")
		return
	}

	if !dedup.IsProductionCompany(author.Name) {
		httputil.RespondWithBadRequest(c, fmt.Sprintf("%q is not a recognized production company", author.Name))
		return
	}

	opID := ulid.Make().String()
	op, err := store.CreateOperation(opID, "resolve-production-author", nil)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	params := resolveProductionAuthorOpParams{
		LegacyOpID:     op.ID,
		AuthorID:       authorID,
		ProdAuthorName: author.Name,
	}
	if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "entities.resolve-production-author", params); enqErr != nil {
		httputil.InternalError(c, "failed to enqueue operation", enqErr)
		return
	}
	httputil.RespondWithSuccess(c, 202, gin.H{"operation": op})
}

func (s *Server) countSeries(c *gin.Context) {
	count, err := s.Store().CountSeries()
	if err != nil {
		httputil.InternalError(c, "failed to count series", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"count": count})
}

func (s *Server) listSeries(c *gin.Context) {
	resp, err := s.authorSeriesService.ListSeriesWithCounts()
	if err != nil {
		httputil.InternalError(c, "failed to list series", err)
		return
	}
	httputil.RespondWithOK(c, resp)
}

func (s *Server) getSeriesBooks(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid series ID")
		return
	}
	store := s.Store()
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		httputil.InternalError(c, "failed to get series books", err)
		return
	}
	
	// Pre-fetch authors and narrators for all books
	bookIDs := make([]string, len(books))
	for i, b := range books {
		bookIDs[i] = b.ID
	}
	bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID := batchFetchBookAuthorsAndNarrators(bookIDs)
	
	enriched := make([]enrichedBookResponse, len(books))
	for i := range books {
		enriched[i] = enrichBookForResponse(&books[i], bookAuthorsMap, authorsByID, bookNarratorsMap, narratorsByID)
	}
	httputil.RespondWithOK(c, gin.H{"items": enriched, "count": len(enriched)})
}

func (s *Server) renameSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		httputil.RespondWithBadRequest(c, "invalid series ID")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.RespondWithBadRequest(c, "name must not be empty")
		return
	}
	store := s.Store()
	if err := store.UpdateSeriesName(seriesID, name); err != nil {
		httputil.InternalError(c, "failed to rename series", err)
		return
	}
	if s.dedupCache != nil {
		s.dedupCache.Invalidate("series-duplicates")
	}
	series, _ := store.GetSeriesByID(seriesID)
	httputil.RespondWithOK(c, series)
}

func (s *Server) splitSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		httputil.RespondWithBadRequest(c, "invalid series ID")
		return
	}
	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if len(req.BookIDs) == 0 {
		httputil.RespondWithBadRequest(c, "book_ids must not be empty")
		return
	}
	store := s.Store()
	oldSeries, err := store.GetSeriesByID(seriesID)
	if err != nil || oldSeries == nil {
		httputil.RespondWithNotFound(c, "series", "")
		return
	}
	newSeries, err := store.CreateSeries(oldSeries.Name+" (Split)", oldSeries.AuthorID)
	if err != nil {
		httputil.InternalError(c, "failed to create new series", err)
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
	httputil.RespondWithOK(c, gin.H{"new_series": newSeries, "books_moved": moved})
}

func (s *Server) deleteEmptySeries(c *gin.Context) {
	seriesID, err := strconv.Atoi(c.Param("id"))
	if err != nil || seriesID <= 0 {
		httputil.RespondWithBadRequest(c, "invalid series ID")
		return
	}
	store := s.Store()
	books, err := store.GetBooksBySeriesID(seriesID)
	if err != nil {
		httputil.InternalError(c, "failed to get series books", err)
		return
	}
	if len(books) > 0 {
		httputil.RespondWithConflict(c, "cannot delete series with books")
		return
	}
	if err := store.DeleteSeries(seriesID); err != nil {
		httputil.InternalError(c, "failed to delete series", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"message": "series deleted"})
}

// bulkDeleteSeries deletes multiple empty series at once.
func (s *Server) bulkDeleteSeries(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	store := s.Store()
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
	httputil.RespondWithOK(c, gin.H{
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
		httputil.RespondWithBadRequest(c, "invalid series id")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.RespondWithBadRequest(c, "name cannot be empty")
		return
	}
	store := s.Store()
	if err := store.UpdateSeriesName(id, name); err != nil {
		httputil.InternalError(c, "failed to update series", err)
		return
	}
	s.dedupCache.Invalidate("series-duplicates")
	series, _ := store.GetSeriesByID(id)
	httputil.RespondWithOK(c, series)
}

func (s *Server) listNarrators(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	narrators, err := s.Store().ListNarrators()
	if err != nil {
		httputil.InternalError(c, "failed to list narrators", err)
		return
	}
	httputil.RespondWithOK(c, narrators)
}

func (s *Server) countNarrators(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	narrators, err := s.Store().ListNarrators()
	if err != nil {
		httputil.InternalError(c, "failed to count narrators", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"count": len(narrators)})
}

func (s *Server) listAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	narrators, err := s.Store().GetBookNarrators(id)
	if err != nil {
		httputil.InternalError(c, "failed to list audiobook narrators", err)
		return
	}
	if narrators == nil {
		narrators = []database.BookNarrator{}
	}
	httputil.RespondWithOK(c, narrators)
}

func (s *Server) setAudiobookNarrators(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	var narrators []database.BookNarrator
	if err := c.ShouldBindJSON(&narrators); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if err := s.Store().SetBookNarrators(id, narrators); err != nil {
		httputil.InternalError(c, "failed to set audiobook narrators", err)
		return
	}
	httputil.RespondWithOK(c, gin.H{"status": "ok"})
}
