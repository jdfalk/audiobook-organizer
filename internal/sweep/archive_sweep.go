// file: internal/sweep/archive_sweep.go
// version: 1.0.0
// guid: a9f8e7d6-c5b4-3a21-9087-654321fedcba
//
// Archive sweep for soft-deleted books (backlog 7.10).
//
// Books marked_for_deletion with a deletion date older than the
// retention window are physically cleaned up: files removed from
// disk, book_files rows deleted, and the book row hard-deleted.
// Runs as a maintenance task.

package sweep

import (
	"log"
	"os"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

const archiveRetentionDays = 30

// SweepArchivedBooks removes soft-deleted books past the retention
// window. Returns the count of books cleaned up.
func SweepArchivedBooks(store interface{ database.BookStore; database.BookFileStore }) int {
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
				if err := os.Remove(f.FilePath); err != nil && !os.IsNotExist(err) {
					log.Printf("[WARN] archive sweep: remove %s: %v", f.FilePath, err)
				}
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
