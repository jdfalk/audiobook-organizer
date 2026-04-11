// file: internal/server/versions_handlers.go
// version: 1.0.0
// guid: b2fde2ee-5301-4240-9def-503917e19a78
//
// Version-grouping HTTP handlers split out of server.go: list/link/
// set-primary/get-group for audiobook version groups, plus split and
// move operations on segments and versions.

package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// listAudiobookVersions lists all versions of an audiobook
func (s *Server) listAudiobookVersions(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		c.JSON(http.StatusOK, gin.H{"versions": []any{book}})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": books})
}

// linkAudiobookVersion links an audiobook as another version
func (s *Server) linkAudiobookVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		OtherID string `json:"other_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book1, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	book2, err := database.GlobalStore.GetBookByID(req.OtherID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "other audiobook not found"})
		return
	}

	versionGroupID := ""
	if book1.VersionGroupID != nil {
		versionGroupID = *book1.VersionGroupID
	} else if book2.VersionGroupID != nil {
		versionGroupID = *book2.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
	}

	book1.VersionGroupID = &versionGroupID
	book2.VersionGroupID = &versionGroupID

	if _, err := database.GlobalStore.UpdateBook(id, book1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
		return
	}

	if _, err := database.GlobalStore.UpdateBook(req.OtherID, book2); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update other audiobook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"version_group_id": versionGroupID})
}

// setAudiobookPrimary sets an audiobook as the primary version
func (s *Server) setAudiobookPrimary(c *gin.Context) {
	id := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	if book.VersionGroupID == nil {
		primaryFlag := true
		book.IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(id, book); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update audiobook"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(*book.VersionGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch versions"})
		return
	}

	for i := range books {
		primaryFlag := books[i].ID == id
		books[i].IsPrimaryVersion = &primaryFlag
		if _, err := database.GlobalStore.UpdateBook(books[i].ID, &books[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update version"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "audiobook set as primary"})
}

// getVersionGroup gets all audiobooks in a version group
func (s *Server) getVersionGroup(c *gin.Context) {
	groupID := c.Param("id")

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	books, err := database.GlobalStore.GetBooksByVersionGroup(groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch version group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"audiobooks": books})
}

// splitVersion moves selected segments from a book into a new version (a new book
// in the same version group).
func (s *Server) splitVersion(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs []string `json:"segment_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// 1. Get source book
	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// 2. Ensure source book has a version group
	versionGroupID := ""
	if sourceBook.VersionGroupID != nil && *sourceBook.VersionGroupID != "" {
		versionGroupID = *sourceBook.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
		sourceBook.VersionGroupID = &versionGroupID
		if _, err := database.GlobalStore.UpdateBook(id, sourceBook); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update source book version group"})
			return
		}
	}

	// 3. Count existing versions to determine suffix
	existingVersions, _ := database.GlobalStore.GetBooksByVersionGroup(versionGroupID)
	versionNum := len(existingVersions) + 1

	// 4. Create new book entry (copy metadata from source, but NOT FilePath —
	// the new version's path will be derived from its segments after they're moved)
	newTitle := fmt.Sprintf("%s (Version %d)", sourceBook.Title, versionNum)
	primaryFlag := false
	newBook := &database.Book{
		Title:            newTitle,
		AuthorID:         sourceBook.AuthorID,
		SeriesID:         sourceBook.SeriesID,
		SeriesSequence:   sourceBook.SeriesSequence,
		FilePath:         "", // Will be set from segments below
		Format:           sourceBook.Format,
		WorkID:           sourceBook.WorkID,
		Narrator:         sourceBook.Narrator,
		Language:         sourceBook.Language,
		Publisher:        sourceBook.Publisher,
		VersionGroupID:   &versionGroupID,
		IsPrimaryVersion: &primaryFlag,
	}

	createdBook, err := database.GlobalStore.CreateBook(newBook)
	if err != nil {
		internalError(c, "failed to create new version", err)
		return
	}

	// 5. Move files to the new book (DB-only, does NOT touch files on disk)
	if err := database.GlobalStore.MoveBookFilesToBook(req.SegmentIDs, sourceBook.ID, createdBook.ID); err != nil {
		internalError(c, "failed to move files", err)
		return
	}

	// 6. Derive the new book's FilePath from its files.
	// For multi-file books, FilePath is the common parent directory.
	// For single-file books, FilePath is the file path itself.
	newFiles, _ := database.GlobalStore.GetBookFiles(createdBook.ID)
	if len(newFiles) > 0 {
		if len(newFiles) == 1 {
			createdBook.FilePath = newFiles[0].FilePath
		} else {
			createdBook.FilePath = filesCommonDir(newFiles)
		}
		database.GlobalStore.UpdateBook(createdBook.ID, createdBook)
	}

	// 7. Also update the source book's FilePath from its remaining files
	remainingFiles, _ := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if len(remainingFiles) > 0 {
		if len(remainingFiles) == 1 {
			sourceBook.FilePath = remainingFiles[0].FilePath
		} else {
			sourceBook.FilePath = filesCommonDir(remainingFiles)
		}
		database.GlobalStore.UpdateBook(sourceBook.ID, sourceBook)
	}

	c.JSON(http.StatusOK, gin.H{
		"book":             createdBook,
		"version_group_id": versionGroupID,
		"segments_moved":   len(req.SegmentIDs),
	})
}

// splitSegmentsToBooks splits selected segments out of a multi-file book into
// independent new books (one per segment), extracting titles from filenames.
// Unlike splitVersion, the new books are NOT version-linked to the source.
func (s *Server) splitSegmentsToBooks(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs []string `json:"segment_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil || sourceBook == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	// Build a lookup of file ID → BookFile
	allFiles, err := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list book files"})
		return
	}
	fileMap := make(map[string]database.BookFile, len(allFiles))
	for _, f := range allFiles {
		fileMap[f.ID] = f
	}

	// Create one new book per selected file
	var createdBooks []interface{}
	for _, fileID := range req.SegmentIDs {
		f, ok := fileMap[fileID]
		if !ok {
			continue
		}

		// Extract title from file name
		// e.g. "01 ASoIaF 1 - A Game of Thrones.m4b" → "A Game of Thrones"
		title := extractTitleFromSegmentFilename(filepath.Base(f.FilePath))
		if title == "" {
			title = sourceBook.Title + " (split)"
		}

		durationSec := f.Duration / 1000 // BookFile stores ms
		newBook := &database.Book{
			Title:     title,
			AuthorID:  sourceBook.AuthorID,
			SeriesID:  sourceBook.SeriesID,
			FilePath:  f.FilePath,
			Format:    f.Format,
			Narrator:  sourceBook.Narrator,
			Language:  sourceBook.Language,
			Publisher: sourceBook.Publisher,
			Duration:  &durationSec,
			FileSize:  &f.FileSize,
		}

		created, createErr := database.GlobalStore.CreateBook(newBook)
		if createErr != nil {
			log.Printf("[WARN] splitSegmentsToBooks: failed to create book for file %s: %v", fileID, createErr)
			continue
		}

		// Copy book_authors from source
		if authors, aErr := database.GlobalStore.GetBookAuthors(sourceBook.ID); aErr == nil && len(authors) > 0 {
			var newAuthors []database.BookAuthor
			for _, ba := range authors {
				newAuthors = append(newAuthors, database.BookAuthor{
					BookID:   created.ID,
					AuthorID: ba.AuthorID,
					Role:     ba.Role,
				})
			}
			_ = database.GlobalStore.SetBookAuthors(created.ID, newAuthors)
		}

		// Move the file to the new book
		_ = database.GlobalStore.MoveBookFilesToBook([]string{fileID}, sourceBook.ID, created.ID)

		// Reassign external ID mappings (iTunes PIDs) that belong to the moved file
		reassignExternalIDsForFiles(sourceBook.ID, created.ID, []database.BookFile{f})

		createdBooks = append(createdBooks, created)
	}

	// Update source book's FilePath from remaining files
	remainingFiles, _ := database.GlobalStore.GetBookFiles(sourceBook.ID)
	if len(remainingFiles) > 0 {
		if len(remainingFiles) == 1 {
			sourceBook.FilePath = remainingFiles[0].FilePath
		} else {
			sourceBook.FilePath = filesCommonDir(remainingFiles)
		}
		database.GlobalStore.UpdateBook(sourceBook.ID, sourceBook)
	}

	c.JSON(http.StatusOK, gin.H{
		"created_books": createdBooks,
		"count":         len(createdBooks),
	})
}

// moveSegments moves segments from one book to another within the same version group.
func (s *Server) moveSegments(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		SegmentIDs   []string `json:"segment_ids" binding:"required"`
		TargetBookID string   `json:"target_book_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.SegmentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "segment_ids must not be empty"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// 1. Get source and target books
	sourceBook, err := database.GlobalStore.GetBookByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source audiobook not found"})
		return
	}

	targetBook, err := database.GlobalStore.GetBookByID(req.TargetBookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "target audiobook not found"})
		return
	}

	// 2. Verify both books are in the same version group
	if sourceBook.VersionGroupID == nil || targetBook.VersionGroupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "both books must be in a version group"})
		return
	}
	if *sourceBook.VersionGroupID != *targetBook.VersionGroupID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "books must be in the same version group"})
		return
	}

	// 3. Verify the files belong to the source book
	sourceFiles, err := database.GlobalStore.GetBookFiles(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list source book files"})
		return
	}
	sourceFileMap := make(map[string]bool, len(sourceFiles))
	for _, f := range sourceFiles {
		sourceFileMap[f.ID] = true
	}
	for _, segID := range req.SegmentIDs {
		if !sourceFileMap[segID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file %s does not belong to source book", segID)})
			return
		}
	}

	// 4. Collect the files being moved (for external ID reassignment)
	var movedFiles []database.BookFile
	movedSet := make(map[string]bool, len(req.SegmentIDs))
	for _, sid := range req.SegmentIDs {
		movedSet[sid] = true
	}
	for _, f := range sourceFiles {
		if movedSet[f.ID] {
			movedFiles = append(movedFiles, f)
		}
	}

	// 5. Move files
	if err := database.GlobalStore.MoveBookFilesToBook(req.SegmentIDs, id, req.TargetBookID); err != nil {
		internalError(c, "failed to move files", err)
		return
	}

	// 6. Reassign external ID mappings (iTunes PIDs) for moved files
	reassignExternalIDsForFiles(id, req.TargetBookID, movedFiles)

	c.JSON(http.StatusOK, gin.H{
		"segments_moved": len(req.SegmentIDs),
		"source_book_id": id,
		"target_book_id": req.TargetBookID,
	})
}
