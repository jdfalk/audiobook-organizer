// file: internal/server/organize_service_test.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-d0e1-f2a3-b4c5d6e7f8a9

package server

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestOrganizeService_FilterBooksNeedingOrganization(t *testing.T) {
	mockDB := &database.MockStore{}
	os := NewOrganizeService(mockDB)

	books := []database.Book{
		{ID: "1", Title: "Book 1", FilePath: "/import/book1.m4b"},
		{ID: "2", Title: "Book 2", FilePath: "/library/book2.m4b"},
	}

	mockProgress := &mockProgressReporter{}
	filtered := os.filterBooksNeedingOrganization(books, mockProgress)

	// Should filter out books already in library
	if len(filtered) > 1 {
		t.Errorf("expected at most 1 book after filtering, got %d", len(filtered))
	}
}

func TestOrganizeService_PerformOrganize_NoBooksToOrganize(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllBooksFunc: func(limit, offset int) ([]database.Book, error) {
			return []database.Book{}, nil
		},
	}
	os := NewOrganizeService(mockDB)

	ctx := context.Background()
	mockProgress := &mockProgressReporter{}
	req := &OrganizeRequest{}

	err := os.PerformOrganize(ctx, req, mockProgress)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
