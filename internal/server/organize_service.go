// file: internal/server/organize_service.go
// version: 2.0.0
// guid: c3d4e5f6-a7b8-c9d0-e1f2-a3b4c5d6e7f8
//
// Thin forwarding layer — the real implementation now lives in
// internal/organizer/service.go. This file provides type aliases and
// constructor wrappers so the rest of the server package can keep using
// the old names without a large rename diff.

package server

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
)

// Type aliases — allow existing server code to keep using the old names.
type OrganizeService = organizer.Service
type OrganizeRequest = organizer.Request
type OrganizeStats = organizer.Stats

// NewOrganizeService creates a new organizer.Service and wires up
// server-specific callbacks (isProtectedPath, iTunes discovery, etc.).
func NewOrganizeService(db database.Store) *OrganizeService {
	svc := organizer.NewService(db)

	// Wire server-specific callbacks
	svc.DiscoverITunesLibraryPath = func(store database.Store) string {
		return discoverITunesLibraryPath(store)
	}
	svc.ExecuteITunesSync = func(ctx context.Context, store database.Store, log logger.Logger, libraryPath string) error {
		return executeITunesSync(ctx, store, log, libraryPath, nil, nil)
	}
	svc.ApplyOrganizedFileMetadata = func(book *database.Book, newPath string) {
		applyOrganizedFileMetadata(book, newPath)
	}
	svc.ComputeITunesPath = metafetch.ComputeITunesPath
	svc.FetchMetadataForBook = func(bookID string) (interface{}, error) {
		mfs := metafetch.NewService(db)
		return mfs.FetchMetadataForBook(bookID)
	}

	return svc
}
