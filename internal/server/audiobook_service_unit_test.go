// file: internal/server/audiobook_service_unit_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- GetAudiobooks ---

func TestAudiobookService_GetAudiobooks_StoreError(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetAllBooks(50, 0).Return(nil, fmt.Errorf("db connection lost"))

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.Error(t, err)
	assert.Nil(t, books)
	assert.Contains(t, err.Error(), "db connection lost")
}

func TestAudiobookService_GetAudiobooks_EmptyResult(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetAllBooks(50, 0).Return([]database.Book{}, nil)

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, books, "should return empty slice, not nil")
	assert.Empty(t, books)
}

func TestAudiobookService_GetAudiobooks_NilResultBecomesEmptySlice(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// Store returns nil slice (some backends do this for zero results)
	mockStore.EXPECT().GetAllBooks(50, 0).Return(nil, nil)

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, books, "nil from store should be converted to empty slice")
	assert.Empty(t, books)
}

func TestAudiobookService_GetAudiobooks_NormalizesLimitAndOffset(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// Negative limit → default 50; negative offset → 0
	mockStore.EXPECT().GetAllBooks(50, 0).Return([]database.Book{}, nil)

	books, err := svc.GetAudiobooks(context.Background(), -10, -5, "", nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, books)
}

func TestAudiobookService_GetAudiobooks_ExcessiveLimitNormalized(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// Limit > 100000 → default 50
	mockStore.EXPECT().GetAllBooks(50, 0).Return([]database.Book{}, nil)

	_, err := svc.GetAudiobooks(context.Background(), 999999, 0, "", nil, nil)
	assert.NoError(t, err)
}

func TestAudiobookService_GetAudiobooks_SearchDelegatesToStore(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	expected := []database.Book{{ID: "b1", Title: "Search Result"}}
	// No Bleve index wired → falls back to store.SearchBooks
	mockStore.EXPECT().SearchBooks("test query", 50, 0).Return(expected, nil)

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "test query", nil, nil)
	assert.NoError(t, err)
	assert.Len(t, books, 1)
	assert.Equal(t, "b1", books[0].ID)
}

func TestAudiobookService_GetAudiobooks_SearchError(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().SearchBooks("bad", 50, 0).Return(nil, fmt.Errorf("search index corrupt"))

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "bad", nil, nil)
	assert.Error(t, err)
	assert.Nil(t, books)
}

func TestAudiobookService_GetAudiobooks_ByAuthorID(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	authorID := 42
	expected := []database.Book{{ID: "b1"}, {ID: "b2"}}
	mockStore.EXPECT().GetBooksByAuthorID(42).Return(expected, nil)

	books, err := svc.GetAudiobooks(context.Background(), 10, 0, "", &authorID, nil)
	assert.NoError(t, err)
	assert.Len(t, books, 2)
}

func TestAudiobookService_GetAudiobooks_BySeriesID(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	seriesID := 7
	expected := []database.Book{{ID: "s1"}}
	mockStore.EXPECT().GetBooksBySeriesID(7).Return(expected, nil)

	books, err := svc.GetAudiobooks(context.Background(), 10, 0, "", nil, &seriesID)
	assert.NoError(t, err)
	assert.Len(t, books, 1)
}

// --- GetAudiobook (single) ---

func TestAudiobookService_GetAudiobook_NilStore(t *testing.T) {
	svc := &AudiobookService{} // store is nil
	book, err := svc.GetAudiobook(context.Background(), "any-id")
	assert.Error(t, err)
	assert.Nil(t, book)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestAudiobookService_GetAudiobook_NotFound(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetBookByID("missing").Return(nil, nil)

	book, err := svc.GetAudiobook(context.Background(), "missing")
	assert.Error(t, err)
	assert.Nil(t, book)
	assert.Contains(t, err.Error(), "audiobook not found")
}

func TestAudiobookService_GetAudiobook_StoreError(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetBookByID("err-id").Return(nil, fmt.Errorf("disk I/O error"))

	book, err := svc.GetAudiobook(context.Background(), "err-id")
	assert.Error(t, err)
	assert.Nil(t, book)
	assert.Contains(t, err.Error(), "disk I/O error")
}

// --- CountAudiobooks ---

func TestAudiobookService_CountAudiobooks_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	count, err := svc.CountAudiobooks(context.Background())
	assert.Error(t, err)
	assert.Equal(t, 0, count)
}

func TestAudiobookService_CountAudiobooks_Success(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().CountBooks().Return(42, nil)

	count, err := svc.CountAudiobooks(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 42, count)
}

func TestAudiobookService_CountAudiobooks_StoreError(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().CountBooks().Return(0, fmt.Errorf("count failed"))

	count, err := svc.CountAudiobooks(context.Background())
	assert.Error(t, err)
	assert.Equal(t, 0, count)
}

// --- DeleteAudiobook ---

func TestAudiobookService_DeleteAudiobook_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	result, err := svc.DeleteAudiobook(context.Background(), "any", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestAudiobookService_DeleteAudiobook_NotFound(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetBookByID("nope").Return(nil, fmt.Errorf("not found"))

	result, err := svc.DeleteAudiobook(context.Background(), "nope", nil)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "audiobook not found")
}

func TestAudiobookService_DeleteAudiobook_HardDelete(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	book := &database.Book{ID: "del-1", Title: "To Delete"}
	mockStore.EXPECT().GetBookByID("del-1").Return(book, nil)
	mockStore.EXPECT().DeleteBook("del-1").Return(nil)

	result, err := svc.DeleteAudiobook(context.Background(), "del-1", &DeleteAudiobookOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "audiobook deleted", result["message"])
	assert.Equal(t, false, result["blocked"])
}

func TestAudiobookService_DeleteAudiobook_SoftDeleteAlreadyDeleted(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	alreadyDeleted := &database.Book{
		ID:                "sd-1",
		MarkedForDeletion: boolPtr(true),
	}
	mockStore.EXPECT().GetBookByID("sd-1").Return(alreadyDeleted, nil)

	result, err := svc.DeleteAudiobook(context.Background(), "sd-1", &DeleteAudiobookOptions{SoftDelete: true})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "already soft deleted")
}

func TestAudiobookService_DeleteAudiobook_SoftDeleteSuccess(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	book := &database.Book{ID: "sd-2", Title: "To Soft Delete"}
	mockStore.EXPECT().GetBookByID("sd-2").Return(book, nil)
	mockStore.EXPECT().UpdateBook("sd-2", mock.AnythingOfType("*database.Book")).Return(book, nil)

	result, err := svc.DeleteAudiobook(context.Background(), "sd-2", &DeleteAudiobookOptions{SoftDelete: true})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "audiobook soft deleted", result["message"])
	assert.Equal(t, true, result["soft_delete"])
}

// --- RestoreAudiobook ---

func TestAudiobookService_RestoreAudiobook_NotFound(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetBookByID("gone").Return(nil, nil)

	book, err := svc.RestoreAudiobook(context.Background(), "gone")
	assert.Error(t, err)
	assert.Nil(t, book)
	assert.Contains(t, err.Error(), "audiobook not found")
}

func TestAudiobookService_RestoreAudiobook_Success(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	deleted := &database.Book{
		ID:                "r-1",
		MarkedForDeletion: boolPtr(true),
		LibraryState:      stringPtr("deleted"),
	}
	restored := &database.Book{
		ID:                "r-1",
		MarkedForDeletion: boolPtr(false),
		LibraryState:      stringPtr("imported"),
	}
	mockStore.EXPECT().GetBookByID("r-1").Return(deleted, nil)
	mockStore.EXPECT().UpdateBook("r-1", mock.AnythingOfType("*database.Book")).Return(restored, nil)

	book, err := svc.RestoreAudiobook(context.Background(), "r-1")
	assert.NoError(t, err)
	assert.NotNil(t, book)
	assert.Equal(t, "imported", *book.LibraryState)
}

// --- GetSoftDeletedBooks ---

func TestAudiobookService_GetSoftDeletedBooks_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	books, err := svc.GetSoftDeletedBooks(context.Background(), 10, 0, nil)
	assert.Error(t, err)
	assert.Nil(t, books)
}

func TestAudiobookService_GetSoftDeletedBooks_NormalizesParams(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// Negative limit → 50, negative offset → 0, nil olderThanDays → nil cutoff
	mockStore.EXPECT().ListSoftDeletedBooks(50, 0, (*time.Time)(nil)).Return(nil, nil)

	books, err := svc.GetSoftDeletedBooks(context.Background(), -1, -1, nil)
	assert.NoError(t, err)
	assert.NotNil(t, books, "nil from store should become empty slice")
}

// --- GetDuplicateBooks ---

func TestAudiobookService_GetDuplicateBooks_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	result, err := svc.GetDuplicateBooks(context.Background())
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestAudiobookService_GetDuplicateBooks_NoDuplicates(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().GetDuplicateBooks().Return(nil, nil)
	mockStore.EXPECT().GetFolderDuplicates().Return(nil, nil)

	result, err := svc.GetDuplicateBooks(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.GroupCount)
	assert.Equal(t, 0, result.DuplicateCount)
	assert.Empty(t, result.Groups)
}

func TestAudiobookService_GetDuplicateBooks_CountsCorrectly(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	groups := [][]database.Book{
		{{ID: "a"}, {ID: "b"}, {ID: "c"}}, // 2 duplicates
		{{ID: "d"}, {ID: "e"}},             // 1 duplicate
	}
	mockStore.EXPECT().GetDuplicateBooks().Return(groups, nil)
	mockStore.EXPECT().GetFolderDuplicates().Return(nil, nil)

	result, err := svc.GetDuplicateBooks(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, result.GroupCount)
	assert.Equal(t, 3, result.DuplicateCount) // (3-1) + (2-1) = 3
}

// --- User Tags ---

func TestAudiobookService_ListAllUserTags_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	tags, err := svc.ListAllUserTags()
	assert.Error(t, err)
	assert.Nil(t, tags)
}

func TestAudiobookService_AddBookUserTag_StoreError(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().AddBookTag("book-1", "favorite").Return(fmt.Errorf("constraint violation"))

	tags, err := svc.AddBookUserTag("book-1", "favorite")
	assert.Error(t, err)
	assert.Nil(t, tags)
}

func TestAudiobookService_RemoveBookUserTag_Success(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	mockStore.EXPECT().RemoveBookTag("book-1", "old-tag").Return(nil)
	mockStore.EXPECT().GetBookTags("book-1").Return([]string{"remaining"}, nil)

	tags, err := svc.RemoveBookUserTag("book-1", "old-tag")
	assert.NoError(t, err)
	assert.Equal(t, []string{"remaining"}, tags)
}

func TestAudiobookService_BatchUpdateUserTags_PartialErrors(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// First book: add succeeds, remove fails
	mockStore.EXPECT().AddBookTag("b1", "new").Return(nil)
	mockStore.EXPECT().RemoveBookTag("b1", "old").Return(fmt.Errorf("not found"))
	// Second book: both succeed
	mockStore.EXPECT().AddBookTag("b2", "new").Return(nil)
	mockStore.EXPECT().RemoveBookTag("b2", "old").Return(nil)

	count, err := svc.BatchUpdateUserTags(
		[]string{"b1", "b2"},
		[]string{"new"},
		[]string{"old"},
	)
	assert.NoError(t, err)
	// Both books count as "updated" even if individual tag ops fail (logged only)
	assert.Equal(t, 2, count)
}

// --- CountAudiobooksFiltered ---

func TestAudiobookService_CountAudiobooksFiltered_NilStore(t *testing.T) {
	svc := &AudiobookService{}
	count, err := svc.CountAudiobooksFiltered(context.Background(), ListFilters{})
	assert.Error(t, err)
	assert.Equal(t, 0, count)
}

func TestAudiobookService_CountAudiobooksFiltered_WithPrimaryFilter(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	isPrimary := true
	books := []database.Book{
		{ID: "1", IsPrimaryVersion: boolPtr(true)},
		{ID: "2", IsPrimaryVersion: boolPtr(false)},
		{ID: "3", IsPrimaryVersion: nil},
	}
	mockStore.EXPECT().GetAllBooks(0, 0).Return(books, nil)

	count, err := svc.CountAudiobooksFiltered(context.Background(), ListFilters{
		IsPrimaryVersion: &isPrimary,
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "only book 1 has IsPrimaryVersion=true")
}

// --- EnrichAudiobooksWithNames ---

func TestAudiobookService_EnrichAudiobooksWithNames_EmptyInput(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	result := svc.EnrichAudiobooksWithNames([]database.Book{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestAudiobookService_EnrichAudiobooksWithNames_WithAuthorAndSeries(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	books := []database.Book{
		{
			ID:    "e1",
			Title: "Test Book",
			Author: &database.Author{
				ID:   1,
				Name: "Author One",
			},
			Series: &database.Series{
				ID:   2,
				Name: "Series Two",
			},
		},
		{
			ID:    "e2",
			Title: "Orphan Book",
			// No author or series
		},
	}

	result := svc.EnrichAudiobooksWithNames(books)
	assert.Len(t, result, 2)

	assert.NotNil(t, result[0].AuthorName)
	assert.Equal(t, "Author One", *result[0].AuthorName)
	assert.NotNil(t, result[0].SeriesName)
	assert.Equal(t, "Series Two", *result[0].SeriesName)

	assert.Nil(t, result[1].AuthorName)
	assert.Nil(t, result[1].SeriesName)
}

// --- InvalidateBookCaches ---

func TestAudiobookService_InvalidateBookCaches_ClearsCache(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookService(mockStore)

	// Populate cache via GetAudiobooks
	mockStore.EXPECT().GetAllBooks(50, 0).Return([]database.Book{{ID: "cached"}}, nil).Once()

	books, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.NoError(t, err)
	assert.Len(t, books, 1)

	// Second call hits cache — no store call expected
	books2, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.NoError(t, err)
	assert.Len(t, books2, 1)

	// Invalidate and call again — should hit store
	svc.InvalidateBookCaches()
	mockStore.EXPECT().GetAllBooks(50, 0).Return([]database.Book{{ID: "fresh"}}, nil).Once()

	books3, err := svc.GetAudiobooks(context.Background(), 0, 0, "", nil, nil)
	assert.NoError(t, err)
	assert.Len(t, books3, 1)
	assert.Equal(t, "fresh", books3[0].ID)
}
