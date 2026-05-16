// file: internal/server/itl_rebuild.go
// version: 2.0.0
// guid: 8f7e6d5c-4b3a-2c1d-0e9f-8a7b6c5d4e3f
// last-edited: 2026-05-11
//
// iTunes library rebuild service: diffs the current DB state
// against the current ITL file and computes the minimal set of
// changes (adds, removes, metadata updates, location patches)
// to synchronize them. Changes are applied in one atomic
// ApplyITLOperations call through the existing itunesservice.SafeWriteITL
// pipeline (backup → validate → apply → validate → rollback on
// failure). Backlog 7.9 — "diff and batch" mode.
//
// This file is now a thin wrapper around the core rebuild logic in
// internal/itunes/rebuild.go.

package server

import (
	"fmt"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// ITLRebuildPreview summarizes the diff between the DB and the
// current ITL file without applying any changes. Returned by
// the dry-run path so the user can review before committing.
//
// Deprecated: Use itunes.ITLRebuildPreview instead.
type ITLRebuildPreview = itunes.ITLRebuildPreview

// ITLRebuildResult is the outcome of an applied rebuild.
//
// Deprecated: Use itunes.ITLRebuildResult instead.
type ITLRebuildResult = itunes.ITLRebuildResult

// rebuildITLHandler handles POST /api/v1/itunes/rebuild.
// Query param: dry_run=true returns the diff preview without
// applying. Otherwise applies the diff via itunesservice.SafeWriteITL.
func (s *Server) rebuildITLHandler(c *gin.Context) {
	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		httputil.RespondWithBadRequest(c, "ITunesLibraryWritePath not configured")
		return
	}

	store := s.Store()
	ops, preview, err := itunes.ComputeITLDiff(store, itlPath)
	if err != nil {
		httputil.RespondWithInternalError(c, fmt.Sprintf("diff failed: %v", err))
		return
	}

	dryRun := c.Query("dry_run") == "true"
	if dryRun {
		httputil.RespondWithOK(c, struct {
			DryRun  bool                      `json:"dry_run"`
			Preview *itunes.ITLRebuildPreview `json:"preview"`
		}{DryRun: true, Preview: preview})
		return
	}

	// Apply.
	if ops.IsEmpty() {
		httputil.RespondWithOK(c, itunes.ITLRebuildResult{
			Preview: *preview,
			Applied: true,
		})
		return
	}

	if err := itunesservice.SafeWriteITL(itlPath, *ops); err != nil {
		httputil.RespondWithSuccess(c, http.StatusInternalServerError, itunes.ITLRebuildResult{
			Preview: *preview,
			Applied: false,
			Error:   err.Error(),
		})
		return
	}

	log.Printf("[INFO] ITL rebuild: removed %d, added %d, updated-meta %d, updated-loc %d",
		preview.ToRemove, preview.ToAdd, preview.ToUpdateMeta, preview.ToUpdateLoc)

	httputil.RespondWithOK(c, itunes.ITLRebuildResult{
		Preview: *preview,
		Applied: true,
	})
}
