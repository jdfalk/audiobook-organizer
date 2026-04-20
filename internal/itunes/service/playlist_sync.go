// file: internal/itunes/service/playlist_sync.go
// version: 2.0.0
// guid: 1e9f0a8b-2c3d-4a70-b8c5-3d7e0f1b9a99
//
// iTunes playlist sync (spec 3.4 tasks 5-6).
//
// Task 5: One-time migration of iTunes dynamic playlists.
//   Reads smart playlists from the ITL, parses their Smart Criteria
//   blob, translates to our DSL, and creates UserPlaylist rows with
//   type=smart. Stores the raw criteria blob in ITunesRawCriteriaB64
//   for audit. Runs once (idempotent — skips playlists already
//   imported by iTunes PID).
//
// Task 6: Push playlists to ITL.
//   For dirty playlists with no iTunes PID, creates a new ITL
//   playlist. For dirty playlists with an existing PID, updates the
//   track list. Smart playlists are pushed as static (materialized)
//   since iTunes will manage its own smart criteria.

package itunesservice

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// playlistSyncStore is the narrow slice of the service's Store that
// PlaylistSync needs.
type playlistSyncStore interface {
	database.UserPlaylistStore
}

// PlaylistSync owns the two-way iTunes-playlist sync paths (import
// smart playlists from the ITL, push dirty playlists back out).
type PlaylistSync struct {
	store    playlistSyncStore
	enqueuer Enqueuer
}

// newPlaylistSync wires a PlaylistSync with the given store and
// enqueuer. A nil enqueuer disables the push direction's ITL write-back
// enqueue (the dirty flag is still cleared).
func newPlaylistSync(store playlistSyncStore, enqueuer Enqueuer) *PlaylistSync {
	return &PlaylistSync{store: store, enqueuer: enqueuer}
}

// MigrateSmartPlaylists reads smart playlists from the ITL library
// and creates UserPlaylist rows for each. Idempotent — playlists
// already imported (by iTunes PID) are skipped.
func (p *PlaylistSync) MigrateSmartPlaylists(lib *itunes.ITLLibrary) (imported, skipped int) {
	if lib == nil {
		return 0, 0
	}

	for _, pl := range lib.Playlists {
		if !pl.IsSmart || len(pl.SmartCriteria) == 0 {
			continue
		}

		pid := hex.EncodeToString(pl.PersistentID[:])

		existing, _ := p.store.GetUserPlaylistByITunesPID(pid)
		if existing != nil {
			skipped++
			continue
		}

		parsed, err := itunes.ParseSmartCriteria(pl.SmartCriteria)
		if err != nil {
			log.Printf("[WARN] parse smart criteria for %q (PID %s): %v", pl.Title, pid, err)
			skipped++
			continue
		}
		dslQuery := itunes.TranslateSmartCriteria(parsed)

		rawB64 := base64.StdEncoding.EncodeToString(pl.SmartCriteria)

		_, err = p.store.CreateUserPlaylist(&database.UserPlaylist{
			Name:                 pl.Title,
			Type:                 database.UserPlaylistTypeSmart,
			Query:                dslQuery,
			ITunesPersistentID:   pid,
			ITunesRawCriteriaB64: rawB64,
			Description:          fmt.Sprintf("Imported from iTunes smart playlist %q", pl.Title),
		})
		if err != nil {
			log.Printf("[WARN] create playlist %q: %v", pl.Title, err)
			skipped++
			continue
		}
		imported++
	}

	return imported, skipped
}

// PushDirty writes dirty playlists to the ITL. Smart playlists are
// materialized first (the materialized_book_ids field is used).
// Returns the number pushed.
//
// Placeholder that enqueues the playlist book IDs for the ITL
// write-back batcher. Full ITL playlist creation requires the ITL
// writer to support playlist insertion, which is tracked separately.
func (p *PlaylistSync) PushDirty() int {
	dirties, err := p.store.ListDirtyUserPlaylists()
	if err != nil {
		log.Printf("[WARN] list dirty playlists: %v", err)
		return 0
	}

	pushed := 0
	for i := range dirties {
		pl := &dirties[i]

		bookIDs := pl.BookIDs
		if pl.Type == database.UserPlaylistTypeSmart {
			bookIDs = pl.MaterializedBookIDs
		}

		if len(bookIDs) == 0 {
			continue
		}

		if p.enqueuer != nil {
			for _, bid := range bookIDs {
				p.enqueuer.Enqueue(bid)
			}
		}

		pl.Dirty = false
		if err := p.store.UpdateUserPlaylist(pl); err != nil {
			log.Printf("[WARN] clear dirty for %s: %v", pl.ID, err)
			continue
		}
		pushed++
	}

	return pushed
}
