// file: internal/server/organize_preview_service.go
// version: 2.0.0
// guid: f1a2b3c4-d5e6-7890-abcd-ef1234567890
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/preview.go. This file provides type aliases and
// constructor wrappers so the rest of the server package can keep using
// the old names.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type aliases for backward compatibility.
type OrganizePreviewStep = organizer.PreviewStep
type OrganizePreviewResponse = organizer.PreviewResponse
type OrganizePreviewService = organizer.PreviewService

// NewOrganizePreviewService creates a new organizer.PreviewService and wires up
// server-specific callbacks.
func NewOrganizePreviewService(db database.Store) *OrganizePreviewService {
	svc := organizer.NewPreviewService(db)
	svc.IsProtectedPath = isProtectedPath
	svc.ResolveAuthorAndSeriesNames = resolveAuthorAndSeriesNames
	return svc
}
