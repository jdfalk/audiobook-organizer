// file: internal/database/chai_books_list_test_helper.go
// version: 1.0.0
// guid: c2d3e4f5-g6h7-48i9-j0k1-l2m3n4o5p6q7
// last-edited: 2026-05-24

package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Helper function to insert a test book with all fields including series_id
func insertTestBookFull(db *sql.DB, book *Book) (*Book, error) {
	now := time.Now()
	if book.CreatedAt == nil {
		book.CreatedAt = &now
	}
	if book.UpdatedAt == nil {
		book.UpdatedAt = &now
	}
	if book.IsPrimaryVersion == nil {
		book.IsPrimaryVersion = boolPtr(true)
	}
	if book.MarkedForDeletion == nil {
		book.MarkedForDeletion = boolPtr(false)
	}

	// Build the insert statement with all fields
	seriesIDVal := "NULL"
	if book.SeriesID != nil {
		seriesIDVal = fmt.Sprintf("%d", *book.SeriesID)
	}
	seriesSeqVal := "NULL"
	if book.SeriesSequence != nil {
		seriesSeqVal = fmt.Sprintf("%d", *book.SeriesSequence)
	}

	query := fmt.Sprintf(`
		INSERT INTO books (
			id, title, file_path, series_id, series_sequence,
			is_primary_version, marked_for_deletion, created_at, updated_at
		)
		VALUES ('%s', '%s', '%s', %s, %s, %v, %v, '%s', '%s')
	`,
		book.ID,
		escapeSQL(book.Title),
		escapeSQL(book.FilePath),
		seriesIDVal,
		seriesSeqVal,
		boolToSQL(*book.IsPrimaryVersion),
		boolToSQL(*book.MarkedForDeletion),
		now.Format("2006-01-02T15:04:05"),
		now.Format("2006-01-02T15:04:05"),
	)

	_, err := db.Exec(query)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// Helper pointer creation functions
func boolPtr(b bool) *bool {
	return &b
}

func boolToSQL(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intPtr(i int) *int {
	return &i
}
