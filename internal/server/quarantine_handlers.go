// file: internal/server/quarantine_handlers.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// quarantineBook handles POST /api/v1/audiobooks/:id/quarantine
func (s *Server) quarantineBook(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Reason == "" {
		req.Reason = "manually quarantined"
	}

	if err := s.QuarantineBook(id, req.Reason); err != nil {
		internalError(c, "quarantine failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "quarantined", "book_id": id})
}

// unquarantineBook handles DELETE /api/v1/audiobooks/:id/quarantine
func (s *Server) unquarantineBook(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	if err := s.UnquarantineBook(id); err != nil {
		internalError(c, "unquarantine failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unquarantined", "book_id": id})
}

// listQuarantinedBooks handles GET /api/v1/audiobooks/quarantined
func (s *Server) listQuarantinedBooks(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	params := ParsePaginationParams(c)
	books, err := s.Store().GetQuarantinedBooks(params.Limit, params.Offset)
	if err != nil {
		internalError(c, "list quarantined books failed", err)
		return
	}
	if books == nil {
		books = []database.Book{}
	}
	c.JSON(http.StatusOK, gin.H{"books": books, "total": len(books)})
}
