// file: internal/itunes/backfill_test.go
// version: 1.0.0
// guid: c9d0e1f2-a3b4-c5d6-e7f8-a9b0c1d2e3f4

package itunes

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// MockBackfillStore provides a minimal mock for testing backfill operations.
type MockBackfillStore struct {
	books          map[string]database.Book
	externalIDMaps []database.ExternalIDMapping
	hasError       bool
}

func NewMockBackfillStore() *MockBackfillStore {
	return &MockBackfillStore{
		books: make(map[string]database.Book),
	}
}

func (m *MockBackfillStore) GetAllBooks(limit, offset int) ([]database.Book, error) {
	if m.hasError {
		return nil, database.ErrNotFound
	}
	var result []database.Book
	for _, b := range m.books {
		result = append(result, b)
	}
	return result, nil
}

func (m *MockBackfillStore) GetBookFiles(bookID string) ([]database.BookFile, error) {
	if m.hasError {
		return nil, database.ErrNotFound
	}
	return []database.BookFile{}, nil
}

func (m *MockBackfillStore) CreateExternalIDMapping(mapping *database.ExternalIDMapping) error {
	if m.hasError {
		return database.ErrNotFound
	}
	m.externalIDMaps = append(m.externalIDMaps, *mapping)
	return nil
}

func (m *MockBackfillStore) BulkCreateExternalIDMappings(mappings []database.ExternalIDMapping) error {
	if m.hasError {
		return database.ErrNotFound
	}
	m.externalIDMaps = append(m.externalIDMaps, mappings...)
	return nil
}

func (m *MockBackfillStore) SetSetting(key, value, dataType string, internal bool) error {
	if m.hasError {
		return database.ErrNotFound
	}
	return nil
}

func TestBackfillExternalIDsWithNilStore(t *testing.T) {
	err := BackfillExternalIDs(nil)
	if err != nil {
		t.Errorf("expected nil error for nil store, got %v", err)
	}
}

func TestBackfillExternalIDsCollectsBookPIDs(t *testing.T) {
	mockStore := NewMockBackfillStore()
	pidValue := "test-pid-123"
	mockStore.books["book1"] = database.Book{
		ID:                   "book1",
		Title:                "Test Book",
		ITunesPersistentID:   &pidValue,
	}

	err := BackfillExternalIDs(mockStore)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have created one mapping from the book PID
	if len(mockStore.externalIDMaps) < 1 {
		t.Errorf("expected at least 1 mapping, got %d", len(mockStore.externalIDMaps))
	}
}

func TestBackfillITunesTrackPIDsWithNoConfiguredPath(t *testing.T) {
	mockStore := NewMockBackfillStore()

	// With no configured path, should return 0 gracefully
	count, err := BackfillITunesTrackPIDs(mockStore)
	if count != 0 {
		t.Errorf("expected 0 registered PIDs with no configured path, got %d", count)
	}
	if err != nil {
		t.Errorf("unexpected error with no path: %v", err)
	}
}
