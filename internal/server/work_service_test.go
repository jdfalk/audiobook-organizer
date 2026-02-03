// file: internal/server/work_service_test.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1e2f-3a4b-5c6d7e8f9a0b

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestWorkService_ListWorks_Empty(t *testing.T) {
	mockDB := &database.MockStore{
		GetAllWorksFunc: func() ([]database.Work, error) {
			return nil, nil
		},
	}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	ws := NewWorkService(mockDB)

	resp, err := ws.ListWorks()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Count)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

func TestWorkService_CreateWork_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CreateWorkFunc: func(w *database.Work) (*database.Work, error) {
			return w, nil
		},
	}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	ws := NewWorkService(mockDB)

	work := &database.Work{Title: "Test Work"}
	result, err := ws.CreateWork(work)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.Title != "Test Work" {
		t.Errorf("expected title 'Test Work', got %q", result.Title)
	}
}

func TestWorkService_CreateWork_MissingTitle(t *testing.T) {
	mockDB := &database.MockStore{}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	ws := NewWorkService(mockDB)

	work := &database.Work{Title: ""}
	_, err := ws.CreateWork(work)

	if err == nil {
		t.Error("expected error for missing title")
	}
}

func TestWorkService_GetWork_NotFound(t *testing.T) {
	mockDB := &database.MockStore{
		GetWorkByIDFunc: func(id string) (*database.Work, error) {
			return nil, nil
		},
	}
	origStore := database.GlobalStore
	database.GlobalStore = mockDB
	t.Cleanup(func() { database.GlobalStore = origStore })

	ws := NewWorkService(mockDB)

	_, err := ws.GetWork("nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent work")
	}
}
