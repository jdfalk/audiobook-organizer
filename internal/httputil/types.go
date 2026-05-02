// file: internal/httputil/types.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901
// last-edited: 2026-05-01

package httputil

// ErrorResponse is the standard error envelope for all API error responses.
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Status int    `json:"status"`
}

// SuccessResponse is the standard envelope for all successful API responses.
type SuccessResponse struct {
	Data   any `json:"data,omitempty"`
	Count  int `json:"count,omitempty"`
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// ListResponse is the standard envelope for paginated list responses.
type ListResponse struct {
	Items  any `json:"items"`
	Count  int `json:"count"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total,omitempty"`
}

// BulkItem represents a single item result in a bulk operation response.
type BulkItem struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"` // "success" or "failed"
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// BulkResponse is the standard envelope for bulk operation responses.
type BulkResponse struct {
	Total     int        `json:"total"`
	Succeeded int        `json:"succeeded"`
	Failed    int        `json:"failed"`
	Results   []BulkItem `json:"results"`
}

// MessageResponse is the standard envelope for plain status message responses.
type MessageResponse struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// StatusResponse is the standard envelope for health/status check responses.
type StatusResponse struct {
	Status string `json:"status"` // "ok", "degraded", "error"
	Code   string `json:"code,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// PaginationParams holds parsed limit/offset/search from query params.
type PaginationParams struct {
	Limit  int
	Offset int
	Search string
}

// SortParams holds parsed sort field and direction.
type SortParams struct {
	Field string
	Order string // "asc" or "desc"
}

// NewListResponse creates a ListResponse with pagination info.
func NewListResponse(items any, count int, limit int, offset int) *ListResponse {
	return &ListResponse{Items: items, Count: count, Limit: limit, Offset: offset, Total: count}
}

// NewListResponseWithTotal creates a ListResponse with a distinct total count.
func NewListResponseWithTotal(items any, count int, limit int, offset int, total int) *ListResponse {
	return &ListResponse{Items: items, Count: count, Limit: limit, Offset: offset, Total: total}
}

// NewBulkResponse creates a BulkResponse, computing succeeded/failed counts.
func NewBulkResponse(total int, results []BulkItem) *BulkResponse {
	succeeded, failed := 0, 0
	for _, item := range results {
		switch item.Status {
		case "success":
			succeeded++
		case "failed":
			failed++
		}
	}
	return &BulkResponse{Total: total, Succeeded: succeeded, Failed: failed, Results: results}
}

// NewMessageResponse creates a MessageResponse.
func NewMessageResponse(message string, code string) *MessageResponse {
	return &MessageResponse{Message: message, Code: code}
}

// NewStatusResponse creates a StatusResponse.
func NewStatusResponse(status string, data any) *StatusResponse {
	return &StatusResponse{Status: status, Data: data}
}
