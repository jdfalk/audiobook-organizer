// file: internal/database/memdb_reads.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000006

package database

import (
	"fmt"
	"sort"
	"strings"
)

// Read-side implementations for the queries previously handled by Chai SQL.
//
// All methods on MemStore here are read-only and use snapshot transactions,
// which means they never block writers and never return errors related to
// concurrency. The signatures intentionally mirror the existing
// `*_Chai`/`*_Pebble` counterparts on PebbleStore so swap-in is a one-line
// delegation change.

// GetAllSeries returns every Series sorted by Name (case-insensitive).
func (m *MemStore) GetAllSeries() ([]Series, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableSeries, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb series scan: %w", err)
	}
	var out []Series
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		out = append(out, *(obj.(*Series)))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GetAllAuthors returns every Author sorted by Name (case-insensitive).
func (m *MemStore) GetAllAuthors() ([]Author, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableAuthors, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb authors scan: %w", err)
	}
	var out []Author
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		out = append(out, *(obj.(*Author)))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GetAllImportPaths returns every ImportPath sorted by Name.
func (m *MemStore) GetAllImportPaths() ([]ImportPath, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableImportPaths, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb import_paths scan: %w", err)
	}
	var out []ImportPath
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		out = append(out, *(obj.(*ImportPath)))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GetAllAuthorAliases returns every AuthorAlias sorted by AuthorID then AliasName.
func (m *MemStore) GetAllAuthorAliases() ([]AuthorAlias, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableAuthorAliases, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb author_aliases scan: %w", err)
	}
	var out []AuthorAlias
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		out = append(out, *(obj.(*AuthorAlias)))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AuthorID != out[j].AuthorID {
			return out[i].AuthorID < out[j].AuthorID
		}
		return strings.ToLower(out[i].AliasName) < strings.ToLower(out[j].AliasName)
	})
	return out, nil
}

// GetAllWorks returns every Work sorted by Title.
func (m *MemStore) GetAllWorks() ([]Work, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableWorks, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb works scan: %w", err)
	}
	var out []Work
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		out = append(out, *(obj.(*Work)))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// CountFiles returns the total number of non-missing BookFile rows.
func (m *MemStore) CountFiles() (int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	// Use the Missing index to scan only non-missing files.
	iter, err := txn.Get(memTableBookFiles, memIdxMissing, false)
	if err != nil {
		return 0, fmt.Errorf("memdb book_files count: %w", err)
	}
	count := 0
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		count++
	}
	return count, nil
}

// GetAllSeriesBookCounts returns map[seriesID] → count of primary, not-deleted
// books that belong to that series.
func (m *MemStore) GetAllSeriesBookCounts() (map[int]int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	out := make(map[int]int)
	// Scan only primary books to cut the candidate set in half.
	iter, err := txn.Get(memTableBooks, memIdxIsPrimaryVersion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.SeriesID == nil {
			continue
		}
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		out[*b.SeriesID]++
	}
	return out, nil
}

// GetAllAuthorBookCounts returns map[authorID] → count of primary, not-deleted
// books for which the author is the primary AuthorID on the Book row.
func (m *MemStore) GetAllAuthorBookCounts() (map[int]int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	out := make(map[int]int)
	iter, err := txn.Get(memTableBooks, memIdxIsPrimaryVersion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.AuthorID == nil {
			continue
		}
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		out[*b.AuthorID]++
	}
	return out, nil
}

// GetAllSeriesFileCounts returns map[seriesID] → count of non-missing
// book_files that belong to a primary book in that series.
func (m *MemStore) GetAllSeriesFileCounts() (map[int]int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	// Pass 1: build bookID → seriesID for primary books only.
	bookToSeries := make(map[string]int, 0)
	bIter, err := txn.Get(memTableBooks, memIdxIsPrimaryVersion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := bIter.Next(); obj != nil; obj = bIter.Next() {
		b := obj.(*Book)
		if b.SeriesID == nil {
			continue
		}
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		bookToSeries[b.ID] = *b.SeriesID
	}

	// Pass 2: count non-missing files for those books.
	out := make(map[int]int)
	fIter, err := txn.Get(memTableBookFiles, memIdxMissing, false)
	if err != nil {
		return nil, fmt.Errorf("memdb book_files scan: %w", err)
	}
	for obj := fIter.Next(); obj != nil; obj = fIter.Next() {
		bf := obj.(*BookFile)
		if seriesID, ok := bookToSeries[bf.BookID]; ok {
			out[seriesID]++
		}
	}
	return out, nil
}

// GetAllAuthorFileCounts returns map[authorID] → count of non-missing
// book_files belonging to primary books whose primary AuthorID matches.
func (m *MemStore) GetAllAuthorFileCounts() (map[int]int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	bookToAuthor := make(map[string]int, 0)
	bIter, err := txn.Get(memTableBooks, memIdxIsPrimaryVersion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := bIter.Next(); obj != nil; obj = bIter.Next() {
		b := obj.(*Book)
		if b.AuthorID == nil {
			continue
		}
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		bookToAuthor[b.ID] = *b.AuthorID
	}

	out := make(map[int]int)
	fIter, err := txn.Get(memTableBookFiles, memIdxMissing, false)
	if err != nil {
		return nil, fmt.Errorf("memdb book_files scan: %w", err)
	}
	for obj := fIter.Next(); obj != nil; obj = fIter.Next() {
		bf := obj.(*BookFile)
		if authorID, ok := bookToAuthor[bf.BookID]; ok {
			out[authorID]++
		}
	}
	return out, nil
}

// GetBooksBySeriesID returns primary, not-deleted books for a series,
// sorted by series_sequence (nulls last) then title, with pagination.
func (m *MemStore) GetBooksBySeriesID(seriesID int, limit, offset int) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxSeriesID, seriesID)
	if err != nil {
		return nil, fmt.Errorf("memdb books by series: %w", err)
	}

	// Collect matches into a slice, filter, sort, then page.
	var all []Book
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		// Match SQL semantics: nil IsPrimaryVersion → treat as true.
		if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
			continue
		}
		all = append(all, *b)
	}
	sort.SliceStable(all, func(i, j int) bool {
		si, sj := all[i].SeriesSequence, all[j].SeriesSequence
		switch {
		case si == nil && sj == nil:
			return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
		case si == nil:
			return false
		case sj == nil:
			return true
		case *si != *sj:
			return *si < *sj
		default:
			return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
		}
	})
	return paginate(all, limit, offset), nil
}

// GetBooksByAuthorID returns primary, not-deleted books for an author (by
// primary AuthorID field), sorted by title with pagination.
func (m *MemStore) GetBooksByAuthorID(authorID int, limit, offset int) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxAuthorID, authorID)
	if err != nil {
		return nil, fmt.Errorf("memdb books by author: %w", err)
	}

	var all []Book
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
			continue
		}
		all = append(all, *b)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
	})
	return paginate(all, limit, offset), nil
}

// GetAllBooks returns books with optional filters and pagination. Filter map
// keys mirror the Chai implementation:
//   - "is_primary_version" (bool)
//   - "marked_for_deletion" (bool)
//   - "series_id" (int)
//   - "author_id" (int)
//   - "version_group_id" (string)
//
// Unknown keys are ignored. Default sort: title (case-insensitive).
func (m *MemStore) GetAllBooks(limit, offset int, filters map[string]interface{}) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	// Choose the most selective index available from the filter set.
	// If no filter narrows the scan, fall back to scanning by ID.
	var (
		iter interface {
			Next() interface{}
		}
		err error
	)

	switch {
	case filters["series_id"] != nil:
		iter, err = txn.Get(memTableBooks, memIdxSeriesID, filters["series_id"])
	case filters["author_id"] != nil:
		iter, err = txn.Get(memTableBooks, memIdxAuthorID, filters["author_id"])
	case filters["version_group_id"] != nil:
		iter, err = txn.Get(memTableBooks, memIdxVersionGroupID, filters["version_group_id"])
	case filters["is_primary_version"] != nil:
		iter, err = txn.Get(memTableBooks, memIdxIsPrimaryVersion, filters["is_primary_version"])
	default:
		iter, err = txn.Get(memTableBooks, memIdxID)
	}
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}

	var all []Book
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if v, ok := filters["is_primary_version"].(bool); ok {
			eff := true
			if b.IsPrimaryVersion != nil {
				eff = *b.IsPrimaryVersion
			}
			if eff != v {
				continue
			}
		}
		if v, ok := filters["marked_for_deletion"].(bool); ok {
			eff := false
			if b.MarkedForDeletion != nil {
				eff = *b.MarkedForDeletion
			}
			if eff != v {
				continue
			}
		}
		if v, ok := filters["series_id"].(int); ok {
			if b.SeriesID == nil || *b.SeriesID != v {
				continue
			}
		}
		if v, ok := filters["author_id"].(int); ok {
			if b.AuthorID == nil || *b.AuthorID != v {
				continue
			}
		}
		if v, ok := filters["version_group_id"].(string); ok {
			if b.VersionGroupID == nil || *b.VersionGroupID != v {
				continue
			}
		}
		all = append(all, *b)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
	})
	return paginate(all, limit, offset), nil
}

func paginate[T any](in []T, limit, offset int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(in) {
		return nil
	}
	end := len(in)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return in[offset:end]
}
