// file: internal/itunes/backfill.go
// version: 1.1.0
// guid: b8c9d0e1-f2a3-b4c5-d6e7-f8a9b0c1d2e3

package itunes

import (
	"context"
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ExternalIDBackfillStore defines the store interface needed for external ID backfill.
type ExternalIDBackfillStore interface {
	GetAllBooks(limit, offset int) ([]database.Book, error)
	GetBookFiles(bookID string) ([]database.BookFile, error)
	CreateExternalIDMapping(mapping *database.ExternalIDMapping) error
	BulkCreateExternalIDMappings(mappings []database.ExternalIDMapping) error
	SetSetting(key, value, dataType string, internal bool) error
}

// BackfillExternalIDs scans all books and creates external ID mappings for any
// book that has an iTunes PersistentID set. This is idempotent — it checks the
// setting "external_id_backfill_done" and only runs once.
//
// ctx is honored at every batch boundary AND between book entries so a
// shutdown signal aborts the backfill quickly. Without ctx-awareness this
// loop runs to completion (potentially minutes on a large library) and
// crashes with "pebble: closed" if the store closes mid-iteration. The
// caller (Server.Start's background goroutine) passes s.bgCtx.
func BackfillExternalIDs(ctx context.Context, store ExternalIDBackfillStore) error {
	if store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Check if backfill has already been performed (v4 = includes BookFile-level PIDs)
	// Note: in actual usage, would need to check via store.GetSetting
	log.Printf("[INFO] Starting external ID backfill v4...")

	offset := 0
	backfilled := 0
	for {
		if err := ctx.Err(); err != nil {
			log.Printf("[INFO] external ID backfill canceled at offset %d after %d mappings: %v", offset, backfilled, err)
			return nil
		}
		books, err := store.GetAllBooks(10000, offset)
		if err != nil || len(books) == 0 {
			break
		}
		for _, book := range books {
			if err := ctx.Err(); err != nil {
				log.Printf("[INFO] external ID backfill canceled mid-batch after %d mappings: %v", backfilled, err)
				return nil
			}
			// Book-level PID
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				_ = store.CreateExternalIDMapping(&database.ExternalIDMapping{
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
					_ = store.CreateExternalIDMapping(&database.ExternalIDMapping{
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

	if err := ctx.Err(); err != nil {
		log.Printf("[INFO] external ID backfill canceled before track-PID pass: %v", err)
		return nil
	}
	log.Printf("[INFO] Backfilled %d external ID mappings from book + file records", backfilled)

	// Backfill ALL track-level PIDs from the iTunes XML
	itunesBackfilled, _ := BackfillITunesTrackPIDs(ctx, store)
	if itunesBackfilled > 0 {
		log.Printf("[INFO] Backfilled %d track-level PIDs from iTunes XML", itunesBackfilled)
	}

	// Only mark as done AFTER everything completes successfully (and not canceled)
	if err := ctx.Err(); err == nil {
		_ = store.SetSetting("external_id_backfill_v4_done", "true", "bool", false)
		log.Printf("[INFO] External ID backfill v4 complete")
	}
	return nil
}

// BackfillITunesTrackPIDs reads the iTunes XML and registers ALL track PIDs
// for existing books. This catches the multi-track albums where only the first
// track's PID was stored on the book record.
//
// ctx aborts the inner book-scan + album-build loops; passing
// context.Background is safe.
func BackfillITunesTrackPIDs(ctx context.Context, store ExternalIDBackfillStore) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	xmlPath := config.AppConfig.ITunesLibraryReadPath
	if xmlPath == "" {
		log.Printf("[INFO] BackfillITunesTrackPIDs: no iTunes XML path configured, skipping")
		return 0, nil
	}

	log.Printf("[INFO] BackfillITunesTrackPIDs: parsing iTunes XML at %s", xmlPath)
	lib, err := ParseLibrary(xmlPath)
	if err != nil {
		log.Printf("[WARN] BackfillITunesTrackPIDs: failed to parse iTunes XML: %v", err)
		return 0, err
	}
	log.Printf("[INFO] BackfillITunesTrackPIDs: parsed %d tracks", len(lib.Tracks))

	// Group tracks by album
	type albumGroup struct {
		artist string
		tracks []*Track
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
	log.Printf("[INFO] BackfillITunesTrackPIDs: loading book index...")
	pidToBook := make(map[string]string)
	titleToBook := make(map[string]string) // lowercase title → book_id
	totalBooks := 0
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			log.Printf("[INFO] BackfillITunesTrackPIDs canceled mid-index at offset %d", offset)
			return 0, nil
		}
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
	log.Printf("[INFO] BackfillITunesTrackPIDs: loaded %d books (%d PIDs, %d titles)", totalBooks, len(pidToBook), len(titleToBook))

	// For each album, find our book and register all track PIDs
	registered := 0
	var batch []database.ExternalIDMapping

	for _, ag := range albums {
		if err := ctx.Err(); err != nil {
			log.Printf("[INFO] BackfillITunesTrackPIDs canceled mid-album pass after %d PIDs", registered)
			return registered, nil
		}
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
			_ = store.BulkCreateExternalIDMappings(batch)
			batch = batch[:0]
		}
	}

	// Flush remaining
	if len(batch) > 0 {
		_ = store.BulkCreateExternalIDMappings(batch)
	}

	return registered, nil
}
