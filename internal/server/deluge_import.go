// file: internal/server/deluge_import.go
// version: 2.0.0
// guid: f3e7a9c1-2b4d-5086-d9f2-4e1c7b0a3e58
// last-edited: 2026-05-11
//
// Thin shim: importToLibrary delegates to deluge.ImportToLibrary.
// The implementation has moved to internal/deluge/import.go.

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
)

// importToLibrary is a package-level shim for backward-compatibility within
// the server package. All callers outside this package should use
// deluge.ImportToLibrary directly.
func importToLibrary(
	cfg *config.Config,
	delugeClient *deluge.Client,
	store database.Store,
	bookFile *database.BookFile,
) (string, error) {
	return deluge.ImportToLibrary(cfg, delugeClient, store, bookFile)
}
