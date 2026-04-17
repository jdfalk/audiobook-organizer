// file: internal/database/iface_book.go
// version: 1.0.0
// guid: 668ec5a2-f8d9-4fdb-b0d5-09937b5d83ea

package database

import "time"

// BookReader is the read-only slice of Store for callers that only
// read books. See spec 2026-04-17-store-interface-segregation-design.md.
type BookReader interface {
	GetBookByID(id string) (*Book, error)
	GetAllBooks(limit, offset int) ([]Book, error)
	GetBookByFilePath(path string) (*Book, error)
	GetBookByITunesPersistentID(persistentID string) (*Book, error)
	GetBookByFileHash(hash string) (*Book, error)
	GetBookByOriginalHash(hash string) (*Book, error)
	GetBookByOrganizedHash(hash string) (*Book, error)
	GetDuplicateBooks() ([][]Book, error)
	GetFolderDuplicates() ([][]Book, error)
	GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error)
	GetBooksByTitleInDir(normalizedTitle, dirPath string) ([]Book, error)
	GetBooksBySeriesID(seriesID int) ([]Book, error)
	GetBooksByAuthorID(authorID int) ([]Book, error)
	GetBooksByVersionGroup(groupID string) ([]Book, error)
	SearchBooks(query string, limit, offset int) ([]Book, error)
	CountBooks() (int, error)
	ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error)
	GetBookSnapshots(id string, limit int) ([]BookSnapshot, error)
	GetBookAtVersion(id string, ts time.Time) (*Book, error)
	GetBookTombstone(id string) (*Book, error)
	ListBookTombstones(limit int) ([]Book, error)
	GetITunesDirtyBooks() ([]Book, error)
}

// BookWriter is the write-only slice of Store for callers that only
// mutate books.
type BookWriter interface {
	CreateBook(book *Book) (*Book, error)
	UpdateBook(id string, book *Book) (*Book, error)
	DeleteBook(id string) error
	SetLastWrittenAt(id string, t time.Time) error
	MarkITunesSynced(bookIDs []string) (int64, error)
	RevertBookToVersion(id string, ts time.Time) (*Book, error)
	PruneBookSnapshots(id string, keepCount int) (int, error)
	CreateBookTombstone(book *Book) error
	DeleteBookTombstone(id string) error
}

// BookStore combines BookReader and BookWriter for callers that need both.
type BookStore interface {
	BookReader
	BookWriter
}
