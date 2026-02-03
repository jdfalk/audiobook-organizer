// file: internal/server/batch_service.go
// version: 1.2.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type BatchService struct {
	db database.Store
}

func NewBatchService(db database.Store) *BatchService {
	return &BatchService{db: db}
}

type BatchUpdateRequest struct {
	IDs     []string               `json:"ids"`
	Updates map[string]any `json:"updates"`
}

type BatchUpdateResult struct {
	ID      string      `json:"id"`
	Success bool        `json:"success,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type BatchUpdateResponse struct {
	Results []BatchUpdateResult `json:"results"`
	Success int                 `json:"success"`
	Failed  int                 `json:"failed"`
	Total   int                 `json:"total"`
}

func (bs *BatchService) UpdateAudiobooks(req *BatchUpdateRequest) *BatchUpdateResponse {
	resp := &BatchUpdateResponse{
		Results: []BatchUpdateResult{},
		Total:   len(req.IDs),
	}

	if len(req.IDs) == 0 {
		return resp
	}

	for _, id := range req.IDs {
		book, err := bs.db.GetBookByID(id)
		if err != nil {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:    id,
				Error: "not found",
			})
			resp.Failed++
			continue
		}

		// Apply updates
		if title, ok := req.Updates["title"].(string); ok {
			book.Title = title
		}
		if format, ok := req.Updates["format"].(string); ok {
			book.Format = format
		}
		if authorID, ok := req.Updates["author_id"].(float64); ok {
			aid := int(authorID)
			book.AuthorID = &aid
		}
		if seriesID, ok := req.Updates["series_id"].(float64); ok {
			sid := int(seriesID)
			book.SeriesID = &sid
		}
		if seriesSeq, ok := req.Updates["series_sequence"].(float64); ok {
			seq := int(seriesSeq)
			book.SeriesSequence = &seq
		}

		if _, err := bs.db.UpdateBook(id, book); err != nil {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:    id,
				Error: err.Error(),
			})
			resp.Failed++
		} else {
			resp.Results = append(resp.Results, BatchUpdateResult{
				ID:      id,
				Success: true,
			})
			resp.Success++
		}
	}

	return resp
}
