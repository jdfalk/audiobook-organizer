// file: internal/itunes/service/path_repair_resolver.go
// version: 1.0.0
// guid: 7d4f25a1-8e29-4b8b-9a02-3c5e1f9d4b27
//
// Pure-function resolvers for the path-repair operation. Each tier
// takes a narrow store interface and an existsFn so tests can drive
// them without a real filesystem.

package itunesservice

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// tierAStore is the slice tier A needs: PID → bookID via the
// external_id_map, then the book + its files.
type tierAStore interface {
	GetBookByExternalID(source, externalID string) (string, error)
	GetBookByID(id string) (*database.Book, error)
	GetBookFiles(bookID string) ([]database.BookFile, error)
}

// resolveTierA looks up the iTunes PID via external_id_map and returns
// the on-disk path the DB thinks the file is at — preferring the
// matching BookFile over Book.FilePath. Returns ok=false when no
// mapping exists or the DB-known path doesn't exist on disk.
//
// existsFn is injected so unit tests can fake the filesystem.
func resolveTierA(s tierAStore, pid string, existsFn func(string) bool) (string, bool) {
	bookID, err := s.GetBookByExternalID("itunes", pid)
	if err != nil || bookID == "" {
		return "", false
	}
	if files, err := s.GetBookFiles(bookID); err == nil {
		for _, bf := range files {
			if bf.ITunesPersistentID == pid && bf.FilePath != "" && existsFn(bf.FilePath) {
				return bf.FilePath, true
			}
		}
	}
	if book, err := s.GetBookByID(bookID); err == nil && book != nil && book.FilePath != "" && existsFn(book.FilePath) {
		return book.FilePath, true
	}
	return "", false
}
