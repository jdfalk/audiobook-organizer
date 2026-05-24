package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaisql/chai"
	"github.com/cockroachdb/pebble/v2"
)

// BenchmarkGetAllSeriesBookCounts_Pebble benchmarks the current Pebble implementation
func BenchmarkGetAllSeriesBookCounts_Pebble(b *testing.B) {
	store := setupTestPebbleStore(b)
	defer store.Close()

	// Add test data: 100 books across 10 series
	for i := 0; i < 100; i++ {
		book := Book{
			ID:             testID(i),
			Title:          "Test Book",
			SeriesID:       ptrInt((i % 10) + 1),
			IsPrimaryVersion: ptrBool(true),
		}
		_ = store.SetBook(book.ID, &book)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.GetAllSeriesBookCounts()
	}
}

// GetAllSeriesBookCounts_Chai demonstrates how it would work with Chai
// This is pseudocode showing the SQL approach
func GetAllSeriesBookCounts_Chai(db *chai.DB, ctx context.Context) (map[int]int, error) {
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

func setupTestPebbleStore(tb testing.TB) *PebbleStore {
	db, err := pebble.Open(tb.TempDir(), &pebble.Options{})
	if err != nil {
		tb.Fatalf("failed to open pebble: %v", err)
	}
	return &PebbleStore{db: db}
}

func testID(i int) string {
	return "id-" + string(rune(i))
}

func ptrInt(i int) *int {
	return &i
}

func ptrBool(b bool) *bool {
	return &b
}
