// file: internal/database/memdb_reads.go
// version: 1.1.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000006

package database

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Read-side implementations for the queries previously handled by Chai SQL.
//
// All methods on MemStore here are read-only and use snapshot transactions,
// which means they never block writers and never return errors related to
// concurrency. The signatures intentionally mirror the existing
// `*_Pebble` counterparts on PebbleStore so swap-in is a one-line
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

// GetAllWorkBookCounts returns map[workID] → count of primary, not-deleted
// books for which the work is the WorkID on the Book row. Mirrors
// GetAllAuthorBookCounts; built with a single memdb iteration so callers
// avoid N+1 GetBooksByWorkID lookups on a 50K-work corpus.
func (m *MemStore) GetAllWorkBookCounts() (map[string]int, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	out := make(map[string]int)
	iter, err := txn.Get(memTableBooks, memIdxIsPrimaryVersion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.WorkID == nil || *b.WorkID == "" {
			continue
		}
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		out[*b.WorkID]++
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

// GetBooksBySeriesID returns primary, not-deleted books for a series with
// pagination. Sort order is series_sequence (nulls last) then title, but
// the comparator pre-lowercases titles ONCE per row instead of on every
// compare to avoid O(n log n) string allocations.
func (m *MemStore) GetBooksBySeriesID(seriesID int, limit, offset int) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxSeriesID, seriesID)
	if err != nil {
		return nil, fmt.Errorf("memdb books by series: %w", err)
	}

	all := make([]Book, 0, 32)
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
	if len(all) > 1 {
		keys := make([]string, len(all))
		for i := range all {
			keys[i] = strings.ToLower(all[i].Title)
		}
		sort.SliceStable(all, func(i, j int) bool {
			si, sj := all[i].SeriesSequence, all[j].SeriesSequence
			switch {
			case si == nil && sj == nil:
				return keys[i] < keys[j]
			case si == nil:
				return false
			case sj == nil:
				return true
			case *si != *sj:
				return *si < *sj
			default:
				return keys[i] < keys[j]
			}
		})
	}
	return paginate(all, limit, offset), nil
}

// GetBooksByAuthorID returns primary, not-deleted books for an author with
// pagination. No default sort — matches the Pebble path (which returned
// books in key/ULID order) and avoids per-call sort cost. Callers that
// need a specific order should sort the slice themselves.
func (m *MemStore) GetBooksByAuthorID(authorID int, limit, offset int) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxAuthorID, authorID)
	if err != nil {
		return nil, fmt.Errorf("memdb books by author: %w", err)
	}

	all := make([]Book, 0, 32)
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
	return paginate(all, limit, offset), nil
}

// GetAllBooks returns books with optional filters and pagination. Filter
// keys: "is_primary_version" (bool), "marked_for_deletion" (bool),
// "series_id" (int), "author_id" (int), "version_group_id" (string).
//
// No default sort. The Pebble path iterated in key (ULID) order without
// sorting; matching that here keeps allocation cost flat. Sorting 68K
// books by lowercase title on every page-load was the prod regression
// that caused 340MB allocations per call and severe GC pressure — never
// again.
func (m *MemStore) GetAllBooks(limit, offset int, filters map[string]interface{}) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

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

	// Pre-allocate to the requested page size. Callers passing limit=1M
	// (the "fetch all" sentinel) get clamped to 1024 to avoid an upfront
	// hundred-megabyte allocation; append grows naturally if needed.
	cap0 := limit
	if cap0 <= 0 || cap0 > 100_000 {
		cap0 = 1024
	}
	all := make([]Book, 0, cap0)

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
	return paginate(all, limit, offset), nil
}

// ListBookIDs returns the IDs of all non-deleted books. Walks the memdb
// books table via the ID index and reads only b.ID off each pointer — no
// struct copy, no JSON unmarshal. Used by callers that only need the ID
// set (e.g., diff'ing against another set of IDs). Saves ~50x memory vs
// GetAllBooks(0,0).
func (m *MemStore) ListBookIDs() ([]string, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb list book ids: %w", err)
	}

	ids := make([]string, 0, 1024)
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		ids = append(ids, b.ID)
	}
	return ids, nil
}

// ListSoftDeletedBooks returns books with MarkedForDeletion=true, with optional
// age filter (olderThan: only books whose MarkedForDeletionAt is on/before this
// time). Uses the marked_for_deletion index so cost is O(deleted_count), not
// O(total_books) — the soft-deleted set is typically tiny relative to 393K
// total books, so this is orders of magnitude faster than the Pebble full-scan.
func (m *MemStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxMarkedForDeletion, true)
	if err != nil {
		return nil, fmt.Errorf("memdb soft-deleted books: %w", err)
	}

	matched := make([]Book, 0, 32)
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if olderThan != nil && b.MarkedForDeletionAt != nil && b.MarkedForDeletionAt.After(*olderThan) {
			continue
		}
		matched = append(matched, *b)
	}
	// Stable sort by MarkedForDeletionAt desc (most recent first), nil last —
	// matches user expectation in the UI ("most recently deleted on top").
	sort.SliceStable(matched, func(i, j int) bool {
		ai, aj := matched[i].MarkedForDeletionAt, matched[j].MarkedForDeletionAt
		switch {
		case ai == nil && aj == nil:
			return matched[i].ID < matched[j].ID
		case ai == nil:
			return false
		case aj == nil:
			return true
		default:
			return ai.After(*aj)
		}
	})
	return paginate(matched, limit, offset), nil
}

// CountBooksByPathPrefix returns the number of (non-deleted) books whose
// SourceImportPath (or FilePath, if SourceImportPath is nil) starts with prefix.
// Falls back to a full memdb scan — no path-prefix index exists — but a memdb
// scan over 393K rows is still ~200× faster than the equivalent Pebble scan
// because the books are in RAM and don't need JSON unmarshal.
func (m *MemStore) CountBooksByPathPrefix(prefix string) (int, error) {
	if prefix == "" {
		return 0, nil
	}
	txn := m.db.Txn(false)
	defer txn.Abort()

	iter, err := txn.Get(memTableBooks, memIdxID)
	if err != nil {
		return 0, fmt.Errorf("memdb books scan: %w", err)
	}
	count := 0
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		b := obj.(*Book)
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		if b.SourceImportPath != nil && *b.SourceImportPath != "" {
			if strings.HasPrefix(*b.SourceImportPath, prefix) {
				count++
			}
			continue
		}
		if strings.HasPrefix(b.FilePath, prefix) {
			count++
		}
	}
	return count, nil
}

// ComputeLibraryStats aggregates per-book and per-file statistics from memdb,
// mirroring PebbleStore.computeLibraryStats but without any JSON unmarshal cost
// (everything is already in RAM as typed structs). rootDir is the configured
// library root (organized vs unorganized classification); importPaths is the
// resolved import-path list used for per-folder counts.
//
// Caller must populate stats.BrokenFiles separately — that count lives in the
// Pebble book_file_errors_by_book: secondary index, not in memdb.
func (m *MemStore) ComputeLibraryStats(rootDir string, importPaths []ImportPath) (*LibraryStats, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()

	stats := &LibraryStats{
		StateDistribution:  make(map[string]int),
		FormatDistribution: make(map[string]int),
		BooksByImportPath:  make(map[int]int),
		SizeByImportPath:   make(map[int]int64),
		ComputedAt:         time.Now(),
	}

	// Pass 1: books
	primaryBookIDs := make(map[string]struct{}, 16384)
	bIter, err := txn.Get(memTableBooks, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb books scan: %w", err)
	}
	for obj := bIter.Next(); obj != nil; obj = bIter.Next() {
		b := obj.(*Book)
		if b.MarkedForDeletion != nil && *b.MarkedForDeletion {
			continue
		}
		stats.TotalBooks++
		if b.Duration != nil {
			stats.TotalDuration += int64(*b.Duration)
		}
		size := int64(0)
		if b.FileSize != nil {
			size = *b.FileSize
			stats.TotalSize += size
		}
		state := "imported"
		if b.LibraryState != nil {
			state = *b.LibraryState
		}
		stats.StateDistribution[state]++
		codec := "unknown"
		if b.Codec != nil {
			codec = *b.Codec
		}
		stats.FormatDistribution[codec]++

		isPrimary := b.IsPrimaryVersion == nil || *b.IsPrimaryVersion
		if !isPrimary {
			continue
		}
		primaryBookIDs[b.ID] = struct{}{}
		if rootDir != "" && strings.HasPrefix(b.FilePath, rootDir) {
			stats.OrganizedBooks++
			stats.OrganizedSize += size
			continue
		}
		stats.UnorganizedBooks++
		stats.UnorganizedSize += size
		for _, ip := range importPaths {
			if strings.HasPrefix(b.FilePath, ip.Path) {
				stats.BooksByImportPath[ip.ID]++
				stats.SizeByImportPath[ip.ID] += size
				break
			}
		}
	}

	// Pass 2: files-per-primary-book.
	// Matches the Pebble semantics: count all files for primary books; books
	// with no file rows still count as 1 (legacy single-file-no-row case).
	bookActiveFiles := make(map[string]int, len(primaryBookIDs))
	fIter, err := txn.Get(memTableBookFiles, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb book_files scan: %w", err)
	}
	for obj := fIter.Next(); obj != nil; obj = fIter.Next() {
		bf := obj.(*BookFile)
		if _, ok := primaryBookIDs[bf.BookID]; !ok {
			continue
		}
		bookActiveFiles[bf.BookID]++
	}
	for id := range primaryBookIDs {
		if n := bookActiveFiles[id]; n > 0 {
			stats.TotalFiles += n
		} else {
			stats.TotalFiles++
		}
	}

	// Authors / Series totals come straight from memdb tables (cheap).
	aIter, err := txn.Get(memTableAuthors, memIdxID)
	if err == nil {
		for obj := aIter.Next(); obj != nil; obj = aIter.Next() {
			stats.TotalAuthors++
		}
	}
	sIter, err := txn.Get(memTableSeries, memIdxID)
	if err == nil {
		for obj := sIter.Next(); obj != nil; obj = sIter.Next() {
			stats.TotalSeries++
		}
	}

	return stats, nil
}

// GetBookFilesForIDs returns book files grouped by bookID, using the memdb
// book_id index — O(sum-of-files-for-IDs), NOT O(all 308K book_files) like
// the Pebble full-scan implementation. For a 500-book page query, this drops
// from ~15s to <5ms.
//
// Returns an empty map for empty input. Caller-supplied IDs absent from
// memdb appear as missing keys in the result (caller filters as needed).
func (m *MemStore) GetBookFilesForIDs(bookIDs []string) (map[string][]BookFile, error) {
	result := make(map[string][]BookFile, len(bookIDs))
	if len(bookIDs) == 0 {
		return result, nil
	}
	txn := m.db.Txn(false)
	defer txn.Abort()
	for _, id := range bookIDs {
		iter, err := txn.Get(memTableBookFiles, memIdxBookID, id)
		if err != nil {
			return nil, fmt.Errorf("memdb book_files for %s: %w", id, err)
		}
		for obj := iter.Next(); obj != nil; obj = iter.Next() {
			bf, ok := obj.(*BookFile)
			if !ok {
				continue
			}
			result[id] = append(result[id], *bf)
		}
	}
	return result, nil
}

// GetAllBookFiles returns every BookFile in the memdb book_files table by
// iterating the primary ID index. O(N) pointer walk over the in-memory table
// — no JSON unmarshal, no Pebble disk scan. For 308K book_files this is
// roughly two orders of magnitude faster than the Pebble full-scan fallback.
func (m *MemStore) GetAllBookFiles() ([]BookFile, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()
	iter, err := txn.Get(memTableBookFiles, memIdxID)
	if err != nil {
		return nil, fmt.Errorf("memdb book_files scan: %w", err)
	}
	var files []BookFile
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		bf, ok := obj.(*BookFile)
		if !ok {
			continue
		}
		files = append(files, *bf)
	}
	return files, nil
}

// GetBookFilesNeedingDelugeImport returns BookFiles that have a non-empty
// DelugeHash AND have not yet been imported (ImportedFromDelugeAt is nil).
//
// Walks the sparse memdb deluge_hash index — only rows with a non-empty
// DelugeHash exist in that index — then post-filters on the
// ImportedFromDelugeAt nil check. This is O(deluge-hash-present rows), not
// O(308K), which mirrors the GetAllBookFiles fastpath from PR #1166 but
// trims the working set to the deluge-relevant subset for the discovery
// handler and centralization plugin (H2 + H8).
func (m *MemStore) GetBookFilesNeedingDelugeImport() ([]BookFile, error) {
	txn := m.db.Txn(false)
	defer txn.Abort()
	iter, err := txn.Get(memTableBookFiles, memIdxDelugeHash)
	if err != nil {
		return nil, fmt.Errorf("memdb deluge_hash scan: %w", err)
	}
	var out []BookFile
	for obj := iter.Next(); obj != nil; obj = iter.Next() {
		bf, ok := obj.(*BookFile)
		if !ok {
			continue
		}
		if bf.ImportedFromDelugeAt != nil {
			continue
		}
		out = append(out, *bf)
	}
	return out, nil
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
