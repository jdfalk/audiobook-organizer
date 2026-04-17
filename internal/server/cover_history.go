// file: internal/server/cover_history.go
// version: 1.0.0
// guid: 6d4e5f3a-7b8c-4a70-b8c5-3d7e0f1b9a99
//
// Cover art history browsing + restore (backlog 3.5).
//
// Each time a book's cover is updated, the previous cover is saved
// to covers/history/{bookID}/. This endpoint lists those versions
// so the user can browse and restore a previous cover.

package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// CoverHistoryEntry represents one saved cover version.
type CoverHistoryEntry struct {
	Filename  string `json:"filename"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"size_bytes"`
	ModTime   string `json:"mod_time"`
}

// handleListCoverHistory returns the cover art history for a book.
// GET /api/v1/audiobooks/:id/cover-history
func (s *Server) handleListCoverHistory(c *gin.Context) {
	bookID := c.Param("id")
	histDir := filepath.Join(config.AppConfig.RootDir, "covers", "history", bookID)

	entries, err := os.ReadDir(histDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"covers": []CoverHistoryEntry{}, "count": 0})
			return
		}
		internalError(c, "read cover history", err)
		return
	}

	var covers []CoverHistoryEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		covers = append(covers, CoverHistoryEntry{
			Filename:  name,
			URL:       "/api/v1/covers/local/" + name,
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().Format("2006-01-02T15:04:05Z"),
		})
	}

	sort.Slice(covers, func(i, j int) bool {
		return covers[i].ModTime > covers[j].ModTime
	})

	c.JSON(http.StatusOK, gin.H{"covers": covers, "count": len(covers)})
}

// handleRestoreCover copies a history cover back as the book's
// current cover.
// POST /api/v1/audiobooks/:id/cover-history/restore
func (s *Server) handleRestoreCover(c *gin.Context) {
	bookID := c.Param("id")
	var req struct {
		Filename string `json:"filename" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	book, err := s.Store().GetBookByID(bookID)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}

	srcPath := filepath.Join(config.AppConfig.RootDir, "covers", "history", bookID, req.Filename)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "cover file not found"})
		return
	}

	// Copy the history file to the current cover location.
	dstDir := filepath.Join(config.AppConfig.RootDir, "covers")
	ext := filepath.Ext(req.Filename)
	dstPath := filepath.Join(dstDir, bookID+ext)

	src, err := os.ReadFile(srcPath)
	if err != nil {
		internalError(c, "read history cover", err)
		return
	}
	if err := os.WriteFile(dstPath, src, 0o644); err != nil {
		internalError(c, "write restored cover", err)
		return
	}

	// Update book's cover_url.
	coverURL := "/api/v1/covers/local/" + bookID + ext
	book.CoverURL = &coverURL
	if _, err := s.Store().UpdateBook(book.ID, book); err != nil {
		internalError(c, "update book cover", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"cover_url": coverURL})
}
