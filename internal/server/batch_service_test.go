// file: internal/server/batch_service_test.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestBatchUpdateAudiobooks_EmptyBatch(t *testing.T) {
	mockDB := &database.MockStore{}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	bs := NewBatchService(mockDB)

	req := &BatchUpdateRequest{
		IDs:     []string{},
		Updates: map[string]interface{}{},
	}

	resp := bs.UpdateAudiobooks(req)

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if resp.Success != 0 {
		t.Errorf("expected success 0, got %d", resp.Success)
	}
	if resp.Failed != 0 {
		t.Errorf("expected failed 0, got %d", resp.Failed)
	}
}

func TestBatchUpdateAudiobooks_SingleBook(t *testing.T) {
	mockDB := &database.MockStore{
		GetBookByIDFunc: func(id string) (*database.Book, error) {
			return &database.Book{
				ID:    id,
				Title: "Original Title",
			}, nil
		},
		UpdateBookFunc: func(id string, book *database.Book) (*database.Book, error) {
			return book, nil
		},
	}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	bs := NewBatchService(mockDB)

	req := &BatchUpdateRequest{
		IDs: []string{"book1"},
		Updates: map[string]interface{}{
			"title": "Updated Title",
		},
	}

	resp := bs.UpdateAudiobooks(req)

	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if resp.Success != 1 {
		t.Errorf("expected success 1, got %d", resp.Success)
	}
	if resp.Failed != 0 {
		t.Errorf("expected failed 0, got %d", resp.Failed)
	}
}
