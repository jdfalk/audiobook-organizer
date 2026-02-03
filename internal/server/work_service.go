// file: internal/server/work_service.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

package server

import (
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type WorkService struct {
	db database.Store
}

func NewWorkService(db database.Store) *WorkService {
	return &WorkService{db: db}
}

type WorkListResponse struct {
	Items []database.Work `json:"items"`
	Count int             `json:"count"`
}

func (ws *WorkService) ListWorks() (*WorkListResponse, error) {
	works, err := ws.db.GetAllWorks()
	if err != nil {
		return nil, err
	}
	if works == nil {
		works = []database.Work{}
	}
	return &WorkListResponse{
		Items: works,
		Count: len(works),
	}, nil
}

func (ws *WorkService) CreateWork(work *database.Work) (*database.Work, error) {
	if strings.TrimSpace(work.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	return ws.db.CreateWork(work)
}

func (ws *WorkService) GetWork(id string) (*database.Work, error) {
	work, err := ws.db.GetWorkByID(id)
	if err != nil {
		return nil, err
	}
	if work == nil {
		return nil, fmt.Errorf("work not found")
	}
	return work, nil
}

func (ws *WorkService) UpdateWork(id string, work *database.Work) (*database.Work, error) {
	if strings.TrimSpace(work.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	updated, err := ws.db.UpdateWork(id, work)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (ws *WorkService) DeleteWork(id string) error {
	return ws.db.DeleteWork(id)
}
