// file: internal/server/user_tags.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef0123456789

package server

import (
	"net/http"
	"strings"

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
	// Filter empty and normalize to lowercase
	validTags := make([]string, 0, len(req.Tags))
	for _, t := range req.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			validTags = append(validTags, t)
		}
	}
	if err := store.SetBookUserTags(id, validTags); err != nil {
		internalError(c, "failed to set tags", err)
		return
	}
	tags, err := store.GetBookUserTags(id)
	if err != nil {
		internalError(c, "failed to get tags after set", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
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
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	if err := store.AddBookUserTag(id, tag); err != nil {
		internalError(c, "failed to add tag", err)
		return
	}
	tags, err := store.GetBookUserTags(id)
	if err != nil {
		internalError(c, "failed to get tags after add", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
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
		internalError(c, "failed to remove tag", err)
		return
	}
	tags, err := store.GetBookUserTags(id)
	if err != nil {
		internalError(c, "failed to get tags after remove", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}
