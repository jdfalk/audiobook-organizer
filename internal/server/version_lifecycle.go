// file: internal/server/version_lifecycle.go
// version: 1.1.0
// guid: 5a3b4c0d-6e7f-4a70-b8c5-3d7e0f1b9a99
//
// Version lifecycle operations (spec 3.1 task 6).
//
// Lifecycle states:
//   active → alt → trash → inactive_purged → (hard delete)
//
// Delete puts a version in trash (14-day TTL). Auto-promote selects
// the most recent alt if the active version was trashed. Restore
// moves trash back to alt. Purge physically deletes files and
// marks inactive_purged (keeps metadata for fingerprint). Hard
// delete removes all traces.

package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/auth"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/versions"
)

const trashTTLDays = 14

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
		if err := autoPromoteAlt(s.Store(), bookID); err != nil {
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

	if err := purgeVersion(s.Store(), ver); err != nil {
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

// autoPromoteAlt selects the most recent alt version and promotes it
// to active. Called when the active version is trashed.
func autoPromoteAlt(store interface { database.BookReader; database.BookVersionStore; database.BookFileStore }, bookID string) error {
	allVers, err := store.GetBookVersionsByBookID(bookID)
	if err != nil {
		return err
	}
	var bestAlt *database.BookVersion
	for i := range allVers {
		v := &allVers[i]
		if v.Status != database.BookVersionStatusAlt {
			continue
		}
		if bestAlt == nil || v.IngestDate.After(bestAlt.IngestDate) {
			bestAlt = v
		}
	}
	if bestAlt == nil {
		return nil
	}
	bestAlt.Status = database.BookVersionStatusActive
	return store.UpdateBookVersion(bestAlt)
}

// purgeVersion physically deletes the version's files and marks it
// inactive_purged. Keeps the DB rows for fingerprint matching.
func purgeVersion(store interface { database.BookReader; database.BookVersionStore; database.BookFileStore }, ver *database.BookVersion) error {
	book, err := store.GetBookByID(ver.BookID)
	if err != nil || book == nil {
		return fmt.Errorf("book %s not found", ver.BookID)
	}

	// Remove files from .versions/{vid}/ or book root.
	bookDir := ""
	if book.FilePath != "" {
		bookDir = book.FilePath[:len(book.FilePath)-len(book.FilePath)+len(book.FilePath)]
		// Use filepath.Dir in a robust way
		for i := len(book.FilePath) - 1; i >= 0; i-- {
			if book.FilePath[i] == '/' {
				bookDir = book.FilePath[:i]
				break
			}
		}
	}

	if bookDir != "" {
		if err := versions.RemoveVersionSlot(bookDir, ver.ID); err != nil {
			log.Printf("[WARN] remove version slot %s: %v", ver.ID, err)
		}
		_ = versions.PruneEmptyVersionsDir(bookDir)
	}

	// Also remove any files directly associated with this version.
	files, _ := store.GetBookFiles(ver.BookID)
	for _, f := range files {
		if f.VersionID == ver.ID {
			_ = os.Remove(f.FilePath)
		}
	}

	now := time.Now()
	ver.Status = database.BookVersionStatusInactivePurged
	ver.PurgedDate = &now
	return store.UpdateBookVersion(ver)
}

// CleanupTrashedVersions is the maintenance task that purges versions
// past their TTL. Called by the scheduler.
func CleanupTrashedVersions(store interface { database.BookReader; database.BookVersionStore; database.BookFileStore }) (purged int) {
	trashed, err := store.ListTrashedBookVersions()
	if err != nil {
		log.Printf("[WARN] list trashed versions: %v", err)
		return 0
	}

	cutoff := time.Now().Add(-time.Duration(trashTTLDays) * 24 * time.Hour)
	for i := range trashed {
		v := &trashed[i]
		if v.CreatedAt.After(cutoff) {
			continue
		}
		if err := purgeVersion(store, v); err != nil {
			log.Printf("[WARN] purge trashed %s: %v", v.ID, err)
			continue
		}
		purged++
	}
	return purged
}

// registerVersionLifecycleRoutes wires the version lifecycle endpoints.
func (s *Server) registerVersionLifecycleRoutes(protected *gin.RouterGroup) {
	protected.DELETE("/books/:id/versions/:vid", s.perm(auth.PermLibraryDelete), s.handleTrashVersion)
	protected.POST("/books/:id/versions/:vid/restore", s.perm(auth.PermLibraryOrganize), s.handleRestoreVersion)
	protected.POST("/books/:id/versions/:vid/purge-now", s.perm(auth.PermLibraryDelete), s.handlePurgeVersion)
	protected.DELETE("/purged-versions/:vid", s.perm(auth.PermLibraryDelete), s.handleHardDeleteVersion)
}
