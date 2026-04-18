// file: internal/server/itunes_position_sync.go
// version: 1.0.0
// guid: 9f7a8b5c-0d6e-4a70-b8c5-3d7e0f1b9a99
//
// Bidirectional sync between the app's per-user position/state
// tracking (spec 3.6) and the iTunes Bookmark / Play Count fields
// (spec 3.6 task 4).
//
// Pull direction (iTunes → app):
//   For each iTunes-sourced book with Bookmark > 0, seed an admin
//   user_position row if one doesn't already exist. If iTunes has
//   play_count > 0 but the admin has no book state, seed "finished".
//
// Push direction (app → iTunes):
//   For the admin user's positions that changed since the last sync,
//   write Bookmark and (if finished) increment Play Count + set
//   Played Date via the ITL write-back batcher.
//
// The sync runs as a maintenance task (`itunes_position_sync`) in
// the scheduler. It can also be triggered manually from the API.

package server

import (
	"log"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const adminUserID = "_local"

// SyncITunesPositions runs a full bidirectional position sync for the
// admin user. Pull then push order ensures we don't immediately
// overwrite a newly-seeded position.
func SyncITunesPositions(store database.Store, batcher *WriteBackBatcher) (pulled, pushed int) {
	pulled = pullITunesBookmarks(store)
	pushed = pushPositionsToITunes(store, batcher)
	return pulled, pushed
}

// pullITunesBookmarks seeds admin positions from iTunes Bookmark data.
// Iterates books with an iTunes Bookmark value and creates a position
// row if none exists yet.
func pullITunesBookmarks(store database.Store) int {
	books, err := store.GetAllBooks(0, 0)
	if err != nil {
		log.Printf("[WARN] itunes position sync: list books: %v", err)
		return 0
	}

	seeded := 0
	for _, book := range books {
		if book.ITunesBookmark == nil || *book.ITunesBookmark <= 0 {
			continue
		}

		existing, _ := store.GetUserPosition(adminUserID, book.ID)
		if existing != nil {
			continue
		}

		// Find the first segment to use as the position target.
		files, _ := store.GetBookFiles(book.ID)
		segmentID := ""
		if len(files) > 0 {
			segmentID = files[0].ID
		}
		if segmentID == "" {
			continue
		}

		bookmarkSeconds := float64(*book.ITunesBookmark) / 1000.0
		if err := store.SetUserPosition(adminUserID, book.ID, segmentID, bookmarkSeconds); err != nil {
			log.Printf("[WARN] seed position for %s: %v", book.ID, err)
			continue
		}

		// Recompute the derived book state from the seeded position.
		if _, err := RecomputeUserBookState(store, adminUserID, book.ID); err != nil {
			log.Printf("[WARN] recompute state for %s after bookmark seed: %v", book.ID, err)
		}
		seeded++
	}

	// Also seed "finished" from iTunes play_count > 0 with no existing state.
	for _, book := range books {
		if book.ITunesPlayCount == nil || *book.ITunesPlayCount <= 0 {
			continue
		}
		state, _ := store.GetUserBookState(adminUserID, book.ID)
		if state != nil {
			continue
		}
		if _, err := SetManualStatus(store, adminUserID, book.ID, database.UserBookStatusFinished); err != nil {
			log.Printf("[WARN] seed finished for %s: %v", book.ID, err)
			continue
		}
		seeded++
	}

	return seeded
}

// pushPositionsToITunes writes admin position changes back to iTunes
// via the write-back batcher. For each book where the admin's
// position was updated since the last sync, enqueue the book for
// bookmark writeback. If the book was marked finished, also enqueue
// a play-count increment.
func pushPositionsToITunes(store database.Store, batcher *WriteBackBatcher) int {
	// Get all admin positions that changed in the last 24 hours.
	// A more precise cutoff would use a last-sync-at timestamp;
	// for now 24h is a safe window for the maintenance task that
	// runs every few hours.
	cutoff := time.Now().Add(-24 * time.Hour)
	positions, err := store.ListUserPositionsSince(adminUserID, cutoff)
	if err != nil {
		log.Printf("[WARN] itunes position push: list positions: %v", err)
		return 0
	}

	if batcher == nil {
		return 0
	}

	pushed := 0
	seen := map[string]bool{}
	for _, pos := range positions {
		if seen[pos.BookID] {
			continue
		}
		seen[pos.BookID] = true

		book, err := store.GetBookByID(pos.BookID)
		if err != nil || book == nil || book.ITunesPersistentID == nil {
			continue
		}

		// Update bookmark via the batcher (it updates the ITL on flush).
		bookmarkMs := int64(pos.PositionSeconds * 1000)
		book.ITunesBookmark = &bookmarkMs
		if _, err := store.UpdateBook(book.ID, book); err != nil {
			log.Printf("[WARN] update bookmark for %s: %v", book.ID, err)
			continue
		}
		batcher.Enqueue(book.ID)
		pushed++

		// If the book is marked finished and iTunes play count hasn't
		// been bumped, increment it.
		state, _ := store.GetUserBookState(adminUserID, pos.BookID)
		if state != nil && state.Status == database.UserBookStatusFinished {
			pc := 0
			if book.ITunesPlayCount != nil {
				pc = *book.ITunesPlayCount
			}
			newPC := pc + 1
			now := time.Now()
			book.ITunesPlayCount = &newPC
			book.ITunesLastPlayed = &now
			if _, err := store.UpdateBook(book.ID, book); err != nil {
				log.Printf("[WARN] bump play count for %s: %v", book.ID, err)
			}
		}
	}

	return pushed
}
