// file: internal/server/archive_sweep.go
// version: 1.1.0
// guid: 2f0a1b9c-3d4e-4a70-b8c5-3d7e0f1b9a99
//
// Archive sweep for soft-deleted books (backlog 7.10).
//
// Books marked_for_deletion with a deletion date older than the
// retention window are physically cleaned up: files removed from
// disk, book_files rows deleted, and the book row hard-deleted.
// Runs as a maintenance task.

package server

import (
	"log"
	"os"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const archiveRetentionDays = 30

// SweepArchivedBooks removes soft-deleted books past the retention
// window. Returns the count of books cleaned up.
func SweepArchivedBooks(store interface { database.BookStore; database.BookFileStore }) int {
	books, err := store.GetAllBooks(0, 0)
	if err != nil {
		log.Printf("[WARN] archive sweep: list books: %v", err)
		return 0
	}

	cutoff := time.Now().Add(-time.Duration(archiveRetentionDays) * 24 * time.Hour)
	cleaned := 0

	for _, book := range books {
		if book.MarkedForDeletion == nil || !*book.MarkedForDeletion {
			continue
		}
		if book.MarkedForDeletionAt == nil || book.MarkedForDeletionAt.After(cutoff) {
			continue
		}

		// Remove files from disk.
		files, _ := store.GetBookFiles(book.ID)
		for _, f := range files {
			if f.FilePath != "" {
				_ = os.Remove(f.FilePath)
			}
		}

		// Hard-delete the book record.
		if err := store.DeleteBook(book.ID); err != nil {
			log.Printf("[WARN] archive sweep: delete %s: %v", book.ID, err)
			continue
		}
		cleaned++
	}

	return cleaned
}
