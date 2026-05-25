// file: internal/database/memdb_summaries.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000008

package database

import "fmt"

// BookSummaryFilter narrows a summary query without forcing the caller to
// materialize a full Book slice. Each non-nil field becomes a predicate
// applied during memdb iteration; nil means "no constraint".
//
// IsPrimaryVersion is the hot one — the UI sends it on every library page
// load, so pushing it down lets memdb iterate only the ~38K primary rows
// instead of the full ~68K.
type BookSummaryFilter struct {
	IsPrimaryVersion  *bool
	MarkedForDeletion *bool // nil → exclude deleted (default behavior)

	// SortBy selects which memdb index drives iteration. Empty string means
	// "any order" (currently equivalent to ID-sorted). Only "title" is
	// honored today — other sort keys fall through to the service-level
	// in-memory sort path. The memdb title index is a sorted radix tree, so
	// iterating it is O(offset+limit), not O(n).
	SortBy string
	// SortAscending controls iteration direction when SortBy is set.
	// True (or zero value with SortBy=="title") = A→Z; false = Z→A.
	SortAscending bool
}

// GetBookSummaries returns a paginated slice of BookSummary records,
// projecting from Book in-place during iteration. Key differences vs.
// "fetch all Books then project":
//
//   - Iterates the most selective index given the filter.
//   - Projects Book → BookSummary inside the loop (no full-Book copies).
//   - Stops as soon as `limit` summaries have been collected past `offset`.
//
// For the typical library list query (is_primary_version=true, limit=50,
// offset=0) this performs ~50 BookSummary allocations and ~50 index node
// reads instead of 68K Book copies + 68K BookSummary projections.
func (m *MemStore) GetBookSummaries(limit, offset int, f BookSummaryFilter) ([]BookSummary, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}

	txn := m.db.Txn(false)
	defer txn.Abort()

	// Index selection priority:
	//   1. SortBy=="title" → iterate title index in order (asc/desc). IsPrimary
	//      is applied as an in-loop filter — title index is the sorted radix
	//      tree we need, and the rejected rows are cheap to skip.
	//   2. IsPrimary set, no SortBy → use the IsPrimary index (most selective
	//      for the typical library page query).
	//   3. Otherwise → ID-ordered scan.
	var (
		iter interface {
			Next() interface{}
		}
		err error
	)
	switch {
	case f.SortBy == "title":
		if f.SortAscending {
			iter, err = txn.Get(memTableBooks, memIdxTitle)
		} else {
			iter, err = txn.GetReverse(memTableBooks, memIdxTitle)
		}
	case f.IsPrimaryVersion != nil:
		iter, err = txn.Get(memTableBooks, memIdxIsPrimaryVersion, *f.IsPrimaryVersion)
	default:
		iter, err = txn.Get(memTableBooks, memIdxID)
	}
	if err != nil {
		return nil, fmt.Errorf("memdb book_summaries scan: %w", err)
	}

	// When iterating by title, IsPrimary becomes an in-loop predicate.
	primaryFilter := f.SortBy == "title" && f.IsPrimaryVersion != nil
	wantPrimary := false
	if primaryFilter {
		wantPrimary = *f.IsPrimaryVersion
	}

	// excludeDeleted: by default we drop marked_for_deletion=true rows
	// (mirrors GetAllBookSummaries_Pebble). An explicit filter overrides.
	excludeDeleted := true
	requireDeleted := false
	if f.MarkedForDeletion != nil {
		excludeDeleted = false
		requireDeleted = *f.MarkedForDeletion
	}

	cap0 := limit
	if cap0 > 4096 {
		cap0 = 4096
	}
	out := make([]BookSummary, 0, cap0)
	skipped := 0

	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)

		// When sorting by title, IsPrimary is enforced here (not by index).
		// nil IsPrimaryVersion on the row counts as "primary" per the
		// historical Pebble fallback semantics in
		// GetAllBookSummariesFiltered.
		if primaryFilter {
			eff := b.IsPrimaryVersion == nil || *b.IsPrimaryVersion
			if eff != wantPrimary {
				continue
			}
		}

		// Apply filters before pagination so offset/limit match the
		// post-filter set, not the pre-filter set.
		if excludeDeleted {
			if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
				continue
			}
		} else {
			isDel := b.MarkedForDeletion != nil && *b.MarkedForDeletion
			if isDel != requireDeleted {
				continue
			}
		}

		if skipped < offset {
			skipped++
			continue
		}

		out = append(out, BookSummary{
			ID:                   b.ID,
			Title:                b.Title,
			AuthorID:             b.AuthorID,
			SeriesID:             b.SeriesID,
			SeriesSequence:       b.SeriesSequence,
			FilePath:             b.FilePath,
			Format:               b.Format,
			Duration:             b.Duration,
			OriginalFilename:     b.OriginalFilename,
			FileSize:             b.FileSize,
			FileHash:             b.FileHash,
			OriginalFileHash:     b.OriginalFileHash,
			OrganizedFileHash:    b.OrganizedFileHash,
			LibraryState:         b.LibraryState,
			QuarantinedAt:        b.QuarantinedAt,
			QuarantineReason:     b.QuarantineReason,
			CoverURL:             b.CoverURL,
			Narrator:             b.Narrator,
			CreatedAt:            b.CreatedAt,
			UpdatedAt:            b.UpdatedAt,
			MetadataUpdatedAt:    b.MetadataUpdatedAt,
			IsPrimaryVersion:     b.IsPrimaryVersion,
			VersionGroupID:       b.VersionGroupID,
			MetadataReviewStatus: b.MetadataReviewStatus,
		})

		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
