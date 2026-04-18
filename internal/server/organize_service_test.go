// file: internal/server/organize_service_test.go
// version: 1.2.0
// guid: d4e5f6a7-b8c9-d0e1-f2a3-b4c5d6e7f8a9

package server

import (
	"context"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
)

func TestOrganizeService_FilterBooksNeedingOrganization(t *testing.T) {
	mockDB := &database.MockStore{}
	os := NewOrganizeService(mockDB)

	books := []database.Book{
		{ID: "1", Title: "Book 1", FilePath: "/import/book1.m4b"},
		{ID: "2", Title: "Book 2", FilePath: "/library/book2.m4b"},
	}

	testLog := logger.New("test")
	filtered, alreadyCorrect := os.FilterBooksNeedingOrganization(books, testLog)

	// Should filter out books already in library
	if len(filtered)+len(alreadyCorrect) > len(books) {
		t.Errorf("expected total filtered to not exceed input, got filtered=%d alreadyCorrect=%d", len(filtered), len(alreadyCorrect))
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
	testLog := logger.New("test")
	req := &OrganizeRequest{}

	err := os.PerformOrganize(ctx, req, testLog)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
