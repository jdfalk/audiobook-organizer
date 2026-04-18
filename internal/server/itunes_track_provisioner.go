// file: internal/server/itunes_track_provisioner.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-0123456789ab
//
// Provisions ITL tracks for non-iTunes books. Generates a random persistent ID,
// stores it in external_id_map with provenance='generated', updates the BookFile,
// and enqueues an ITL add operation.

package server

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// ProvisionITLTrack generates a PID for a book file, stores it in the DB,
// and enqueues an ITL add operation. Called after importing a non-iTunes book.
//
// Skips if the book file already has an iTunes PID or if auto write-back is disabled.
func ProvisionITLTrack(store interface { database.AuthorReader; database.BookFileStore; database.ExternalIDStore }, book *database.Book, bookFile *database.BookFile, batcher *WriteBackBatcher) error {
	if !config.AppConfig.ITunesAutoWriteBack {
		return nil
	}
	if bookFile.ITunesPersistentID != "" {
		return nil // already has a PID
	}

	// Generate a new PID
	pid := itunes.GeneratePIDHex()

	// Build the Windows-mapped path for iTunes
	itunesPath := linuxToWindowsPath(bookFile.FilePath)

	// Store PID in external_id_map with provenance='generated'
	trackNum := bookFile.TrackNumber
	mapping := &database.ExternalIDMapping{
		Source:     "itunes",
		ExternalID: pid,
		BookID:     book.ID,
		FilePath:   bookFile.FilePath,
		Provenance: "generated",
	}
	if trackNum > 0 {
		mapping.TrackNumber = &trackNum
	}
	if err := store.CreateExternalIDMapping(mapping); err != nil {
		return fmt.Errorf("storing PID mapping: %w", err)
	}

	// Set provenance (CreateExternalIDMapping doesn't set it)
	_ = store.SetExternalIDProvenance("itunes", pid, "generated")

	// Update BookFile with the new PID
	bookFile.ITunesPersistentID = pid
	bookFile.ITunesPath = itunesPath
	if err := store.UpsertBookFile(bookFile); err != nil {
		return fmt.Errorf("updating book file with PID: %w", err)
	}

	// Determine format-based "Kind" string
	kind := kindFromExt(filepath.Ext(bookFile.FilePath))

	// Enqueue the ITL add
	if batcher != nil {
		batcher.EnqueueAdd(itunes.ITLNewTrack{
			Location:    itunesPath,
			Name:        bookFile.Title,
			Album:       book.Title,
			Artist:      bookAuthor(store, book),
			Genre:       "Audiobook",
			Kind:        kind,
			Size:        int(bookFile.FileSize),
			TotalTime:   bookFile.Duration * 1000, // seconds to ms
			TrackNumber: bookFile.TrackNumber,
			BitRate:     bookFile.BitrateKbps,
			SampleRate:  bookFile.SampleRateHz,
			DiscNumber:  bookFile.DiscNumber,
		})
	}

	log.Printf("[INFO] Provisioned ITL track: book=%s pid=%s path=%s", book.ID, pid, itunesPath)
	return nil
}

// ProvisionITLTracksForBook provisions ITL tracks for all files of a book.
func ProvisionITLTracksForBook(store interface { database.AuthorReader; database.BookFileStore; database.ExternalIDStore }, book *database.Book, batcher *WriteBackBatcher) error {
	files, err := store.GetBookFiles(book.ID)
	if err != nil {
		return err
	}
	for i := range files {
		if err := ProvisionITLTrack(store, book, &files[i], batcher); err != nil {
			log.Printf("[WARN] Failed to provision ITL track for file %s: %v", files[i].ID, err)
		}
	}
	return nil
}

// linuxToWindowsPath maps a Linux path to the Windows SMB path.
func linuxToWindowsPath(p string) string {
	const linuxRoot = "/mnt/bigdata/books/audiobook-organizer/"
	const windowsRoot = `W:\audiobook-organizer\`
	if strings.HasPrefix(p, linuxRoot) {
		return windowsRoot + strings.ReplaceAll(
			strings.TrimPrefix(p, linuxRoot), "/", `\`,
		)
	}
	return p
}

// bookAuthor returns the author name for a book.
func bookAuthor(store database.AuthorReader, book *database.Book) string {
	if book.AuthorID == nil {
		return ""
	}
	author, err := store.GetAuthorByID(*book.AuthorID)
	if err != nil || author == nil {
		return ""
	}
	return author.Name
}

// kindFromExt returns the iTunes "Kind" string for a file extension.
func kindFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".m4b", ".m4a", ".aac":
		return "AAC audio file"
	case ".mp3":
		return "MPEG audio file"
	case ".ogg":
		return "Ogg Vorbis file"
	case ".flac":
		return "FLAC audio file"
	case ".wav":
		return "WAV audio file"
	default:
		return "AAC audio file"
	}
}
