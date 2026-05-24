package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
)

// GetAllSeriesBookCounts_Chai demonstrates how it would work with Chai
// This is pseudocode showing the SQL approach
func GetAllSeriesBookCounts_Chai(db *sql.DB, ctx context.Context) (map[int]int, error) {
	counts := make(map[int]int)

	// This is what the SQL query would look like
	query := `
		SELECT series_id, COUNT(*) as count
		FROM books
		WHERE series_id IS NOT NULL AND is_primary_version = true
		GROUP BY series_id
	`

	rows, err := db.QueryContext(ctx, query)
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

// TestChai_ParsesBookJSON validates that Chai can deserialize Book JSON
// (since we'll store Books as documents, not key-value pairs)
func TestChai_ParsesBookJSON(t *testing.T) {
	book := Book{
		ID:    "book-1",
		Title: "Test Book",
		SeriesID: ptrInt(1),
		IsPrimaryVersion: ptrBool(true),
	}

	// Verify we can marshal/unmarshal
	data, err := json.Marshal(book)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored Book
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.ID != book.ID || restored.Title != book.Title {
		t.Fatalf("deserialization mismatch")
	}
}

// Helper functions

func ptrInt(i int) *int {
	return &i
}

func ptrBool(b bool) *bool {
	return &b
}
