package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chaisql/chai"
	"github.com/cockroachdb/pebble/v2"
)

// ChaiStore wraps a Chai database (which uses Pebble as backend)
// This POC demonstrates how to replace manual indexing with SQL queries
type ChaiStore struct {
	db *chai.DB
}

// NewChaiStore creates a new Chai database with Pebble backend
func NewChaiStore(pebbleDB *pebble.DB) (*ChaiStore, error) {
	// Chai can use a custom engine; we'll create one that wraps Pebble
	// For now, using in-memory for POC
	// TODO: Replace with actual Pebble backend integration
	return &ChaiStore{}, nil
}

// GetAllSeriesBookCounts_SQL shows what the SQL query looks like
// This replaces 33 lines of Pebble iteration + JSON unmarshaling + filtering
func (cs *ChaiStore) GetAllSeriesBookCounts_SQL(ctx context.Context) (map[int]int, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	counts := make(map[int]int)

	// Single SQL query instead of:
	// 1. Creating iterator over "book:0" to "book:;"
	// 2. Checking string prefix
	// 3. Splitting key by colons
	// 4. Unmarshaling JSON
	// 5. Checking SeriesID != nil and IsPrimaryVersion == true
	// 6. Building map
	rows, err := cs.db.QueryContext(ctx, `
		SELECT series_id, COUNT(*) as count
		FROM books
		WHERE series_id IS NOT NULL
		  AND is_primary_version = true
		  AND marked_for_deletion = false
		GROUP BY series_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var seriesID int
		var count int
		if err := rows.Scan(&seriesID, &count); err != nil {
			continue
		}
		counts[seriesID] = count
	}

	return counts, nil
}

// GetAllAuthorBookCounts_SQL shows SQL version of author book counts
// Current Pebble version: 33 lines of string parsing + filtering
func (cs *ChaiStore) GetAllAuthorBookCounts_SQL(ctx context.Context) (map[int]int, error) {
	counts := make(map[int]int)

	// Instead of iterating book:author index and parsing key parts
	rows, err := cs.db.QueryContext(ctx, `
		SELECT author_id, COUNT(*) as count
		FROM book_authors
		WHERE marked_for_deletion = false
		GROUP BY author_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var authorID int
		var count int
		if err := rows.Scan(&authorID, &count); err != nil {
			continue
		}
		counts[authorID] = count
	}

	return counts, nil
}

// GetAllSeriesFileCounts_SQL shows how file counting changes
// Current: Two-phase scan (books -> IDs, files -> count)
// SQL: Single query with join
func (cs *ChaiStore) GetAllSeriesFileCounts_SQL(ctx context.Context) (map[int]int, error) {
	counts := make(map[int]int)

	// Single query replaces:
	// 1. Scan all books, build bookID->seriesID map
	// 2. Scan all files, filter by map, count per series
	rows, err := cs.db.QueryContext(ctx, `
		SELECT b.series_id, COUNT(f.id) as file_count
		FROM books b
		LEFT JOIN book_files f ON b.id = f.book_id
		WHERE b.series_id IS NOT NULL
		  AND b.marked_for_deletion = false
		  AND b.is_primary_version = true
		GROUP BY b.series_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var seriesID int
		var count int
		if err := rows.Scan(&seriesID, &count); err != nil {
			continue
		}
		counts[seriesID] = count
	}

	return counts, nil
}

// ListBooks_SQL demonstrates filtering without manual loop logic
// Current: Parses filters, builds conditions in code, manually slices for pagination
// SQL: WHERE clause + LIMIT/OFFSET
func (cs *ChaiStore) ListBooks_SQL(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]map[string]interface{}, error) {
	// Build WHERE clause from filters
	whereClause := "WHERE marked_for_deletion = false AND is_primary_version = true"

	// Instead of:
	// iter, err := p.db.NewIter(&pebble.IterOptions{LowerBound: []byte("book:0"), UpperBound: []byte("book:;")})
	// for iter.Valid() { ... check filters ... continue ... }
	// selected := bookIDs[start:end]

	// We just add filter conditions:
	if seriesID, ok := filters["series_id"]; ok {
		whereClause += fmt.Sprintf(" AND series_id = %v", seriesID)
	}
	if authorID, ok := filters["author_id"]; ok {
		whereClause += fmt.Sprintf(" AND id IN (SELECT book_id FROM book_authors WHERE author_id = %v)", authorID)
	}

	rows, err := cs.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, title, author_id, series_id FROM books
		%s
		ORDER BY title
		LIMIT %d OFFSET %d
	`, whereClause, limit, offset))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var book map[string]interface{}
		if err := rows.Scan(&book); err != nil {
			continue
		}
		results = append(results, book)
	}

	return results, nil
}

// CompareSQLtoManual documents the differences
func compareImplementations() {
	// PEBBLE VERSION (33 lines in GetAllSeriesBookCounts):
	// 1. Create iterator with bounds
	// 2. Loop through all keys
	// 3. Check string prefix "book:"
	// 4. Parse key string by splitting on ":"
	// 5. Unmarshal JSON
	// 6. Check SeriesID != nil
	// 7. Check IsPrimaryVersion == true
	// 8. Build map incrementally
	pebbleLines := 33

	// CHAI/SQL VERSION (10 lines):
	// 1. Write SQL query with GROUP BY
	// 2. Execute query
	// 3. Scan results into map
	sqlLines := 10

	fmt.Printf("Code reduction: %d lines → %d lines (%.0f%% smaller)\n",
		pebbleLines, sqlLines, float64(pebbleLines-sqlLines)/float64(pebbleLines)*100)
	// Expected output: Code reduction: 33 lines → 10 lines (70% smaller)

	// PERFORMANCE ESTIMATE:
	// Pebble: O(n) full scan, JSON unmarshal per record, manual filtering
	// SQL: Indexed range scan on series_id, server-side aggregation
	// Expected: 10-100x faster on large libraries due to index usage
}
