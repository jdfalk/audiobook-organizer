// file: internal/database/iface_book.go
// version: 1.6.0
// guid: 668ec5a2-f8d9-4fdb-b0d5-09937b5d83ea
// last-edited: 2026-04-30

package database

import "time"

// UpdateBookRatingRequest carries partial-update fields for user ratings.
// Each field uses a pointer so the caller can distinguish "omitted" (nil)
// from "set to zero/empty" (non-nil pointing to zero value).
// To clear a rating to NULL, set ClearOverall = true (etc.).
type UpdateBookRatingRequest struct {
	Overall      *float64
	ClearOverall bool
	Story        *float64
	ClearStory   bool
	Performance  *float64
	ClearPerf    bool
	Notes        *string
	ClearNotes   bool
}

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
	GetBooksByMetadataSourceHash(hash string) ([]Book, error)
	SearchBooks(query string, limit, offset int) ([]Book, error)
	CountBooks() (int, error)
	GetDistinctGenres() ([]string, error)
	GetDistinctLanguages() ([]string, error)
	ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error)
	GetBookSnapshots(id string, limit int) ([]BookSnapshot, error)
	GetBookAtVersion(id string, ts time.Time) (*Book, error)
	GetBookTombstone(id string) (*Book, error)
	ListBookTombstones(limit int) ([]Book, error)
	GetITunesDirtyBooks() ([]Book, error)
	GetITunesPurgePendingBooks() ([]Book, error)
	GetQuarantinedBooks(limit, offset int) ([]Book, error)
	CountQuarantinedBooks() (int, error)
}

// BookWriter is the write-only slice of Store for callers that only
// mutate books.
type BookWriter interface {
	CreateBook(book *Book) (*Book, error)
	UpdateBook(id string, book *Book) (*Book, error)
	UpdateBookRating(id string, req UpdateBookRatingRequest) error
	DeleteBook(id string) error
	SetLastWrittenAt(id string, t time.Time) error
	MarkITunesSynced(bookIDs []string) (int64, error)
	RevertBookToVersion(id string, ts time.Time) (*Book, error)
	PruneBookSnapshots(id string, keepCount int) (int, error)
	CreateBookTombstone(book *Book) error
	DeleteBookTombstone(id string) error
	// Scan-fail counter for auto-quarantine (keyed on sha256[:8] of path).
	GetScanFailCount(pathHash string) (int, error)
	IncrScanFailCount(pathHash string) (int, error)
	ResetScanFailCount(pathHash string) error
}

// BookStore combines BookReader and BookWriter for callers that need both.
type BookStore interface {
	BookReader
	BookWriter
}
