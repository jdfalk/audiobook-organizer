// file: internal/server/interfaces.go
// version: 1.0.0
// guid: 9a8b7c6d-5e4f-3a2b-1c0d-efaebdacbbaa
// last-edited: 2026-05-04

package server

import (
	"github.com/falkcorp/audiobook-organizer/internal/database"
)

// bookHandlerStore is the narrow slice of database.Store used by
// book/audiobook handlers (audiobooks_handlers.go). Documents the
// Store subset these handlers actually depend on.
type bookHandlerStore interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.NarratorStore
	database.BookFileStore
	database.BookSegmentStore
	database.MetadataStore
	database.BookVersionStore
	database.RejectedMetadataStore
	database.TagStore
	database.UserPositionStore
	database.UserTagStore
}

// userHandlerStore is the narrow slice of database.Store used by
// user/entity handlers (entities_handlers.go). Covers authors, narrators,
// series, works, and related entity management.
type userHandlerStore interface {
	database.UserStore
	database.AuthorStore
	database.NarratorStore
	database.SeriesStore
	database.WorkStore
	database.TagStore
	database.UserTagStore
}

// playlistHandlerStore is the narrow slice of database.Store used by
// playlist handlers (playlist_handlers.go).
type playlistHandlerStore interface {
	database.UserPlaylistStore
	database.PlaylistStore
	database.BookStore
	database.UserPositionStore
}

// metadataHandlerStore is the narrow slice of database.Store used by
// metadata handlers (metadata_handlers.go). Covers metadata field state,
// change history, alternative titles, and rejected metadata.
type metadataHandlerStore interface {
	database.BookStore
	database.MetadataStore
	database.AuthorStore
	database.RejectedMetadataStore
	database.BookVersionStore
}

// Compile-time assertions verify that database.SQLiteStore satisfies
// all narrow handler interfaces. If these fail, the Store interface
// has changed and these narrow interfaces need updating.
var (
	_ bookHandlerStore     = (*database.SQLiteStore)(nil)
	_ userHandlerStore     = (*database.SQLiteStore)(nil)
	_ playlistHandlerStore = (*database.SQLiteStore)(nil)
	_ metadataHandlerStore = (*database.SQLiteStore)(nil)
)
