// file: internal/server/maintenance_fixups.go
// version: 1.31.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d
// last-edited: 2026-05-01

package server

// maintenanceStore is the narrow slice of database.Store that
// maintenance-fixup helpers share. Every free function across the
// maintenance_*.go files accepts it — the shape is wide but still
// far narrower than full Store (no sessions, no tags, no operations
// tracking, no auth).
//
// All maintenance functions are now split across domain-specific files:
//   - maintenance_readby.go: read-by/narrator fixes
//   - maintenance_series.go: series cleanup and merge
//   - maintenance_files.go: book file backfill, cleanup, enrichment
//   - maintenance_author_version.go: author/narrator swap, version groups
//   - maintenance_dedup.go: book deduplication
//   - maintenance_wipe.go: database wipe operations
//   - maintenance_itunes.go: iTunes integration and repair
//   - maintenance_hashes.go: file hash operations
import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type maintenanceStore interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.BookFileStore
	database.UserTagStore
	database.ExternalIDStore
	database.StatsStore
}
