// file: internal/merge/service.go
// version: 1.3.0
// guid: 7d736d2d-e0df-40bd-9f4b-0a07bc2eb6ae

package merge

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// WriteBackEnqueuer is satisfied by anything that can enqueue an iTunes
// track removal (e.g. *server.WriteBackBatcher).
type WriteBackEnqueuer interface {
	EnqueueRemove(pid string)
}

// ExternalIDReassigner is the subset of external-ID operations that
// Service needs. Satisfied by the concrete store when it implements
// ReassignExternalIDs.
type ExternalIDReassigner interface {
	ReassignExternalIDs(oldBookID, newBookID string) error
}

// AsExternalIDReassigner returns the ExternalIDReassigner if the given
// store implements it, or nil otherwise.
func AsExternalIDReassigner(s any) ExternalIDReassigner {
	if s == nil {
		return nil
	}
	if eid, ok := s.(ExternalIDReassigner); ok {
		return eid
	}
	return nil
}

// Service handles merging duplicate books into version groups.
type Service struct {
	db               database.Store
	writeBackBatcher WriteBackEnqueuer
}

// SetWriteBackBatcher sets the iTunes write-back batcher.
func (ms *Service) SetWriteBackBatcher(b WriteBackEnqueuer) {
	ms.writeBackBatcher = b
}

// Result contains the outcome of a merge operation.
type Result struct {
	PrimaryID      string `json:"primary_id"`
	VersionGroupID string `json:"version_group_id"`
	MergedCount    int    `json:"merged_count"`
}

// NewService creates a new Service.
func NewService(db database.Store) *Service {
	return &Service{db: db}
}

// MergeBooks merges a set of books into a single version group.
//
// Semantics (confirmed 2026-04-11 after an investigation into
// orphaned ITL entries):
//
//  1. Winner is chosen (user-supplied primaryID or auto-picked
//     via BookIsBetter) and given IsPrimaryVersion=true. Losers
//     get IsPrimaryVersion=false and are soft-deleted.
//  2. External IDs (iTunes PIDs, Audible ASINs, etc.) are
//     reassigned from losers to the winner so lookups still
//     resolve to the surviving entity.
//  3. **iTunes ITL cleanup**: before reassignment, we collect
//     each loser's iTunes PIDs and enqueue them for removal via
//     writeBackBatcher.EnqueueRemove. This matches the
//     behavior of maintenance_fixups.mergeDuplicateBook — the
//     UI merge path used to skip this step, which left the
//     losers' tracks alive in the iTunes library forever.
//  4. Loser DB rows are soft-deleted (MarkedForDeletion=true).
//     They stay recoverable via the existing soft-delete
//     restore flow for at least the retention window.
//  5. Loser files on disk are NOT touched — they remain
//     playable until an archive sweep (not yet implemented)
//     cleans them up.
//
// If primaryID is empty, the best book is auto-selected (M4B
// preferred, then highest bitrate, then largest file).
// If primaryID is provided, that book is set as the primary.
func (ms *Service) MergeBooks(bookIDs []string, primaryID string) (*Result, error) {
	if len(bookIDs) < 2 {
		return nil, fmt.Errorf("need at least 2 book IDs to merge")
	}

	// Fetch all books
	var books []*database.Book
	for _, id := range bookIDs {
		book, err := ms.db.GetBookByID(id)
		if err != nil || book == nil {
			return nil, fmt.Errorf("book %s not found", id)
		}
		books = append(books, book)
	}

	// Determine primary index
	bestIdx := 0
	if primaryID != "" {
		found := false
		for i, b := range books {
			if b.ID == primaryID {
				bestIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("primary_id %s not in book_ids", primaryID)
		}
	} else {
		// Auto-select best: M4B preferred, then highest bitrate, then largest file
		for i := 1; i < len(books); i++ {
			if BookIsBetter(books[i], books[bestIdx]) {
				bestIdx = i
			}
		}
	}

	// Determine version group ID (reuse if any book already has one)
	versionGroupID := ""
	for _, b := range books {
		if b.VersionGroupID != nil && *b.VersionGroupID != "" {
			versionGroupID = *b.VersionGroupID
			break
		}
	}
	if versionGroupID == "" {
		versionGroupID = ulid.Make().String()
	}

	// Update all books to share the version group. Winner is
	// marked primary; losers are marked non-primary. We still
	// persist the flag on losers here so the version group is
	// queryable and the relationship survives through the
	// soft-delete call below.
	resolvedPrimaryID := books[bestIdx].ID
	for i, book := range books {
		book.VersionGroupID = &versionGroupID
		isPrimary := i == bestIdx
		book.IsPrimaryVersion = &isPrimary
		if _, err := ms.db.UpdateBook(book.ID, book); err != nil {
			return nil, fmt.Errorf("failed to update book %s: %w", book.ID, err)
		}
	}

	// --- Per-loser cleanup ---
	//
	// For each non-primary book we:
	//  (a) collect its iTunes PIDs BEFORE reassignment so we
	//      know which tracks to remove from the ITL,
	//  (b) reassign all external IDs to the winner so future
	//      lookups resolve,
	//  (c) enqueue ITL removals for the collected PIDs so
	//      iTunes no longer shows duplicate tracks for this
	//      version group,
	//  (d) soft-delete the loser so it drops off the default
	//      library view. Files on disk are left alone for the
	//      archive sweep to handle later.
	eidStore := AsExternalIDReassigner(ms.db)
	for _, book := range books {
		if book.ID == resolvedPrimaryID {
			continue
		}

		// (a) Collect PIDs before reassignment.
		var dupPIDs []string
		if mappings, err := ms.db.GetExternalIDsForBook(book.ID); err == nil {
			for _, m := range mappings {
				if m.Source == "itunes" && m.ExternalID != "" && !m.Tombstoned {
					dupPIDs = append(dupPIDs, m.ExternalID)
				}
			}
		}

		// (b) Reassign external IDs to the winner.
		if eidStore != nil {
			if err := eidStore.ReassignExternalIDs(book.ID, resolvedPrimaryID); err != nil {
				log.Printf("[WARN] merge: ReassignExternalIDs %s → %s: %v", book.ID, resolvedPrimaryID, err)
			}
		}

		// (c) Queue iTunes removals for the loser's tracks so
		// the ITL stops showing them. Best-effort — a nil
		// batcher (e.g. tests, or iTunes write-back disabled)
		// means we just skip.
		if ms.writeBackBatcher != nil && len(dupPIDs) > 0 {
			for _, pid := range dupPIDs {
				ms.writeBackBatcher.EnqueueRemove(pid)
			}
			log.Printf("[INFO] merge: queued %d ITL removals for loser %s", len(dupPIDs), book.ID)
		}

		// (d) Soft-delete the loser. If UpdateBook fails inside
		// SoftDeleteBook it falls back to hard delete, so we
		// never leave a zombie non-primary row behind.
		if err := SoftDeleteBook(ms.db, book.ID); err != nil {
			log.Printf("[WARN] merge: soft-delete %s: %v", book.ID, err)
		}
	}

	return &Result{
		PrimaryID:      resolvedPrimaryID,
		VersionGroupID: versionGroupID,
		MergedCount:    len(books),
	}, nil
}

// SoftDeleteBook marks a book as deleted using the MarkedForDeletion flag.
// If UpdateBook fails, falls back to hard-delete via DeleteBook.
func SoftDeleteBook(store database.Store, bookID string) error {
	current, err := store.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("GetBookByID %s: %w", bookID, err)
	}
	if current == nil {
		return nil // Already gone
	}

	t := true
	now := time.Now()
	current.MarkedForDeletion = &t
	current.MarkedForDeletionAt = &now

	if _, upErr := store.UpdateBook(bookID, current); upErr != nil {
		// Fall back to hard delete.
		log.Printf("[WARN] dedup-books: soft-delete failed for %s (%v), falling back to hard delete", bookID, upErr)
		return store.DeleteBook(bookID)
	}
	return nil
}

// IsITunesGhostPath reports whether a book's file path points at the
// iTunes media folder rather than the managed audiobook-organizer library.
func IsITunesGhostPath(p string) bool {
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	return strings.Contains(lower, "/itunes media/") || strings.Contains(lower, "/itunes/itunes")
}

// BookCurationScore returns a coarse "how much effort has the user put into
// this entry" score. Higher means more curated.
//
// Signals, each worth one point:
//   - MetadataReviewStatus == "matched" (user explicitly accepted a match)
//   - LastWrittenAt set (tags have been written back to the file)
//   - MetadataUpdatedAt strictly newer than CreatedAt (user-visible metadata
//     field has been edited since the row was created)
func BookCurationScore(b *database.Book) int {
	score := 0
	if b.MetadataReviewStatus != nil && *b.MetadataReviewStatus == "matched" {
		score++
	}
	if b.LastWrittenAt != nil {
		score++
	}
	if b.MetadataUpdatedAt != nil && b.CreatedAt != nil && b.MetadataUpdatedAt.After(*b.CreatedAt) {
		score++
	}
	return score
}

// BookIsBetter returns true if a is a "better" primary version than b.
// Preference order (strongest first):
//  1. Organized library path over iTunes-ghost path
//  2. Higher curation score (user effort beats technical quality)
//  3. M4B over other formats
//  4. Higher bitrate
//  5. Larger file size
func BookIsBetter(a, b *database.Book) bool {
	aGhost := IsITunesGhostPath(a.FilePath)
	bGhost := IsITunesGhostPath(b.FilePath)
	if aGhost != bGhost {
		return !aGhost
	}

	aCur := BookCurationScore(a)
	bCur := BookCurationScore(b)
	if aCur != bCur {
		return aCur > bCur
	}

	aM4B := strings.EqualFold(a.Format, "m4b")
	bM4B := strings.EqualFold(b.Format, "m4b")
	if aM4B != bM4B {
		return aM4B
	}
	aBitrate := 0
	if a.Bitrate != nil {
		aBitrate = *a.Bitrate
	}
	bBitrate := 0
	if b.Bitrate != nil {
		bBitrate = *b.Bitrate
	}
	if aBitrate != bBitrate {
		return aBitrate > bBitrate
	}
	aSize := int64(0)
	if a.FileSize != nil {
		aSize = *a.FileSize
	}
	bSize := int64(0)
	if b.FileSize != nil {
		bSize = *b.FileSize
	}
	return aSize > bSize
}
