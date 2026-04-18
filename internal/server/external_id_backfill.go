// file: internal/server/external_id_backfill.go
// version: 1.2.0
// guid: a3b4c5d6-e7f8-4a9b-0c1d-2e3f4a5b6c7d

package server

import (
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// ExternalIDStore defines the external ID mapping operations.
// Store implementations should satisfy this interface. Integration code
// performs a runtime type assertion: if the underlying store does not yet
// implement these methods the calls gracefully no-op.
type ExternalIDStore interface {
	CreateExternalIDMapping(mapping *database.ExternalIDMapping) error
	GetBookByExternalID(source, externalID string) (string, error)
	GetExternalIDsForBook(bookID string) ([]database.ExternalIDMapping, error)
	IsExternalIDTombstoned(source, externalID string) (bool, error)
	TombstoneExternalID(source, externalID string) error
	ReassignExternalIDs(oldBookID, newBookID string) error
	BulkCreateExternalIDMappings(mappings []database.ExternalIDMapping) error
}

// asExternalIDStore returns the ExternalIDStore if the given store implements
// it, or nil otherwise. Callers should check for nil before using. Accepts
// `any` so callers holding a narrow sub-interface of database.Store (e.g.
// audiobookStore) can still pass through — the type assertion checks the
// underlying concrete type regardless of the static handle type.
func asExternalIDStore(s any) ExternalIDStore {
	if s == nil {
		return nil
	}
	if eid, ok := s.(ExternalIDStore); ok {
		return eid
	}
	return nil
}

// backfillExternalIDs scans all books and creates external ID mappings for any
// book that has an iTunes PersistentID set. This is idempotent — it checks the
// setting "external_id_backfill_done" and only runs once.
func (s *Server) backfillExternalIDs() {
	store := s.Store()
	if store == nil {
		return
	}

	eidStore := asExternalIDStore(store)
	if eidStore == nil {
		log.Printf("[DEBUG] backfillExternalIDs: store does not implement ExternalIDStore, skipping")
		return
	}

	// Check if backfill has already been performed (v4 = includes BookFile-level PIDs)
	if setting, err := store.GetSetting("external_id_backfill_v4_done"); err == nil && setting != nil && setting.Value == "true" {
		log.Printf("[INFO] External ID backfill v4 already completed, skipping")
		return
	}
	log.Printf("[INFO] Starting external ID backfill v4...")

	offset := 0
	backfilled := 0
	for {
		books, err := store.GetAllBooks(10000, offset)
		if err != nil || len(books) == 0 {
			break
		}
		for _, book := range books {
			// Book-level PID
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				_ = eidStore.CreateExternalIDMapping(&database.ExternalIDMapping{
					Source:     "itunes",
					ExternalID: *book.ITunesPersistentID,
					BookID:     book.ID,
				})
				backfilled++
			}

			// BookFile-level PIDs (catches split books, multi-file books, etc.)
			files, fErr := store.GetBookFiles(book.ID)
			if fErr != nil {
				continue
			}
			for _, f := range files {
				if f.ITunesPersistentID != "" {
					_ = eidStore.CreateExternalIDMapping(&database.ExternalIDMapping{
						Source:     "itunes",
						ExternalID: f.ITunesPersistentID,
						BookID:     book.ID,
						FilePath:   f.FilePath,
						Provenance: "backfill_v4",
					})
					backfilled++
				}
			}
		}
		offset += 10000
	}

	log.Printf("[INFO] Backfilled %d external ID mappings from book + file records", backfilled)

	// Backfill ALL track-level PIDs from the iTunes XML
	itunesBackfilled := s.backfillITunesTrackPIDs(store, eidStore)
	if itunesBackfilled > 0 {
		log.Printf("[INFO] Backfilled %d track-level PIDs from iTunes XML", itunesBackfilled)
	}

	// Only mark as done AFTER everything completes successfully
	_ = store.SetSetting("external_id_backfill_v4_done", "true", "bool", false)
	log.Printf("[INFO] External ID backfill v4 complete")
}

// backfillITunesTrackPIDs reads the iTunes XML and registers ALL track PIDs
// for existing books. This catches the multi-track albums where only the first
// track's PID was stored on the book record.
func (s *Server) backfillITunesTrackPIDs(store database.Store, eidStore ExternalIDStore) int {
	xmlPath := config.AppConfig.ITunesLibraryReadPath
	if xmlPath == "" {
		log.Printf("[INFO] backfillITunesTrackPIDs: no iTunes XML path configured, skipping")
		return 0
	}

	log.Printf("[INFO] backfillITunesTrackPIDs: parsing iTunes XML at %s", xmlPath)
	lib, err := itunes.ParseLibrary(xmlPath)
	if err != nil {
		log.Printf("[WARN] backfillITunesTrackPIDs: failed to parse iTunes XML: %v", err)
		return 0
	}
	log.Printf("[INFO] backfillITunesTrackPIDs: parsed %d tracks", len(lib.Tracks))

	// Group tracks by album
	type albumGroup struct {
		artist string
		tracks []*itunes.Track
	}
	albums := make(map[string]*albumGroup)
	for _, track := range lib.Tracks {
		album := track.Album
		if album == "" {
			album = track.Name
		}
		if album == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(album))
		if _, ok := albums[key]; !ok {
			artist := track.AlbumArtist
			if artist == "" {
				artist = track.Artist
			}
			albums[key] = &albumGroup{artist: artist}
		}
		albums[key].tracks = append(albums[key].tracks, track)
	}

	// Build PID→book_id index from existing books
	log.Printf("[INFO] backfillITunesTrackPIDs: loading book index...")
	pidToBook := make(map[string]string)
	titleToBook := make(map[string]string) // lowercase title → book_id
	totalBooks := 0
	offset := 0
	for {
		books, err := store.GetAllBooks(10000, offset)
		if err != nil || len(books) == 0 {
			break
		}
		for _, book := range books {
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				pidToBook[*book.ITunesPersistentID] = book.ID
			}
			titleToBook[strings.ToLower(strings.TrimSpace(book.Title))] = book.ID
		}
		totalBooks += len(books)
		offset += 10000
	}
	log.Printf("[INFO] backfillITunesTrackPIDs: loaded %d books (%d PIDs, %d titles)", totalBooks, len(pidToBook), len(titleToBook))

	// For each album, find our book and register all track PIDs
	registered := 0
	var batch []database.ExternalIDMapping

	for _, ag := range albums {
		// Find our book: match by any track PID first, then by title
		var bookID string
		for _, t := range ag.tracks {
			if bid, ok := pidToBook[t.PersistentID]; ok {
				bookID = bid
				break
			}
		}
		if bookID == "" {
			// Try title match
			for _, t := range ag.tracks {
				album := strings.ToLower(strings.TrimSpace(t.Album))
				if bid, ok := titleToBook[album]; ok {
					bookID = bid
					break
				}
			}
		}
		if bookID == "" {
			continue // No matching book found
		}

		// Register all track PIDs for this book
		for _, t := range ag.tracks {
			if t.PersistentID == "" {
				continue
			}
			trackNum := t.TrackNumber
			batch = append(batch, database.ExternalIDMapping{
				Source:      "itunes",
				ExternalID:  t.PersistentID,
				BookID:      bookID,
				TrackNumber: &trackNum,
			})
			registered++
		}

		// Flush in batches of 5000
		if len(batch) >= 5000 {
			_ = eidStore.BulkCreateExternalIDMappings(batch)
			batch = batch[:0]
		}
	}

	// Flush remaining
	if len(batch) > 0 {
		_ = eidStore.BulkCreateExternalIDMappings(batch)
	}

	return registered
}
