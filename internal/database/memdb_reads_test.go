// file: internal/database/memdb_reads_test.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-000000000007

package database

import (
	"reflect"
	"testing"
)

// Local ptr helpers — names suffixed _mem to avoid conflict with poc_chai_test.go.
func ptrBool_mem(b bool) *bool      { return &b }
func ptrInt_mem(i int) *int         { return &i }
func ptrInt64_mem(i int64) *int64   { return &i } //nolint:unused // kept for future tests
func ptrString_mem(s string) *string { return &s } //nolint:unused // kept for future tests

// seed inserts the given objects into the appropriate memdb tables.
// Used to set up deterministic state for the read query tests.
func seedMemStore(t *testing.T, m *MemStore, books []Book, files []BookFile, authors []Author, series []Series) {
	t.Helper()
	txn := m.db.Txn(true)
	defer txn.Commit()
	for i := range books {
		b := books[i]
		if err := txn.Insert(memTableBooks, &b); err != nil {
			t.Fatalf("seed book: %v", err)
		}
	}
	for i := range files {
		f := files[i]
		if err := txn.Insert(memTableBookFiles, &f); err != nil {
			t.Fatalf("seed file: %v", err)
		}
	}
	for i := range authors {
		a := authors[i]
		if err := txn.Insert(memTableAuthors, &a); err != nil {
			t.Fatalf("seed author: %v", err)
		}
	}
	for i := range series {
		s := series[i]
		if err := txn.Insert(memTableSeries, &s); err != nil {
			t.Fatalf("seed series: %v", err)
		}
	}
}

func TestMemStore_GetAllSeriesBookCounts(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}

	books := []Book{
		// Primary, not deleted — counted
		{ID: "b1", Title: "B1", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(true)},
		// Primary, not deleted — counted (same series)
		{ID: "b2", Title: "B2", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(true)},
		// Different series, counted
		{ID: "b3", Title: "B3", SeriesID: ptrInt_mem(20), IsPrimaryVersion: ptrBool_mem(true)},
		// Marked for deletion — skipped
		{ID: "b4", Title: "B4", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(true), MarkedForDeletion: ptrBool_mem(true)},
		// Not primary — skipped (effectiveBoolFieldIndex stores false here)
		{ID: "b5", Title: "B5", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(false)},
		// nil SeriesID — skipped
		{ID: "b6", Title: "B6", IsPrimaryVersion: ptrBool_mem(true)},
		// nil IsPrimaryVersion → effectively true, counted
		{ID: "b7", Title: "B7", SeriesID: ptrInt_mem(20)},
	}
	seedMemStore(t, m, books, nil, nil, nil)

	got, err := m.GetAllSeriesBookCounts()
	if err != nil {
		t.Fatalf("GetAllSeriesBookCounts: %v", err)
	}
	want := map[int]int{10: 2, 20: 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("counts mismatch: got %v, want %v", got, want)
	}
}

func TestMemStore_GetAllAuthorBookCounts(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	books := []Book{
		{ID: "b1", Title: "A", AuthorID: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b2", Title: "B", AuthorID: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b3", Title: "C", AuthorID: ptrInt_mem(2), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b4", Title: "D", AuthorID: ptrInt_mem(1), MarkedForDeletion: ptrBool_mem(true), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b5", Title: "E", AuthorID: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(false)},
	}
	seedMemStore(t, m, books, nil, nil, nil)

	got, err := m.GetAllAuthorBookCounts()
	if err != nil {
		t.Fatalf("GetAllAuthorBookCounts: %v", err)
	}
	want := map[int]int{1: 2, 2: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("counts mismatch: got %v, want %v", got, want)
	}
}

func TestMemStore_GetAllSeriesFileCounts(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	books := []Book{
		{ID: "b1", Title: "B1", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b2", Title: "B2", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b3", Title: "B3", SeriesID: ptrInt_mem(20), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b4", Title: "B4", SeriesID: ptrInt_mem(10), IsPrimaryVersion: ptrBool_mem(false)}, // skipped
	}
	files := []BookFile{
		{ID: "f1", BookID: "b1", Missing: false},
		{ID: "f2", BookID: "b1", Missing: false},
		{ID: "f3", BookID: "b2", Missing: false},
		{ID: "f4", BookID: "b3", Missing: false},
		{ID: "f5", BookID: "b3", Missing: true}, // missing — skipped
		{ID: "f6", BookID: "b4", Missing: false}, // non-primary book — skipped
		{ID: "f7", BookID: "orphan", Missing: false}, // unknown book — skipped
	}
	seedMemStore(t, m, books, files, nil, nil)

	got, err := m.GetAllSeriesFileCounts()
	if err != nil {
		t.Fatalf("GetAllSeriesFileCounts: %v", err)
	}
	want := map[int]int{10: 3, 20: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("counts mismatch: got %v, want %v", got, want)
	}
}

func TestMemStore_GetBooksBySeriesID(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	books := []Book{
		{ID: "b1", Title: "Book One", SeriesID: ptrInt_mem(10), SeriesSequence: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b2", Title: "Book Three", SeriesID: ptrInt_mem(10), SeriesSequence: ptrInt_mem(3), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b3", Title: "Book Two", SeriesID: ptrInt_mem(10), SeriesSequence: ptrInt_mem(2), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b4", Title: "Other Series", SeriesID: ptrInt_mem(20), IsPrimaryVersion: ptrBool_mem(true)},
	}
	seedMemStore(t, m, books, nil, nil, nil)

	got, err := m.GetBooksBySeriesID(10, 10, 0)
	if err != nil {
		t.Fatalf("GetBooksBySeriesID: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 books, got %d", len(got))
	}
	titles := []string{got[0].Title, got[1].Title, got[2].Title}
	wantTitles := []string{"Book One", "Book Two", "Book Three"}
	if !reflect.DeepEqual(titles, wantTitles) {
		t.Errorf("order wrong: got %v, want %v", titles, wantTitles)
	}
}

func TestMemStore_GetBooksByAuthorID(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	books := []Book{
		{ID: "b1", Title: "Zebra", AuthorID: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b2", Title: "Alpha", AuthorID: ptrInt_mem(1), IsPrimaryVersion: ptrBool_mem(true)},
		{ID: "b3", Title: "Other", AuthorID: ptrInt_mem(2), IsPrimaryVersion: ptrBool_mem(true)},
	}
	seedMemStore(t, m, books, nil, nil, nil)

	got, err := m.GetBooksByAuthorID(1, 10, 0)
	if err != nil {
		t.Fatalf("GetBooksByAuthorID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 books, got %d", len(got))
	}
	if got[0].Title != "Alpha" || got[1].Title != "Zebra" {
		t.Errorf("sort order wrong: %s, %s", got[0].Title, got[1].Title)
	}
}

func TestMemStore_GetAllBooks_Filters(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	books := []Book{
		{ID: "b1", Title: "A", IsPrimaryVersion: ptrBool_mem(true), AuthorID: ptrInt_mem(1), MarkedForDeletion: ptrBool_mem(false)},
		{ID: "b2", Title: "B", IsPrimaryVersion: ptrBool_mem(true), AuthorID: ptrInt_mem(2), MarkedForDeletion: ptrBool_mem(false)},
		{ID: "b3", Title: "C", IsPrimaryVersion: ptrBool_mem(false), AuthorID: ptrInt_mem(1), MarkedForDeletion: ptrBool_mem(false)},
		{ID: "b4", Title: "D", IsPrimaryVersion: ptrBool_mem(true), AuthorID: ptrInt_mem(1), MarkedForDeletion: ptrBool_mem(true)},
	}
	seedMemStore(t, m, books, nil, nil, nil)

	// author_id=1 && is_primary && !deleted → only b1
	got, err := m.GetAllBooks(100, 0, map[string]interface{}{
		"author_id":           1,
		"is_primary_version":  true,
		"marked_for_deletion": false,
	})
	if err != nil {
		t.Fatalf("GetAllBooks: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b1" {
		ids := []string{}
		for _, b := range got {
			ids = append(ids, b.ID)
		}
		t.Errorf("expected [b1], got %v", ids)
	}
}

func TestMemStore_CountFiles(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	files := []BookFile{
		{ID: "f1", BookID: "b1", Missing: false},
		{ID: "f2", BookID: "b1", Missing: false},
		{ID: "f3", BookID: "b2", Missing: true},
	}
	seedMemStore(t, m, nil, files, nil, nil)

	got, err := m.CountFiles()
	if err != nil {
		t.Fatalf("CountFiles: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestMemStore_GetAllAuthorsSeriesImportPaths_Sort(t *testing.T) {
	m, err := NewMemStore()
	if err != nil {
		t.Fatalf("NewMemStore: %v", err)
	}
	authors := []Author{
		{ID: 3, Name: "Zoe"},
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	series := []Series{
		{ID: 1, Name: "Z Series"},
		{ID: 2, Name: "A Series"},
	}
	seedMemStore(t, m, nil, nil, authors, series)

	gotAuthors, err := m.GetAllAuthors()
	if err != nil {
		t.Fatalf("GetAllAuthors: %v", err)
	}
	if gotAuthors[0].Name != "Alice" || gotAuthors[2].Name != "Zoe" {
		t.Errorf("authors not sorted: %v", gotAuthors)
	}

	gotSeries, err := m.GetAllSeries()
	if err != nil {
		t.Fatalf("GetAllSeries: %v", err)
	}
	if gotSeries[0].Name != "A Series" || gotSeries[1].Name != "Z Series" {
		t.Errorf("series not sorted: %v", gotSeries)
	}
}
