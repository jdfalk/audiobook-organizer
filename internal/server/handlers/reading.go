// file: internal/server/handlers/reading.go
// version: 1.0.0
// guid: b8c9d0e1-f2a3-4567-bcde-567890123456
// last-edited: 2026-06-01

package handlers

// SetPositionRequest is the JSON body for POST /api/v1/books/:id/position.
type SetPositionRequest struct {
	SegmentID       string  `json:"segment_id" binding:"required"`
	PositionSeconds float64 `json:"position_seconds"`
}

// PatchStatusRequest is the JSON body for PATCH /api/v1/books/:id/status.
type PatchStatusRequest struct {
	Status string `json:"status" binding:"required"`
}
