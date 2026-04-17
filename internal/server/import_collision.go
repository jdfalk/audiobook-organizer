// file: internal/server/import_collision.go
// version: 1.0.0
// guid: 4b2c3d1e-5f6a-4a70-b8c5-3d7e0f1b9a99
//
// Import-time collision preview (backlog 1.6). Before importing a
// file, check whether it collides with an existing book (by title
// match or file hash) so the user can decide whether to skip,
// merge, or create a new version.

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// CollisionCandidate describes one existing book that may collide
// with a file about to be imported.
type CollisionCandidate struct {
	BookID    string `json:"book_id"`
	Title     string `json:"title"`
	MatchType string `json:"match_type"` // "title", "file_hash", "fingerprint"
	FilePath  string `json:"file_path,omitempty"`
}

// handleImportCollisionPreview checks whether importing a file
// would collide with existing library content.
// POST /api/v1/import/collision-preview
func (s *Server) handleImportCollisionPreview(c *gin.Context) {
	var req struct {
		FilePath    string `json:"file_path" binding:"required"`
		TorrentHash string `json:"torrent_hash,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var candidates []CollisionCandidate

	// 1. Fingerprint check (purged/blocked content).
	if req.TorrentHash != "" {
		match := CheckFingerprint(s.Store(), req.TorrentHash, nil)
		if match != nil && match.Matched {
			candidates = append(candidates, CollisionCandidate{
				BookID:    match.BookID,
				Title:     bookTitle(s.Store(), match.BookID),
				MatchType: "fingerprint",
			})
		}
	}

	// 2. File hash check against existing books.
	if _, err := os.Stat(req.FilePath); err == nil {
		hash := quickHash(req.FilePath)
		if hash != "" {
			existing, _ := s.Store().GetBookByFileHash(hash)
			if existing != nil {
				candidates = append(candidates, CollisionCandidate{
					BookID:    existing.ID,
					Title:     existing.Title,
					MatchType: "file_hash",
					FilePath:  existing.FilePath,
				})
			}
		}
	}

	// 3. Title match via metadata extraction.
	meta, err := metadata.ExtractMetadata(req.FilePath, nil)
	if err == nil && meta.Title != "" {
		titleLower := strings.ToLower(strings.TrimSpace(meta.Title))
		books, _ := s.Store().GetAllBooks(0, 0)
		for _, b := range books {
			if strings.ToLower(strings.TrimSpace(b.Title)) == titleLower {
				alreadyListed := false
				for _, c := range candidates {
					if c.BookID == b.ID {
						alreadyListed = true
						break
					}
				}
				if !alreadyListed {
					candidates = append(candidates, CollisionCandidate{
						BookID:    b.ID,
						Title:     b.Title,
						MatchType: "title",
						FilePath:  b.FilePath,
					})
				}
			}
		}
	}

	if candidates == nil {
		candidates = []CollisionCandidate{}
	}
	c.JSON(http.StatusOK, gin.H{
		"collisions": candidates,
		"count":      len(candidates),
		"has_collision": len(candidates) > 0,
	})
}

func bookTitle(store database.Store, id string) string {
	b, _ := store.GetBookByID(id)
	if b != nil {
		return b.Title
	}
	return ""
}

func quickHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, 1<<20)); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
