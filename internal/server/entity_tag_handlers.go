// file: internal/server/entity_tag_handlers.go
// version: 1.0.0
// guid: 7e5f6a4b-8c9d-4a70-b8c5-3d7e0f1b9a99
//
// HTTP endpoints for author and series tags (backlog 7.7).
// Store methods already exist — this wires them to HTTP.

package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
)

// handleGetAuthorTags returns tags for an author.
// GET /api/v1/authors/:id/tags
func (s *Server) handleGetAuthorTags(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author id"})
		return
	}
	tags, err := s.Store().GetAuthorTagsDetailed(id)
	if err != nil {
		internalError(c, "get author tags", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// handleAddAuthorTag adds a tag to an author.
// POST /api/v1/authors/:id/tags
func (s *Server) handleAddAuthorTag(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid author id"})
		return
	}
	var req struct {
		Tag    string `json:"tag" binding:"required"`
		Source string `json:"source,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Source != "" {
		err = s.Store().AddAuthorTagWithSource(id, req.Tag, req.Source)
	} else {
		err = s.Store().AddAuthorTag(id, req.Tag)
	}
	if err != nil {
		internalError(c, "add author tag", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": req.Tag})
}

// handleGetSeriesTags returns tags for a series.
// GET /api/v1/series/:id/tags
func (s *Server) handleGetSeriesTags(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}
	tags, err := s.Store().GetSeriesTagsDetailed(id)
	if err != nil {
		internalError(c, "get series tags", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// handleAddSeriesTag adds a tag to a series.
// POST /api/v1/series/:id/tags
func (s *Server) handleAddSeriesTag(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}
	var req struct {
		Tag    string `json:"tag" binding:"required"`
		Source string `json:"source,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.Store().AddSeriesTag(id, req.Tag); err != nil {
		internalError(c, "add series tag", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": req.Tag})
}

// registerEntityTagRoutes wires the author/series tag endpoints.
func (s *Server) registerEntityTagRoutes(protected *gin.RouterGroup) {
	protected.GET("/authors/:id/tags", s.perm(auth.PermLibraryView), s.handleGetAuthorTags)
	protected.POST("/authors/:id/tags", s.perm(auth.PermLibraryEditMetadata), s.handleAddAuthorTag)
	protected.GET("/series/:id/tags", s.perm(auth.PermLibraryView), s.handleGetSeriesTags)
	protected.POST("/series/:id/tags", s.perm(auth.PermLibraryEditMetadata), s.handleAddSeriesTag)
}
