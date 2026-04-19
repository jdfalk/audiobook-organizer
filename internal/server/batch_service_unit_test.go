// file: internal/server/batch_service_unit_test.go
// version: 1.0.0
// guid: f1a2b3c4-d5e6-7f89-0a1b-2c3d4e5f6a7b

package server

import (
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBatchService_UpdateAudiobooks_PartialFailures(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	// book1 exists and updates fine
	store.On("GetBookByID", "book1").Return(&database.Book{ID: "book1", Title: "Book One"}, nil)
	store.On("UpdateBook", "book1", mock.AnythingOfType("*database.Book")).Return(&database.Book{}, nil)

	// book2 not found
	store.On("GetBookByID", "book2").Return(nil, errors.New("not found"))

	// book3 exists but update fails
	store.On("GetBookByID", "book3").Return(&database.Book{ID: "book3", Title: "Book Three"}, nil)
	store.On("UpdateBook", "book3", mock.AnythingOfType("*database.Book")).Return(nil, errors.New("write error"))

	bs := NewBatchService(store)
	resp := bs.UpdateAudiobooks(&BatchUpdateRequest{
		IDs:     []string{"book1", "book2", "book3"},
		Updates: map[string]any{"title": "New Title"},
	})

	assert.Equal(t, 3, resp.Total)
	assert.Equal(t, 1, resp.Success)
	assert.Equal(t, 2, resp.Failed)

	// Verify individual results
	assert.True(t, resp.Results[0].Success)
	assert.Equal(t, "not found", resp.Results[1].Error)
	assert.Equal(t, "write error", resp.Results[2].Error)
}

func TestBatchService_UpdateAudiobooks_EmptyIDs(t *testing.T) {
	store := mocks.NewMockBookStore(t)
	bs := NewBatchService(store)

	resp := bs.UpdateAudiobooks(&BatchUpdateRequest{
		IDs:     []string{},
		Updates: map[string]any{"title": "X"},
	})

	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 0, resp.Success)
	assert.Equal(t, 0, resp.Failed)
}

func TestBatchService_UpdateAudiobooks_DuplicateIDs(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	store.On("GetBookByID", "dup").Return(&database.Book{ID: "dup", Title: "Dup"}, nil)
	store.On("UpdateBook", "dup", mock.AnythingOfType("*database.Book")).Return(&database.Book{}, nil)

	bs := NewBatchService(store)
	resp := bs.UpdateAudiobooks(&BatchUpdateRequest{
		IDs:     []string{"dup", "dup"},
		Updates: map[string]any{"title": "Changed"},
	})

	// Both get processed individually — duplicates are not deduplicated
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 2, resp.Success)
}

func TestBatchService_ExecuteOperations_MixedActions(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	// update target
	store.On("GetBookByID", "u1").Return(&database.Book{ID: "u1", Title: "U1"}, nil)
	store.On("UpdateBook", "u1", mock.AnythingOfType("*database.Book")).Return(&database.Book{}, nil)

	// soft-delete target
	store.On("GetBookByID", "d1").Return(&database.Book{ID: "d1", Title: "D1"}, nil)
	store.On("UpdateBook", "d1", mock.AnythingOfType("*database.Book")).Return(&database.Book{}, nil)

	// restore target
	store.On("GetBookByID", "r1").Return(&database.Book{ID: "r1", Title: "R1"}, nil)
	store.On("UpdateBook", "r1", mock.AnythingOfType("*database.Book")).Return(&database.Book{}, nil)

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "u1", Action: "update", Updates: map[string]any{"title": "Updated"}},
			{ID: "d1", Action: "delete"},
			{ID: "r1", Action: "restore"},
		},
	})

	assert.Equal(t, 3, resp.Total)
	assert.Equal(t, 3, resp.Success)
	assert.Equal(t, 0, resp.Failed)
}

func TestBatchService_ExecuteOperations_HardDelete(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	store.On("GetBookByID", "hd1").Return(&database.Book{ID: "hd1"}, nil)
	store.On("DeleteBook", "hd1").Return(nil)

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "hd1", Action: "delete", HardDelete: true},
		},
	})

	assert.Equal(t, 1, resp.Success)
	store.AssertCalled(t, "DeleteBook", "hd1")
}

func TestBatchService_ExecuteOperations_HardDeleteError(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	store.On("GetBookByID", "hd2").Return(&database.Book{ID: "hd2"}, nil)
	store.On("DeleteBook", "hd2").Return(errors.New("permission denied"))

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "hd2", Action: "delete", HardDelete: true},
		},
	})

	assert.Equal(t, 0, resp.Success)
	assert.Equal(t, 1, resp.Failed)
	assert.Equal(t, "permission denied", resp.Results[0].Error)
}

func TestBatchService_ExecuteOperations_UnknownAction(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "x1", Action: "explode"},
		},
	})

	assert.Equal(t, 1, resp.Failed)
	assert.Contains(t, resp.Results[0].Error, "unknown action")
}

func TestBatchService_ExecuteOperations_DeleteNotFound(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	store.On("GetBookByID", "gone").Return(nil, errors.New("not found"))

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "gone", Action: "delete", HardDelete: true},
		},
	})

	assert.Equal(t, 1, resp.Failed)
	assert.Equal(t, "not found", resp.Results[0].Error)
	// DeleteBook should never be called since GetBookByID failed
	store.AssertNotCalled(t, "DeleteBook", mock.Anything)
}

func TestBatchService_ExecuteOperations_RestoreSetsFields(t *testing.T) {
	store := mocks.NewMockBookStore(t)

	marked := true
	store.On("GetBookByID", "r1").Return(&database.Book{ID: "r1", MarkedForDeletion: &marked}, nil)
	store.On("UpdateBook", "r1", mock.MatchedBy(func(b *database.Book) bool {
		return b.MarkedForDeletion != nil && !*b.MarkedForDeletion && b.MarkedForDeletionAt == nil
	})).Return(&database.Book{}, nil)

	bs := NewBatchService(store)
	resp := bs.ExecuteOperations(&BatchOperationsRequest{
		Operations: []BatchOperationItem{
			{ID: "r1", Action: "restore"},
		},
	})

	assert.Equal(t, 1, resp.Success)
}
