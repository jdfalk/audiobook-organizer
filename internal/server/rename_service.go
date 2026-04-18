// file: internal/server/rename_service.go
// version: 2.0.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/rename.go. This file provides type aliases and
// constructor wrappers so the rest of the server package can keep using
// the old names.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type aliases for backward compatibility.
type RenameService = organizer.RenameService
type TagChange = organizer.TagChange
type RenamePreview = organizer.RenamePreview
type RenameApplyResult = organizer.RenameApplyResult

// NewRenameService creates a new organizer.RenameService and wires up
// server-specific callbacks.
func NewRenameService(db database.Store) *RenameService {
	svc := organizer.NewRenameService(db)
	svc.IsProtectedPath = isProtectedPath
	svc.ResolveAuthorAndSeriesNames = resolveAuthorAndSeriesNames
	svc.FilterUnchangedTags = metafetch.FilterUnchangedTags
	svc.ComputeITunesPath = metafetch.ComputeITunesPath
	return svc
}
