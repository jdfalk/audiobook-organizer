// file: internal/server/version_lifecycle.go
// version: 1.1.0
// guid: 5a3b4c0d-6e7f-4a70-b8c5-3d7e0f1b9a99
//
// Version lifecycle HTTP handlers. Core logic lives in internal/versions.

package server

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

const trashTTLDays = versions.TrashTTLDays

// handleTrashVersion moves a version to trash.
// DELETE /api/v1/books/:id/versions/:vid
func (s *Server) handleTrashVersion(c *gin.Context) {
	bookID := c.Param("id")
	versionID := c.Param("vid")

	ver, err := s.Store().GetBookVersion(versionID)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	if ver.BookID != bookID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version/book mismatch"})
		return
	}

	wasActive := ver.Status == database.BookVersionStatusActive

	ver.Status = database.BookVersionStatusTrash
	if err := s.Store().UpdateBookVersion(ver); err != nil {
		internalError(c, "trash version", err)
		return
	}

	if wasActive {
		if err := versions.AutoPromoteAlt(s.Store(), bookID); err != nil {
			log.Printf("[WARN] auto-promote after trash: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"version": ver})
}

// handleRestoreVersion restores a trashed version to alt.
// POST /api/v1/books/:id/versions/:vid/restore
func (s *Server) handleRestoreVersion(c *gin.Context) {
	bookID := c.Param("id")
	versionID := c.Param("vid")

	ver, err := s.Store().GetBookVersion(versionID)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	if ver.BookID != bookID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version/book mismatch"})
		return
	}
	if ver.Status != database.BookVersionStatusTrash {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is not in trash"})
		return
	}

	ver.Status = database.BookVersionStatusAlt
	if err := s.Store().UpdateBookVersion(ver); err != nil {
		internalError(c, "restore version", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"version": ver})
}

// handlePurgeVersion physically deletes files and marks purged.
// POST /api/v1/books/:id/versions/:vid/purge-now
func (s *Server) handlePurgeVersion(c *gin.Context) {
	bookID := c.Param("id")
	versionID := c.Param("vid")

	ver, err := s.Store().GetBookVersion(versionID)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	if ver.BookID != bookID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version/book mismatch"})
		return
	}

	if err := versions.PurgeVersion(s.Store(), ver); err != nil {
		internalError(c, "purge version", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"version": ver})
}

// handleHardDeleteVersion removes all traces of a purged version.
// DELETE /api/v1/purged-versions/:vid
func (s *Server) handleHardDeleteVersion(c *gin.Context) {
	versionID := c.Param("vid")

	ver, err := s.Store().GetBookVersion(versionID)
	if err != nil || ver == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	if ver.Status != database.BookVersionStatusInactivePurged {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is not purged"})
		return
	}

	if err := s.Store().DeleteBookVersion(ver.ID); err != nil {
		internalError(c, "hard delete version", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": ver.ID})
}

// CleanupTrashedVersions delegates to versions.CleanupTrashedVersions.
func CleanupTrashedVersions(store database.Store) (purged int) {
	return versions.CleanupTrashedVersions(store)
}

// registerVersionLifecycleRoutes wires the version lifecycle endpoints.
func (s *Server) registerVersionLifecycleRoutes(protected *gin.RouterGroup) {
	protected.DELETE("/books/:id/versions/:vid", s.perm(auth.PermLibraryDelete), s.handleTrashVersion)
	protected.POST("/books/:id/versions/:vid/restore", s.perm(auth.PermLibraryOrganize), s.handleRestoreVersion)
	protected.POST("/books/:id/versions/:vid/purge-now", s.perm(auth.PermLibraryDelete), s.handlePurgeVersion)
	protected.DELETE("/purged-versions/:vid", s.perm(auth.PermLibraryDelete), s.handleHardDeleteVersion)
}
