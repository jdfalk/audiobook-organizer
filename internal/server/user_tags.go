// file: internal/server/user_tags.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef0123456789

package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// setupUserTagRoutes registers the user tag API routes on the given router group.
func (s *Server) setupUserTagRoutes(protected *gin.RouterGroup) {
	protected.PUT("/audiobooks/:id/user-tags", s.setBookUserTags)
	protected.POST("/audiobooks/:id/user-tags", s.addBookUserTag)
	protected.DELETE("/audiobooks/:id/user-tags/:tag", s.removeBookUserTag)
}

// setBookUserTags replaces all user-defined tags on a book.
func (s *Server) setBookUserTags(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	// Verify the book exists before creating tag entries to prevent orphaned rows.
	if _, err := store.GetBookByID(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}
	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.SetBookUserTags(id, req.Tags); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to set tags: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": req.Tags})
}

// addBookUserTag adds a single user-defined tag to a book.
func (s *Server) addBookUserTag(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	// Verify the book exists before creating tag entries to prevent orphaned rows.
	if _, err := store.GetBookByID(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}
	var req struct {
		Tag string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.AddBookUserTag(id, req.Tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to add tag: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tag": req.Tag})
}

// removeBookUserTag removes a single user-defined tag from a book.
func (s *Server) removeBookUserTag(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	id := c.Param("id")
	// Verify the book exists before modifying tag entries to prevent orphaned rows.
	if _, err := store.GetBookByID(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag parameter required"})
		return
	}
	if err := store.RemoveBookUserTag(id, tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove tag: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "tag removed"})
}
