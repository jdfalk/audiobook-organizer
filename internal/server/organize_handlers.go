// file: internal/server/organize_handlers.go
// version: 2.4.0
// last-edited: 2026-05-11
// guid: 1522f0ec-663c-4527-a6d0-645658206a24
//
// Organize/rename HTTP handlers split out of server.go: preview/apply
// for rename templates and the single-book organize entry point.

package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	ulid "github.com/oklog/ulid/v2"
)

// organizeHandlerDeps documents the narrow Server surface needed by the
// organize/rename handlers in this file. *Server satisfies this interface
// automatically via Store(), OrganizeService(), and publishEvent().
type organizeHandlerDeps interface {
	Store() database.Store
	OrganizeService() *OrganizeService
	publishEvent(ctx context.Context, event plugin.Event)
}

var _ organizeHandlerDeps = (*Server)(nil)

// previewRename returns current path, proposed path, and tag diff for a book.
func (s *Server) previewRename(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "book id is required")
		return
	}

	svc := NewRenameService(s.Store())
	preview, err := svc.PreviewRename(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithNotFound(c, "book", id)
			return
		}
		httputil.InternalError(c, "failed to preview rename", err)
		return
	}

	httputil.RespondWithOK(c, preview)
}

// applyRename executes the rename + tag write + DB update for a book.
func (s *Server) applyRename(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "book id is required")
		return
	}

	// Create an operation for tracking and undo support
	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "rename", stringPtr(id))
	if err != nil {
		log.Printf("[ERROR] rename: failed to create operation: %v", err)
		httputil.RespondWithInternalError(c, "failed to create operation record")
		return
	}

	svc := NewRenameService(s.Store())
	result, err := svc.ApplyRename(id, op.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithNotFound(c, "book", id)
			return
		}
		httputil.InternalError(c, "failed to apply rename", err)
		return
	}

	// Rename moved the file on disk → push a location update to iTunes
	// so the ITL keeps pointing at the right path. The batcher already
	// filters non-primary books and is nil-safe.
	if s.writeBackBatcher != nil {
		s.writeBackBatcher.Enqueue(id)
	}

	httputil.RespondWithOK(c, result)
}

// previewOrganize returns a step-by-step preview of what organizing a single book would do.
func (s *Server) previewOrganize(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "book id is required")
		return
	}

	svc := NewOrganizePreviewService(s.Store())
	preview, err := svc.PreviewOrganize(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithNotFound(c, "book", id)
			return
		}
		httputil.InternalError(c, "failed to preview organize", err)
		return
	}

	httputil.RespondWithOK(c, preview)
}

// organizeBook executes the full organize pipeline for a single book.
// It uses the same logic as the batch organize: book_files for multi-file
// books, organizeDirectoryBook for directory-based books, and
// createOrganizedVersion for version-aware DB tracking. This correctly handles
// directory books and author-flat directories used by iTunes.
func (s *Server) organizeBook(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		httputil.RespondWithBadRequest(c, "book id is required")
		return
	}

	// Create an operation for tracking and undo support
	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "organize", stringPtr(id))
	if err != nil {
		log.Printf("[ERROR] organize: failed to create operation: %v", err)
		httputil.RespondWithInternalError(c, "failed to create operation record")
		return
	}

	book, err := s.Store().GetBookByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.RespondWithNotFound(c, "book", id)
			return
		}
		httputil.InternalError(c, "failed to fetch book", err)
		return
	}

	oldPath := book.FilePath
	org := organizer.NewOrganizer(&config.AppConfig)
	org.SetStore(s.Store())
	log2 := logger.NewWithActivityLog("organize", s.Store())

	// Determine whether this is a directory-based (multi-file) book.
	// Prefer book_files count; fall back to os.Stat only when necessary.
	bookFiles, _ := s.Store().GetBookFiles(id)
	isDir := false
	if len(bookFiles) > 1 {
		isDir = true
	} else if len(bookFiles) == 0 {
		if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
			isDir = true
		}
	} else if len(bookFiles) == 1 {
		// Single book_file entry — treat as file unless it has no extension
		if info, statErr := os.Stat(oldPath); statErr == nil && info.IsDir() {
			isDir = true
		}
	}

	alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)

	var newPath string
	if alreadyInRoot {
		newPath, err = s.organizeService.ReOrganizeInPlace(book, log2)
	} else if isDir {
		newPath, err = s.organizeService.OrganizeDirectoryBook(org, book, log2)
	} else {
		newPath, _, err = org.OrganizeBook(book)
	}

	if err != nil {
		httputil.InternalError(c, "failed to organize book", err)
		return
	}

	if oldPath == newPath {
		httputil.RespondWithOK(c, gin.H{
			"message":      "already organized",
			"book_id":      book.ID,
			"old_path":     oldPath,
			"new_path":     newPath,
			"operation_id": op.ID,
		})
		return
	}

	if alreadyInRoot {
		// Re-organized in place — stamp the existing record
		now := time.Now()
		book.LastOrganizeOperationID = &opID
		book.LastOrganizedAt = &now
		if _, updateErr := s.Store().UpdateBook(book.ID, book); updateErr != nil {
			log.Printf("[WARN] organize: failed to stamp book %s: %v", book.ID, updateErr)
		}
		_ = s.Store().CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: op.ID,
			BookID:      book.ID,
			ChangeType:  "organize_rename",
			FieldName:   "file_path",
			OldValue:    oldPath,
			NewValue:    newPath,
		})
		s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventFileOrganized, book.ID, map[string]any{
			"old_path":     oldPath,
			"new_path":     newPath,
			"operation_id": op.ID,
		}))
		httputil.RespondWithOK(c, gin.H{
			"message":      fmt.Sprintf("re-organized: %s → %s", oldPath, newPath),
			"book_id":      book.ID,
			"old_path":     oldPath,
			"new_path":     newPath,
			"operation_id": op.ID,
		})
		return
	}

	// Version-aware organize: create a new organized book record linked to the original
	createdBook, createErr := s.organizeService.CreateOrganizedVersion(org, book, newPath, isDir, op.ID, log2)
	if createErr != nil {
		httputil.InternalError(c, "failed to create organized version", createErr)
		return
	}

	now := time.Now()
	createdBook.LastOrganizeOperationID = &opID
	createdBook.LastOrganizedAt = &now
	if _, updateErr := s.Store().UpdateBook(createdBook.ID, createdBook); updateErr != nil {
		log.Printf("[WARN] organize: failed to stamp organized book %s: %v", createdBook.ID, updateErr)
	}

	// Notify Deluge that the file moved so the torrent client keeps
	// seeding from the new library path. Best-effort — errors are logged
	// inside NotifyDelugeAfterOrganize; the organize itself already succeeded.
	deluge.NotifyDelugeAfterOrganize(s.Store(), book.ID, newPath)

	s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventFileOrganized, createdBook.ID, map[string]any{
		"old_path":         oldPath,
		"new_path":         newPath,
		"original_book_id": book.ID,
		"operation_id":     op.ID,
	}))

	httputil.RespondWithOK(c, gin.H{
		"message":          fmt.Sprintf("organized: %s → %s", oldPath, newPath),
		"book_id":          createdBook.ID,
		"original_book_id": book.ID,
		"old_path":         oldPath,
		"new_path":         newPath,
		"operation_id":     op.ID,
	})
}
