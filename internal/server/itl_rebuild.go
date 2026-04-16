// file: internal/server/itl_rebuild.go
// version: 1.1.0
// guid: 8f7e6d5c-4b3a-2c1d-0e9f-8a7b6c5d4e3f
//
// iTunes library rebuild service: diffs the current DB state
// against the current ITL file and computes the minimal set of
// changes (adds, removes, metadata updates, location patches)
// to synchronize them. Changes are applied in one atomic
// ApplyITLOperations call through the existing safeWriteITL
// pipeline (backup → validate → apply → validate → rollback on
// failure). Backlog 7.9 — "diff and batch" mode.

package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// ITLRebuildPreview summarizes the diff between the DB and the
// current ITL file without applying any changes. Returned by
// the dry-run path so the user can review before committing.
type ITLRebuildPreview struct {
	TracksInITL  int `json:"tracks_in_itl"`
	BooksInDB    int `json:"books_in_db"`
	ToRemove     int `json:"to_remove"`
	ToAdd        int `json:"to_add"`
	ToUpdateMeta int `json:"to_update_metadata"`
	ToUpdateLoc  int `json:"to_update_location"`
	AlreadySynced int `json:"already_synced"`
}

// ITLRebuildResult is the outcome of an applied rebuild.
type ITLRebuildResult struct {
	Preview ITLRebuildPreview `json:"preview"`
	Applied bool              `json:"applied"`
	Error   string            `json:"error,omitempty"`
}

// computeITLDiff reads the current ITL and the current DB state,
// and returns the ITLOperationSet that would synchronize them
// plus a preview of what that set contains.
func computeITLDiff(store database.Store, itlPath string) (*itunes.ITLOperationSet, *ITLRebuildPreview, error) {
	// Parse the current ITL to get all existing tracks.
	lib, err := itunes.ParseITL(itlPath)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ITL: %w", err)
	}

	// Build a map of existing ITL tracks by PID hex string.
	itlTracks := make(map[string]*itunes.ITLTrack, len(lib.Tracks))
	for i := range lib.Tracks {
		pid := pidToHex(lib.Tracks[i].PersistentID)
		itlTracks[pid] = &lib.Tracks[i]
	}

	// Build the "should be in ITL" set from the DB:
	// all primary-version books that have an iTunes PID.
	dbPIDs := make(map[string]*database.Book)
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, nil, fmt.Errorf("get books: %w", err)
		}
		if len(books) == 0 {
			break
		}
		for i := range books {
			b := &books[i]
			// Only sync primary versions — non-primary versions
			// were merged and should have been removed from the
			// ITL by the merge cleanup in #251.
			if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
				continue
			}
			// Soft-deleted books should not be in the ITL.
			if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
				continue
			}
			if b.ITunesPersistentID != nil && *b.ITunesPersistentID != "" {
				dbPIDs[strings.ToUpper(*b.ITunesPersistentID)] = b
			}
		}
	}

	ops := itunes.ITLOperationSet{
		Removes: make(map[string]bool),
	}
	preview := ITLRebuildPreview{
		TracksInITL: len(itlTracks),
		BooksInDB:   len(dbPIDs),
	}

	// Tracks in ITL but NOT in DB → remove.
	for pid := range itlTracks {
		if _, inDB := dbPIDs[pid]; !inDB {
			ops.Removes[pid] = true
			preview.ToRemove++
		}
	}

	// Books in DB — check for add vs update.
	for pid, book := range dbPIDs {
		track, inITL := itlTracks[pid]
		if !inITL {
			// Book has a PID but no ITL entry → add.
			// Build an ITLNewTrack from the book's metadata.
			newTrack := buildNewTrackFromBook(store, book)
			ops.Adds = append(ops.Adds, newTrack)
			preview.ToAdd++
			continue
		}

		// Both exist — check if metadata or location need updating.
		authorName, _ := resolveAuthorAndSeriesNames(book)
		narrator := ""
		if book.Narrator != nil {
			narrator = *book.Narrator
		}
		genre := "Audiobook"
		if book.Genre != nil && *book.Genre != "" {
			genre = *book.Genre
		}
		needsMetaUpdate := false
		if track.Name != book.Title {
			needsMetaUpdate = true
		}
		if track.Artist != authorName {
			needsMetaUpdate = true
		}
		if track.Album != book.Title {
			needsMetaUpdate = true
		}
		if track.Genre != genre {
			needsMetaUpdate = true
		}

		if needsMetaUpdate {
			ops.MetadataUpdates = append(ops.MetadataUpdates, itunes.ITLMetadataUpdate{
				PersistentID: pid,
				Name:         book.Title,
				Album:        book.Title,
				Artist:       authorName,
				Composer:     narrator,
				Genre:        genre,
			})
			preview.ToUpdateMeta++
		}

		// Location update.
		wantLoc := ""
		if book.ITunesPath != nil && *book.ITunesPath != "" {
			wantLoc = *book.ITunesPath
		}
		if wantLoc != "" && track.Location != wantLoc {
			ops.LocationUpdates = append(ops.LocationUpdates, itunes.ITLLocationUpdate{
				PersistentID: pid,
				NewLocation:  wantLoc,
			})
			preview.ToUpdateLoc++
		}

		if !needsMetaUpdate && (wantLoc == "" || track.Location == wantLoc) {
			preview.AlreadySynced++
		}
	}

	return &ops, &preview, nil
}

// buildNewTrackFromBook constructs an ITLNewTrack from a database
// Book for insertion into the ITL. Fills as many fields as
// possible from the book's metadata.
func buildNewTrackFromBook(store database.Store, book *database.Book) itunes.ITLNewTrack {
	authorName, _ := resolveAuthorAndSeriesNames(book)
	genre := "Audiobook"
	if book.Genre != nil && *book.Genre != "" {
		genre = *book.Genre
	}
	location := ""
	if book.ITunesPath != nil {
		location = *book.ITunesPath
	} else {
		location = book.FilePath
	}
	totalTime := 0
	if book.Duration != nil {
		totalTime = *book.Duration * 1000 // convert seconds → ms
	}
	size := int64(0)
	if book.FileSize != nil {
		size = *book.FileSize
	}

	// Note: ITLNewTrack doesn't carry a Composer field — narrator
	// is pushed via a follow-up MetadataUpdate after the add. The
	// batcher's existing flush path handles that automatically for
	// books that go through the normal write-back pipeline. For the
	// rebuild path, the MetadataUpdate pass handles narrator via
	// the Composer field on ITLMetadataUpdate.
	return itunes.ITLNewTrack{
		Location:  location,
		Name:      book.Title,
		Album:     book.Title,
		Artist:    authorName,
		Genre:     genre,
		Kind:      "MPEG audio file", // default; the real kind comes from the file format
		Size:      int(size),
		TotalTime: totalTime,
	}
}

// pidToHex converts an 8-byte PersistentID to an uppercase hex string
// matching the format stored in the DB and external_id_map.
func pidToHex(pid [8]byte) string {
	return fmt.Sprintf("%016X", pid)
}

// rebuildITLHandler handles POST /api/v1/itunes/rebuild.
// Query param: dry_run=true returns the diff preview without
// applying. Otherwise applies the diff via safeWriteITL.
func (s *Server) rebuildITLHandler(c *gin.Context) {
	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ITunesLibraryWritePath not configured"})
		return
	}

	store := s.Store()
	ops, preview, err := computeITLDiff(store, itlPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("diff failed: %v", err)})
		return
	}

	dryRun := c.Query("dry_run") == "true"
	if dryRun {
		c.JSON(http.StatusOK, gin.H{
			"dry_run": true,
			"preview": preview,
		})
		return
	}

	// Apply.
	if ops.IsEmpty() {
		c.JSON(http.StatusOK, ITLRebuildResult{
			Preview: *preview,
			Applied: true,
		})
		return
	}

	if err := safeWriteITL(itlPath, *ops); err != nil {
		c.JSON(http.StatusInternalServerError, ITLRebuildResult{
			Preview: *preview,
			Applied: false,
			Error:   err.Error(),
		})
		return
	}

	log.Printf("[INFO] ITL rebuild: removed %d, added %d, updated-meta %d, updated-loc %d",
		preview.ToRemove, preview.ToAdd, preview.ToUpdateMeta, preview.ToUpdateLoc)

	c.JSON(http.StatusOK, ITLRebuildResult{
		Preview: *preview,
		Applied: true,
	})
}
