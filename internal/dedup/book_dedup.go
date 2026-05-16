// file: internal/dedup/book_dedup.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-123456789012

// Package dedup: book_dedup.go contains the extracted execution logic for the
// "dedup.book-scan" and "dedup.book-merge" async operations.  The *Server
// wrappers in internal/server/duplicates_ops.go are now thin callers that hand
// results back to server-owned state (dedupCache, etc.).
package dedup

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// BookDupGroup is a group of books that are likely duplicates of each other.
type BookDupGroup struct {
	Books      []database.Book `json:"books"`
	Confidence string          `json:"confidence"` // "high", "medium", "low"
	Reason     string          `json:"reason"`
	GroupKey   string          `json:"group_key"`
}

// BookScanResult is the result of ScanBookDuplicates.
type BookScanResult struct {
	Groups          []BookDupGroup
	TotalDuplicates int
}

// ScanBookDuplicates runs the three-tier duplicate-book scan (hash, folder,
// metadata fuzzy) against store, filters out keys present in dismissed, and
// returns a consolidated list of BookDupGroup values along with the total
// number of duplicate books (i.e. sum of len(group.Books)-1 across all
// groups).
//
// dismissed maps stable group keys (sorted book IDs joined by "+") to true;
// any group whose key is in the map is silently dropped.
//
// progress may be nil; all calls are guarded.
func ScanBookDuplicates(
	_ context.Context,
	store database.Store,
	dismissed map[string]bool,
	progress ProgressReporter,
) (BookScanResult, error) {
	report := func(pct int, msg string) {
		if progress != nil {
			_ = progress.UpdateProgress(pct, 100, msg)
		}
	}

	report(0, "Scanning for duplicate books...")

	// Step 1: Hash-based duplicates (high confidence)
	report(10, "Finding hash-based duplicates...")
	hashGroups, err := store.GetDuplicateBooks()
	if err != nil {
		return BookScanResult{}, fmt.Errorf("hash-based dedup failed: %w", err)
	}

	// Step 2: Folder duplicates (same title in same folder)
	report(30, "Finding folder-based duplicates...")
	folderGroups, err := store.GetFolderDuplicates()
	if err != nil {
		log.Printf("[WARN] folder dedup failed: %v", err)
		folderGroups = nil
	}

	// Step 3: Metadata-based fuzzy matching
	report(50, "Finding metadata-based duplicates...")
	metadataGroups, err := store.GetDuplicateBooksByMetadata(0.85)
	if err != nil {
		log.Printf("[WARN] metadata dedup failed: %v", err)
		metadataGroups = nil
	}

	report(80, "Merging results...")

	// Combine all groups, deduplicating by book ID.
	seenBookIDs := map[string]bool{}
	var allGroups []BookDupGroup

	addGroups := func(groups [][]database.Book, confidence, reason string) {
		for _, group := range groups {
			// Skip if every book in this group has already been claimed.
			allSeen := true
			for _, b := range group {
				if !seenBookIDs[b.ID] {
					allSeen = false
					break
				}
			}
			if allSeen {
				continue
			}
			// Generate a stable group key from sorted book IDs.
			ids := make([]string, len(group))
			for i, b := range group {
				ids[i] = b.ID
			}
			groupKey := strings.Join(ids, "+")
			if dismissed[groupKey] {
				continue
			}
			allGroups = append(allGroups, BookDupGroup{
				Books:      group,
				Confidence: confidence,
				Reason:     reason,
				GroupKey:   groupKey,
			})
			for _, b := range group {
				seenBookIDs[b.ID] = true
			}
		}
	}

	addGroups(hashGroups, "high", "Identical file hash")
	addGroups(folderGroups, "medium", "Same title in same folder")
	addGroups(metadataGroups, "low", "Similar title and author")

	totalDuplicates := 0
	for _, g := range allGroups {
		totalDuplicates += len(g.Books) - 1
	}

	report(100, fmt.Sprintf("Found %d duplicate groups (%d duplicates)", len(allGroups), totalDuplicates))

	return BookScanResult{
		Groups:          allGroups,
		TotalDuplicates: totalDuplicates,
	}, nil
}

// BookMergeResult summarises the outcome of MergeBooks.
type BookMergeResult struct {
	// UpdatedKeepBook is the keep book after iTunes metadata has been transferred
	// in and the UpdateBook call has been issued.  Callers may use it to
	// invalidate caches or issue further side-effects.
	UpdatedKeepBook *database.Book
	MergedCount     int
	Errors          []string
}

// MergeBooks transfers useful metadata from each merge book to the keep book,
// deletes the merge books, and records OperationChange rows.  It does NOT
// invalidate any server-side cache — the caller is responsible for that.
//
// opID is the legacy operation ID written into OperationChange records.
// keepID is the ID of the book to keep; every ID in mergeIDs is deleted.
func MergeBooks(
	_ context.Context,
	store database.Store,
	opID, keepID string,
	mergeIDs []string,
	progress ProgressReporter,
) (BookMergeResult, error) {
	keepBook, err := store.GetBookByID(keepID)
	if err != nil || keepBook == nil {
		return BookMergeResult{}, fmt.Errorf("keep book %s not found", keepID)
	}

	if progress != nil {
		_ = progress.Log("info",
			fmt.Sprintf("Merging %d book(s) into %q", len(mergeIDs), keepBook.Title), nil)
		_ = progress.UpdateProgress(0, len(mergeIDs), "Starting book merge...")
	}

	kBook, err := store.GetBookByID(keepID)
	if err != nil || kBook == nil {
		return BookMergeResult{}, fmt.Errorf("keep book %s not found", keepID)
	}

	var result BookMergeResult
	for i, mergeID := range mergeIDs {
		if progress != nil && progress.IsCanceled() {
			return result, fmt.Errorf("cancelled")
		}
		if mergeID == keepID {
			continue
		}
		mergeBook, err := store.GetBookByID(mergeID)
		if err != nil || mergeBook == nil {
			result.Errors = append(result.Errors, fmt.Sprintf("book %s not found", mergeID))
			continue
		}

		// Transfer useful iTunes metadata from merge book to keep book (first-win).
		if (kBook.ITunesPersistentID == nil || *kBook.ITunesPersistentID == "") &&
			mergeBook.ITunesPersistentID != nil && *mergeBook.ITunesPersistentID != "" {
			kBook.ITunesPersistentID = mergeBook.ITunesPersistentID
		}
		if kBook.ITunesPlayCount == nil && mergeBook.ITunesPlayCount != nil {
			kBook.ITunesPlayCount = mergeBook.ITunesPlayCount
		}
		if kBook.ITunesRating == nil && mergeBook.ITunesRating != nil {
			kBook.ITunesRating = mergeBook.ITunesRating
		}
		if kBook.ITunesDateAdded == nil && mergeBook.ITunesDateAdded != nil {
			kBook.ITunesDateAdded = mergeBook.ITunesDateAdded
		}
		if kBook.ITunesLastPlayed == nil && mergeBook.ITunesLastPlayed != nil {
			kBook.ITunesLastPlayed = mergeBook.ITunesLastPlayed
		}
		if kBook.ITunesBookmark == nil && mergeBook.ITunesBookmark != nil {
			kBook.ITunesBookmark = mergeBook.ITunesBookmark
		}

		if err := store.DeleteBook(mergeID); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("failed to delete book %s: %v", mergeID, err))
		} else {
			_ = store.CreateOperationChange(&database.OperationChange{
				ID:          ulid.Make().String(),
				OperationID: opID,
				BookID:      mergeID,
				ChangeType:  "book_delete",
				FieldName:   "book",
				OldValue:    fmt.Sprintf("%s (%s)", mergeBook.Title, mergeBook.FilePath),
				NewValue:    fmt.Sprintf("merged_into:%s", keepID),
			})
			result.MergedCount++
		}

		if progress != nil {
			_ = progress.UpdateProgress(i+1, len(mergeIDs),
				fmt.Sprintf("Merged %d/%d books", i+1, len(mergeIDs)))
		}
	}

	if _, err := store.UpdateBook(kBook.ID, kBook); err != nil {
		result.Errors = append(result.Errors,
			fmt.Sprintf("failed to update keep book: %v", err))
	}

	if progress != nil {
		msg := fmt.Sprintf("Book merge complete: merged %d, %d errors",
			result.MergedCount, len(result.Errors))
		_ = progress.Log("info", msg, nil)
	}

	result.UpdatedKeepBook = kBook
	return result, nil
}
