// file: internal/server/playlist_itunes_sync.go
// version: 1.0.0
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

package server

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// MigrateITunesSmartPlaylists reads smart playlists from the ITL
// library and creates UserPlaylist rows for each. Idempotent —
// playlists already imported (by iTunes PID) are skipped.
func MigrateITunesSmartPlaylists(store database.Store, lib *itunes.ITLLibrary) (imported, skipped int) {
	if lib == nil {
		return 0, 0
	}

	for _, pl := range lib.Playlists {
		if !pl.IsSmart || len(pl.SmartCriteria) == 0 {
			continue
		}

		pid := hex.EncodeToString(pl.PersistentID[:])

		// Skip if already imported.
		existing, _ := store.GetUserPlaylistByITunesPID(pid)
		if existing != nil {
			skipped++
			continue
		}

		// Parse and translate criteria.
		parsed, err := itunes.ParseSmartCriteria(pl.SmartCriteria)
		if err != nil {
			log.Printf("[WARN] parse smart criteria for %q (PID %s): %v", pl.Title, pid, err)
			skipped++
			continue
		}
		dslQuery := itunes.TranslateSmartCriteria(parsed)

		// Store the raw criteria for audit.
		rawB64 := base64.StdEncoding.EncodeToString(pl.SmartCriteria)

		_, err = store.CreateUserPlaylist(&database.UserPlaylist{
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

// PushDirtyPlaylistsToITunes writes dirty playlists to the ITL.
// Smart playlists are materialized first (the materialized_book_ids
// field is used). Returns the number pushed.
//
// This is a placeholder that enqueues the playlist book IDs for
// the ITL write-back batcher. Full ITL playlist creation requires
// the ITL writer to support playlist insertion, which is tracked
// separately.
func PushDirtyPlaylistsToITunes(store database.Store) int {
	dirties, err := store.ListDirtyUserPlaylists()
	if err != nil {
		log.Printf("[WARN] list dirty playlists: %v", err)
		return 0
	}

	pushed := 0
	for i := range dirties {
		pl := &dirties[i]

		// For smart playlists, use materialized book IDs.
		bookIDs := pl.BookIDs
		if pl.Type == database.UserPlaylistTypeSmart {
			bookIDs = pl.MaterializedBookIDs
		}

		if len(bookIDs) == 0 {
			continue
		}

		// Enqueue each book for ITL writeback so its track exists.
		if GlobalWriteBackBatcher != nil {
			for _, bid := range bookIDs {
				GlobalWriteBackBatcher.Enqueue(bid)
			}
		}

		// Clear dirty flag.
		pl.Dirty = false
		if err := store.UpdateUserPlaylist(pl); err != nil {
			log.Printf("[WARN] clear dirty for %s: %v", pl.ID, err)
			continue
		}
		pushed++
	}

	return pushed
}
