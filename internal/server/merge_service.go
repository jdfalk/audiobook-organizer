// file: internal/server/merge_service.go
// version: 1.1.0
// guid: 7d736d2d-e0df-40bd-9f4b-0a07bc2eb6ae

package server

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	ulid "github.com/oklog/ulid/v2"
)

// MergeService handles merging duplicate books into version groups.
type MergeService struct {
	db database.Store
}

// MergeResult contains the outcome of a merge operation.
type MergeResult struct {
	PrimaryID      string `json:"primary_id"`
	VersionGroupID string `json:"version_group_id"`
	MergedCount    int    `json:"merged_count"`
}

// NewMergeService creates a new MergeService.
func NewMergeService(db database.Store) *MergeService {
	return &MergeService{db: db}
}

// MergeBooks merges a set of books into a single version group.
// If primaryID is empty, the best book is auto-selected (M4B preferred, then highest bitrate, then largest file).
// If primaryID is provided, that book is set as the primary version.
func (ms *MergeService) MergeBooks(bookIDs []string, primaryID string) (*MergeResult, error) {
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
			if bookIsBetter(books[i], books[bestIdx]) {
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

	// Update all books
	resolvedPrimaryID := books[bestIdx].ID
	for i, book := range books {
		book.VersionGroupID = &versionGroupID
		isPrimary := i == bestIdx
		book.IsPrimaryVersion = &isPrimary
		if _, err := ms.db.UpdateBook(book.ID, book); err != nil {
			return nil, fmt.Errorf("failed to update book %s: %w", book.ID, err)
		}
	}

	// Reassign external IDs from merged books to the primary book
	if eidStore := asExternalIDStore(ms.db); eidStore != nil {
		for _, book := range books {
			if book.ID != resolvedPrimaryID {
				_ = eidStore.ReassignExternalIDs(book.ID, resolvedPrimaryID)
			}
		}
	}

	return &MergeResult{
		PrimaryID:      resolvedPrimaryID,
		VersionGroupID: versionGroupID,
		MergedCount:    len(books),
	}, nil
}

